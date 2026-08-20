package execution

import (
	"apih/internal/domain"
	"apih/internal/reporter"
	"apih/pkg/errs"
	"apih/pkg/runner"
	"context"
	"errors"
	"sync"
)

// ErrInvalidDirectoryTree reports malformed Suite directory relationships.
var ErrInvalidDirectoryTree = errors.New("invalid directory tree")

// ErrExecutionCanceled reports cancellation between execution stages.
var ErrExecutionCanceled = errors.New("execution canceled")

// StepRunner coordinates step preparation, execution, validation, and reporting.
type StepRunner struct {
	varProc *VariableProcessor
	val     *Validator
	report  *reporter.Reporter
}

// NewStepRunner retains the collaborators used by the preparation and execution phases.
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

// Prepare traverses the suite tree and prepares every resolved step.
func (s *StepRunner) Prepare(
	ctx context.Context,
	suite *domain.Suite,
) error {
	dirs, err := collectDirs(suite)
	if err != nil {
		return err
	}

	return prepareDirs(ctx, dirs, s.varProc.Load, s.varProc.ParseRequestBody)
}

type preparePhase func(context.Context, *domain.Step) (int, error)

func prepareDirs(
	ctx context.Context,
	dirs [][]*domain.Directory,
	load preparePhase,
	parseRequestBody preparePhase,
) error {
	for _, stage := range dirs {
		for _, dir := range stage {
			for groupIndex := range dir.ResolvedSteps {
				for stepIndex := range dir.ResolvedSteps[groupIndex] {
					step := &dir.ResolvedSteps[groupIndex][stepIndex]
					if _, err := load(ctx, step); err != nil {
						return err
					}
					if _, err := parseRequestBody(ctx, step); err != nil {
						return err
					}
				}
			}
		}
	}

	return nil
}

// Execute validates the suite tree and executes its directories one stage at a time.
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
	if suite.Root.Parent != nil {
		return nil, errs.Build(errs.ExitConfiguration, ErrInvalidDirectoryTree, nil, "root parent is not nil")
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

type stepOperations struct {
	curl                  func(context.Context, string, string, map[string]string, int, int, string, string) (string, int, error)
	parseResponseExpected preparePhase
	validateTypes         func(context.Context, *domain.Step) []error
	validateExpected      func(context.Context, *domain.Step) error
	capture               preparePhase
	reportTypes           func(context.Context, *domain.Step, error) error
	reportExpected        func(context.Context, *domain.Step, error) error
}

func (s *StepRunner) processDir(ctx context.Context, dir *domain.Directory) (int, error) {
	if err := ctx.Err(); err != nil {
		return errs.ExitInternal, errs.Build(errs.ExitInternal, ErrExecutionCanceled, err)
	}

	operations := stepOperations{
		curl:                  runner.Curl,
		parseResponseExpected: s.varProc.ParseResponseExpected,
		validateTypes:         s.val.ValidateTypes,
		validateExpected:      s.val.ValidateExpected,
		capture:               s.varProc.Capture,
		reportTypes:           s.report.FailureTypes,
		reportExpected:        s.report.FailureDiff,
	}
	return processResolvedSteps(ctx, dir, operations)
}

func processResolvedSteps(
	ctx context.Context,
	dir *domain.Directory,
	operations stepOperations,
) (int, error) {
	validationFailed := false

	for groupIndex := range dir.ResolvedSteps {
		for stepIndex := range dir.ResolvedSteps[groupIndex] {
			step := &dir.ResolvedSteps[groupIndex][stepIndex]
			failed, exitCode, err := processStep(ctx, step, operations)
			if err != nil {
				return exitCode, err
			}
			validationFailed = validationFailed || failed
		}
	}

	if validationFailed {
		return errs.ExitValidation, errs.Build(errs.ExitValidation, ErrValidation, nil)
	}
	return errs.ExitSuccess, nil
}

func processStep(
	ctx context.Context,
	step *domain.Step,
	operations stepOperations,
) (bool, int, error) {
	responseBody, exitCode, err := operations.curl(
		ctx,
		step.Request.Method,
		step.Request.BaseURL+step.Request.BasePath+step.Request.Path,
		step.Request.Headers,
		step.Request.Timeout,
		step.Request.Retries,
		step.Request.Query,
		string(step.Request.Body),
	)
	if err != nil {
		return false, exitCode, err
	}
	step.Response.Body = responseBody

	if exitCode, err = operations.parseResponseExpected(ctx, step); err != nil {
		return false, exitCode, err
	}

	validationFailed := false
	for _, failure := range operations.validateTypes(ctx, step) {
		if failure == nil {
			continue
		}
		validationFailed = true
		if err := operations.reportTypes(ctx, step, failure); err != nil {
			return true, errs.Code(err, errs.ExitInternal), err
		}
	}

	if failure := operations.validateExpected(ctx, step); failure != nil {
		validationFailed = true
		if err := operations.reportExpected(ctx, step, failure); err != nil {
			return true, errs.Code(err, errs.ExitInternal), err
		}
	}

	if exitCode, err = operations.capture(ctx, step); err != nil {
		return validationFailed, exitCode, err
	}

	return validationFailed, errs.ExitSuccess, nil
}
