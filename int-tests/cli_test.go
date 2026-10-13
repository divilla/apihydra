//go:build integration

package inttests

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func repositoryRoot(t *testing.T) string {
	t.Helper()
	workDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("working directory: %v", err)
	}
	return filepath.Dir(workDir)
}

func buildCoveredCLI(t *testing.T, ctx context.Context, repoRoot, binary string) {
	t.Helper()
	cmd := exec.CommandContext(ctx, "go", coveredCLIBuildArguments(binary)...)
	cmd.Dir = repoRoot
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("build covered CLI: %v\n%s", err, output)
	}
}

func coveredCLIBuildArguments(binary string) []string {
	return []string{"build", "-buildvcs=false", "-cover", "-covermode=atomic", "-coverpkg=github.com/divilla/apihydra/...", "-o", binary, "./cmd/apih"}
}

type cliResult struct {
	exitCode int
	stdout   string
	stderr   string
}

func runCLI(t *testing.T, ctx context.Context, binary, workDir, coverageDir, suite string) cliResult {
	t.Helper()
	var stdout strings.Builder
	result := runCLICommand(t, ctx, binary, workDir, coverageDir, suite, &stdout)
	result.stdout = stdout.String()
	return result
}

func runCLIWithEnv(t *testing.T, ctx context.Context, binary, workDir, coverageDir, suite string, env ...string) cliResult {
	t.Helper()
	var stdout strings.Builder
	result := runCLICommand(t, ctx, binary, workDir, coverageDir, suite, &stdout, env...)
	result.stdout = stdout.String()
	return result
}

func runCLIWithOutput(t *testing.T, ctx context.Context, binary, workDir, coverageDir, suite string, output io.Writer) cliResult {
	t.Helper()
	return runCLICommand(t, ctx, binary, workDir, coverageDir, suite, output)
}

func runCLICommand(t *testing.T, ctx context.Context, binary, workDir, coverageDir, suite string, output io.Writer, env ...string) cliResult {
	t.Helper()
	return runCLIArguments(t, ctx, binary, workDir, coverageDir, []string{suite}, output, env...)
}

func runCLIArguments(t *testing.T, ctx context.Context, binary, workDir, coverageDir string, args []string, output io.Writer, env ...string) cliResult {
	t.Helper()
	cmd := exec.CommandContext(ctx, binary, args...)
	cmd.Dir = workDir
	cmd.Env = cliEnvironment(coverageDir, env...)
	var stderr strings.Builder
	cmd.Stdout = output
	cmd.Stderr = &stderr
	err := cmd.Run()

	exitCode := 0
	if err != nil {
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) {
			t.Fatalf("run %v: %v", args, err)
		}
		exitCode = exitErr.ExitCode()
	}
	return cliResult{exitCode: exitCode, stderr: stderr.String()}
}

func runCLICombined(t *testing.T, ctx context.Context, binary, workDir, coverageDir, suite string) (cliResult, string) {
	t.Helper()
	cmd := exec.CommandContext(ctx, binary, suite)
	cmd.Dir = workDir
	cmd.Env = cliEnvironment(coverageDir)
	var combined bytes.Buffer
	cmd.Stdout = &combined
	cmd.Stderr = &combined
	err := cmd.Run()
	exitCode := 0
	if err != nil {
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) {
			t.Fatalf("run combined %s: %v", suite, err)
		}
		exitCode = exitErr.ExitCode()
	}
	return cliResult{exitCode: exitCode}, combined.String()
}

func runCLIWithDelayedOutput(t *testing.T, ctx context.Context, binary, workDir, coverageDir, suite string, delay time.Duration) cliResult {
	t.Helper()
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("create delayed-output pipe: %v", err)
	}
	defer reader.Close()

	cmd := exec.CommandContext(ctx, binary, suite)
	cmd.Dir = workDir
	cmd.Env = cliEnvironment(coverageDir)
	var stdout strings.Builder
	var stderr strings.Builder
	cmd.Stdout = writer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		_ = writer.Close()
		t.Fatalf("start delayed-output %s: %v", suite, err)
	}

	drained := make(chan error, 1)
	go func() {
		time.Sleep(delay)
		_, copyErr := io.Copy(&stdout, reader)
		drained <- copyErr
	}()
	err = cmd.Wait()
	if closeErr := writer.Close(); closeErr != nil {
		t.Fatalf("close delayed-output writer: %v", closeErr)
	}
	if copyErr := <-drained; copyErr != nil {
		t.Fatalf("drain delayed output: %v", copyErr)
	}

	exitCode := 0
	if err != nil {
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) {
			t.Fatalf("run delayed-output %s: %v", suite, err)
		}
		exitCode = exitErr.ExitCode()
	}
	return cliResult{exitCode: exitCode, stdout: stdout.String(), stderr: stderr.String()}
}

func cliEnvironment(coverageDir string, overrides ...string) []string {
	environment := os.Environ()
	environment = setEnvironmentValue(environment, "GOCOVERDIR="+coverageDir)
	for _, value := range userCacheEnvironment(filepath.Join(filepath.Dir(coverageDir), "cache")) {
		environment = setEnvironmentValue(environment, value)
	}
	for _, override := range overrides {
		environment = setEnvironmentValue(environment, override)
	}
	return environment
}

func userCacheDirectory(root string) string {
	if runtime.GOOS == "darwin" {
		return filepath.Join(root, "Library", "Caches")
	}
	return root
}

func userCacheEnvironment(root string) []string {
	switch runtime.GOOS {
	case "darwin":
		return []string{"HOME=" + root}
	case "windows":
		return []string{"LocalAppData=" + root}
	default:
		return []string{"XDG_CACHE_HOME=" + root}
	}
}

func unavailableUserCacheEnvironment() []string {
	switch runtime.GOOS {
	case "darwin":
		return []string{"HOME="}
	case "windows":
		return []string{"LocalAppData="}
	default:
		return []string{"XDG_CACHE_HOME=", "HOME="}
	}
}

func setEnvironmentValue(environment []string, value string) []string {
	key, _, found := strings.Cut(value, "=")
	if !found {
		return append(environment, value)
	}
	prefix := key + "="
	filtered := environment[:0]
	for _, existing := range environment {
		if !strings.HasPrefix(existing, prefix) {
			filtered = append(filtered, existing)
		}
	}
	return append(filtered, value)
}

func permissionDenialScenariosSupported(effectiveUserID int) bool {
	return effectiveUserID != 0
}

func requirePermissionDenialScenarios(t *testing.T) {
	t.Helper()
	if !permissionDenialScenariosSupported(effectiveUserID()) {
		t.Skip("permission-denial scenarios require an unprivileged user")
	}
}
