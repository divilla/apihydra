package execution

import (
	"bytes"
	"context"
	"errors"
	"slices"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/divilla/apihydra/skeleton/internal/domain"
	"github.com/divilla/apihydra/skeleton/internal/reporting"
	"github.com/divilla/apihydra/skeleton/pkg/errs"
)

func TestValidateDirectoriesAndPlanStagesSupportArbitraryValidDepth(t *testing.T) {
	root := &domain.Directory{Stage: 0, Path: "/"}
	parent := root
	for stage := 1; stage <= 255; stage++ {
		child := &domain.Directory{
			Stage:  stage,
			Path:   "/" + strconv.Itoa(stage),
			Parent: parent,
		}
		parent.Children = []*domain.Directory{child}
		parent = child
	}

	suite := &domain.Suite{Root: root}
	executor := NewExecutor(nil, nil, nil, domain.Config{})
	exitCode, err := executor.ValidateDirectories(suite)
	if err != nil {
		t.Fatalf("ValidateDirectories() error = %v", err)
	}
	if exitCode != 0 {
		t.Fatalf("ValidateDirectories() exit code = %d, want 0", exitCode)
	}
	dirs := executor.PlanStages(suite)
	if got, want := len(dirs), 256; got != want {
		t.Fatalf("len(dirs) = %d, want %d", got, want)
	}
	if got := dirs[255][0]; got != parent {
		t.Fatalf("dirs[255][0] = %p, want %p", got, parent)
	}
}

func TestValidateDirectoriesRejectsInvalidTrees(t *testing.T) {
	validRoot := func() *domain.Directory {
		return &domain.Directory{Stage: 0, Path: "/"}
	}

	tests := map[string]func() *domain.Suite{
		"nil suite": func() *domain.Suite {
			return nil
		},
		"nil root": func() *domain.Suite {
			return &domain.Suite{}
		},
		"invalid root parent": func() *domain.Suite {
			return &domain.Suite{
				Root: &domain.Directory{
					Stage:  0,
					Path:   "/",
					Parent: &domain.Directory{},
				},
			}
		},
		"invalid root stage": func() *domain.Suite {
			return &domain.Suite{Root: &domain.Directory{Stage: -1, Path: "/"}}
		},
		"nil child": func() *domain.Suite {
			root := validRoot()
			root.Children = []*domain.Directory{nil}
			return &domain.Suite{Root: root}
		},
		"invalid parent": func() *domain.Suite {
			root := validRoot()
			root.Children = []*domain.Directory{{Stage: 1, Path: "/child"}}
			return &domain.Suite{Root: root}
		},
		"invalid child stage": func() *domain.Suite {
			root := validRoot()
			root.Children = []*domain.Directory{{Stage: 2, Path: "/child", Parent: root}}
			return &domain.Suite{Root: root}
		},
		"repeated directory": func() *domain.Suite {
			root := validRoot()
			child := &domain.Directory{Stage: 1, Path: "/child", Parent: root}
			root.Children = []*domain.Directory{child, child}
			return &domain.Suite{Root: root}
		},
		"cycle": func() *domain.Suite {
			root := validRoot()
			child := &domain.Directory{Stage: 1, Path: "/child", Parent: root}
			root.Children = []*domain.Directory{child}
			child.Children = []*domain.Directory{root}
			return &domain.Suite{Root: root}
		},
	}

	for name, suite := range tests {
		t.Run(name, func(t *testing.T) {
			executor := NewExecutor(nil, nil, nil, domain.Config{})
			exitCode, err := executor.ValidateDirectories(suite())
			if exitCode != errs.ExitConfiguration {
				t.Fatalf("ValidateDirectories() exit code = %d, want %d", exitCode, errs.ExitConfiguration)
			}
			if !errors.Is(err, ErrInvalidDirectoryTree) {
				t.Fatalf("ValidateDirectories() error = %v, want ErrInvalidDirectoryTree", err)
			}
		})
	}
}

