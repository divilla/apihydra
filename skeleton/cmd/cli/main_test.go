package main

import (
	"apih/skeleton/internal/domain"
	"apih/skeleton/internal/reporting"
	"apih/skeleton/pkg/errs"
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/spf13/pflag"
)

type failingWriter struct {
	err error
}

func (w failingWriter) Write([]byte) (int, error) {
	return 0, w.err
}

func TestRunReturnsConfigurationExitCodeForInvalidPath(t *testing.T) {
	var output bytes.Buffer

	exitCode, err := run(context.Background(), domain.Config{Directory: "path-that-does-not-exist", Parallelism: 1}, reporting.NewReporter(&output, false))
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
	t.Setenv("XDG_CACHE_HOME", t.TempDir())

	exitCode, err := run(context.Background(), domain.Config{Parallelism: 1}, reporting.NewReporter(&output, false))
	if err != nil {
		t.Fatalf("run() error = %v", err)
	}
	if exitCode != 0 {
		t.Fatalf("run() exit code = %d, want 0", exitCode)
	}
	if output.Len() == 0 {
		t.Fatal("run() did not report the working directory")
	}
}

func TestRunReturnsInternalExitCodeForOutputFailure(t *testing.T) {
	wantErr := errors.New("output failed")
	t.Setenv("XDG_CACHE_HOME", t.TempDir())

	exitCode, err := run(context.Background(), domain.Config{Parallelism: 1}, reporting.NewReporter(failingWriter{err: wantErr}, false))
	if exitCode != errs.ExitInternal {
		t.Fatalf("run() exit code = %d, want %d", exitCode, errs.ExitInternal)
	}
	if !errors.Is(err, wantErr) {
		t.Fatalf("run() error = %v, want %v", err, wantErr)
	}
}

func TestParseConfigUsesNativePflagFormsAndRejectsInvalidApplicationValues(t *testing.T) {
	tests := map[string]struct {
		args        []string
		parallelism int
		directory   string
		wantErr     bool
	}{
		"defaults":               {args: []string{"apih"}, parallelism: 1},
		"attached shorthand":     {args: []string{"apih", "-p0", "suite"}, parallelism: 0, directory: "suite"},
		"equals long":            {args: []string{"apih", "--parallelism=2", "suite"}, parallelism: 2, directory: "suite"},
		"interspersed":           {args: []string{"apih", "suite", "-p", "2"}, parallelism: 2, directory: "suite"},
		"repeated last wins":     {args: []string{"apih", "-p", "0", "--parallelism", "2"}, parallelism: 2},
		"too many directories":   {args: []string{"apih", "one", "two"}, wantErr: true},
		"invalid parallelism":    {args: []string{"apih", "-p", "3"}, wantErr: true},
		"nonnumeric parallelism": {args: []string{"apih", "-p", "many"}, wantErr: true},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			var usage bytes.Buffer
			config, err := parseConfig(test.args, &usage)
			if test.wantErr {
				if err == nil || !errors.Is(err, ErrInvalidArguments) {
					t.Fatalf("parseConfig() error = %v, want ErrInvalidArguments", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseConfig() error = %v", err)
			}
			if config.Parallelism != test.parallelism || config.Directory != test.directory || config.TempRunDir != "" {
				t.Fatalf("parseConfig() = %#v, want parallelism %d, directory %q", config, test.parallelism, test.directory)
			}
		})
	}
}

func TestParseConfigHelpUsesPflag(t *testing.T) {
	var usage bytes.Buffer
	_, err := parseConfig([]string{"apih", "--help"}, &usage)
	if !errors.Is(err, pflag.ErrHelp) {
		t.Fatalf("parseConfig(--help) error = %v, want pflag.ErrHelp", err)
	}
	if !strings.Contains(usage.String(), "--parallelism") {
		t.Fatalf("parseConfig(--help) output = %q, want parallelism usage", usage.String())
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
	os.Args = []string{"apih", "path-that-does-not-exist"}
	main()
}
