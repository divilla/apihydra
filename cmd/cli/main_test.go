package main

import (
	"apih/internal/reporting"
	"apih/pkg/errs"
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
)

type failingWriter struct {
	err error
}

func (w failingWriter) Write([]byte) (int, error) {
	return 0, w.err
}

func TestMainSourceMatchesSkeletonContract(t *testing.T) {
	production, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatalf("read production main: %v", err)
	}
	reference, err := os.ReadFile(filepath.Join("..", "..", "skeleton", "cmd", "cli", "main.go"))
	if err != nil {
		t.Fatalf("read reference main: %v", err)
	}
	reference = bytes.ReplaceAll(reference, []byte("apih/skeleton/"), []byte("apih/"))
	if !bytes.Equal(production, reference) {
		t.Fatal("production main does not match the binding skeleton after the import-prefix rewrite")
	}
}

func TestRunReturnsConfigurationExitCodeForInvalidPath(t *testing.T) {
	t.Chdir(t.TempDir())
	var output bytes.Buffer

	exitCode, err := run(context.Background(), []string{"apih", "path-that-does-not-exist"}, reporting.NewReporter(&output))
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

	exitCode, err := run(context.Background(), []string{"apih", "suite.yaml"}, reporting.NewReporter(&output))
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
	workDir := t.TempDir()
	selected := filepath.Join(workDir, "suite")
	if err := os.Mkdir(selected, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Chdir(workDir)
	var output bytes.Buffer

	exitCode, err := run(context.Background(), []string{"apih", "suite"}, reporting.NewReporter(&output))
	if err != nil {
		t.Fatalf("run() error = %v", err)
	}
	if exitCode != 0 {
		t.Fatalf("run() exit code = %d, want 0", exitCode)
	}
	if got, want := output.String(), "Working Directory: "+selected+"\n\nSuccess: /\n\n"; got != want {
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

	exitCode, err := run(context.Background(), []string{"apih"}, reporting.NewReporter(&bytes.Buffer{}))
	if exitCode != errs.ExitInternal {
		t.Fatalf("run() exit code = %d, want %d", exitCode, errs.ExitInternal)
	}
	if !errors.Is(err, ErrWorkingDirectory) {
		t.Fatalf("run() error = %v, want ErrWorkingDirectory", err)
	}
}

func TestRunReturnsInternalExitCodeForOutputFailure(t *testing.T) {
	t.Chdir(t.TempDir())
	wantErr := errors.New("output failed")

	exitCode, err := run(context.Background(), []string{"apih"}, reporting.NewReporter(failingWriter{err: wantErr}))
	if exitCode != errs.ExitInternal {
		t.Fatalf("run() exit code = %d, want %d", exitCode, errs.ExitInternal)
	}
	if !errors.Is(err, wantErr) {
		t.Fatalf("run() error = %v, want %v", err, wantErr)
	}
}

func TestRunReturnsConfigurationExitCodeForCanceledDefinitionPipeline(t *testing.T) {
	t.Chdir(t.TempDir())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var output bytes.Buffer

	exitCode, err := run(ctx, []string{"apih"}, reporting.NewReporter(&output))
	if exitCode != errs.ExitConfiguration {
		t.Fatalf("run() exit code = %d, want %d", exitCode, errs.ExitConfiguration)
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("run() error = %v, want context.Canceled", err)
	}
	if !strings.HasPrefix(output.String(), "Working Directory: ") {
		t.Fatalf("run() output = %q, want working-directory output", output.String())
	}
}

func TestRunReturnsConfigurationExitCodeForDefinitionFailures(t *testing.T) {
	tests := map[string]string{
		"base definition":    "[",
		"decoded definition": "app: apihydra\nkind: defaults\nspec:\n  timeout: invalid\n",
	}

	for name, contents := range tests {
		t.Run(name, func(t *testing.T) {
			workDir := t.TempDir()
			if err := os.WriteFile(filepath.Join(workDir, "definition.yaml"), []byte(contents), 0o600); err != nil {
				t.Fatal(err)
			}
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

			exitCode, err := run(context.Background(), []string{"apih"}, reporting.NewReporter(&output))
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

	workDir := t.TempDir()
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
		exitCode, err := run(ctx, []string{"apih"}, reporting.NewReporter(&bytes.Buffer{}))
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
