package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/divilla/apihydra/internal/definition"
	"github.com/divilla/apihydra/internal/domain"
	"github.com/divilla/apihydra/internal/execution"
	"github.com/divilla/apihydra/internal/reporting"
	"github.com/divilla/apihydra/pkg/errs"
	"github.com/divilla/apihydra/pkg/runner"

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

func setTestUserCacheDir(t *testing.T, root string) string {
	t.Helper()
	for _, value := range testUserCacheEnvironment(root) {
		key, setting, _ := strings.Cut(value, "=")
		t.Setenv(key, setting)
	}
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		t.Fatalf("resolve test user cache directory: %v", err)
	}
	return cacheDir
}

func clearTestUserCacheDir(t *testing.T) {
	t.Helper()
	switch runtime.GOOS {
	case "darwin":
		t.Setenv("HOME", "")
	case "windows":
		t.Setenv("LocalAppData", "")
	default:
		t.Setenv("XDG_CACHE_HOME", "")
		t.Setenv("HOME", "")
	}
}

func testUserCacheEnvironment(root string) []string {
	switch runtime.GOOS {
	case "darwin":
		return []string{"HOME=" + root}
	case "windows":
		return []string{"LocalAppData=" + root}
	default:
		return []string{"XDG_CACHE_HOME=" + root}
	}
}

func TestMainSourceMatchesSkeletonContract(t *testing.T) {
	production, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatalf("read production main: %v", err)
	}
	reference, err := os.ReadFile(filepath.Join("..", "..", "skeleton", "cmd", "apih", "main.go"))
	if err != nil {
		t.Fatalf("read reference main: %v", err)
	}
	reference = bytes.ReplaceAll(
		reference,
		[]byte("github.com/divilla/apihydra/skeleton/"),
		[]byte("github.com/divilla/apihydra/"),
	)
	if !bytes.Equal(production, reference) {
		t.Fatal("production main does not match the binding skeleton after the import-prefix rewrite")
	}
}