func TestExecuteStagesCancelsAndJoinsActiveStageOnError(t *testing.T) {
	wantErr := errors.New("fatal execution error")
	wantExitCode := errs.ExitInternal
	failing := &domain.Directory{Stage: 0, Path: "/failing"}
	sibling := &domain.Directory{Stage: 0, Path: "/sibling"}
	later := &domain.Directory{Stage: 1, Path: "/later"}

	siblingReturned := make(chan struct{})
	var laterStarted atomic.Bool
	process := func(ctx context.Context, _ resultPublisher, dir *domain.Directory) (int, error) {
		switch dir {
		case failing:
			return wantExitCode, wantErr
		case sibling:
			<-ctx.Done()
			close(siblingReturned)
			return errs.ExitInternal, ctx.Err()
		case later:
			laterStarted.Store(true)
		}
		return 0, nil
	}

	exitCode, err := executeStages(
		context.Background(),
		[][]*domain.Directory{{failing, sibling}, {later}},
		1,
		nil,
		process,
	)
	if exitCode != wantExitCode {
		t.Fatalf("executeStages() exit code = %d, want %d", exitCode, wantExitCode)
	}
	if !errors.Is(err, wantErr) {
		t.Fatalf("executeStages() error = %v, want %v", err, wantErr)
	}
	select {
	case <-siblingReturned:
	default:
		t.Fatal("executeStages() returned before the active-stage sibling joined")
	}
	if laterStarted.Load() {
		t.Fatal("executeStages() started a later stage after a fatal error")
	}
}

func TestParallelismModesControlDirectoryOverlapWithoutParallelStages(t *testing.T) {
	first := &domain.Directory{Stage: 0, Path: "/first"}
	second := &domain.Directory{Stage: 0, Path: "/second"}
	later := &domain.Directory{Stage: 1, Path: "/later"}

	t.Run("mode 0 is ordered and serial", func(t *testing.T) {
		var active atomic.Int32
		var maximum atomic.Int32
		var mu sync.Mutex
		visited := make([]string, 0, 3)
		_, err := executeStages(
			context.Background(),
			[][]*domain.Directory{{first, second}, {later}},
			0,
			nil,
			func(_ context.Context, _ resultPublisher, dir *domain.Directory) (int, error) {
				current := active.Add(1)
				for current > maximum.Load() && !maximum.CompareAndSwap(maximum.Load(), current) {
				}
				mu.Lock()
				visited = append(visited, dir.Path)
				mu.Unlock()
				active.Add(-1)
				return 0, nil
			},
		)
		if err != nil {
			t.Fatalf("executeStages(mode 0) error = %v", err)
		}
		if maximum.Load() != 1 || !slices.Equal(visited, []string{"/first", "/second", "/later"}) {
			t.Fatalf("executeStages(mode 0) maximum = %d, visited = %v", maximum.Load(), visited)
		}
	})

	t.Run("mode 1 overlaps directories and retains stage barrier", func(t *testing.T) {
		started := make(chan struct{}, 2)
		release := make(chan struct{})
		var active atomic.Int32
		var laterSawActive atomic.Int32
		done := make(chan error, 1)
		go func() {
			_, err := executeStages(
				context.Background(),
				[][]*domain.Directory{{first, second}, {later}},
				1,
				nil,
				func(_ context.Context, _ resultPublisher, dir *domain.Directory) (int, error) {
					if dir == later {
						laterSawActive.Store(active.Load())
						return 0, nil
					}
					active.Add(1)
					started <- struct{}{}
					<-release
					active.Add(-1)
					return 0, nil
				},
			)
			done <- err
		}()
		<-started
		<-started
		if active.Load() != 2 {
			t.Fatalf("mode 1 active directories = %d, want 2", active.Load())
		}
		close(release)
		if err := <-done; err != nil {
			t.Fatalf("executeStages(mode 1) error = %v", err)
		}
		if laterSawActive.Load() != 0 {
			t.Fatalf("later stage saw %d active prior directories", laterSawActive.Load())
		}
	})
}

