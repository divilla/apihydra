package reporter

import (
	"apih/skeleton/internal/domain"
	"apih/skeleton/pkg/errs"
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
)

var ErrReporter = errors.New("reporter error")
var ErrTypeValidation = errors.New("type validation failed for")
var ErrExpectedValidation = errors.New("response does not match expected")

// Reporter owns all human-readable execution output. The writer is normally
// os.Stdout and may be replaced by a buffer or another writer in tests.
// Reporter never writes fatal diagnostics to standard error; reporting
// failures are returned to the caller.
type Reporter struct {
	output io.Writer
	mu     sync.Mutex
}

func NewReporter(output io.Writer) *Reporter {
	return &Reporter{output: output}
}

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
	return nil
}

// ValidationTypes writes one nonfatal response-type validation failure to the
// injected standard-output writer. It returns only reporting failures; the
// validation failure itself does not terminate execution.
func (r *Reporter) ValidationTypes(ctx context.Context, step *domain.Step, failure error) error {
	return nil
}

// ValidationExpected writes one nonfatal expected-response validation failure
// to the injected standard-output writer. Any command colors carried by
// failure are preserved when the output block is rendered. It returns only
// reporting failures; the validation failure itself does not terminate
// execution.
func (r *Reporter) ValidationExpected(ctx context.Context, step *domain.Step, failure error) error {
	return nil
}

// Debug reports the final runtime state of a selected debug step to the
// injected standard-output writer. It returns a reporting error without
// terminating execution.
func (r *Reporter) Debug(ctx context.Context, step *domain.Step) error {
	return nil
}
