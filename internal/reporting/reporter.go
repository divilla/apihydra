package reporting

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"

	"github.com/divilla/apihydra/internal/domain"
	"github.com/divilla/apihydra/pkg/errs"
	"github.com/divilla/apihydra/pkg/runner"
)

// ErrReporter classifies a failure to write execution output.
var ErrReporter = errors.New("reporting error")

// ErrTypeValidation labels a reported response-type mismatch.
var ErrTypeValidation = errors.New("type validation failed for")

// ErrStatusValidation labels a reported response-status mismatch.
var ErrStatusValidation = errors.New("response status does not match expected")

// ErrBodyValidation labels a reported response-body mismatch.
var ErrBodyValidation = errors.New("response body does not match expected")

// Reporter owns all human-readable execution output. The writer is normally
// os.Stdout and may be replaced by a buffer or another writer in tests.
// Reporter never writes fatal diagnostics to standard error; reporting
// failures are returned to the caller.
// Validation output groups failures under one file header and one heading per
// failing step: resolved request path, effective method, and light blue line:<N>
// (terminal palette color 117, #87d7ff).
// N is the one-based source line of the first reported failing expectation key
// (expected_types, expected_status, or expected_body), read from File.Bytes.
// If the key is absent, use the step's source line; if no source position can
// be recovered, print line:unknown. Further failures share that step heading.
type Reporter struct {
	output         io.Writer
	terminal       bool
	terminalWidth  int
	terminalHeight int
	mu             sync.Mutex
	stage          *stageOutput
	stopped        bool
}

type stageOutput struct {
	order           []*domain.StepsDefinition
	files           map[*domain.StepsDefinition]*fileOutput
	debug           string
	renderedContent string
	renderedRows    int
	renderedWidth   int
	renderedHeight  int
	rendered        bool
	implicit        bool
}

type fileOutput struct {
	block       strings.Builder
	failed      bool
	failedSteps map[int]struct{}
}

// NewReporter returns a Reporter that serializes writes to output. terminal
// selects live ANSI redraws; non-terminal output is buffered by stage and
// written once at the stage barrier.
func NewReporter(output io.Writer, terminal bool) *Reporter {
	reporter := &Reporter{
		output:         output,
		terminal:       terminal,
		terminalWidth:  80,
		terminalHeight: 24,
	}
	if terminal {
		reporter.refreshTerminalDimensionsLocked()
	}
	return reporter
}

