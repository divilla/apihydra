package execution

import (
	"apih/skeleton/internal/domain"
	"apih/skeleton/internal/reporting"
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
	report  *reporting.Reporter
}

func NewStepRunner(
	variableProcessor *VariableProcessor,
	validator *Validator,
	report *reporting.Reporter,
) *StepRunner {
	return &StepRunner{
		varProc: variableProcessor,
		val:     validator,
		report:  report,
	}
}

func (s *StepRunner) ValidateDirectories(
	suite *domain.Suite,
) (int, error) {
	dc := newDirsValidator(suite)
	if err := dc.validateRoot(); err != nil {
		return errs.ExitConfiguration, err
	}
	return 0, nil
}

func (s *StepRunner) PlanStages(
	suite *domain.Suite,
) [][]*domain.Directory {
	maxStages := findMaxStage(suite.Root, 0)
	sd := newStagedDirs(maxStages)
	sd.setStages(suite.Root)
	return sd.stagedDirs
}

// Prepare deep-copies every directory's ResolvedSteps into RuntimeSteps so
// execution can mutate runtime steps without modifying resolved steps. All
// mutable slices and maps are copied; Step.Definition retains its original
// provenance pointer.
// Variable loading and interpolation are runtime phases performed by Execute.
func (s *StepRunner) Prepare(
	suite *domain.Suite,
) {
	// TODO: implement
}

// Execute processes the directory tree in stages, running directories in the
// same stage concurrently. For every runtime step, it calls
// VariableProcessor.LoadVariables, VariableProcessor.InterpolateRequestBody,
// VariableProcessor.InterpolateResponseExpected, runner.Curl,
// Validator.ValidateTypes, Validator.ValidateExpected, and
// VariableProcessor.CaptureResponseVariables in that order. Validation failures
// are reported through s.report. The response body returned by runner.Curl is
// assigned to step.Response.Body for validation and capture. After all work
// finishes, Execute returns exit code 101 and a nil error when one or more
// validations failed. Validation status does not cancel remaining work.
func (s *StepRunner) Execute(
	ctx context.Context,
	stages [][]*domain.Directory,
) (int, error) {
	exitCode, err := executeStages(ctx, stages, s.processDir)
	if err != nil {
		return exitCode, err
	}

	return exitCode, nil
}

type (
	dirsValidator struct {
		suite        *domain.Suite
		isChildCount map[*domain.Directory]int
	}
	stagedDirs struct {
		stagedDirs [][]*domain.Directory
	}
)

func newDirsValidator(suite *domain.Suite) *dirsValidator {
	return &dirsValidator{
		suite:        suite,
		isChildCount: make(map[*domain.Directory]int),
	}
}

func (d *dirsValidator) validateRoot() error {
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
	if err := d.validateChildren(root); err != nil {
		return err
	}

	return nil
}

func (d *dirsValidator) validateChildren(parent *domain.Directory) error {
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

		d.isChildCount[child]++
		if d.isChildCount[child] > 1 {
			return errs.Build(errs.ExitConfiguration, ErrInvalidDirectoryTree, nil, fmt.Sprintf("%s has multiple parents", child.Path))
		}
		if err := d.validateChildren(child); err != nil {
			return err
		}
	}

	return nil
}

func findMaxStage(dir *domain.Directory, maxStage int) int {
	if dir.Stage > maxStage {
		maxStage = dir.Stage
	}
	for _, child := range dir.Children {
		maxStage = findMaxStage(child, maxStage)
	}
	return maxStage
}

func newStagedDirs(maxStage int) *stagedDirs {
	return &stagedDirs{
		stagedDirs: make([][]*domain.Directory, maxStage+1),
	}
}

func (d *stagedDirs) setStages(dir *domain.Directory) {
	d.stagedDirs[dir.Stage] = append(d.stagedDirs[dir.Stage], dir)

	for _, child := range dir.Children {
		d.setStages(child)
	}
}

type directoryProcessor func(context.Context, *domain.Directory) (int, error)

type processResult struct {
	mu   sync.Mutex
	code int
	err  error
}

func (r *processResult) setResult(code int, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if code > r.code {
		r.code = code
		r.err = err
	}
}

func executeStages(
	ctx context.Context,
	dirs [][]*domain.Directory,
	process directoryProcessor,
) (int, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	var firstStatusCode int

	for _, stage := range dirs {
		exitCode, err := executeStage(ctx, cancel, stage, process)
		if err != nil {
			return exitCode, err
		}
		if firstStatusCode == 0 && exitCode != 0 {
			firstStatusCode = exitCode
		}
		if err := ctx.Err(); err != nil {
			return errs.ExitInternal, errs.Build(errs.ExitInternal, ErrExecutionCanceled, err)
		}
	}

	return firstStatusCode, nil
}

func executeStage(
	ctx context.Context,
	cancel context.CancelFunc,
	dirs []*domain.Directory,
	process directoryProcessor,
) (int, error) {
	var wg sync.WaitGroup
	var result processResult

	for _, dir := range dirs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			exitCode, err := process(ctx, dir)
			result.setResult(exitCode, err)
			if err != nil {
				cancel()
			}
		}()
	}

	wg.Wait()
	return result.code, result.err
}

func (s *StepRunner) processDir(ctx context.Context, dir *domain.Directory) (int, error) {
	if err := ctx.Err(); err != nil {
		return errs.ExitInternal, errs.Build(errs.ExitInternal, ErrExecutionCanceled, err)
	}

	// TODO: implement
	_ = s
	_ = dir
	return 0, nil
}
