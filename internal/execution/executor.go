package execution

import (
	"apih/internal/domain"
	"apih/internal/reporting"
	"apih/pkg/errs"
	"apih/pkg/runner"
	"context"
	"errors"
	"fmt"
	"sync"
)

// ErrInvalidDirectoryTree classifies a malformed Suite directory tree.
var ErrInvalidDirectoryTree = errors.New("invalid directory tree")

// ErrExecutionCanceled classifies cancellation of staged execution.
var ErrExecutionCanceled = errors.New("execution canceled")

var errDebugStop = errors.New("debug stop")

// Executor prepares, schedules, executes, validates, and reports runtime
// steps.
type Executor struct {
	binder *Binder
	val    *Validator
	report *reporting.Reporter
}

// NewExecutor retains the collaborators used during execution.
func NewExecutor(
	binder *Binder,
	validator *Validator,
	report *reporting.Reporter,
) *Executor {
	return &Executor{
		binder: binder,
		val:    validator,
		report: report,
	}
}

// ValidateDirectories verifies the root, parent links, stages, and uniqueness of
// every directory reachable from suite.Root.
func (e *Executor) ValidateDirectories(
	suite *domain.Suite,
) (int, error) {
	dc := newDirsValidator(suite)
	if err := dc.validateRoot(); err != nil {
		return errs.ExitConfiguration, err
	}
	return 0, nil
}

