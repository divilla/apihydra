package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/divilla/apihydra/skeleton/internal/definition"
	"github.com/divilla/apihydra/skeleton/internal/domain"
	"github.com/divilla/apihydra/skeleton/internal/execution"
	"github.com/divilla/apihydra/skeleton/internal/reporting"
	"github.com/divilla/apihydra/skeleton/pkg/errs"
	"github.com/divilla/apihydra/skeleton/pkg/runner"

	"github.com/spf13/pflag"
)

type failingWriter struct {
	err error
}

func (w failingWriter) Write([]byte) (int, error) {
	return 0, w.err
}

func writeRootDefinition(t *testing.T, directory, name string) {
	t.Helper()
	contents := []byte("app: apihydra\nkind: root\nspec: {}\n")
	if err := os.WriteFile(filepath.Join(directory, name), contents, 0o600); err != nil {
		t.Fatal(err)
	}
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
	workDir := t.TempDir()
	writeRootDefinition(t, workDir, "suite.yml")
	t.Chdir(workDir)
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

func TestRunSelectsDirectoryContainingRootDefinition(t *testing.T) {
	parent := t.TempDir()
	suiteDir := filepath.Join(parent, "suite")
	if err := os.Mkdir(suiteDir, 0o700); err != nil {
		t.Fatal(err)
	}
	writeRootDefinition(t, suiteDir, "configuration.yaml")
	t.Chdir(parent)
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	var output bytes.Buffer

	exitCode, err := run(context.Background(), domain.Config{Directory: "suite", Parallelism: 1}, reporting.NewReporter(&output, false))
	if err != nil || exitCode != 0 {
		t.Fatalf("run() = (%d, %v), want (0, nil)", exitCode, err)
	}
	if got, want := output.String(), "Working Directory: "+suiteDir+"\n\n"; got != want {
		t.Fatalf("run() output = %q, want %q", got, want)
	}
}

func TestRunRejectsMissingRootBeforeApplicationOutput(t *testing.T) {
	t.Chdir(t.TempDir())
	var output bytes.Buffer

	exitCode, err := run(context.Background(), domain.Config{Parallelism: 1}, reporting.NewReporter(&output, false))
	if exitCode != errs.ExitConfiguration || !errors.Is(err, definition.ErrRootDefinitionMissing) {
		t.Fatalf("run() = (%d, %v), want (%d, ErrRootDefinitionMissing)", exitCode, err, errs.ExitConfiguration)
	}
	if output.Len() != 0 {
		t.Fatalf("run() output = %q, want empty output", output.String())
	}
}

func TestRunRejectsSelectedDirectoryMissingRootBeforeApplicationOutput(t *testing.T) {
	parent := t.TempDir()
	if err := os.Mkdir(filepath.Join(parent, "suite"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Chdir(parent)
	var output bytes.Buffer

	exitCode, err := run(context.Background(), domain.Config{Directory: "suite", Parallelism: 1}, reporting.NewReporter(&output, false))
	if exitCode != errs.ExitConfiguration || !errors.Is(err, definition.ErrRootDefinitionMissing) {
		t.Fatalf("run() = (%d, %v), want (%d, ErrRootDefinitionMissing)", exitCode, err, errs.ExitConfiguration)
	}
	if output.Len() != 0 {
		t.Fatalf("run() output = %q, want empty output", output.String())
	}
}

func TestRunReturnsInternalExitCodeForOutputFailure(t *testing.T) {
	workDir := t.TempDir()
	writeRootDefinition(t, workDir, "root.yaml")
	t.Chdir(workDir)
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

func TestFatalDiagnosticUsesSpecificManualAnchor(t *testing.T) {
	err := errs.Build(errs.ExitConfiguration, definition.ErrRootDefinitionMissing, nil)
	want := "error: root definition missing\n\n" +
		"please check user manual: " + userManualReference + "#root-definition-missing\n"
	if got := fatalDiagnostic(err); got != want {
		t.Fatalf("fatalDiagnostic() = %q, want %q", got, want)
	}
	if got := fatalDiagnostic(nil); got != "" {
		t.Fatalf("fatalDiagnostic(nil) = %q, want empty output", got)
	}
}

func TestTroubleshootingAnchorClassifiesFatalErrors(t *testing.T) {
	tests := map[string]struct {
		err  error
		want string
	}{
		"root missing":       {definition.ErrRootDefinitionMissing, "root-definition-missing"},
		"arguments":          {ErrInvalidArguments, "invalid-arguments"},
		"parallelism":        {execution.ErrInvalidParallelism, "invalid-arguments"},
		"selected directory": {ErrInvalidPath, "invalid-selected-directory"},
		"invalid definition": {definition.ErrInvalidDefinition, "invalid-yaml-definition"},
		"discovery":          {definition.ErrDefinitionDiscovery, "definition-discovery-error"},
		"variable":           {execution.ErrVariable, "missing-or-duplicate-variable"},
		"missing key":        {execution.ErrNotFound, "missing-or-duplicate-variable"},
		"duplicate key":      {execution.ErrKeyExists, "missing-or-duplicate-variable"},
		"external command":   {runner.ErrCommand, "external-tool-failure"},
		"curl":               {runner.ErrCurl, "external-tool-failure"},
		"jq selector":        {runner.ErrJQSelector, "external-tool-failure"},
		"jq pretty":          {runner.ErrJQPretty, "external-tool-failure"},
		"git diff":           {runner.ErrGitDiff, "external-tool-failure"},
		"capture":            {execution.ErrCapture, "capture-error"},
		"unknown":            {errors.New("unexpected"), "internal-errors"},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			if got := troubleshootingAnchor(test.err); got != test.want {
				t.Fatalf("troubleshootingAnchor() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestMainLogsFatalErrorAndPreservesProductExitCode(t *testing.T) {
	cmd := exec.Command(os.Args[0], "-test.run=TestMainHelperProcess")
	cmd.Env = append(os.Environ(), "APIH_TEST_MAIN_HELPER=invalid-path")
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
	if !strings.HasPrefix(stderr.String(), "error: "+ErrInvalidPath.Error()+":") {
		t.Fatalf("main process stderr = %q, want prefixed ErrInvalidPath", stderr.String())
	}
	wantFooter := "\n\nplease check user manual: " + userManualReference + "#invalid-selected-directory\n"
	if !strings.HasSuffix(stderr.String(), wantFooter) {
		t.Fatalf("main process stderr = %q, want footer %q", stderr.String(), wantFooter)
	}
	if strings.Count(stderr.String(), "error: ") != 1 || strings.Count(stderr.String(), "please check user manual: ") != 1 {
		t.Fatalf("main process stderr = %q, want one diagnostic and one footer", stderr.String())
	}
}

func TestMainReportsExactMissingRootDiagnostic(t *testing.T) {
	cmd := exec.Command(os.Args[0], "-test.run=TestMainHelperProcess")
	cmd.Dir = t.TempDir()
	cmd.Env = append(os.Environ(), "APIH_TEST_MAIN_HELPER=missing-root")
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != errs.ExitConfiguration {
		t.Fatalf("main process error = %v, want exit %d", err, errs.ExitConfiguration)
	}
	if stdout.Len() != 0 {
		t.Fatalf("main process stdout = %q, want empty output", stdout.String())
	}
	want := "error: root definition missing\n\n" +
		"please check user manual: " + userManualReference + "#root-definition-missing\n"
	if stderr.String() != want {
		t.Fatalf("main process stderr = %q, want %q", stderr.String(), want)
	}
}

func TestMainHelperProcess(t *testing.T) {
	scenario := os.Getenv("APIH_TEST_MAIN_HELPER")
	if scenario == "" {
		return
	}
	os.Args = []string{"apih"}
	if scenario == "invalid-path" {
		os.Args = append(os.Args, "path-that-does-not-exist")
	}
	main()
}
