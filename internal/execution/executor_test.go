package execution

import (
	"apih/internal/domain"
	"apih/internal/reporting"
	"apih/pkg/errs"
	"apih/pkg/runner"
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
)

func TestExecutorExportedContractAndConstructorState(t *testing.T) {
	binder := NewBinder(NewKeyValueStore())
	validator := NewValidator(domain.Config{})
	reporter := reporting.NewReporter(&bytes.Buffer{}, false)
	config := domain.Config{Parallelism: 1}
	executor := NewExecutor(binder, validator, reporter, config)

	if executor.binder != binder || executor.val != validator || executor.report != reporter || executor.config != config {
		t.Fatalf("NewExecutor() did not retain collaborators: %+v", executor)
	}
	if got, want := ErrInvalidDirectoryTree.Error(), "invalid directory tree"; got != want {
		t.Fatalf("ErrInvalidDirectoryTree = %q, want %q", got, want)
	}
	if got, want := ErrExecutionCanceled.Error(), "execution canceled"; got != want {
		t.Fatalf("ErrExecutionCanceled = %q, want %q", got, want)
	}

	var _ func(*Binder, *Validator, *reporting.Reporter, domain.Config) *Executor = NewExecutor
	var _ func(*Executor, *domain.Suite) (int, error) = (*Executor).ValidateDirectories
	var _ func(*Executor, *domain.Suite) = (*Executor).Prepare
	var _ func(*Executor, *domain.Suite) [][]*domain.Directory = (*Executor).PlanStages
	var _ func(*Executor, context.Context, [][]*domain.Directory) (int, error) = (*Executor).Execute
}

func TestPrepareDeepCopiesAllMutableStepStateAcrossTree(t *testing.T) {
	definition := &domain.StepsDefinition{}
	step := domain.Step{
		Vars:       map[string]domain.YAMLString{"request": "one"},
		Definition: definition,
		Index:      4,
	}
	step.Request.Body = `{"value":"$request"}`
	step.Request.Defaults.Headers = map[string]string{"Accept": "application/json"}
	disableCookies := true
	step.Request.Defaults.DisableCookies = &disableCookies
	step.Response.ExpectedBody = `{"value":"$request"}`
	step.Response.ExpectedTypes = map[string][]string{
		".empty": {},
		".nil":   nil,
		".value": {"string", "null"},
	}
	step.Response.Capture = map[string]domain.YAMLString{"captured": ".value"}
	step.RawCurl = "curl --header Authorization: Bearer preserved"

	childStep := step
	childStep.Index = 5
	root := &domain.Directory{ResolvedSteps: [][]domain.Step{{step}, nil}}
	child := &domain.Directory{Parent: root, ResolvedSteps: [][]domain.Step{{childStep}}}
	root.Children = []*domain.Directory{child}
	suite := &domain.Suite{Root: root}
	binder := NewBinder(NewKeyValueStore())

	NewExecutor(binder, nil, nil, domain.Config{}).Prepare(suite)

	if !reflect.DeepEqual(root.RuntimeSteps, root.ResolvedSteps) || !reflect.DeepEqual(child.RuntimeSteps, child.ResolvedSteps) {
		t.Fatalf("Prepare() runtime steps differ from resolved values: root=%+v child=%+v", root.RuntimeSteps, child.RuntimeSteps)
	}
	if root.RuntimeSteps[1] != nil {
		t.Fatalf("Prepare() nil group = %#v, want nil", root.RuntimeSteps[1])
	}
	runtimeStep := &root.RuntimeSteps[0][0]
	if runtimeStep.Response.ExpectedTypes[".empty"] == nil {
		t.Fatal("Prepare() changed non-nil empty ExpectedTypes slice to nil")
	}
	if runtimeStep.Response.ExpectedTypes[".nil"] != nil {
		t.Fatalf("Prepare() changed nil ExpectedTypes slice to %#v", runtimeStep.Response.ExpectedTypes[".nil"])
	}
	if runtimeStep.Definition != definition {
		t.Fatalf("Prepare() Definition = %p, want preserved pointer %p", runtimeStep.Definition, definition)
	}
	if runtimeStep.Index != 4 {
		t.Fatalf("Prepare() Index = %d, want 4", runtimeStep.Index)
	}

	runtimeStep.Vars["request"] = "changed"
	runtimeStep.Request.Defaults.Headers["Accept"] = "text/plain"
	*runtimeStep.Request.Defaults.DisableCookies = false
	runtimeStep.Response.ExpectedTypes[".value"][0] = "number"
	runtimeStep.Response.Capture["captured"] = ".other"
	runtimeStep.Request.Body = "interpolated"
	if got := root.ResolvedSteps[0][0].Vars["request"]; got != "one" {
		t.Fatalf("Prepare() shared Vars map, resolved value = %q", got)
	}
	if got := root.ResolvedSteps[0][0].Request.Defaults.Headers["Accept"]; got != "application/json" {
		t.Fatalf("Prepare() shared Headers map, resolved value = %q", got)
	}
	if got := root.ResolvedSteps[0][0].Request.Defaults.DisableCookies; got == nil || !*got {
		t.Fatalf("Prepare() shared DisableCookies pointer, resolved value = %v", got)
	}
	if got := root.ResolvedSteps[0][0].Response.ExpectedTypes[".value"][0]; got != "string" {
		t.Fatalf("Prepare() shared ExpectedTypes slice, resolved value = %q", got)
	}
	if got := root.ResolvedSteps[0][0].Response.Capture["captured"]; got != ".value" {
		t.Fatalf("Prepare() shared Capture map, resolved value = %q", got)
	}
	if got := root.ResolvedSteps[0][0].Request.Body; got != `{"value":"$request"}` {
		t.Fatalf("Prepare() mutated ResolvedSteps body = %q", got)
	}
	if _, err := binder.kvs.Get("request"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Prepare() loaded variables, Get(request) error = %v", err)
	}
}

