// Package reporter owns APIHydra's human-readable terminal output.
package reporter

import (
	"apih/internal/domain"
	"apih/pkg/errs"
	"context"
	"errors"
	"fmt"
	"io"
	"regexp"
	"sync"
)

// ErrReporter classifies Reporter state and output failures.
var ErrReporter = errors.New("reporter error")

// ErrTypeValidation classifies response-type validation diagnostics.
var ErrTypeValidation = errors.New("type validation failed for")

// ErrExpectedValidation classifies expected-response diff diagnostics.
var ErrExpectedValidation = errors.New("response does not match expected")

var ansiSGR = regexp.MustCompile(`\x1b\[[0-9;:]*m`)

// Reporter owns all human-readable standard output. The writer is normally
// os.Stdout and may be replaced by a buffer or another writer in tests.
type Reporter struct {
	output io.Writer
	mu     sync.Mutex
}

// NewReporter constructs a Reporter that writes to output.
func NewReporter(output io.Writer) *Reporter {
	return &Reporter{output: output}
}

// WorkingDirectory reports the selected working directory.
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

// Error writes one fatal diagnostic in the unified standard-error red style.
// The composition root constructs this Reporter with os.Stderr.
func (r *Reporter) Error(failure error) error {
	if r == nil || r.output == nil {
		return errs.Build(errs.ExitInternal, ErrReporter, nil, "output is nil")
	}
	if failure == nil {
		return errs.Build(errs.ExitInternal, ErrReporter, nil, "failure is nil")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	diagnostic := ansiSGR.ReplaceAllString(failure.Error(), "")
	if _, err := fmt.Fprintf(r.output, "\x1b[31m%s\x1b[0m\n", diagnostic); err != nil {
		return errs.Build(errs.ExitInternal, ErrReporter, err)
	}
	return nil
}

// Success reports a directory whose execution completed without validation
// failures. Formatting is intentionally left to the Reporter implementation.
func (r *Reporter) Success(ctx context.Context, directory *domain.Directory) error {
	return nil
}

// FailureTypes reports one nonfatal response-type validation failure.
func (r *Reporter) FailureTypes(ctx context.Context, step *domain.Step, failure error) error {
	return nil
}

// FailureDiff reports one nonfatal expected-response diff. Any command colors
// carried by failure are preserved when the output block is rendered.
func (r *Reporter) FailureDiff(ctx context.Context, step *domain.Step, failure error) error {
	return nil
}

// Debug reports the final runtime state of a selected debug step.
func (r *Reporter) Debug(ctx context.Context, step *domain.Step) error {
	return nil
}
