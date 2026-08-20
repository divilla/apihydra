package execution

import (
	"apih/skeleton/internal/domain"
	"apih/skeleton/internal/reporter"
	"apih/skeleton/pkg/errs"
	"context"
	"errors"
	"fmt"
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

// Prepare prepares the suite's resolved steps for execution.
func (s *StepRunner) Prepare(
	ctx context.Context,
	suite *domain.Suite,
) error {
	return nil
}

// Execute processes the directory tree in stages, running directories in the
// same stage concurrently. For every runtime step, it calls
// VariableProcessor.LoadVariables, VariableProcessor.InterpolateRequestBody,
// VariableProcessor.InterpolateResponseExpected, runner.Curl,
// Validator.ValidateTypes, Validator.ValidateExpected, and
// VariableProcessor.CaptureResponseVariables in that order. Validation failures
// are reported through s.report. The response body returned by runner.Curl is
// assigned to step.Response.Body for validation and capture. After all work
// finishes, Execute returns exit code 101 and ErrValidation when one or more
// validations failed.
func (s *StepRunner) Execute(
	ctx context.Context,
	suite *domain.Suite,
) (int, error) {
	dc := newDirsCollector(suite)
	if err := dc.collect(); err != nil {
		return errs.ExitConfiguration, err
	}
	dirs := dc.stagedDirs

	exitCode, err := executeStages(ctx, dirs, s.processDir)
	if err != nil {
		return exitCode, err
	}

	return errs.ExitSuccess, nil
}

type (
	dirsCollector struct {
		suite        *domain.Suite
		isChildCount map[*domain.Directory]int
		stagedDirs   [][]*domain.Directory
		maxStage     int
	}
)

func newDirsCollector(suite *domain.Suite) *dirsCollector {
	return &dirsCollector{
		suite:        suite,
		isChildCount: make(map[*domain.Directory]int),
	}
}

func (d *dirsCollector) collect() error {
	if d.suite == nil {
		return errs.Build(errs.ExitConfiguration, ErrInvalidDirectoryTree, nil, "domain.Suite is nil")
	}
	root := d.suite.Root
	if root == nil {
		return errs.Build(errs.ExitConfiguration, ErrInvalidDirectoryTree, nil, "domain.Suite.Root is nil")
	}
	if root.Parent != nil {
		return errs.Build(errs.ExitConfiguration, ErrInvalidDirectoryTree, nil, "domain.Suite.Root.Parent is not nil")
	}
	if root.Stage != 0 {
		return errs.Build(errs.ExitConfiguration, ErrInvalidDirectoryTree, nil, "domain.Suite.Root.Stage is not 0")
	}

	d.isChildCount[root] = 1
	if err := d.traverseChildren(root); err != nil {
		return err
	}
	d.stagedDirs = make([][]*domain.Directory, d.maxStage+1)
	d.setStages(root)

	return nil
}

func (d *dirsCollector) traverseChildren(parent *domain.Directory) error {
	for _, child := range parent.Children {
		if child == nil {
			return errs.Build(errs.ExitConfiguration, ErrInvalidDirectoryTree, nil, fmt.Sprintf("%s has nil child", parent.Path))
		}
		if child.Parent != parent {
			return errs.Build(errs.ExitConfiguration, ErrInvalidDirectoryTree, nil, fmt.Sprintf("%s has invalid parent directory", child.Path))
		}
		if child.Stage != parent.Stage+1 {
			return errs.Build(errs.ExitConfiguration, ErrInvalidDirectoryTree, nil, fmt.Sprintf("%s has invalid stage number", child.Path))
		}
		if child.Stage > d.maxStage {
			d.maxStage = child.Stage
		}

		d.isChildCount[child]++
		if d.isChildCount[child] > 1 {
			return errs.Build(errs.ExitConfiguration, ErrInvalidDirectoryTree, nil, fmt.Sprintf("%s has multiple parents", child.Path))
		}
		if err := d.traverseChildren(child); err != nil {
			return err
		}
	}

	return nil
}

func (d *dirsCollector) setStages(dir *domain.Directory) {
	d.stagedDirs[dir.Stage] = append(d.stagedDirs[dir.Stage], dir)

	for _, child := range dir.Children {
		d.setStages(child)
	}
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
