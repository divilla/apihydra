package execution

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/divilla/apihydra/skeleton/internal/domain"
	"github.com/divilla/apihydra/skeleton/internal/reporting"
	"github.com/divilla/apihydra/skeleton/pkg/errs"
)

// ErrInvalidDirectoryTree classifies a malformed Suite directory tree.
var ErrInvalidDirectoryTree = errors.New("invalid directory tree")

// ErrExecutionCanceled classifies cancellation of staged execution.
var ErrExecutionCanceled = errors.New("execution canceled")

// ErrInvalidParallelism classifies a Config.Parallelism value outside 0..2.
var ErrInvalidParallelism = errors.New("invalid parallelism")

// ErrStepExecution classifies a terminal step failure before errs adds its
// definition-file and spec.steps[index] provenance.
var ErrStepExecution = errors.New("step execution error")

var errDebugStop = errors.New("debug stop")

// Executor prepares, schedules, executes, validates, and reports runtime
// steps.
type Executor struct {
	binder *Binder
	val    *Validator
	report *reporting.Reporter
	config domain.Config
}

// NewExecutor retains the collaborators and run configuration used during
// execution.
func NewExecutor(
	binder *Binder,
	validator *Validator,
	report *reporting.Reporter,
	config domain.Config,
) *Executor {
	return &Executor{
		binder: binder,
		val:    validator,
		report: report,
		config: config,
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
	// TODO: implement
}

// Execute processes stages sequentially with a barrier after each stage.
// Config.Parallelism 0 processes directories and files serially, 1 processes
// same-stage directories concurrently and each directory's files serially,
// and 2 processes same-stage directories and each directory's files
// concurrently. Directory and file task sets are unbounded. Steps within one
// file are always serial. All modes retain plan, file-slice, and step-slice
// order as the canonical reporting order and use the same shared Binder store.
//
// Execute creates every cookie jar below Config.TempRunDir before its owning
// work executes, even when that work currently has cookies disabled. A request
// whose effective DisableCookies is true passes an empty cookieJar to Runner
// and leaves its owning jar unchanged; every other request passes its owning
// jar. All jars inherit Config.TempRunDir's lifecycle and are never reused by
// another run. Missing run storage and jar create, initialize, or copy failures
// are internal failures and never silently disable cookies.
//
// Config.Parallelism also selects cookie-jar ownership. Mode 0 creates one jar
// for the run and creates no stage-transition copies. Mode 1 creates one jar
// per directory; after a stage joins and before the next starts, every direct
// child receives a distinct byte-for-byte copy of its parent's final jar.
// Mode 2 creates one jar per steps file. Root file jars start empty. After each
// step finishes, including a cookie-disabled step, the owning file's jar is
// recorded as that directory's latest completed jar. After the stage joins,
// every file in each direct child receives a distinct copy of the parent jar
// whose step completion was observed last. Runtime completion order and Go
// scheduling intentionally select that source; filesystem modification times
// are not used and jars are never merged. A directory with no executed steps
// preserves its incoming state. A root with no steps-file jars creates one
// additional empty inheritance jar. At every stage transition copies flow only
// from each parent to its direct children, and no writable jar is shared by
// concurrent work.
//
// Before and after each stage, Execute calls Reporter.BeginStage and
// Reporter.EndStage so concurrent output is redrawn or committed in canonical
// directory/file/step order.
//
// For every non-debug runtime step, it calls
// Binder.LoadVariables, Binder.InterpolateRequestBody,
// Binder.InterpolateResponseExpectedBody, runner.Curl with the selected cookie
// jar path or an empty path when cookies are disabled,
// Validator.ValidateTypes, Validator.ValidateStatus, Validator.ValidateBody,
// and Binder.CaptureResponseVariables in that order. A debug step replaces the
// runner.Curl phase with runner.CurlBuild using that same cookie selection,
// runner.CurlRaw, assignment of the raw statement to Step.RawCurl, and
// runner.CurlExecute using the unchanged executable, arguments, and request
// body. Before a debug step finishes or a
// terminal error is returned, Reporter.Debug receives the latest mutated Step.
// A successful debug report stops later execution as a clean breakpoint; a
// terminal execution error retains its original exit code and error after the
// debug report. A non-empty
// failed string from ValidateTypes, an ErrValidation from ValidateStatus, and a
// non-empty diff from ValidateBody are reported through e.report. The response
// status and body returned by runner.Curl or runner.CurlExecute are assigned to
// step.Response.ActualStatus and step.Response.ActualBody for validation and
// capture. After all work finishes, Execute returns exit code 101 and a nil
// error when one or more validations failed. Validation status does not cancel
// remaining work. A terminal step error is wrapped with ErrStepExecution plus
// definition-file and spec.steps[index] provenance. Active work is canceled
// and joined, Reporter.EndStage makes the ordered stage render final, and the
// returned fatal diagnostic is the last application output. No later step,
// file, directory, stage, or reporting event is allowed.
func (e *Executor) Execute(
	ctx context.Context,
	stages [][]*domain.Directory,
) (int, error) {
	exitCode, err := executeStages(ctx, stages, e.config.Parallelism, e.report, e.processDir)
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

type resultPublisher func(int, error)

type directoryProcessor func(context.Context, resultPublisher, *domain.Directory) (int, error)

type fileProcessor func(context.Context, *domain.Directory, int) (int, error)

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
	parallelism int,
	report *reporting.Reporter,
	process directoryProcessor,
) (int, error) {
	if parallelism < 0 || parallelism > 2 {
		return errs.ExitConfiguration, errs.Build(errs.ExitConfiguration, ErrInvalidParallelism, nil, parallelism)
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	var firstStatusCode int

	for _, stage := range dirs {
		if err := ctx.Err(); err != nil {
			return errs.ExitInternal, errs.Build(errs.ExitInternal, ErrExecutionCanceled, err)
		}
		if report != nil {
			if err := report.BeginStage(ctx, stage); err != nil {
				return errs.Code(err, errs.ExitInternal), err
			}
		}

		exitCode, err := executeStage(ctx, cancel, stage, parallelism > 0, process)
		var endErr error
		if report != nil {
			endErr = report.EndStage(context.WithoutCancel(ctx))
		}
		if errors.Is(err, errDebugStop) {
			if endErr != nil {
				return errs.Code(endErr, errs.ExitInternal), endErr
			}
			return 0, nil
		}
		if err != nil {
			return exitCode, err
		}
		if endErr != nil {
			return errs.Code(endErr, errs.ExitInternal), endErr
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
	parallel bool,
	process directoryProcessor,
) (int, error) {
	var result processResult
	publish := func(code int, err error) {
		result.setResult(code, err)
		if err != nil {
			cancel()
		}
	}
	if !parallel {
		for _, dir := range dirs {
			exitCode, err := process(ctx, publish, dir)
			publish(exitCode, err)
			if err != nil {
				break
			}
		}
		return result.code, result.err
	}

	var wg sync.WaitGroup
	for _, dir := range dirs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			exitCode, err := process(ctx, publish, dir)
			publish(exitCode, err)
		}()
	}

	wg.Wait()
	return result.code, result.err
}

func executeFiles(
	ctx context.Context,
	cancel context.CancelFunc,
	dir *domain.Directory,
	parallel bool,
	process fileProcessor,
) (int, error) {
	var result processResult
	if !parallel {
		for fileIndex := range dir.RuntimeSteps {
			exitCode, err := process(ctx, dir, fileIndex)
			result.setResult(exitCode, err)
			if err != nil {
				cancel()
				break
			}
		}
		return result.code, result.err
	}

	var wg sync.WaitGroup
	for fileIndex := range dir.RuntimeSteps {
		wg.Add(1)
		go func() {
			defer wg.Done()
			exitCode, err := process(ctx, dir, fileIndex)
			result.setResult(exitCode, err)
			if err != nil {
				cancel()
			}
		}()
	}
	wg.Wait()
	return result.code, result.err
}

func executeDirectoryFiles(
	ctx context.Context,
	publish resultPublisher,
	dir *domain.Directory,
	parallel bool,
	process fileProcessor,
) (int, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	return executeFiles(ctx, cancel, dir, parallel, func(ctx context.Context, dir *domain.Directory, fileIndex int) (int, error) {
		exitCode, err := process(ctx, dir, fileIndex)
		if err != nil && publish != nil {
			publish(exitCode, err)
		}
		return exitCode, err
	})
}

func (e *Executor) processDir(ctx context.Context, publish resultPublisher, dir *domain.Directory) (int, error) {
	if err := ctx.Err(); err != nil {
		return errs.ExitInternal, errs.Build(errs.ExitInternal, ErrExecutionCanceled, err)
	}
	return executeDirectoryFiles(ctx, publish, dir, e.config.Parallelism == 2, e.processFile)
}

// processFile executes one RuntimeSteps group serially. It retains the existing
// per-step phase, validation, capture, Debug, and cookie-selection contracts;
// after each finished step it records this file's jar as the directory's latest
// completed jar; reports successful completion for the corresponding
// StepsDefinitions entry; and returns validation status when any step in the
// file mismatched. Before returning a terminal error it wraps that error with
// ErrStepExecution and spec.steps[index] provenance through
// errs.StepExecutionError. A successful Debug report returns errDebugStop.
func (e *Executor) processFile(ctx context.Context, dir *domain.Directory, fileIndex int) (int, error) {
	// TODO: implement
	_ = e
	_ = dir
	_ = fileIndex
	return 0, nil
}