func TestRunReturnsConfigurationExitCodeForInvalidPath(t *testing.T) {
	t.Chdir(t.TempDir())
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

func TestRunRejectsSelectedFile(t *testing.T) {
	workDir := t.TempDir()
	t.Chdir(workDir)
	if err := os.WriteFile(filepath.Join(workDir, "suite.yaml"), []byte("kind: steps\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer

	exitCode, err := run(context.Background(), domain.Config{Directory: "suite.yaml", Parallelism: 1}, reporting.NewReporter(&output, false))
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

func TestRunSelectsDirectoryAndCompletesDefinitionPipeline(t *testing.T) {
	setTestUserCacheDir(t, t.TempDir())
	workDir := t.TempDir()
	selected := filepath.Join(workDir, "suite")
	if err := os.Mkdir(selected, 0o700); err != nil {
		t.Fatal(err)
	}
	writeRootDefinition(t, selected, "suite.yml")
	t.Chdir(workDir)
	var output bytes.Buffer

	exitCode, err := run(context.Background(), domain.Config{Directory: "suite", Parallelism: 1}, reporting.NewReporter(&output, false))
	if err != nil {
		t.Fatalf("run() error = %v", err)
	}
	if exitCode != 0 {
		t.Fatalf("run() exit code = %d, want 0", exitCode)
	}
	if got, want := output.String(), "Working Directory: "+selected+"\n\n"; got != want {
		t.Fatalf("run() output = %q, want %q", got, want)
	}
}

func TestRunReturnsInternalExitCodeWhenWorkingDirectoryIsUnavailable(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows does not allow removing the current directory")
	}

	original, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	parent := t.TempDir()
	removed := filepath.Join(parent, "removed")
	if err := os.Mkdir(removed, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(removed); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(original); err != nil {
			t.Errorf("restore working directory: %v", err)
		}
	})
	if err := os.Remove(removed); err != nil {
		t.Fatal(err)
	}

	exitCode, err := run(context.Background(), domain.Config{Parallelism: 1}, reporting.NewReporter(&bytes.Buffer{}, false))
	if exitCode != errs.ExitInternal {
		t.Fatalf("run() exit code = %d, want %d", exitCode, errs.ExitInternal)
	}
	if !errors.Is(err, ErrWorkingDirectory) {
		t.Fatalf("run() error = %v, want ErrWorkingDirectory", err)
	}
}

func TestRunReturnsInternalExitCodeForOutputFailure(t *testing.T) {
	setTestUserCacheDir(t, t.TempDir())
	workDir := t.TempDir()
	writeRootDefinition(t, workDir, "root.yaml")
	t.Chdir(workDir)
	wantErr := errors.New("output failed")

	exitCode, err := run(context.Background(), domain.Config{Parallelism: 1}, reporting.NewReporter(failingWriter{err: wantErr}, false))
	if exitCode != errs.ExitInternal {
		t.Fatalf("run() exit code = %d, want %d", exitCode, errs.ExitInternal)
	}
	if !errors.Is(err, wantErr) {
		t.Fatalf("run() error = %v, want %v", err, wantErr)
	}
}

func TestRunReturnsConfigurationExitCodeForCanceledDefinitionPipeline(t *testing.T) {
	setTestUserCacheDir(t, t.TempDir())
	t.Chdir(t.TempDir())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var output bytes.Buffer

	exitCode, err := run(ctx, domain.Config{Parallelism: 1}, reporting.NewReporter(&output, false))
	if exitCode != errs.ExitConfiguration {
		t.Fatalf("run() exit code = %d, want %d", exitCode, errs.ExitConfiguration)
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("run() error = %v, want context.Canceled", err)
	}
	if output.Len() != 0 {
		t.Fatalf("run() output = %q, want cancellation before application output", output.String())
	}
}

func TestRunReturnsConfigurationExitCodeForDefinitionFailures(t *testing.T) {
	tests := map[string]string{
		"base definition":    "[",
		"decoded definition": "app: apihydra\nkind: steps\nspec:\n  defaults:\n    timeout: invalid\n  steps: []\n",
	}

	for name, contents := range tests {
		t.Run(name, func(t *testing.T) {
			setTestUserCacheDir(t, t.TempDir())
			workDir := t.TempDir()
			if err := os.WriteFile(filepath.Join(workDir, "definition.yaml"), []byte(contents), 0o600); err != nil {
				t.Fatal(err)
			}
			writeRootDefinition(t, workDir, "root.yaml")
			original, err := os.Getwd()
			if err != nil {
				t.Fatal(err)
			}
			if err := os.Chdir(workDir); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := os.Chdir(original); err != nil {
					t.Errorf("restore working directory: %v", err)
				}
			})
			var output bytes.Buffer

			exitCode, err := run(context.Background(), domain.Config{Parallelism: 1}, reporting.NewReporter(&output, false))
			if exitCode != errs.ExitConfiguration {
				t.Fatalf("run() exit code = %d, want %d", exitCode, errs.ExitConfiguration)
			}
			if err == nil {
				t.Fatal("run() error = nil, want definition failure")
			}
			if !strings.HasPrefix(output.String(), "Working Directory: ") {
				t.Fatalf("run() output = %q, want working-directory output", output.String())
			}
		})
	}
}