// WorkingDirectory writes the selected working directory to the injected
// writer.
func (r *Reporter) WorkingDirectory(workDir string) error {
	if r == nil || r.output == nil {
		return errs.Build(errs.ExitInternal, ErrReporter, nil, "output is nil")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.stopped {
		return nil
	}
	if _, err := fmt.Fprintf(r.output, "Working Directory: %s\n\n", workDir); err != nil {
		return errs.Build(errs.ExitInternal, ErrReporter, err)
	}
	return nil
}

// BeginStage starts one ordered reporting transaction. directories are in
// PlanStages order, and each directory's StepsDefinitions order defines file
// order. Reporter retains one buffer per steps definition. On a terminal, each
// later reporting event clears and redraws only the active stage region in
// directory/file/step order; the working-directory heading and completed
// stages remain fixed. BeginStage itself produces no stage output.
func (r *Reporter) BeginStage(ctx context.Context, directories []*domain.Directory) error {
	if err := r.checkContext(ctx); err != nil {
		return err
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if r.stopped {
		return nil
	}
	if err := r.contextError(ctx); err != nil {
		return err
	}

	stage := &stageOutput{files: make(map[*domain.StepsDefinition]*fileOutput)}
	for _, directory := range directories {
		if directory == nil {
			continue
		}
		for _, definition := range directory.StepsDefinitions {
			stage.order = append(stage.order, definition)
			stage.files[definition] = newFileOutput()
		}
	}
	r.stage = stage
	return nil
}

// EndStage commits the active stage. On non-terminal output it writes the
// complete stage exactly once in directory/file/step order. On a terminal it
// leaves the final redraw in place without duplicating it. After EndStage,
// Reporter never rewrites output belonging to that completed stage.
func (r *Reporter) EndStage(ctx context.Context) error {
	if err := r.checkContext(ctx); err != nil {
		return err
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.contextError(ctx); err != nil {
		return err
	}
	if r.stage == nil {
		return nil
	}
	if !r.terminal && !r.stage.implicit {
		if err := r.writeLocked(r.stage.render()); err != nil {
			return err
		}
	}
	r.stage = nil
	return nil
}

// Success records one steps definition whose execution completed without
// validation failures. Its file buffer is redrawn immediately on terminals or
// retained until EndStage on non-terminals. It returns a reporting error
// without terminating execution.
func (r *Reporter) Success(ctx context.Context, definition *domain.StepsDefinition) error {
	return r.updateFile(ctx, definition, func(file *fileOutput) {
		if file.failed {
			return
		}
		fmt.Fprintf(&file.block, "[\x1b[38;5;10m✓\x1b[0m] %s\n", definitionReference(definition))
	})
}

// ValidationTypes writes the failed output from nonfatal response-type
// validation to the injected standard-output writer. It returns only reporting
// failures; the validation failure itself does not terminate execution.
func (r *Reporter) ValidationTypes(ctx context.Context, step *domain.Step, failed string) error {
	return r.writeValidation(ctx, step, "expected_types", formatExpectedTypes(step, failed))
}

// ValidationStatus writes one nonfatal response-status validation failure to
// the injected standard-output writer. It returns only reporting failures; the
// validation failure itself does not terminate execution.
// It prints expected_status first (palette 10), then actual_status (palette
// 210), each with four leading spaces and its original field name.
func (r *Reporter) ValidationStatus(ctx context.Context, step *domain.Step, failure error) error {
	actualStatus := 0
	expectedStatus := 0
	if step != nil {
		actualStatus = step.Response.ActualStatus
		expectedStatus = step.Response.ExpectedStatus
	}
	return r.writeValidation(ctx, step, "expected_status", fmt.Sprintf(
		"    expected_status: \x1b[38;5;10m%d\x1b[0m\n    actual_status: \x1b[38;5;210m%d\x1b[0m\n",
		expectedStatus,
		actualStatus,
	))
}

// ValidationBody writes one nonfatal response-body validation diff to the
// injected standard-output writer. Any command colors carried by diff are
// preserved when the output block is rendered. It returns only reporting
// failures; the validation failure itself does not terminate execution.
func (r *Reporter) ValidationBody(ctx context.Context, step *domain.Step, diff string) error {
	return r.writeValidation(ctx, step, "expected_body", formatExpectedBody(diff))
}

// Debug records the latest runtime state of a selected debug step with exactly
// these fields and blank lines:
//
//	stage: <Step.DirectoryStage()>
//	dir-path: <Step.DirectoryPath()>
//	file-path: <Step.FilePath()>
//
//	curl-command:
//	<Step.RawCurl>
//
//	<prettified-and-ANSI-colored-Step-JSON>
//
// RawCurl and Definition remain absent from the Step JSON according to their
// JSON tags. Debug preserves every other Step member and value, projecting only
// Request.Body, Response.ExpectedBody, and Response.ActualBody for display:
// valid JSON strings are embedded as JSON values, while empty or invalid JSON
// remains encoded as a string. It neither redacts nor omits data. Debug is kept
// outside the per-file buffers and rendered after every previously accumulated
// file block, so it is the final stdout block even when its file is not last in
// plan order. It atomically suppresses all later reporting calls after
// successfully recording and, on a terminal, redrawing the complete block. On
// non-terminals EndStage performs the single final write. It returns a
// reporting error without terminating execution.
func (r *Reporter) Debug(ctx context.Context, step *domain.Step) error {
	if err := r.checkContext(ctx); err != nil {
		return err
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if r.stopped {
		return nil
	}
	if err := r.contextError(ctx); err != nil {
		return err
	}

	payload, err := json.Marshal(debugStepValue(step))
	if err != nil {
		return errs.Build(errs.ExitInternal, ErrReporter, err)
	}
	pretty, _, err := runner.JQPretty(ctx, string(payload))
	if err != nil {
		return errs.Build(errs.ExitInternal, ErrReporter, err)
	}
	block := fmt.Sprintf(
		"stage: %d\ndir-path: %s\nfile-path: %s\n\ncurl-command:\n%s\n\n%s\n",
		step.DirectoryStage(),
		step.DirectoryPath(),
		step.FilePath(),
		step.RawCurl,
		colorizeJQJSON(pretty),
	)
	stage := r.ensureStageLocked()
	previous := stage.debug
	stage.debug = block
	if r.terminal {
		if err := r.redrawLocked(); err != nil {
			stage.debug = previous
			return err
		}
	} else if stage.implicit {
		if err := r.writeLocked(block); err != nil {
			stage.debug = previous
			return err
		}
	}
	r.stopped = true
	return nil
}

func newFileOutput() *fileOutput {
	return &fileOutput{failedSteps: make(map[int]struct{})}
}

func (s *stageOutput) render() string {
	if s == nil {
		return ""
	}
	var output strings.Builder
	for _, definition := range s.order {
		if file := s.files[definition]; file != nil {
			output.WriteString(file.block.String())
		}
	}
	output.WriteString(s.debug)
	return output.String()
}

func (s *stageOutput) implicitSnapshot() string {
	if s == nil || !s.implicit {
		return ""
	}
	return s.render()
}

func (r *Reporter) updateFile(
	ctx context.Context,
	definition *domain.StepsDefinition,
	update func(*fileOutput),
) error {
	if err := r.checkContext(ctx); err != nil {
		return err
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if r.stopped {
		return nil
	}
	if err := r.contextError(ctx); err != nil {
		return err
	}

	stage := r.ensureStageLocked()
	before := stage.implicitSnapshot()
	file := stage.files[definition]
	if file == nil {
		file = newFileOutput()
		stage.files[definition] = file
		stage.order = append(stage.order, definition)
	}
	update(file)
	if r.terminal {
		return r.redrawLocked()
	}
	if stage.implicit {
		after := stage.render()
		return r.writeLocked(strings.TrimPrefix(after, before))
	}
	return nil
}

func (r *Reporter) ensureStageLocked() *stageOutput {
	if r.stage == nil {
		r.stage = &stageOutput{
			files:    make(map[*domain.StepsDefinition]*fileOutput),
			implicit: true,
		}
	}
	return r.stage
}

func (r *Reporter) writeLocked(content string) error {
	written, err := io.WriteString(r.output, content)
	if err == nil && written != len(content) {
		err = io.ErrShortWrite
	}
	if err != nil {
		return errs.Build(errs.ExitInternal, ErrReporter, err)
	}
	return nil
}

func (r *Reporter) checkContext(ctx context.Context) error {
	if r == nil || r.output == nil {
		return errs.Build(errs.ExitInternal, ErrReporter, nil, "output is nil")
	}
	return r.contextError(ctx)
}

func (r *Reporter) contextError(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return errs.Build(errs.ExitInternal, ErrReporter, err)
	}
	return nil
}
