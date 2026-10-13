package execution

import (
	"context"
	"errors"
	"fmt"

	"github.com/divilla/apihydra/internal/domain"
	"github.com/divilla/apihydra/internal/reporting"
	"github.com/divilla/apihydra/pkg/errs"
	"github.com/divilla/apihydra/pkg/runner"
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
	binder  *Binder
	val     *Validator
	report  *reporting.Reporter
	config  domain.Config
	cookies *cookieJars
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
		binder:  binder,
		val:     validator,
		report:  report,
		config:  config,
		cookies: newCookieJars(config),
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
// Binder.InterpolateResponseExpectedBody, runner.Curl,
// Validator.ValidateTypes, Validator.ValidateStatus, Validator.ValidateBody,
// and Binder.CaptureResponseVariables in that order. A debug step replaces the
// runner.Curl phase with runner.CurlBuild, runner.CurlRaw, assignment of the
// raw statement to Step.RawCurl, and runner.CurlExecute using the unchanged
// executable, arguments, and request body. Before a debug step finishes or a
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
	return executeStagesPrepared(ctx, stages, e.config.Parallelism, e.report, e.cookies.prepareStage, e.processDir)
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
// StepsDefinitions entry; and returns
// validation status when any step in the file mismatched. Before returning a
// terminal error it wraps that error with ErrStepExecution and
// spec.steps[index] provenance through errs.StepExecutionError. A successful
// Debug report returns errDebugStop.
func (e *Executor) processFile(ctx context.Context, dir *domain.Directory, fileIndex int) (int, error) {
	validationFailed := false
	for stepIndex := range dir.RuntimeSteps[fileIndex] {
		step := &dir.RuntimeSteps[fileIndex][stepIndex]

		if err := ctx.Err(); err != nil {
			return fatalResult(0, wrapStepFailure(step, err))
		}
		owningJar := e.cookies.jarFor(dir, fileIndex)
		cookieJar := owningJar
		if step.Request.Defaults.DisableCookies != nil && *step.Request.Defaults.DisableCookies {
			cookieJar = ""
		}
		recordCompletion := func() {
			e.cookies.recordCompletion(dir, fileIndex)
		}
		if exitCode, err := e.binder.LoadVariables(ctx, step); err != nil {
			recordCompletion()
			return e.finishStep(ctx, step, exitCode, err)
		}
		if exitCode, err := e.binder.InterpolateRequestBody(ctx, step); err != nil {
			recordCompletion()
			return e.finishStep(ctx, step, exitCode, err)
		}
		if exitCode, err := e.binder.InterpolateResponseExpectedBody(ctx, step); err != nil {
			recordCompletion()
			return e.finishStep(ctx, step, exitCode, err)
		}

		body, status, err := executeRequest(ctx, step, cookieJar)
		if err != nil {
			recordCompletion()
			return e.finishStep(ctx, step, status, err)
		}
		step.Response.ActualStatus = status
		step.Response.ActualBody = domain.YAMLString(body)

		failedTypes, err := e.val.ValidateTypes(ctx, step)
		if err != nil {
			recordCompletion()
			return e.finishStep(ctx, step, 0, err)
		}
		var statusFailure error
		if err := e.val.ValidateStatus(ctx, step); err != nil {
			if !errors.Is(err, ErrValidation) {
				recordCompletion()
				return e.finishStep(ctx, step, 0, err)
			}
			statusFailure = err
		}
		diff, err := e.val.ValidateBody(ctx, step)
		if err != nil {
			recordCompletion()
			return e.finishStep(ctx, step, 0, err)
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
				recordCompletion()
				return e.finishStep(ctx, step, 0, err)
			}
		}
		if statusFailure != nil {
			if err := e.report.ValidationStatus(nextReportContext(), step, statusFailure); err != nil {
				recordCompletion()
				return e.finishStep(ctx, step, 0, err)
			}
		}
		if diff != "" {
			if err := e.report.ValidationBody(nextReportContext(), step, diff); err != nil {
				recordCompletion()
				return e.finishStep(ctx, step, 0, err)
			}
		}

		if exitCode, err := e.binder.CaptureResponseVariables(ctx, step); err != nil {
			recordCompletion()
			return e.finishStep(ctx, step, exitCode, err)
		}
		recordCompletion()
		if step.Debug {
			return e.finishStep(ctx, step, 0, nil)
		}
	}

	if err := e.report.Success(ctx, dir.StepsDefinitions[fileIndex]); err != nil {
		return fatalResult(0, err)
	}
	if validationFailed {
		return errs.ExitValidation, nil
	}
	return 0, nil
}

func executeRequest(ctx context.Context, step *domain.Step, cookieJar string) (string, int, error) {
	url := step.Request.Defaults.BaseURL + step.Request.Defaults.BasePath + step.Request.Path
	body := string(step.Request.Body)
	if !step.Debug {
		return runner.Curl(
			ctx,
			step.Request.Method,
			url,
			step.Request.Defaults.Headers,
			cookieJar,
			step.Request.Defaults.Timeout,
			step.Request.Defaults.Retries,
			step.Request.Query,
			body,
		)
	}

	executable, args, err := runner.CurlBuild(
		ctx,
		step.Request.Method,
		url,
		step.Request.Defaults.Headers,
		cookieJar,
		step.Request.Defaults.Timeout,
		step.Request.Defaults.Retries,
		step.Request.Query,
		body,
	)
	if err != nil {
		return "", 0, err
	}
	step.RawCurl = runner.CurlRaw(executable, args)
	return runner.CurlExecute(ctx, executable, args, body)
}

func (e *Executor) finishStep(ctx context.Context, step *domain.Step, exitCode int, terminalErr error) (int, error) {
	if !step.Debug {
		return fatalResult(exitCode, wrapStepFailure(step, terminalErr))
	}
	if terminalErr != nil && ctx.Err() != nil && errors.Is(terminalErr, ctx.Err()) {
		return fatalResult(exitCode, wrapStepFailure(step, terminalErr))
	}

	reportCtx := ctx
	if terminalErr != nil {
		reportCtx = context.WithoutCancel(ctx)
	}
	reportErr := e.report.Debug(reportCtx, step)
	if terminalErr != nil {
		return fatalResult(exitCode, wrapStepFailure(step, terminalErr))
	}
	if reportErr != nil {
		return fatalResult(0, wrapStepFailure(step, reportErr))
	}
	return 0, errDebugStop
}

func wrapStepFailure(step *domain.Step, terminalErr error) error {
	if terminalErr == nil {
		return nil
	}
	return errs.StepExecutionError(
		step,
		fmt.Sprintf("spec.steps[%d]", step.Index),
		ErrStepExecution,
		terminalErr,
	)
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