func TestExecuteFilesControlsFileOverlap(t *testing.T) {
	dir := &domain.Directory{RuntimeSteps: make([][]domain.Step, 2)}
	for _, parallel := range []bool{false, true} {
		t.Run(strconv.FormatBool(parallel), func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			var active atomic.Int32
			var maximum atomic.Int32
			started := make(chan struct{}, 2)
			release := make(chan struct{})
			done := make(chan error, 1)
			go func() {
				_, err := executeFiles(ctx, cancel, dir, parallel, func(context.Context, *domain.Directory, int) (int, error) {
					current := active.Add(1)
					for current > maximum.Load() && !maximum.CompareAndSwap(maximum.Load(), current) {
					}
					started <- struct{}{}
					if parallel {
						<-release
					}
					active.Add(-1)
					return 0, nil
				})
				done <- err
			}()
			<-started
			if parallel {
				<-started
				close(release)
			}
			if err := <-done; err != nil {
				t.Fatalf("executeFiles(parallel %t) error = %v", parallel, err)
			}
			want := int32(1)
			if parallel {
				want = 2
			}
			if maximum.Load() != want {
				t.Fatalf("executeFiles(parallel %t) maximum = %d, want %d", parallel, maximum.Load(), want)
			}
		})
	}
}

func TestFileStopCancelsStageBeforeJoiningFileSiblingsAndPreservesResult(t *testing.T) {
	wantErr := errors.New("originating file failure")
	t.Run("fatal", func(t *testing.T) {
		testFileStopCancelsStage(t, errs.ExitConfiguration, wantErr, errs.ExitConfiguration, wantErr)
	})
	t.Run("debug", func(t *testing.T) {
		testFileStopCancelsStage(t, 0, errDebugStop, 0, nil)
	})
}

func testFileStopCancelsStage(t *testing.T, fileCode int, fileErr error, wantCode int, wantErr error) {
	t.Helper()
	origin := &domain.Directory{Path: "/origin", RuntimeSteps: make([][]domain.Step, 2)}
	peer := &domain.Directory{Path: "/peer"}
	peerStarted := make(chan struct{})
	peerCanceled := make(chan struct{})
	fileSiblingStarted := make(chan struct{})
	fileSiblingCanceled := make(chan struct{})
	releaseFileSibling := make(chan struct{})
	type executionResult struct {
		code int
		err  error
	}
	done := make(chan executionResult, 1)

	go func() {
		code, err := executeStages(
			context.Background(),
			[][]*domain.Directory{{origin, peer}},
			2,
			nil,
			func(ctx context.Context, publish resultPublisher, dir *domain.Directory) (int, error) {
				if dir == peer {
					close(peerStarted)
					<-ctx.Done()
					close(peerCanceled)
					return errs.ExitInternal, ctx.Err()
				}
				return executeDirectoryFiles(ctx, publish, dir, true, func(ctx context.Context, _ *domain.Directory, fileIndex int) (int, error) {
					if fileIndex == 0 {
						<-peerStarted
						<-fileSiblingStarted
						return fileCode, fileErr
					}
					close(fileSiblingStarted)
					<-ctx.Done()
					close(fileSiblingCanceled)
					<-releaseFileSibling
					return errs.ExitInternal, ctx.Err()
				})
			},
		)
		done <- executionResult{code: code, err: err}
	}()

	<-peerCanceled
	<-fileSiblingCanceled
	select {
	case result := <-done:
		t.Fatalf("executeStages returned before the local file sibling joined: (%d, %v)", result.code, result.err)
	default:
	}
	close(releaseFileSibling)
	result := <-done
	if result.code != wantCode || (wantErr == nil && result.err != nil) || (wantErr != nil && !errors.Is(result.err, wantErr)) {
		t.Fatalf("nested stop result = (%d, %v), want (%d, %v)", result.code, result.err, wantCode, wantErr)
	}
}

func TestExecuteStagesRejectsInvalidParallelism(t *testing.T) {
	exitCode, err := executeStages(context.Background(), nil, 3, nil, nil)
	if exitCode != errs.ExitConfiguration || !errors.Is(err, ErrInvalidParallelism) {
		t.Fatalf("executeStages(invalid) = (%d, %v), want configuration ErrInvalidParallelism", exitCode, err)
	}
}

func TestExecuteStagesClassifiesCancellationBeforeReporting(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	report := reporting.NewReporter(&bytes.Buffer{}, false)

	exitCode, err := executeStages(ctx, [][]*domain.Directory{{{}}}, 0, report, nil)
	if exitCode != errs.ExitInternal || !errors.Is(err, ErrExecutionCanceled) || !errors.Is(err, context.Canceled) {
		t.Fatalf("executeStages(canceled) = (%d, %v), want internal ErrExecutionCanceled", exitCode, err)
	}
}

