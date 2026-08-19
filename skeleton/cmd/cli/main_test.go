package main

import (
	"apih/skeleton/internal/reporter"
	"apih/skeleton/pkg/errs"
	"bytes"
	"context"
	"errors"
	"testing"
)

type failingWriter struct {
	err error
}

func (w failingWriter) Write([]byte) (int, error) {
	return 0, w.err
}

func TestRunReturnsConfigurationExitCodeForInvalidPath(t *testing.T) {
	var output bytes.Buffer

	exitCode, err := run(context.Background(), []string{"apih", "path-that-does-not-exist"}, reporter.NewReporter(&output))
	if exitCode != errs.ExitConfiguration {
		t.Fatalf("run() exit code = %d, want %d", exitCode, errs.ExitConfiguration)
	}
	if !errors.Is(err, ErrInvalidPath) {
		t.Fatalf("run() error = %v, want ErrInvalidPath", err)
	}
	if output.Len() != 0 {
		t.Fatalf("run() output = %q, want empty output", output.String())
	}
}

func TestRunReturnsSuccessAfterDefinitionPipeline(t *testing.T) {
	var output bytes.Buffer

	exitCode, err := run(context.Background(), []string{"apih"}, reporter.NewReporter(&output))
	if err != nil {
		t.Fatalf("run() error = %v", err)
	}
	if exitCode != errs.ExitSuccess {
		t.Fatalf("run() exit code = %d, want %d", exitCode, errs.ExitSuccess)
	}
	if output.Len() == 0 {
		t.Fatal("run() did not report the working directory")
	}
}

func TestRunReturnsInternalExitCodeForOutputFailure(t *testing.T) {
	wantErr := errors.New("output failed")

	exitCode, err := run(context.Background(), []string{"apih"}, reporter.NewReporter(failingWriter{err: wantErr}))
	if exitCode != errs.ExitInternal {
		t.Fatalf("run() exit code = %d, want %d", exitCode, errs.ExitInternal)
	}
	if !errors.Is(err, wantErr) {
		t.Fatalf("run() error = %v, want %v", err, wantErr)
	}
}
