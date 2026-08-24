package reporting

import (
	"apih/internal/domain"
	"apih/pkg/errs"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"
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
type Reporter struct {
	output io.Writer
	mu     sync.Mutex
}

// NewReporter returns a Reporter that serializes writes to output.
func NewReporter(output io.Writer) *Reporter {
	return &Reporter{output: output}
}

// WorkingDirectory writes the selected working directory to the injected
// writer.
func (r *Reporter) WorkingDirectory(workDir string) error {
	if r == nil || r.output == nil {
		return errs.Build(errs.ExitInternal, ErrReporter, nil, "output is nil")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, err := fmt.Fprintf(r.output, "Working Directory: %s\n\n", workDir); err != nil {
		return errs.Build(errs.ExitInternal, ErrReporter, err)
	}
	return nil
}

// Success reports a directory whose execution completed without validation
// failures to the injected standard-output writer. It returns a reporting
// error without terminating execution.
func (r *Reporter) Success(ctx context.Context, directory *domain.Directory) error {
	path := "<unknown directory>"
	if directory != nil {
		path = directory.Path
	}
	return r.write(ctx, fmt.Sprintf("Success: %s\n\n", path))
}

// ValidationTypes writes the failed output from nonfatal response-type
// validation to the injected standard-output writer. It returns only reporting
// failures; the validation failure itself does not terminate execution.
func (r *Reporter) ValidationTypes(ctx context.Context, step *domain.Step, failed string) error {
	return r.write(ctx, fmt.Sprintf("%s %s:\n%s\n\n", ErrTypeValidation, stepReference(step), failed))
}

// ValidationStatus writes one nonfatal response-status validation failure to
// the injected standard-output writer. It returns only reporting failures; the
// validation failure itself does not terminate execution.
func (r *Reporter) ValidationStatus(ctx context.Context, step *domain.Step, failure error) error {
	return r.write(ctx, fmt.Sprintf("%s for %s: %v\n\n", ErrStatusValidation, stepReference(step), failure))
}

// ValidationBody writes one nonfatal response-body validation diff to the
// injected standard-output writer. Any command colors carried by diff are
// preserved when the output block is rendered. It returns only reporting
// failures; the validation failure itself does not terminate execution.
func (r *Reporter) ValidationBody(ctx context.Context, step *domain.Step, diff string) error {
	return r.write(ctx, fmt.Sprintf("%s for %s:\n%s\n\n", ErrBodyValidation, stepReference(step), diff))
}

// Debug reports the final runtime state of a selected debug step to the
// injected standard-output writer. It returns a reporting error without
// terminating execution.
func (r *Reporter) Debug(ctx context.Context, step *domain.Step) error {
	payload, _ := json.MarshalIndent(step, "", "  ")
	return r.write(ctx, fmt.Sprintf("Debug %s:\n%s\n\n", stepReference(step), payload))
}

func (r *Reporter) write(ctx context.Context, block string) error {
	if r == nil || r.output == nil {
		return errs.Build(errs.ExitInternal, ErrReporter, nil, "output is nil")
	}
	if err := ctx.Err(); err != nil {
		return errs.Build(errs.ExitInternal, ErrReporter, err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return errs.Build(errs.ExitInternal, ErrReporter, err)
	}

	written, err := io.WriteString(r.output, block)
	if err == nil && written != len(block) {
		err = io.ErrShortWrite
	}
	if err != nil {
		return errs.Build(errs.ExitInternal, ErrReporter, err)
	}
	return nil
}

func stepReference(step *domain.Step) string {
	if step == nil {
		return "<unknown step>"
	}
	if step.Definition == nil || step.Definition.File == nil {
		return fmt.Sprintf("step %d", step.Index)
	}
	return fmt.Sprintf("%s step %d", step.Definition.File.Path, step.Index)
}