func TestExecuteStagesNeverReturnsSuccessWithError(t *testing.T) {
	wantErr := errors.New("uncoded error")

	exitCode, err := executeStages(
		context.Background(),
		[][]*domain.Directory{{{Stage: 0, Path: "/"}}},
		1,
		nil,
		func(context.Context, resultPublisher, *domain.Directory) (int, error) {
			return 0, wantErr
		},
	)
	if exitCode != errs.ExitInternal {
		t.Fatalf("executeStages() exit code = %d, want %d", exitCode, errs.ExitInternal)
	}
	if !errors.Is(err, ErrExecutionCanceled) {
		t.Fatalf("executeStages() error = %v, want ErrExecutionCanceled", err)
	}
}

func TestProcessResultReplacesValidationWithFirstFatalResult(t *testing.T) {
	wantCode := 19
	wantErr := errors.New("first fatal error")
	var result processResult

	result.setResult(errs.ExitValidation, nil)
	result.setResult(wantCode, wantErr)
	result.setResult(errs.ExitInternal, errors.New("later fatal error"))

	if result.code != wantCode {
		t.Fatalf("result code = %d, want %d", result.code, wantCode)
	}
	if !errors.Is(result.err, wantErr) {
		t.Fatalf("result error = %v, want %v", result.err, wantErr)
	}
}

func TestExecutionErrorsUseProductExitCodes(t *testing.T) {
	tests := map[string]struct {
		err      error
		exitCode int
	}{
		"invalid directory tree": {errs.Build(errs.ExitConfiguration, ErrInvalidDirectoryTree, nil), errs.ExitConfiguration},
		"missing variable":       {errs.Build(errs.ExitConfiguration, ErrNotFound, nil), errs.ExitConfiguration},
		"duplicate variable":     {errs.Build(errs.ExitConfiguration, ErrKeyExists, nil), errs.ExitConfiguration},
		"validation":             {errs.Build(errs.ExitValidation, ErrValidation, nil), errs.ExitValidation},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			if got := errs.Code(test.err, errs.ExitInternal); got != test.exitCode {
				t.Fatalf("exit code = %d, want %d", got, test.exitCode)
			}
		})
	}
}

func TestExecuteStagesContinuesAfterValidationStatus(t *testing.T) {
	var processed atomic.Int32
	exitCode, err := executeStages(
		context.Background(),
		[][]*domain.Directory{
			{{Stage: 0, Path: "/"}},
			{{Stage: 1, Path: "/child"}},
		},
		1,
		nil,
		func(_ context.Context, _ resultPublisher, dir *domain.Directory) (int, error) {
			processed.Add(1)
			if dir.Stage == 0 {
				return errs.ExitValidation, nil
			}
			return 0, nil
		},
	)
	if exitCode != errs.ExitValidation {
		t.Fatalf("executeStages() exit code = %d, want %d", exitCode, errs.ExitValidation)
	}
	if err != nil {
		t.Fatalf("executeStages() error = %v, want nil", err)
	}
	if got, want := processed.Load(), int32(2); got != want {
		t.Fatalf("processed directories = %d, want %d", got, want)
	}
}

func TestExecuteStagesFatalErrorTakesPrecedenceOverValidationStatus(t *testing.T) {
	wantErr := errors.New("fatal execution error")
	exitCode, err := executeStages(
		context.Background(),
		[][]*domain.Directory{
			{{Stage: 0, Path: "/"}},
			{{Stage: 1, Path: "/child"}},
		},
		1,
		nil,
		func(_ context.Context, _ resultPublisher, dir *domain.Directory) (int, error) {
			if dir.Stage == 0 {
				return errs.ExitValidation, nil
			}
			return errs.ExitInternal, wantErr
		},
	)
	if exitCode != errs.ExitInternal {
		t.Fatalf("executeStages() exit code = %d, want %d", exitCode, errs.ExitInternal)
	}
	if !errors.Is(err, wantErr) {
		t.Fatalf("executeStages() error = %v, want %v", err, wantErr)
	}
}
