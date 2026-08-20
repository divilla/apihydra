package main

import (
	"apih/skeleton/internal/reporter"
	"apih/skeleton/pkg/errs"
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
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

func TestMainLogsFatalErrorAndPreservesProductExitCode(t *testing.T) {
	cmd := exec.Command(os.Args[0], "-test.run=TestMainHelperProcess")
	cmd.Env = append(os.Environ(), "APIH_TEST_MAIN_HELPER=1")
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("main process error = %v, want *exec.ExitError", err)
	}
	if got, want := exitErr.ExitCode(), errs.ExitConfiguration; got != want {
		t.Fatalf("main process exit code = %d, want %d", got, want)
	}
	if stdout.Len() != 0 {
		t.Fatalf("main process stdout = %q, want empty output", stdout.String())
	}
	if !strings.Contains(stderr.String(), ErrInvalidPath.Error()) {
		t.Fatalf("main process stderr = %q, want ErrInvalidPath", stderr.String())
	}
}

func TestMainHelperProcess(t *testing.T) {
	if os.Getenv("APIH_TEST_MAIN_HELPER") != "1" {
		return
	}
	main()
}
