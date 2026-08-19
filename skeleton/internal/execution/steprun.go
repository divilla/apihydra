package execution

import (
	"apih/skeleton/internal/domain"
	"apih/skeleton/internal/reporter"
	"apih/skeleton/pkg/errs"
	"context"
	"errors"
	"sync"
)

var ErrInvalidDirectoryTree = errors.New("invalid directory tree")
var ErrExecutionCanceled = errors.New("execution canceled")

type StepRunner struct {
	varProc *VariableProcessor
	val     *Validator
	report  *reporter.Reporter
}

func NewStepRunner(
	variableProcessor *VariableProcessor,
	validator *Validator,
	report *reporter.Reporter,
) *StepRunner {
	return &StepRunner{
		varProc: variableProcessor,
		val:     validator,
		report:  report,
	}
}

// Prepare traverses directories through children, starting from suite.Root
// For each directory it iterates directory.ResolvedSteps and executes varProc.Load and varProc.ParseRequestBody
func (s *StepRunner) Prepare(
	ctx context.Context,
	suite *domain.Suite,
) error {
	return nil
}

// Execute traverses directories through children, starting from suite.Root, from goroutines, one for each directory,
// until entire same number stage is executed. For each directory it iterates directory.ResolvedSteps and executes
// runner.Curl, varProc.ParseResponseExpected, val.ValidateTypes, val.ValidateExpected and finally varProc.Capture
// On detected validation error, Execute reports failed validation through s.report.
// Once it finishes traversing with one or more validation failures it returns exit code 101 and ErrValidation.
func (s *StepRunner) Execute(
	ctx context.Context,
	suite *domain.Suite,
) (int, error) {
	dirs, err := collectDirs(suite)
	if err != nil {
		return errs.ExitConfiguration, err
	}

	exitCode, err := executeStages(ctx, dirs, s.processDir)
	if err != nil {
		return exitCode, err
	}

	return errs.ExitSuccess, nil
}

func collectDirs(suite *domain.Suite) ([][]*domain.Directory, error) {
	if suite == nil {
		return nil, errs.Build(errs.ExitConfiguration, ErrInvalidDirectoryTree, nil, "suite is nil")
	}
	if suite.Root == nil {
		return nil, errs.Build(errs.ExitConfiguration, ErrInvalidDirectoryTree, nil, "root is nil")
	}
	if suite.Root.Stage != 0 {
		return nil, errs.Build(
			errs.ExitConfiguration,
			ErrInvalidDirectoryTree,
			nil,
			"root stage is ", suite.Root.Stage, ", expected 0",
		)
	}

	dirs := make([][]*domain.Directory, 1)
	seen := make(map[*domain.Directory]struct{})

	var visit func(*domain.Directory, *domain.Directory) error
	visit = func(dir, parent *domain.Directory) error {
		if dir == nil {
			return errs.Build(errs.ExitConfiguration, ErrInvalidDirectoryTree, nil, "nil child")
		}
		if _, ok := seen[dir]; ok {
			return errs.Build(errs.ExitConfiguration, ErrInvalidDirectoryTree, nil, "repeated directory ", dir.Path)
		}
		seen[dir] = struct{}{}

		if parent != nil {
			if dir.Parent != parent {
				return errs.Build(
					errs.ExitConfiguration,
					ErrInvalidDirectoryTree,
					nil,
					"directory ", dir.Path, " has an invalid parent",
				)
			}
			if dir.Stage != parent.Stage+1 {
				return errs.Build(
					errs.ExitConfiguration,
					ErrInvalidDirectoryTree,
					nil,
					"directory ", dir.Path, " has stage ", dir.Stage,
					", expected ", parent.Stage+1,
				)
			}
		}

		for len(dirs) <= dir.Stage {
			dirs = append(dirs, nil)
		}
		dirs[dir.Stage] = append(dirs[dir.Stage], dir)

		for _, child := range dir.Children {
			if err := visit(child, dir); err != nil {
				return err
			}
		}

		return nil
	}

	if err := visit(suite.Root, nil); err != nil {
		return nil, err
	}

	return dirs, nil
}

type directoryProcessor func(context.Context, *domain.Directory) (int, error)

func executeStages(
	ctx context.Context,
	dirs [][]*domain.Directory,
	process directoryProcessor,
) (int, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	for _, stage := range dirs {
		exitCode, err := executeStage(ctx, cancel, stage, process)
		if err != nil {
			return exitCode, err
		}
		if err := ctx.Err(); err != nil {
			return errs.ExitInternal, errs.Build(errs.ExitInternal, ErrExecutionCanceled, err)
		}
	}

	return errs.ExitSuccess, nil
}

func executeStage(
	ctx context.Context,
	cancel context.CancelFunc,
	dirs []*domain.Directory,
	process directoryProcessor,
) (int, error) {
	var wg sync.WaitGroup
	var firstExitCode int
	var firstErr error
	var firstErrOnce sync.Once

	for _, dir := range dirs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			exitCode, err := process(ctx, dir)
			if err != nil {
				firstErrOnce.Do(func() {
					if exitCode == errs.ExitSuccess {
						exitCode = errs.Code(err, errs.ExitInternal)
					}
					firstExitCode = exitCode
					firstErr = err
					cancel()
				})
			}
		}()
	}

	wg.Wait()
	return firstExitCode, firstErr
}

func (s *StepRunner) processDir(ctx context.Context, dir *domain.Directory) (int, error) {
	if err := ctx.Err(); err != nil {
		return errs.ExitInternal, errs.Build(errs.ExitInternal, ErrExecutionCanceled, err)
	}

	_ = s
	_ = dir
	return errs.ExitSuccess, nil
}