// PlanStages groups a validated directory tree by stage while preserving each
// directory pointer.
func (e *Executor) PlanStages(
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
func (e *Executor) Prepare(
	suite *domain.Suite,
) {
	prepareDirectory(suite.Root)
}

// Execute processes the directory tree in stages, running directories in the
// same stage concurrently. For every runtime step, it calls
// Binder.LoadVariables, Binder.InterpolateRequestBody,
// Binder.InterpolateResponseExpectedBody, runner.Curl,
// Validator.ValidateTypes, Validator.ValidateStatus, Validator.ValidateBody,
// and Binder.CaptureResponseVariables in that order. A non-empty
// failed string from ValidateTypes, an ErrValidation from ValidateStatus, and a
// non-empty diff from ValidateBody are reported through e.report. The response
// status and body returned by runner.Curl are assigned to
// step.Response.ActualStatus and step.Response.ActualBody for validation and
// capture. After all work finishes, Execute returns exit code 101 and a nil
// error when one or more validations failed. Validation status does not cancel
// remaining work.
func (e *Executor) Execute(
	ctx context.Context,
	stages [][]*domain.Directory,
) (int, error) {
	exitCode, err := executeStages(ctx, stages, e.processDir)
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

	if errors.Is(err, errDebugStop) {
		r.code = 0
		r.err = err
		return
	}
	if errors.Is(r.err, errDebugStop) {
		return
	}
	if code == 0 {
		return
	}
	if r.code == 0 {
		r.code = code
		r.err = err
	}
	if r.code == errs.ExitValidation && r.err == nil && err != nil {
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
		if errors.Is(err, errDebugStop) {
			return 0, nil
		}
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

func (e *Executor) processDir(ctx context.Context, dir *domain.Directory) (int, error) {
	if err := ctx.Err(); err != nil {
		return errs.ExitInternal, errs.Build(errs.ExitInternal, ErrExecutionCanceled, err)
	}

	validationFailed := false
	for groupIndex := range dir.RuntimeSteps {
		for stepIndex := range dir.RuntimeSteps[groupIndex] {
			step := &dir.RuntimeSteps[groupIndex][stepIndex]

			if exitCode, err := e.binder.LoadVariables(ctx, step); err != nil {
				return fatalResult(exitCode, err)
			}
			if exitCode, err := e.binder.InterpolateRequestBody(ctx, step); err != nil {
				return fatalResult(exitCode, err)
			}
			if exitCode, err := e.binder.InterpolateResponseExpectedBody(ctx, step); err != nil {
				return fatalResult(exitCode, err)
			}

			body, status, err := runner.Curl(
				ctx,
				step.Request.Method,
				step.Request.Defaults.BaseURL+step.Request.Defaults.BasePath+step.Request.Path,
				step.Request.Defaults.Headers,
				step.Request.Defaults.Timeout,
				step.Request.Defaults.Retries,
				step.Request.Query,
				string(step.Request.Body),
			)
			if err != nil {
				return fatalResult(status, err)
			}
			step.Response.ActualStatus = status
			step.Response.ActualBody = domain.YAMLString(body)

			failedTypes, err := e.val.ValidateTypes(ctx, step)
			if err != nil {
				return fatalResult(0, err)
			}

			var statusFailure error
			if err := e.val.ValidateStatus(ctx, step); err != nil {
				if !errors.Is(err, ErrValidation) {
					return fatalResult(0, err)
				}
				statusFailure = err
			}

			diff, err := e.val.ValidateBody(ctx, step)
			if err != nil {
				return fatalResult(0, err)
			}
			failureCount := 0
			if failedTypes != "" {
				failureCount++
			}
			if statusFailure != nil {
				failureCount++
			}
			if diff != "" {
				failureCount++
			}
			if failureCount > 0 {
				validationFailed = true
			}
			nextReportContext := func() context.Context {
				failureCount--
				if failureCount == 0 {
					return context.WithValue(ctx, e.report, true)
				}
				return ctx
			}
			if failedTypes != "" {
				if err := e.report.ValidationTypes(nextReportContext(), step, failedTypes); err != nil {
					return fatalResult(0, err)
				}
			}
			if statusFailure != nil {
				if err := e.report.ValidationStatus(nextReportContext(), step, statusFailure); err != nil {
					return fatalResult(0, err)
				}
			}
			if diff != "" {
				if err := e.report.ValidationBody(nextReportContext(), step, diff); err != nil {
					return fatalResult(0, err)
				}
			}

			if exitCode, err := e.binder.CaptureResponseVariables(ctx, step); err != nil {
				return fatalResult(exitCode, err)
			}
			if step.Debug {
				if err := e.report.Debug(ctx, step); err != nil {
					return fatalResult(0, err)
				}
				return 0, errDebugStop
			}
		}
	}

	if err := e.report.Success(ctx, dir); err != nil {
		return fatalResult(0, err)
	}
	if validationFailed {
		return errs.ExitValidation, nil
	}
	return 0, nil
}

func prepareDirectory(dir *domain.Directory) {
	dir.RuntimeSteps = cloneStepGroups(dir.ResolvedSteps)
	for _, child := range dir.Children {
		prepareDirectory(child)
	}
}

func cloneStepGroups(groups [][]domain.Step) [][]domain.Step {
	if groups == nil {
		return nil
	}

	cloned := make([][]domain.Step, len(groups))
	for groupIndex, group := range groups {
		if group == nil {
			continue
		}
		cloned[groupIndex] = make([]domain.Step, len(group))
		for stepIndex := range group {
			cloned[groupIndex][stepIndex] = cloneStep(group[stepIndex])
		}
	}
	return cloned
}

func cloneStep(step domain.Step) domain.Step {
	cloned := step
	cloned.Vars = cloneMap(step.Vars)
	cloned.Request.Defaults.Headers = cloneMap(step.Request.Defaults.Headers)
	cloned.Response.Capture = cloneMap(step.Response.Capture)
	if step.Response.ExpectedTypes != nil {
		cloned.Response.ExpectedTypes = make(map[string][]string, len(step.Response.ExpectedTypes))
		for selector, expected := range step.Response.ExpectedTypes {
			if expected == nil {
				cloned.Response.ExpectedTypes[selector] = nil
				continue
			}
			clonedExpected := make([]string, len(expected))
			copy(clonedExpected, expected)
			cloned.Response.ExpectedTypes[selector] = clonedExpected
		}
	}
	return cloned
}

func cloneMap[K comparable, V any](source map[K]V) map[K]V {
	if source == nil {
		return nil
	}
	cloned := make(map[K]V, len(source))
	for key, value := range source {
		cloned[key] = value
	}
	return cloned
}

func fatalResult(exitCode int, err error) (int, error) {
	if exitCode == 0 {
		exitCode = errs.Code(err, errs.ExitInternal)
		if exitCode == 0 {
			exitCode = errs.ExitInternal
		}
	}
	return exitCode, err
}
