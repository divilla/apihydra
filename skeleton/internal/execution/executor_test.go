package execution

import (
	"apih/skeleton/internal/domain"
	"apih/skeleton/pkg/errs"
	"context"
	"errors"
	"strconv"
	"sync/atomic"
	"testing"
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
	executor := NewExecutor(nil, nil, nil)
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
			executor := NewExecutor(nil, nil, nil)
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
	process := func(ctx context.Context, dir *domain.Directory) (int, error) {
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
		func(context.Context, *domain.Directory) (int, error) {
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
		func(_ context.Context, dir *domain.Directory) (int, error) {
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
		func(_ context.Context, dir *domain.Directory) (int, error) {
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