func TestRunPropagatesCancellationFromLongDefinitionPhases(t *testing.T) {
	setTestUserCacheDir(t, t.TempDir())
	workDir := t.TempDir()
	var rootDefaults strings.Builder
	rootDefaults.WriteString("app: apihydra\nkind: root\nspec:\n  headers:\n")
	for index := range 1000 {
		fmt.Fprintf(&rootDefaults, "    Header-%04d: value\n", index)
	}
	if err := os.WriteFile(filepath.Join(workDir, "root.yaml"), []byte(rootDefaults.String()), 0o600); err != nil {
		t.Fatal(err)
	}
	stepsContents := "app: apihydra\nkind: steps\nspec:\n  steps: []\n"
	if testing.CoverMode() != "" {
		stepsContents = "app: apihydra\nkind: steps\nspec:\n  steps:\n" + strings.Repeat("    - {}\n", 20)
	}
	for index := range 1000 {
		directory := filepath.Join(workDir, fmt.Sprintf("suite-%04d", index))
		if err := os.Mkdir(directory, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(
			filepath.Join(directory, "defaults.yaml"),
			[]byte("app: apihydra\nkind: defaults\nspec: {}\n"),
			0o600,
		); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(
			filepath.Join(directory, "steps.yaml"),
			[]byte(stepsContents),
			0o600,
		); err != nil {
			t.Fatal(err)
		}
	}
	t.Chdir(workDir)

	assertRunCanceledInPhase(t, "definition.(*Resolver).ResolveDefaults")
	assertRunCanceledInPhase(t, "definition.(*Loader).LoadDirectoryFiles")
	if testing.CoverMode() != "" {
		assertRunCanceledInPhase(t, "definition.(*Resolver).ResolveSteps")
	}
}

func TestRunCoversCancellationFromStepsValidation(t *testing.T) {
	if testing.CoverMode() == "" {
		t.Skip("short phase observation is only required for statement coverage")
	}
	setTestUserCacheDir(t, t.TempDir())

	workDir := t.TempDir()
	writeRootDefinition(t, workDir, "root.yaml")
	contents := []byte("app: apihydra\nkind: steps\nspec:\n  steps: []\n")
	for index := range 20000 {
		if err := os.WriteFile(filepath.Join(workDir, fmt.Sprintf("steps-%05d.yaml", index)), contents, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	t.Chdir(workDir)

	assertRunCanceledInPhase(t, "ValidateStepsDefinitions")
}

type runResult struct {
	exitCode int
	err      error
}

var cancellationPhaseCovered sync.Map

func assertRunCanceledInPhase(t *testing.T, phase string) {
	t.Helper()
	if testing.CoverMode() != "" {
		if _, covered := cancellationPhaseCovered.Load(phase); covered {
			return
		}
	}
	startingCoverage := testing.Coverage()
	lastCoverage := startingCoverage
	for attempt := 1; attempt <= 10; attempt++ {
		got, observed := runUntilPhase(t, phase)
		lastCoverage = testing.Coverage()
		coverageIncreased := testing.CoverMode() != "" && lastCoverage > startingCoverage
		if !observed && !coverageIncreased {
			continue
		}
		if got.exitCode != errs.ExitConfiguration {
			continue
		}
		if !errors.Is(got.err, context.Canceled) {
			continue
		}
		if testing.CoverMode() != "" && !coverageIncreased {
			continue
		}
		if testing.CoverMode() != "" {
			cancellationPhaseCovered.Store(phase, struct{}{})
		}
		return
	}
	t.Fatalf("run() did not consume cancellation in phase %s in 10 attempts (coverage %.1f%%)", phase, lastCoverage*100)
}

func runUntilPhase(t *testing.T, phase string) (runResult, bool) {
	t.Helper()
	previousProcs := runtime.GOMAXPROCS(max(runtime.GOMAXPROCS(0), 2))
	defer runtime.GOMAXPROCS(previousProcs)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	finished := make(chan runResult, 1)
	observed := make(chan struct{}, 1)
	stopObservers := make(chan struct{})
	var observers sync.WaitGroup
	observerCount := min(runtime.GOMAXPROCS(0), 8)
	observersReady := make(chan struct{}, observerCount)
	for range observerCount {
		observers.Add(1)
		go func() {
			defer observers.Done()
			observersReady <- struct{}{}
			for {
				select {
				case <-stopObservers:
					return
				default:
				}
				if goroutineProfileContains(phase) {
					cancel()
					select {
					case observed <- struct{}{}:
					default:
					}
					return
				}
				runtime.Gosched()
			}
		}()
	}
	for range observerCount {
		<-observersReady
	}
	defer func() {
		close(stopObservers)
		observers.Wait()
	}()
	go func() {
		exitCode, err := run(ctx, domain.Config{Parallelism: 1}, reporting.NewReporter(&bytes.Buffer{}, false))
		finished <- runResult{exitCode: exitCode, err: err}
	}()

	var got runResult
	select {
	case got = <-finished:
		select {
		case <-observed:
			return got, true
		default:
			return got, false
		}
	case <-observed:
		got = <-finished
	case <-time.After(30 * time.Second):
		t.Fatal("run() did not reach " + phase)
	}
	return got, true
}

func goroutineProfileContains(function string) bool {
	records := make([]runtime.StackRecord, runtime.NumGoroutine()+16)
	count, ok := runtime.GoroutineProfile(records)
	if !ok {
		return false
	}
	for _, record := range records[:count] {
		frames := runtime.CallersFrames(record.Stack())
		for {
			frame, more := frames.Next()
			if strings.Contains(frame.Function, function) {
				return true
			}
			if !more {
				break
			}
		}
	}
	return false
}

func TestParseConfigUsesNativePflagBehavior(t *testing.T) {
	tests := map[string]struct {
		args        []string
		parallelism int
		directory   string
		wantErr     bool
	}{
		"defaults":             {args: []string{"apih"}, parallelism: 1},
		"attached shorthand":   {args: []string{"apih", "-p0", "suite"}, parallelism: 0, directory: "suite"},
		"equals long":          {args: []string{"apih", "--parallelism=2", "suite"}, parallelism: 2, directory: "suite"},
		"interspersed":         {args: []string{"apih", "suite", "-p", "2"}, parallelism: 2, directory: "suite"},
		"repeated last wins":   {args: []string{"apih", "-p", "0", "--parallelism", "2"}, parallelism: 2},
		"terminator":           {args: []string{"apih", "--", "-p2"}, parallelism: 1, directory: "-p2"},
		"empty argv":           {args: nil, parallelism: 1},
		"too many directories": {args: []string{"apih", "one", "two"}, wantErr: true},
		"invalid parallelism":  {args: []string{"apih", "-p", "3"}, wantErr: true},
		"negative parallelism": {args: []string{"apih", "-p=-1"}, wantErr: true},
		"malformed value":      {args: []string{"apih", "-p", "many"}, wantErr: true},
		"unknown flag":         {args: []string{"apih", "--unknown"}, wantErr: true},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			var usage bytes.Buffer
			config, err := parseConfig(test.args, &usage)
			if test.wantErr {
				if !errors.Is(err, ErrInvalidArguments) || errs.Code(err, 0) != errs.ExitConfiguration {
					t.Fatalf("parseConfig() error = %v, want configuration ErrInvalidArguments", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if config != (domain.Config{Parallelism: test.parallelism, Directory: test.directory}) {
				t.Fatalf("parseConfig() = %#v", config)
			}
		})
	}
}

func TestParseConfigHelpUsesPflagOutput(t *testing.T) {
	for _, help := range []string{"-h", "--help"} {
		var output bytes.Buffer
		config, err := parseConfig([]string{"apih", help}, &output)
		if !errors.Is(err, pflag.ErrHelp) || config != (domain.Config{}) {
			t.Fatalf("parseConfig(%s) = (%#v, %v)", help, config, err)
		}
		if !strings.Contains(output.String(), "--parallelism") {
			t.Fatalf("help output = %q", output.String())
		}
	}
}

func TestRunRequiresRootInCurrentOrSelectedDirectory(t *testing.T) {
	tests := map[string]struct {
		setup  func(*testing.T) domain.Config
		config domain.Config
	}{
		"current directory": {
			setup: func(t *testing.T) domain.Config {
				t.Chdir(t.TempDir())
				return domain.Config{Parallelism: 1}
			},
		},
		"selected directory": {
			setup: func(t *testing.T) domain.Config {
				parent := t.TempDir()
				if err := os.Mkdir(filepath.Join(parent, "suite"), 0o700); err != nil {
					t.Fatal(err)
				}
				t.Chdir(parent)
				return domain.Config{Directory: "suite", Parallelism: 1}
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			config := test.setup(t)
			var output bytes.Buffer
			exitCode, err := run(context.Background(), config, reporting.NewReporter(&output, false))
			if exitCode != errs.ExitConfiguration || !errors.Is(err, definition.ErrRootDefinitionMissing) {
				t.Fatalf("run() = (%d, %v), want (%d, ErrRootDefinitionMissing)", exitCode, err, errs.ExitConfiguration)
			}
			if output.Len() != 0 {
				t.Fatalf("run() output = %q, want empty output", output.String())
			}
		})
	}
}

func TestFatalDiagnosticUsesSpecificManualAnchors(t *testing.T) {
	tests := map[string]struct {
		err    error
		anchor string
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
			if got := troubleshootingAnchor(test.err); got != test.anchor {
				t.Fatalf("troubleshootingAnchor() = %q, want %q", got, test.anchor)
			}
			got := fatalDiagnostic(test.err)
			wantFooter := "\n\nplease check user manual: " + userManualReference + "#" + test.anchor + "\n"
			if !strings.HasPrefix(got, "error: ") || !strings.HasSuffix(got, wantFooter) {
				t.Fatalf("fatalDiagnostic() = %q, want error prefix and footer %q", got, wantFooter)
			}
		})
	}
	if got := fatalDiagnostic(nil); got != "" {
		t.Fatalf("fatalDiagnostic(nil) = %q, want empty output", got)
	}
}

func TestRunDirectoryIsUniqueRunScopedAndCleaned(t *testing.T) {
	cacheRoot := setTestUserCacheDir(t, t.TempDir())
	workDir := t.TempDir()
	writeRootDefinition(t, workDir, "root.yaml")
	t.Chdir(workDir)
	cacheDir := filepath.Join(cacheRoot, "apih")
	if err := os.Mkdir(cacheDir, 0o777); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(cacheDir, 0o777); err != nil {
		t.Fatal(err)
	}

	first, err := createTempRunDirectory()
	if err != nil {
		t.Fatal(err)
	}
	second, err := createTempRunDirectory()
	if err != nil {
		t.Fatal(err)
	}
	if first == second || filepath.Dir(first) != filepath.Join(cacheRoot, "apih") || filepath.Dir(second) != filepath.Join(cacheRoot, "apih") {
		t.Fatalf("run directories = %q and %q", first, second)
	}
	cacheInfo, err := os.Stat(cacheDir)
	if err != nil || cacheInfo.Mode().Perm() != 0o700 {
		t.Fatalf("cache directory info = (%v, %v), want mode 0700", cacheInfo, err)
	}
	for _, path := range []string{first, second} {
		info, statErr := os.Stat(path)
		if statErr != nil || !info.IsDir() || info.Mode().Perm()&0o077 != 0 {
			t.Fatalf("run directory %q info = (%v, %v)", path, info, statErr)
		}
		if err := os.RemoveAll(path); err != nil {
			t.Fatal(err)
		}
	}

	var output bytes.Buffer
	exitCode, err := run(context.Background(), domain.Config{Parallelism: 1}, reporting.NewReporter(&output, false))
	if exitCode != 0 || err != nil {
		t.Fatalf("run() = (%d, %v)", exitCode, err)
	}
	entries, err := os.ReadDir(filepath.Join(cacheRoot, "apih"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("run cleanup retained entries: %v", entries)
	}
}

func TestCreateTempRunDirectoryClassifiesCacheFailures(t *testing.T) {
	t.Run("user cache unavailable", func(t *testing.T) {
		clearTestUserCacheDir(t)
		_, err := createTempRunDirectory()
		if !errors.Is(err, ErrUserCacheDirectory) {
			t.Fatalf("createTempRunDirectory() error = %v", err)
		}
	})

	t.Run("cache path is a file", func(t *testing.T) {
		cachePath := setTestUserCacheDir(t, filepath.Join(t.TempDir(), "cache-file"))
		if err := os.MkdirAll(filepath.Dir(cachePath), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(cachePath, []byte("file"), 0o600); err != nil {
			t.Fatal(err)
		}
		_, err := createTempRunDirectory()
		if !errors.Is(err, ErrUserCacheDirectory) {
			t.Fatalf("createTempRunDirectory() error = %v", err)
		}
	})

	t.Run("existing cache permissions cannot be secured", func(t *testing.T) {
		if runtime.GOOS != "linux" {
			t.Skip("uses Linux procfs as an unchmodable directory")
		}
		cacheRoot := setTestUserCacheDir(t, t.TempDir())
		if err := os.Symlink("/proc", filepath.Join(cacheRoot, "apih")); err != nil {
			t.Fatal(err)
		}
		_, err := createTempRunDirectory()
		if !errors.Is(err, ErrUserCacheDirectory) {
			t.Fatalf("createTempRunDirectory() error = %v, want ErrUserCacheDirectory", err)
		}
	})
}

func TestRunReturnsInternalWhenRunDirectoryCannotBeCreated(t *testing.T) {
	workDir := t.TempDir()
	writeRootDefinition(t, workDir, "root.yaml")
	t.Chdir(workDir)
	clearTestUserCacheDir(t)
	var output bytes.Buffer
	exitCode, err := run(context.Background(), domain.Config{Parallelism: 1}, reporting.NewReporter(&output, false))
	if exitCode != errs.ExitInternal || !errors.Is(err, ErrUserCacheDirectory) {
		t.Fatalf("run() = (%d, %v), want internal user-cache error", exitCode, err)
	}
	if output.Len() != 0 {
		t.Fatalf("run() output = %q, want empty", output.String())
	}
}

func TestIsTerminalUsesCharacterDeviceMode(t *testing.T) {
	regular, err := os.CreateTemp(t.TempDir(), "regular-")
	if err != nil {
		t.Fatal(err)
	}
	defer regular.Close()
	if isTerminal(regular) {
		t.Fatal("regular file detected as terminal")
	}
	device, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	defer device.Close()
	if !isTerminal(device) {
		t.Fatal("character device was not detected")
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
	if !strings.Contains(stderr.String(), ErrInvalidPath.Error()) {
		t.Fatalf("main process stderr = %q, want ErrInvalidPath", stderr.String())
	}
	if !strings.HasPrefix(stderr.String(), "error: ") || !strings.HasSuffix(stderr.String(), "#invalid-selected-directory\n") {
		t.Fatalf("main process stderr = %q, want prefixed anchored diagnostic", stderr.String())
	}
}

func TestMainHandlesHelpInvalidArgumentsSuccessAndHelpWriteFailure(t *testing.T) {
	tests := map[string]struct {
		mode       string
		wantExit   int
		wantStdout string
		wantStderr string
	}{
		"help": {
			mode:       "help",
			wantStdout: "--parallelism",
		},
		"invalid arguments": {
			mode:       "invalid-arguments",
			wantExit:   errs.ExitConfiguration,
			wantStderr: ErrInvalidArguments.Error(),
		},
		"success": {
			mode:       "success",
			wantStdout: "Working Directory:",
		},
		"help write failure": {
			mode:       "help-write-failure",
			wantExit:   errs.ExitInternal,
			wantStderr: "file already closed",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			workDir := t.TempDir()
			writeRootDefinition(t, workDir, "root.yaml")
			cacheDir := t.TempDir()
			cmd := exec.Command(os.Args[0], "-test.run=TestMainHelperProcess")
			cmd.Dir = workDir
			cmd.Env = append(os.Environ(), "APIH_TEST_MAIN_HELPER="+test.mode)
			cmd.Env = append(cmd.Env, testUserCacheEnvironment(cacheDir)...)
			var stdout bytes.Buffer
			var stderr bytes.Buffer
			cmd.Stdout = &stdout
			cmd.Stderr = &stderr
			err := cmd.Run()
			gotExit := 0
			if err != nil {
				var exitErr *exec.ExitError
				if !errors.As(err, &exitErr) {
					t.Fatal(err)
				}
				gotExit = exitErr.ExitCode()
			}
			if gotExit != test.wantExit {
				t.Fatalf("exit code = %d, want %d; stderr = %q", gotExit, test.wantExit, stderr.String())
			}
			if test.wantStdout != "" && !strings.Contains(stdout.String(), test.wantStdout) {
				t.Fatalf("stdout = %q, want %q", stdout.String(), test.wantStdout)
			}
			if test.wantStderr != "" && !strings.Contains(stderr.String(), test.wantStderr) {
				t.Fatalf("stderr = %q, want %q", stderr.String(), test.wantStderr)
			}
			if test.mode == "invalid-arguments" && stdout.Len() != 0 {
				t.Fatalf("invalid stdout = %q, want empty", stdout.String())
			}
			if test.wantStderr != "" && (!strings.HasPrefix(stderr.String(), "error: ") || !strings.Contains(stderr.String(), "\n\nplease check user manual: ")) {
				t.Fatalf("stderr = %q, want one fatal diagnostic with manual footer", stderr.String())
			}
		})
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
	switch os.Getenv("APIH_TEST_MAIN_HELPER") {
	case "invalid-path":
		os.Args = []string{"apih", "path-that-does-not-exist"}
	case "invalid-arguments":
		os.Args = []string{"apih", "--parallelism=invalid"}
	case "help":
		os.Args = []string{"apih", "--help"}
	case "help-write-failure":
		os.Args = []string{"apih", "--help"}
		if err := os.Stdout.Close(); err != nil {
			os.Exit(99)
		}
	case "success":
		os.Args = []string{"apih"}
	case "missing-root":
		os.Args = []string{"apih"}
	default:
		return
	}
	main()
}