func TestPreparePreservesNilResolvedSteps(t *testing.T) {
	root := &domain.Directory{ResolvedSteps: [][]domain.Step{{{}}}}
	NewExecutor(nil, nil, nil, domain.Config{}).Prepare(&domain.Suite{Root: root})
	if root.RuntimeSteps[0][0].Vars != nil || root.RuntimeSteps[0][0].Request.Defaults.Headers != nil || root.RuntimeSteps[0][0].Response.Capture != nil {
		t.Fatalf("Prepare() changed nil maps: %+v", root.RuntimeSteps[0][0])
	}

	nilRoot := &domain.Directory{}
	NewExecutor(nil, nil, nil, domain.Config{}).Prepare(&domain.Suite{Root: nilRoot})
	if nilRoot.RuntimeSteps != nil {
		t.Fatalf("RuntimeSteps = %#v, want nil", nilRoot.RuntimeSteps)
	}
}

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

func TestPlanStagesGroupsSiblingsAndPreservesTreeSliceOrder(t *testing.T) {
	root := &domain.Directory{Path: "/"}
	first := &domain.Directory{Stage: 1, Path: "/first", Parent: root}
	second := &domain.Directory{Stage: 1, Path: "/second", Parent: root}
	grandchild := &domain.Directory{Stage: 2, Path: "/first/grandchild", Parent: first}
	root.Children = []*domain.Directory{first, second}
	first.Children = []*domain.Directory{grandchild}

	got := NewExecutor(nil, nil, nil, domain.Config{}).PlanStages(&domain.Suite{Root: root})
	want := [][]*domain.Directory{{root}, {first, second}, {grandchild}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("PlanStages() = %#v, want %#v", got, want)
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

func TestExecuteStagesUsesSameStageConcurrencyAndLaterStageBarrier(t *testing.T) {
	first := &domain.Directory{Path: "/first"}
	second := &domain.Directory{Path: "/second"}
	later := &domain.Directory{Stage: 1, Path: "/later"}
	started := make(chan *domain.Directory, 2)
	release := make(chan struct{})
	var active atomic.Int32
	var laterSawActive atomic.Int32

	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = executeStages(context.Background(), [][]*domain.Directory{{first, second}, {later}}, 1, nil, func(_ context.Context, _ resultPublisher, dir *domain.Directory) (int, error) {
			if dir == later {
				laterSawActive.Store(active.Load())
				return 0, nil
			}
			active.Add(1)
			started <- dir
			<-release
			active.Add(-1)
			return 0, nil
		})
	}()

	<-started
	<-started
	if got := active.Load(); got != 2 {
		t.Fatalf("same-stage active directories = %d, want 2", got)
	}
	close(release)
	<-done
	if got := laterSawActive.Load(); got != 0 {
		t.Fatalf("later stage observed %d active earlier directories, want 0", got)
	}
}

func TestParallelismModesControlDirectoryAndFileOverlap(t *testing.T) {
	first := &domain.Directory{Path: "/first", RuntimeSteps: make([][]domain.Step, 2)}
	second := &domain.Directory{Path: "/second"}

	t.Run("mode zero serializes directories", func(t *testing.T) {
		var active atomic.Int32
		var maximum atomic.Int32
		visited := make([]string, 0, 2)
		exitCode, err := executeStages(
			context.Background(),
			[][]*domain.Directory{{first, second}},
			0,
			nil,
			func(_ context.Context, _ resultPublisher, directory *domain.Directory) (int, error) {
				current := active.Add(1)
				if current > maximum.Load() {
					maximum.Store(current)
				}
				visited = append(visited, directory.Path)
				active.Add(-1)
				return 0, nil
			},
		)
		if err != nil || exitCode != 0 || maximum.Load() != 1 || !reflect.DeepEqual(visited, []string{"/first", "/second"}) {
			t.Fatalf("mode 0 = (%d, %v), maximum %d, visited %v", exitCode, err, maximum.Load(), visited)
		}
	})

	for _, parallel := range []bool{false, true} {
		t.Run("files parallel="+strconv.FormatBool(parallel), func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			started := make(chan struct{}, 2)
			release := make(chan struct{})
			var active atomic.Int32
			var maximum atomic.Int32
			done := make(chan error, 1)
			go func() {
				_, err := executeFiles(ctx, cancel, first, parallel, func(context.Context, *domain.Directory, int) (int, error) {
					current := active.Add(1)
					if current > maximum.Load() {
						maximum.Store(current)
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
				t.Fatal(err)
			}
			want := int32(1)
			if parallel {
				want = 2
			}
			if maximum.Load() != want {
				t.Fatalf("maximum active files = %d, want %d", maximum.Load(), want)
			}
		})
	}
}

func TestExecuteFilesHasNoArtificialTaskLimit(t *testing.T) {
	const files = 64
	directory := &domain.Directory{RuntimeSteps: make([][]domain.Step, files)}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	started := make(chan struct{}, files)
	release := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		_, err := executeFiles(ctx, cancel, directory, true, func(context.Context, *domain.Directory, int) (int, error) {
			started <- struct{}{}
			<-release
			return 0, nil
		})
		done <- err
	}()
	for range files {
		<-started
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestExecuteStageHasNoArtificialTaskLimit(t *testing.T) {
	const directories = 64
	stage := make([]*domain.Directory, directories)
	for index := range stage {
		stage[index] = &domain.Directory{Path: "/" + strconv.Itoa(index)}
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	started := make(chan struct{}, directories)
	release := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		_, err := executeStage(ctx, cancel, stage, true, func(context.Context, resultPublisher, *domain.Directory) (int, error) {
			started <- struct{}{}
			<-release
			return 0, nil
		})
		done <- err
	}()
	for range directories {
		<-started
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestSerialFilesCanShareOneWriteOnceStore(t *testing.T) {
	directory := &domain.Directory{RuntimeSteps: make([][]domain.Step, 2)}
	store := NewKeyValueStore()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	exitCode, err := executeFiles(ctx, cancel, directory, false, func(_ context.Context, _ *domain.Directory, fileIndex int) (int, error) {
		if fileIndex == 0 {
			return 0, store.Set("shared", "value")
		}
		value, err := store.Get("shared")
		if err != nil || value != "value" {
			return errs.ExitInternal, err
		}
		return 0, nil
	})
	if exitCode != 0 || err != nil {
		t.Fatalf("serial shared store = (%d, %v)", exitCode, err)
	}
}

func TestExecuteFilesCancelsAndJoinsSiblingsOnFatalError(t *testing.T) {
	directory := &domain.Directory{RuntimeSteps: make([][]domain.Step, 2)}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	siblingStarted := make(chan struct{})
	siblingReturned := make(chan struct{})
	exitCode, err := executeFiles(ctx, cancel, directory, true, func(ctx context.Context, _ *domain.Directory, fileIndex int) (int, error) {
		if fileIndex == 0 {
			<-siblingStarted
			return errs.ExitInternal, ErrStepExecution
		}
		close(siblingStarted)
		<-ctx.Done()
		close(siblingReturned)
		return errs.ExitInternal, ctx.Err()
	})
	if exitCode != errs.ExitInternal || !errors.Is(err, ErrStepExecution) {
		t.Fatalf("fatal files = (%d, %v)", exitCode, err)
	}
	select {
	case <-siblingReturned:
	default:
		t.Fatal("executeFiles returned before canceled sibling joined")
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

func TestExecuteStagesRejectsInvalidModeAndBracketsReporter(t *testing.T) {
	exitCode, err := executeStages(context.Background(), nil, 3, nil, nil)
	if exitCode != errs.ExitConfiguration || !errors.Is(err, ErrInvalidParallelism) {
		t.Fatalf("invalid mode = (%d, %v)", exitCode, err)
	}

	directory := &domain.Directory{StepsDefinitions: []*domain.StepsDefinition{{}}}
	var output bytes.Buffer
	report := reporting.NewReporter(&output, false)
	exitCode, err = executeStages(
		context.Background(),
		[][]*domain.Directory{{directory}},
		0,
		report,
		func(context.Context, resultPublisher, *domain.Directory) (int, error) {
			return 0, nil
		},
	)
	if exitCode != 0 || err != nil || output.Len() != 0 {
		t.Fatalf("reported empty stage = (%d, %v, %q)", exitCode, err, output.String())
	}

	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	exitCode, err = executeStages(canceled, [][]*domain.Directory{{directory}}, 0, report, nil)
	if exitCode != errs.ExitInternal || !errors.Is(err, ErrExecutionCanceled) || !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled execution before BeginStage = (%d, %v), want internal ErrExecutionCanceled", exitCode, err)
	}
}

func TestExecuteStagesReturnsEndStageFailure(t *testing.T) {
	wantErr := errors.New("end stage failed")
	report := reporting.NewReporter(executorFailingWriter{err: wantErr}, false)
	directory := &domain.Directory{}
	exitCode, err := executeStages(
		context.Background(),
		[][]*domain.Directory{{directory}},
		0,
		report,
		func(context.Context, resultPublisher, *domain.Directory) (int, error) { return 0, nil },
	)
	if exitCode != errs.ExitInternal || !errors.Is(err, wantErr) {
		t.Fatalf("EndStage failure = (%d, %v)", exitCode, err)
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

func TestExecuteStagesPreservesInFlightCancellationResult(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	started := make(chan struct{})
	wantExitCode := -1
	type result struct {
		exitCode int
		err      error
	}
	done := make(chan result)

	go func() {
		exitCode, err := executeStages(
			ctx,
			[][]*domain.Directory{{{Stage: 0, Path: "/"}}},
			1,
			nil,
			func(ctx context.Context, _ resultPublisher, _ *domain.Directory) (int, error) {
				close(started)
				<-ctx.Done()
				return wantExitCode, errs.Build(wantExitCode, runner.ErrJQSelector, ctx.Err())
			},
		)
		done <- result{exitCode: exitCode, err: err}
	}()

	<-started
	cancel()
	got := <-done
	if got.exitCode != wantExitCode {
		t.Fatalf("executeStages() exit code = %d, want %d", got.exitCode, wantExitCode)
	}
	if !errors.Is(got.err, runner.ErrJQSelector) || !errors.Is(got.err, context.Canceled) {
		t.Fatalf("executeStages() error = %v, want ErrJQSelector and context.Canceled", got.err)
	}
	if errors.Is(got.err, ErrExecutionCanceled) {
		t.Fatalf("executeStages() error = %v, unexpectedly normalized to ErrExecutionCanceled", got.err)
	}
}

func TestProcessResultReplacesValidationWithFirstFatalResult(t *testing.T) {
	wantCode := 19
	wantErr := errors.New("first fatal error")
	var result processResult

	result.setResult(0, errors.New("ignored zero-code error"))
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

func TestProcessResultPreservesFatalValidationExitCode(t *testing.T) {
	wantErr := errors.New("fatal error with validation exit code")
	var result processResult

	result.setResult(errs.ExitValidation, wantErr)
	result.setResult(errs.ExitInternal, errors.New("later cancellation error"))

	if result.code != errs.ExitValidation {
		t.Fatalf("result code = %d, want %d", result.code, errs.ExitValidation)
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

func TestExecuteWithReporterClassifiesCancellationBeforeStage(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	report := reporting.NewReporter(&bytes.Buffer{}, false)
	exitCode, err := NewExecutor(nil, nil, report, domain.Config{}).Execute(ctx, [][]*domain.Directory{{}})
	if exitCode != errs.ExitInternal || !errors.Is(err, ErrExecutionCanceled) || !errors.Is(err, context.Canceled) {
		t.Fatalf("Execute(canceled) = (%d, %v), want internal ErrExecutionCanceled", exitCode, err)
	}
}

func TestExecuteReturnsSuccessWithoutStages(t *testing.T) {
	exitCode, err := NewExecutor(nil, nil, nil, domain.Config{}).Execute(context.Background(), nil)
	if exitCode != 0 || err != nil {
		t.Fatalf("Execute(nil) = (%d, %v), want success", exitCode, err)
	}
}

func TestProcessDirRunsEightPhasesMutatesRuntimeAndReportsDebugSuccess(t *testing.T) {
	commandDir := newExecutorCommandDir(t)
	logPath := filepath.Join(commandDir, "phases")
	t.Setenv("APIH_EXECUTOR_LOG", logPath)
	installExecutorCommand(t, commandDir, "curl", `
url=
previous=
for argument do
  case "$previous" in
    url) url=$argument; previous=; continue ;;
  esac
  case "$argument" in
    --url) previous=url ;;
  esac
done
body=$(/bin/cat)
printf 'curl|%s|%s\n' "$url" "$body" >> "$APIH_EXECUTOR_LOG"
printf '%s' "$body"
printf 'http-code:201' >&2
`)
	installExecutorCommand(t, commandDir, "jq", `
input=$(/bin/cat)
last=
for argument do last=$argument; done
case "$last" in
  .token)
    printf 'capture\n' >> "$APIH_EXECUTOR_LOG"
    printf '"one"\n'
    ;;
  *'.[]'*)
    printf 'types\n' >> "$APIH_EXECUTOR_LOG"
    ;;
  *)
    case "$*" in
      *'--compact-output .'*) printf 'curl-raw-body\n' >> "$APIH_EXECUTOR_LOG" ;;
      *'--sort-keys --'*) printf 'body-actual\n' >> "$APIH_EXECUTOR_LOG" ;;
      *) printf 'body-expected\n' >> "$APIH_EXECUTOR_LOG" ;;
    esac
    printf '%s\n' "$input"
    ;;
esac
`)
	installExecutorCommand(t, commandDir, "git", `printf 'git must not run' >&2; exit 99`)

	first := executorStep("steps.yaml", 0)
	first.Vars = map[string]domain.YAMLString{"request": "one"}
	first.Request.Method = "POST"
	first.Request.Defaults.BaseURL = "https://example.test"
	first.Request.Defaults.BasePath = "/api"
	first.Request.Path = "/first"
	first.Request.Body = `{"token":"$request"}`
	first.Response.ExpectedStatus = 201
	first.Response.ExpectedBody = `{"token":"${request}"}`
	first.Response.Capture = map[string]domain.YAMLString{"captured": ".token"}

	second := executorStep("steps.yaml", 1)
	second.Request.Method = "POST"
	second.Request.Defaults.BaseURL = "https://example.test"
	second.Request.Defaults.BasePath = "/api"
	second.Request.Path = "/second"
	second.Request.Body = `{"token":$captured}`
	second.Response.ExpectedStatus = 201
	second.Response.ExpectedBody = `{"token":${captured}}`
	second.Debug = true
	third := executorStep("steps.yaml", 2)
	third.Request.Method = "GET"
	third.Request.Defaults.BaseURL = "https://example.test"
	third.Request.Path = "/must-not-run"

	dir := first.Definition.File.Directory
	dir.RuntimeSteps = [][]domain.Step{{first, second, third}}
	var output bytes.Buffer
	config := domain.Config{TempRunDir: t.TempDir()}
	executor := NewExecutor(NewBinder(NewKeyValueStore()), NewValidator(config), reporting.NewReporter(&output, false), config)
	if err := executor.cookies.prepareStage([]*domain.Directory{dir}); err != nil {
		t.Fatal(err)
	}

	exitCode, err := executor.processDir(context.Background(), discardResult, dir)
	if exitCode != 0 || !errors.Is(err, errDebugStop) {
		t.Fatalf("processDir() = (%d, %v), want debug stop", exitCode, err)
	}
	wantBodies := []string{`{"token":"one"}`, `{"token":"one"}`}
	for index, want := range wantBodies {
		step := &dir.RuntimeSteps[0][index]
		if got := string(step.Request.Body); got != want {
			t.Errorf("step %d Request.Body = %q, want %q", index, got, want)
		}
		if got := string(step.Response.ExpectedBody); got != want {
			t.Errorf("step %d ExpectedBody = %q, want %q", index, got, want)
		}
		if string(step.Response.ActualBody) != want || step.Response.ActualStatus != 201 {
			t.Errorf("step %d actual response = (%d, %q), want (201, %q)", index, step.Response.ActualStatus, step.Response.ActualBody, want)
		}
	}
	phaseLog := readExecutorFile(t, logPath)
	wantOrder := []string{
		`curl|https://example.test/api/first|{"token":"one"}`,
		"types", "body-expected", "body-actual", "capture",
		"curl-raw-body",
		`curl|https://example.test/api/second|{"token":"one"}`,
		"types", "body-expected", "body-actual",
		"body-expected",
	}
	if got := strings.Fields(phaseLog); !reflect.DeepEqual(got, wantOrder) {
		t.Fatalf("phase order = %v, want %v", got, wantOrder)
	}
	debugStep := &dir.RuntimeSteps[0][1]
	selectedJar := executor.cookies.jarFor(dir, 0)
	for _, want := range []string{
		"stage: 0\ndir-path: /suite\nfile-path: steps.yaml\n\ncurl-command:\n",
		debugStep.RawCurl,
		"--cookie " + selectedJar + " --cookie-jar " + selectedJar,
		`"actual_body"`,
	} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("processDir() output = %q, want %q", output.String(), want)
		}
	}
	if !strings.Contains(debugStep.RawCurl, "curl --disable --globoff --silent --show-error --request POST") ||
		strings.Contains(output.String(), "[\x1b[38;5;10m✓\x1b[0m]") {
		t.Fatalf("processDir() output = %q, want terminal debug without success", output.String())
	}
}

func TestDebugTerminalCurlErrorReportsLatestUnredactedStateAndPreservesError(t *testing.T) {
	commandDir := newExecutorCommandDir(t)
	installExecutorCommand(t, commandDir, "curl", `/bin/cat >/dev/null; printf 'connection refused' >&2; exit 27`)
	installPassthroughExecutorJQ(t, commandDir)

	step := executorStep("secure/steps.yaml", 4)
	step.Debug = true
	step.Request.Method = "POST"
	step.Request.Defaults.BaseURL = "https://example.test"
	step.Request.Path = "/private"
	step.Request.Defaults.Headers = map[string]string{
		"Authorization": "Bearer complete-secret",
		"Cookie":        "session=unredacted-cookie",
	}
	step.Request.Body = `{"latest":"request"}`
	step.Response.ExpectedBody = `{"latest":"expected"}`
	var output bytes.Buffer
	executor := executorForStep(t, NewBinder(NewKeyValueStore()), step, &output)

	exitCode, err := executor.processDir(context.Background(), discardResult, step.Definition.File.Directory)
	if exitCode != 27 || !errors.Is(err, runner.ErrCurl) {
		t.Fatalf("processDir() = (%d, %v), want original Curl exit 27", exitCode, err)
	}
	runtimeStep := &step.Definition.File.Directory.RuntimeSteps[0][0]
	if runtimeStep.RawCurl == "" || runtimeStep.Response.ActualStatus != 0 || runtimeStep.Response.ActualBody != "" {
		t.Fatalf("latest runtime state = RawCurl %q, status %d, body %q", runtimeStep.RawCurl, runtimeStep.Response.ActualStatus, runtimeStep.Response.ActualBody)
	}
	for _, want := range []string{
		"stage: 0\ndir-path: /suite\nfile-path: secure/steps.yaml\n\ncurl-command:\n" + runtimeStep.RawCurl,
		"Authorization: Bearer complete-secret",
		"Cookie: session=unredacted-cookie",
		`"body"`,
		`"latest"`,
		`"request"`,
	} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("terminal Debug output = %q, want %q", output.String(), want)
		}
	}
}

func TestDebugTerminalErrorReportsAfterExecutionContextCancellation(t *testing.T) {
	commandDir := newExecutorCommandDir(t)
	installPassthroughExecutorJQ(t, commandDir)

	step := executorStep("steps.yaml", 0)
	step.Debug = true
	step.RawCurl = "curl --url https://example.test"
	var output bytes.Buffer
	executor := NewExecutor(nil, nil, reporting.NewReporter(&output, false), domain.Config{})
	wantErr := errors.New("terminal execution error")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	exitCode, err := executor.finishStep(ctx, &step, 27, wantErr)
	if exitCode != 27 || !errors.Is(err, wantErr) {
		t.Fatalf("finishStep() = (%d, %v), want original terminal error and exit 27", exitCode, err)
	}
	if got := output.String(); !strings.Contains(got, "stage: 0\ndir-path: /suite\nfile-path: steps.yaml") ||
		!strings.Contains(got, step.RawCurl) {
		t.Fatalf("finishStep() output = %q, want terminal Debug state", got)
	}
}

func TestDebugPeerCancellationSuppressesDebugReport(t *testing.T) {
	step := executorStep("steps.yaml", 0)
	step.Debug = true
	var output bytes.Buffer
	executor := NewExecutor(nil, nil, reporting.NewReporter(&output, false), domain.Config{})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	exitCode, err := executor.finishStep(ctx, &step, 27, context.Canceled)
	if exitCode != 27 || !errors.Is(err, context.Canceled) {
		t.Fatalf("finishStep() = (%d, %v), want canceled terminal result", exitCode, err)
	}
	if output.Len() != 0 {
		t.Fatalf("finishStep() output = %q, want no Debug report after peer cancellation", output.String())
	}
}

func TestDebugReportingFailureIncludesStepProvenance(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	step := executorStep("debug/steps.yaml", 6)
	step.Debug = true
	executor := NewExecutor(nil, nil, reporting.NewReporter(&bytes.Buffer{}, false), domain.Config{})

	exitCode, err := executor.finishStep(context.Background(), &step, 0, nil)
	if exitCode != errs.ExitInternal || !errors.Is(err, reporting.ErrReporter) || !errors.Is(err, ErrStepExecution) {
		t.Fatalf("finishStep() = (%d, %v), want internal reporting ErrStepExecution", exitCode, err)
	}
	if !strings.Contains(err.Error(), "file debug/steps.yaml") || !strings.Contains(err.Error(), "yaml path spec.steps[6]") {
		t.Fatalf("Debug reporting provenance = %q", err)
	}
}

func TestExecuteTreatsDebugStopAsCleanCompletionAndSkipsLaterStages(t *testing.T) {
	debug := &domain.Directory{Path: "/debug"}
	later := &domain.Directory{Path: "/later"}
	visited := make([]string, 0, 1)

	exitCode, err := executeStages(context.Background(), [][]*domain.Directory{{debug}, {later}}, 1, nil, func(_ context.Context, _ resultPublisher, directory *domain.Directory) (int, error) {
		visited = append(visited, directory.Path)
		return 0, errDebugStop
	})
	if exitCode != 0 || err != nil {
		t.Fatalf("executeStages() = (%d, %v), want clean debug completion", exitCode, err)
	}
	if !reflect.DeepEqual(visited, []string{"/debug"}) {
		t.Fatalf("visited directories = %v, want debug stage only", visited)
	}
}

func TestProcessDirReportsEveryValidationAndContinuesThroughCaptureAndLaterSteps(t *testing.T) {
	commandDir := newExecutorCommandDir(t)
	logPath := filepath.Join(commandDir, "phases")
	t.Setenv("APIH_EXECUTOR_LOG", logPath)
	installExecutorCommand(t, commandDir, "curl", `
url=
previous=
for argument do
  case "$previous" in
    url) url=$argument; previous=; continue ;;
  esac
  case "$argument" in
    --url) previous=url ;;
  esac
done
/bin/cat >/dev/null
case "$url" in
  */bad) printf 'actual'; status=500 ;;
  *) printf 'same'; status=200 ;;
esac
printf 'curl|%s\n' "$url" >> "$APIH_EXECUTOR_LOG"
printf 'http-code:%s' "$status" >&2
`)
	installExecutorCommand(t, commandDir, "jq", `
input=$(/bin/cat)
last=
for argument do last=$argument; done
case "$last" in
  .saved)
    printf 'capture\n' >> "$APIH_EXECUTOR_LOG"
    printf '"captured"\n'
    ;;
  *'.bad'*) printf 'type mismatch\n' ;;
  *'.[]'*) ;;
  *) printf '%s\n' "$input" ;;
esac
`)
	installExecutorCommand(t, commandDir, "git", `
printf '%s\n' 'diff --git actual expected' '--- actual' '+++ expected' '@@ -1 +1 @@' '-actual' '+expected'
exit 1
`)

	bad := executorStep("steps.yaml", 0)
	bad.Request.Method = "GET"
	bad.Request.Defaults.BaseURL = "https://example.test"
	bad.Request.Path = "/bad"
	bad.Response.ExpectedStatus = 200
	bad.Response.ExpectedBody = "expected"
	bad.Response.ExpectedTypes = map[string][]string{".bad": {"string"}}
	bad.Response.Capture = map[string]domain.YAMLString{"saved": ".saved"}
	ok := executorStep("steps.yaml", 1)
	ok.Request.Method = "GET"
	ok.Request.Defaults.BaseURL = "https://example.test"
	ok.Request.Path = "/ok"
	ok.Response.ExpectedStatus = 200
	ok.Response.ExpectedBody = "same"
	dir := bad.Definition.File.Directory
	dir.RuntimeSteps = [][]domain.Step{{bad, ok}}
	var output bytes.Buffer
	binder := NewBinder(NewKeyValueStore())

	config := domain.Config{TempRunDir: t.TempDir()}
	executor := NewExecutor(binder, NewValidator(config), reporting.NewReporter(&output, false), config)
	if err := executor.cookies.prepareStage([]*domain.Directory{dir}); err != nil {
		t.Fatal(err)
	}
	exitCode, err := executor.processDir(context.Background(), discardResult, dir)
	if exitCode != errs.ExitValidation || err != nil {
		t.Fatalf("processDir() = (%d, %v), want (%d, nil)", exitCode, err, errs.ExitValidation)
	}
	for _, want := range []string{
		"expected_types:",
		"actual_status:",
		"expected_status:",
		"expected_body:",
	} {
		if !strings.Contains(output.String(), want) {
			t.Errorf("output = %q, want %q", output.String(), want)
		}
	}
	if strings.Contains(output.String(), "[\x1b[38;5;10m✓\x1b[0m]") {
		t.Fatalf("validation output includes success: %q", output.String())
	}
	if got := readExecutorFile(t, logPath); !strings.Contains(got, "capture") || !strings.Contains(got, "/ok") {
		t.Fatalf("phase log = %q, want capture and later step", got)
	}
	if got, err := binder.kvs.Get("saved"); err != nil || got != `"captured"` {
		t.Fatalf("captured value = (%q, %v), want quoted captured", got, err)
	}
}

func TestProcessDirReturnsFatalCollaboratorFailures(t *testing.T) {
	t.Run("load variables", func(t *testing.T) {
		store := NewKeyValueStore()
		if err := store.Set("duplicate", "first"); err != nil {
			t.Fatal(err)
		}
		step := executorStep("steps.yaml", 0)
		step.Vars = map[string]domain.YAMLString{"duplicate": "second"}
		exitCode, err := executorForStep(t, NewBinder(store), step, &bytes.Buffer{}).processDir(context.Background(), discardResult, step.Definition.File.Directory)
		if exitCode != errs.ExitInternal || !errors.Is(err, ErrKeyExists) {
			t.Fatalf("processDir() = (%d, %v), want internal ErrKeyExists", exitCode, err)
		}
	})

	t.Run("request interpolation", func(t *testing.T) {
		step := executorStep("steps.yaml", 0)
		step.Request.Body = "$missing"
		exitCode, err := executorForStep(t, NewBinder(NewKeyValueStore()), step, &bytes.Buffer{}).processDir(context.Background(), discardResult, step.Definition.File.Directory)
		if exitCode != errs.ExitInternal || !errors.Is(err, ErrNotFound) {
			t.Fatalf("processDir() = (%d, %v), want internal ErrNotFound", exitCode, err)
		}
	})

	t.Run("expected interpolation", func(t *testing.T) {
		step := executorStep("steps.yaml", 0)
		step.Response.ExpectedBody = "$missing"
		exitCode, err := executorForStep(t, NewBinder(NewKeyValueStore()), step, &bytes.Buffer{}).processDir(context.Background(), discardResult, step.Definition.File.Directory)
		if exitCode != errs.ExitInternal || !errors.Is(err, ErrNotFound) {
			t.Fatalf("processDir() = (%d, %v), want internal ErrNotFound", exitCode, err)
		}
	})

	t.Run("curl", func(t *testing.T) {
		commandDir := newExecutorCommandDir(t)
		installExecutorCommand(t, commandDir, "curl", `/bin/cat >/dev/null; printf 'curl failed' >&2; exit 27`)
		step := executorStep("dir/steps.yaml", 0)
		step.Request.Method = "GET"
		exitCode, err := executorForStep(t, NewBinder(NewKeyValueStore()), step, &bytes.Buffer{}).processDir(context.Background(), discardResult, step.Definition.File.Directory)
		if exitCode != 27 || !errors.Is(err, runner.ErrCurl) || !errors.Is(err, ErrStepExecution) {
			t.Fatalf("processDir() = (%d, %v), want curl code 27", exitCode, err)
		}
		if !strings.Contains(err.Error(), "file dir/steps.yaml") || !strings.Contains(err.Error(), "yaml path spec.steps[0]") {
			t.Fatalf("terminal step provenance = %q", err)
		}
	})

	t.Run("type validation", func(t *testing.T) {
		commandDir := newExecutorCommandDir(t)
		installSuccessfulExecutorCurl(t, commandDir, "same", 200)
		installExecutorCommand(t, commandDir, "jq", `/bin/cat >/dev/null; printf 'jq failed' >&2; exit 28`)
		step := executorStep("steps.yaml", 0)
		step.Request.Method = "GET"
		step.Response.ExpectedBody = "same"
		exitCode, err := executorForStep(t, NewBinder(NewKeyValueStore()), step, &bytes.Buffer{}).processDir(context.Background(), discardResult, step.Definition.File.Directory)
		if exitCode != 28 || !errors.Is(err, ErrValidatorFatal) {
			t.Fatalf("processDir() = (%d, %v), want validator code 28", exitCode, err)
		}
	})

	t.Run("body validation", func(t *testing.T) {
		commandDir := newExecutorCommandDir(t)
		installSuccessfulExecutorCurl(t, commandDir, "same", 200)
		installExecutorCommand(t, commandDir, "jq", `
input=$(/bin/cat)
last=
for argument do last=$argument; done
case "$last" in
  *'.[]'*) ;;
  *) printf 'body jq failed' >&2; exit 29 ;;
esac
`)
		step := executorStep("steps.yaml", 0)
		step.Request.Method = "GET"
		step.Response.ExpectedBody = "same"
		exitCode, err := executorForStep(t, NewBinder(NewKeyValueStore()), step, &bytes.Buffer{}).processDir(context.Background(), discardResult, step.Definition.File.Directory)
		if exitCode != 29 || !errors.Is(err, ErrValidatorFatal) {
			t.Fatalf("processDir() = (%d, %v), want validator code 29", exitCode, err)
		}
	})

	t.Run("capture", func(t *testing.T) {
		commandDir := newExecutorCommandDir(t)
		installSuccessfulExecutorCurl(t, commandDir, "same", 200)
		installExecutorCommand(t, commandDir, "jq", `
input=$(/bin/cat)
last=
for argument do last=$argument; done
case "$last" in
  .capture) printf 'capture failed' >&2; exit 30 ;;
  *'.[]'*) ;;
  *) printf '%s\n' "$input" ;;
esac
`)
		step := executorStep("steps.yaml", 0)
		step.Request.Method = "GET"
		step.Response.ExpectedBody = "same"
		step.Response.Capture = map[string]domain.YAMLString{"captured": ".capture"}
		exitCode, err := executorForStep(t, NewBinder(NewKeyValueStore()), step, &bytes.Buffer{}).processDir(context.Background(), discardResult, step.Definition.File.Directory)
		if exitCode != 30 || !errors.Is(err, runner.ErrJQSelector) {
			t.Fatalf("processDir() = (%d, %v), want capture code 30", exitCode, err)
		}
	})
}

func TestProcessDirReturnsReporterFailures(t *testing.T) {
	wantErr := errors.New("write failed")
	t.Run("success", func(t *testing.T) {
		commandDir := newExecutorCommandDir(t)
		installSuccessfulExecutorCurl(t, commandDir, "same", 200)
		installPassthroughExecutorJQ(t, commandDir)
		step := executorStep("steps.yaml", 0)
		step.Request.Method = "GET"
		step.Response.ExpectedBody = "same"
		exitCode, err := executorForStep(t, NewBinder(NewKeyValueStore()), step, executorFailingWriter{err: wantErr}).processDir(context.Background(), discardResult, step.Definition.File.Directory)
		if exitCode != errs.ExitInternal || !errors.Is(err, reporting.ErrReporter) || !errors.Is(err, wantErr) {
			t.Fatalf("processDir() = (%d, %v), want reporting failure", exitCode, err)
		}
	})

	t.Run("validation", func(t *testing.T) {
		commandDir := newExecutorCommandDir(t)
		installSuccessfulExecutorCurl(t, commandDir, "same", 500)
		installPassthroughExecutorJQ(t, commandDir)
		step := executorStep("steps.yaml", 0)
		step.Request.Method = "GET"
		step.Response.ExpectedStatus = 200
		step.Response.ExpectedBody = "same"
		exitCode, err := executorForStep(t, NewBinder(NewKeyValueStore()), step, executorFailingWriter{err: wantErr}).processDir(context.Background(), discardResult, step.Definition.File.Directory)
		if exitCode != errs.ExitInternal || !errors.Is(err, reporting.ErrReporter) {
			t.Fatalf("processDir() = (%d, %v), want validation reporting failure", exitCode, err)
		}
	})

	t.Run("type validation", func(t *testing.T) {
		commandDir := newExecutorCommandDir(t)
		installSuccessfulExecutorCurl(t, commandDir, "same", 200)
		installExecutorCommand(t, commandDir, "jq", `/bin/cat >/dev/null; printf 'type mismatch\n'`)
		step := executorStep("steps.yaml", 0)
		step.Request.Method = "GET"
		step.Response.ExpectedBody = "same"
		exitCode, err := executorForStep(t, NewBinder(NewKeyValueStore()), step, executorFailingWriter{err: wantErr}).processDir(context.Background(), discardResult, step.Definition.File.Directory)
		if exitCode != errs.ExitInternal || !errors.Is(err, reporting.ErrReporter) {
			t.Fatalf("processDir() = (%d, %v), want type reporting failure", exitCode, err)
		}
	})

	t.Run("body validation", func(t *testing.T) {
		commandDir := newExecutorCommandDir(t)
		installSuccessfulExecutorCurl(t, commandDir, "actual", 200)
		installExecutorCommand(t, commandDir, "jq", `
input=$(/bin/cat)
last=
for argument do last=$argument; done
case "$last" in
  *'.[]'*) ;;
  *) printf '%s\n' "$input" ;;
esac
`)
		installExecutorCommand(t, commandDir, "git", `
printf '%s\n' 'diff --git actual expected' '--- actual' '+++ expected' '@@ -1 +1 @@' '-actual' '+expected'
exit 1
`)
		step := executorStep("steps.yaml", 0)
		step.Request.Method = "GET"
		step.Response.ExpectedBody = "expected"
		exitCode, err := executorForStep(t, NewBinder(NewKeyValueStore()), step, executorFailingWriter{err: wantErr}).processDir(context.Background(), discardResult, step.Definition.File.Directory)
		if exitCode != errs.ExitInternal || !errors.Is(err, reporting.ErrReporter) {
			t.Fatalf("processDir() = (%d, %v), want body reporting failure", exitCode, err)
		}
	})

	t.Run("debug", func(t *testing.T) {
		commandDir := newExecutorCommandDir(t)
		installSuccessfulExecutorCurl(t, commandDir, "same", 200)
		installPassthroughExecutorJQ(t, commandDir)
		step := executorStep("steps.yaml", 0)
		step.Request.Method = "GET"
		step.Response.ExpectedBody = "same"
		step.Debug = true
		exitCode, err := executorForStep(t, NewBinder(NewKeyValueStore()), step, executorFailingWriter{err: wantErr}).processDir(context.Background(), discardResult, step.Definition.File.Directory)
		if exitCode != errs.ExitInternal || !errors.Is(err, reporting.ErrReporter) {
			t.Fatalf("processDir() = (%d, %v), want debug reporting failure", exitCode, err)
		}
	})
}

func TestProcessDirRejectsCanceledContextBeforeWork(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	exitCode, err := NewExecutor(nil, nil, nil, domain.Config{}).processDir(ctx, discardResult, &domain.Directory{})
	if exitCode != errs.ExitInternal || !errors.Is(err, ErrExecutionCanceled) || !errors.Is(err, context.Canceled) {
		t.Fatalf("processDir(canceled) = (%d, %v), want internal ErrExecutionCanceled", exitCode, err)
	}
}

func TestProcessFileRejectsCanceledContextBeforeLoadingVariables(t *testing.T) {
	store := NewKeyValueStore()
	step := executorStep("steps.yaml", 0)
	step.Vars = map[string]domain.YAMLString{"late": "value"}
	step.Definition.File.Directory.RuntimeSteps = [][]domain.Step{{step}}
	executor := NewExecutor(NewBinder(store), nil, reporting.NewReporter(&bytes.Buffer{}, false), domain.Config{})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	exitCode, err := executor.processFile(ctx, step.Definition.File.Directory, 0)
	if exitCode != errs.ExitInternal || !errors.Is(err, context.Canceled) {
		t.Fatalf("processFile() = (%d, %v), want canceled terminal result", exitCode, err)
	}
	if _, err := store.Get("late"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("canceled processFile stored variable: %v", err)
	}
}

func TestFatalResultPreservesExplicitAndCodedResults(t *testing.T) {
	wantErr := errors.New("fatal")
	if code, err := fatalResult(19, wantErr); code != 19 || !errors.Is(err, wantErr) {
		t.Fatalf("fatalResult(explicit) = (%d, %v)", code, err)
	}
	coded := errs.WithExitCode(23, wantErr)
	if code, err := fatalResult(0, coded); code != 23 || !errors.Is(err, wantErr) {
		t.Fatalf("fatalResult(coded) = (%d, %v)", code, err)
	}
	if code, err := fatalResult(0, nil); code != errs.ExitInternal || err != nil {
		t.Fatalf("fatalResult(nil) = (%d, %v), want internal nil", code, err)
	}
}

type executorFailingWriter struct {
	err error
}

func (w executorFailingWriter) Write([]byte) (int, error) {
	return 0, w.err
}

func executorStep(path string, index int) domain.Step {
	directory := &domain.Directory{Path: "/suite"}
	file := &domain.File{Path: path, Directory: directory}
	definition := &domain.StepsDefinition{File: file}
	directory.StepsDefinitions = []*domain.StepsDefinition{definition}
	return domain.Step{Definition: definition, Index: index}
}

func executorForStep(t *testing.T, binder *Binder, step domain.Step, output interface{ Write([]byte) (int, error) }) *Executor {
	t.Helper()
	step.Definition.File.Directory.RuntimeSteps = [][]domain.Step{{step}}
	config := domain.Config{TempRunDir: t.TempDir()}
	executor := NewExecutor(binder, NewValidator(config), reporting.NewReporter(output, false), config)
	if err := executor.cookies.prepareStage([]*domain.Directory{step.Definition.File.Directory}); err != nil {
		t.Fatal(err)
	}
	return executor
}

func discardResult(int, error) {}

func newExecutorCommandDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return dir
}

func installExecutorCommand(t *testing.T, dir, name, body string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("#!/bin/sh\nset -eu\n"+body+"\n"), 0o755); err != nil {
		t.Fatalf("WriteFile(%s) error = %v", path, err)
	}
}

func installSuccessfulExecutorCurl(t *testing.T, dir, body string, status int) {
	t.Helper()
	installExecutorCommand(t, dir, "curl", `
/bin/cat >/dev/null
printf '%s' '`+body+`'
printf 'http-code:`+strconv.Itoa(status)+`' >&2
`)
}

func installPassthroughExecutorJQ(t *testing.T, dir string) {
	t.Helper()
	installExecutorCommand(t, dir, "jq", `
input=$(/bin/cat)
last=
for argument do last=$argument; done
case "$last" in
  *'.[]'*) ;;
  *) printf '%s\n' "$input" ;;
esac
`)
}

func readExecutorFile(t *testing.T, path string) string {
	t.Helper()
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", path, err)
	}
	return string(contents)
}
