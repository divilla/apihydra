package reporter

import (
	"apih/internal/domain"
	"apih/pkg/errs"
	"bytes"
	"context"
	"errors"
	"io"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
)

type failingWriter struct {
	err error
}

func (w failingWriter) Write([]byte) (int, error) {
	return 0, w.err
}

type overlapWriter struct {
	active  atomic.Int32
	overlap atomic.Bool
}

func (w *overlapWriter) Write(p []byte) (int, error) {
	if w.active.Add(1) != 1 {
		w.overlap.Store(true)
	}
	for range 100 {
		runtime.Gosched()
	}
	w.active.Add(-1)
	return len(p), nil
}

func TestPublicContractAndWriterInjection(t *testing.T) {
	var newReporter func(io.Writer) *Reporter = NewReporter
	var workingDirectory func(*Reporter, string) error = (*Reporter).WorkingDirectory
	var reportError func(*Reporter, error) error = (*Reporter).Error
	var success func(*Reporter, context.Context, *domain.Directory) error = (*Reporter).Success
	var failureTypes func(*Reporter, context.Context, *domain.Step, error) error = (*Reporter).FailureTypes
	var failureDiff func(*Reporter, context.Context, *domain.Step, error) error = (*Reporter).FailureDiff
	var debug func(*Reporter, context.Context, *domain.Step) error = (*Reporter).Debug
	_, _, _, _, _, _, _ = newReporter, workingDirectory, reportError, success, failureTypes, failureDiff, debug

	staticErrors := map[string]struct {
		got  error
		want string
	}{
		"reporter":            {ErrReporter, "reporter error"},
		"type validation":     {ErrTypeValidation, "type validation failed for"},
		"expected validation": {ErrExpectedValidation, "response does not match expected"},
	}
	for name, test := range staticErrors {
		t.Run(name, func(t *testing.T) {
			if got := test.got.Error(); got != test.want {
				t.Fatalf("static error = %q, want %q", got, test.want)
			}
		})
	}

	var output bytes.Buffer
	report := NewReporter(&output)
	if err := report.WorkingDirectory("/injected"); err != nil {
		t.Fatalf("WorkingDirectory() error = %v", err)
	}
	if got, want := output.String(), "Working Directory: /injected\n\n"; got != want {
		t.Fatalf("injected writer output = %q, want %q", got, want)
	}
}

func TestWorkingDirectoryOutputIsByteExact(t *testing.T) {
	var output bytes.Buffer
	report := NewReporter(&output)

	if err := report.WorkingDirectory("/work dir"); err != nil {
		t.Fatalf("WorkingDirectory() error = %v", err)
	}
	if got, want := output.String(), "Working Directory: /work dir\n\n"; got != want {
		t.Fatalf("WorkingDirectory() output = %q, want %q", got, want)
	}
}

func TestWorkingDirectoryFailuresAreReporterErrors(t *testing.T) {
	wantErr := errors.New("write failed")
	tests := map[string]struct {
		report *Reporter
		cause  error
	}{
		"nil reporter": {nil, nil},
		"nil writer":   {NewReporter(nil), nil},
		"writer error": {NewReporter(failingWriter{err: wantErr}), wantErr},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			err := test.report.WorkingDirectory("/work")
			assertReporterError(t, err, test.cause)
		})
	}
}

func TestErrorUsesOneRedWrapperAndRemovesEmbeddedSGR(t *testing.T) {
	var output bytes.Buffer
	report := NewReporter(&output)
	failure := errors.New("before \x1b[1;32mgreen\x1b[0m and \x1b[38:5:9mred\x1b[m after")

	if err := report.Error(failure); err != nil {
		t.Fatalf("Error() error = %v", err)
	}
	if got, want := output.String(), "\x1b[31mbefore green and red after\x1b[0m\n"; got != want {
		t.Fatalf("Error() output = %q, want %q", got, want)
	}
}

func TestErrorRejectsInvalidStateAndPreservesWriteCause(t *testing.T) {
	wantErr := errors.New("write failed")
	tests := map[string]struct {
		report  *Reporter
		failure error
		cause   error
	}{
		"nil reporter": {nil, errors.New("failure"), nil},
		"nil writer":   {NewReporter(nil), errors.New("failure"), nil},
		"nil failure":  {NewReporter(io.Discard), nil, nil},
		"writer error": {NewReporter(failingWriter{err: wantErr}), errors.New("failure"), wantErr},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			err := test.report.Error(test.failure)
			assertReporterError(t, err, test.cause)
		})
	}
}

func TestImplementedWritesSerializeWriterAccess(t *testing.T) {
	output := &overlapWriter{}
	report := NewReporter(output)
	start := make(chan struct{})
	var calls sync.WaitGroup

	for index := range 64 {
		calls.Add(1)
		go func() {
			defer calls.Done()
			<-start
			var err error
			if index%2 == 0 {
				err = report.WorkingDirectory("/work")
			} else {
				err = report.Error(errors.New("failure"))
			}
			if err != nil {
				t.Errorf("reporting error = %v", err)
			}
		}()
	}

	close(start)
	calls.Wait()
	if output.overlap.Load() {
		t.Fatal("Reporter allowed concurrent writer access")
	}
}

func TestStubbedOperationsRemainNoOpsWithinReportingBoundaries(t *testing.T) {
	var output bytes.Buffer
	report := NewReporter(&output)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	step := &domain.Step{}
	directory := &domain.Directory{}
	coloredDiff := errors.New("\x1b[31m-expected\x1b[0m\n\x1b[32m+actual\x1b[0m")

	tests := map[string]func() error{
		"success":       func() error { return report.Success(ctx, directory) },
		"failure types": func() error { return report.FailureTypes(ctx, step, errors.New("mismatch")) },
		"failure diff":  func() error { return report.FailureDiff(ctx, step, coloredDiff) },
		"debug":         func() error { return report.Debug(ctx, step) },
	}
	for name, call := range tests {
		t.Run(name, func(t *testing.T) {
			if err := call(); err != nil {
				t.Fatalf("stubbed operation error = %v, want nil", err)
			}
		})
	}

	if output.Len() != 0 {
		t.Fatalf("stubbed operations output = %q, want empty", output.String())
	}
	if got, want := coloredDiff.Error(), "\x1b[31m-expected\x1b[0m\n\x1b[32m+actual\x1b[0m"; got != want {
		t.Fatalf("FailureDiff input = %q, want preserved %q", got, want)
	}
}

func assertReporterError(t *testing.T, err, cause error) {
	t.Helper()
	if !errors.Is(err, ErrReporter) {
		t.Fatalf("error = %v, want ErrReporter", err)
	}
	if got := errs.Code(err, errs.ExitSuccess); got != errs.ExitInternal {
		t.Fatalf("error code = %d, want %d", got, errs.ExitInternal)
	}
	if cause != nil && !errors.Is(err, cause) {
		t.Fatalf("error = %v, want cause %v", err, cause)
	}
}
