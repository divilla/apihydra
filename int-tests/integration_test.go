//go:build integration

package inttests

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/goccy/go-yaml"
)

const (
	serverMarker       = "http://APIH_TEST_SERVER"
	minimumCoverage    = 90.0
	integrationTimeout = 2 * time.Minute
)

type observedRequests struct {
	mu     sync.Mutex
	bodies []string
}

func (o *observedRequests) add(body string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.bodies = append(o.bodies, body)
}

func (o *observedRequests) joinedBodies() string {
	o.mu.Lock()
	defer o.mu.Unlock()
	return strings.Join(o.bodies, "\n")
}

func TestApplicationScenariosAndCoverage(t *testing.T) {
	repoRoot := repositoryRoot(t)
	cliPackage := filepath.Join(repoRoot, "cmd", "cli")
	if _, err := os.Stat(cliPackage); errors.Is(err, os.ErrNotExist) {
		t.Skip("integration prerequisite missing: implement agent/specs/011-main-app-int-tests.md")
	} else if err != nil {
		t.Fatalf("stat CLI package: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), integrationTimeout)
	defer cancel()

	var requests observedRequests
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestBody, readErr := readRequestBody(r)
		if readErr != nil {
			http.Error(w, readErr.Error(), http.StatusBadRequest)
			return
		}
		requests.add(requestBody)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":7,"ok":true}`))
	}))
	defer server.Close()

	tempRoot := t.TempDir()
	coverageDir := filepath.Join(tempRoot, "coverage")
	if err := os.Mkdir(coverageDir, 0o755); err != nil {
		t.Fatalf("create coverage directory: %v", err)
	}
	binary := filepath.Join(tempRoot, "apih")
	buildCoveredCLI(t, ctx, repoRoot, binary)

	runRoot := filepath.Join(tempRoot, "runs")
	if err := os.Mkdir(runRoot, 0o755); err != nil {
		t.Fatalf("create run directory: %v", err)
	}
	for _, fixture := range []string{"test1", "test2"} {
		source := filepath.Join(repoRoot, "int-tests", "input", fixture)
		destination := filepath.Join(runRoot, fixture)
		copyFixture(t, source, destination, server.URL)
	}

	invalid := runCLI(t, ctx, binary, runRoot, coverageDir, "missing-suite")
	if invalid.exitCode != 102 {
		t.Fatalf("invalid path exit code = %d, want 102; stderr = %q", invalid.exitCode, invalid.stderr)
	}
	if invalid.stdout != "" {
		t.Fatalf("invalid path stdout = %q, want empty", invalid.stdout)
	}
	if !strings.Contains(invalid.stderr, "invalid path") {
		t.Fatalf("invalid path stderr = %q, want invalid-path diagnostic", invalid.stderr)
	}
	nondirectory := runCLI(t, ctx, binary, runRoot, coverageDir, filepath.Join("test1", "root.yaml"))
	if nondirectory.exitCode != 102 {
		t.Fatalf("file path exit code = %d, want 102; stderr = %q", nondirectory.exitCode, nondirectory.stderr)
	}
	if nondirectory.stdout != "" {
		t.Fatalf("file path stdout = %q, want empty", nondirectory.stdout)
	}

	success := runCLI(t, ctx, binary, runRoot, coverageDir, "test1")
	if success.exitCode != 0 {
		t.Fatalf("test1 exit code = %d, want 0; stdout = %q; stderr = %q", success.exitCode, success.stdout, success.stderr)
	}
	if success.stderr != "" {
		t.Fatalf("test1 stderr = %q, want empty", success.stderr)
	}
	if !strings.Contains(success.stdout, "Working Directory:") {
		t.Fatalf("test1 stdout = %q, want working-directory output", success.stdout)
	}

	validation := runCLI(t, ctx, binary, runRoot, coverageDir, "test2")
	if validation.exitCode != 101 {
		t.Fatalf("test2 exit code = %d, want 101; stdout = %q; stderr = %q", validation.exitCode, validation.stdout, validation.stderr)
	}
	if validation.stderr != "" {
		t.Fatalf("test2 stderr = %q, want empty for nonfatal validation", validation.stderr)
	}
	if !strings.Contains(validation.stdout, "Working Directory:") {
		t.Fatalf("test2 stdout = %q, want working-directory output", validation.stdout)
	}
	workingDirectoryOnly := fmt.Sprintf("Working Directory: %s\n\n", filepath.Join(runRoot, "test2"))
	if validation.stdout == workingDirectoryOnly {
		t.Fatalf("test2 stdout = %q, want reported validation output", validation.stdout)
	}

	gotBodies := requests.joinedBodies()
	for _, want := range []string{`"name":"alpha"`, `"id":7`} {
		if !strings.Contains(gotBodies, want) {
			t.Fatalf("HTTP request bodies = %q, want interpolated fragment %q", gotBodies, want)
		}
	}

	server.Close()
	commandFailure := runCLI(t, ctx, binary, runRoot, coverageDir, "test1")
	if commandFailure.exitCode == 0 {
		t.Fatalf("unavailable HTTP server exit code = 0, want fatal failure; stdout = %q", commandFailure.stdout)
	}
	if commandFailure.stderr == "" {
		t.Fatal("unavailable HTTP server stderr is empty, want fatal diagnostic")
	}

	assertProductionCoverage(t, ctx, repoRoot, coverageDir, minimumCoverage)
}

func TestTotalCoverage(t *testing.T) {
	percentage, err := totalCoverage("apih/pkg\tcoverage: 91.2% of statements\ntotal:\t(statements)\t91.2%\n")
	if err != nil {
		t.Fatalf("totalCoverage() error = %v", err)
	}
	if percentage != 91.2 {
		t.Fatalf("totalCoverage() = %.1f, want 91.2", percentage)
	}
	if _, err := totalCoverage("no total"); err == nil {
		t.Fatal("totalCoverage() error = nil for malformed summary")
	}
}

func TestStaticFixturesAreYAML(t *testing.T) {
	inputRoot := filepath.Join(repositoryRoot(t), "int-tests", "input")
	files := 0
	err := filepath.WalkDir(inputRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || (filepath.Ext(path) != ".yaml" && filepath.Ext(path) != ".yml") {
			return nil
		}
		files++
		contents, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		var document map[string]any
		if err := yaml.Unmarshal(contents, &document); err != nil {
			return fmt.Errorf("decode %s: %w", path, err)
		}
		for _, key := range []string{"app", "kind", "spec"} {
			if _, ok := document[key]; !ok {
				return fmt.Errorf("%s has no %q field", path, key)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if files == 0 {
		t.Fatal("no integration YAML fixtures found")
	}
}

func readRequestBody(r *http.Request) (string, error) {
	defer r.Body.Close()
	contents, err := io.ReadAll(r.Body)
	if err != nil {
		return "", err
	}
	return string(contents), nil
}

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
	cmd := exec.CommandContext(ctx, "go", "build", "-cover", "-covermode=atomic", "-coverpkg=apih/...", "-o", binary, "./cmd/cli")
	cmd.Dir = repoRoot
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("build covered CLI: %v\n%s", err, output)
	}
}

type cliResult struct {
	exitCode int
	stdout   string
	stderr   string
}

func runCLI(t *testing.T, ctx context.Context, binary, workDir, coverageDir, suite string) cliResult {
	t.Helper()
	cmd := exec.CommandContext(ctx, binary, suite)
	cmd.Dir = workDir
	cmd.Env = append(os.Environ(), "GOCOVERDIR="+coverageDir)
	var stdout strings.Builder
	var stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()

	exitCode := 0
	if err != nil {
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) {
			t.Fatalf("run %s: %v", suite, err)
		}
		exitCode = exitErr.ExitCode()
	}
	return cliResult{exitCode: exitCode, stdout: stdout.String(), stderr: stderr.String()}
}

func copyFixture(t *testing.T, source, destination, serverURL string) {
	t.Helper()
	err := filepath.WalkDir(source, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		target := filepath.Join(destination, relative)
		if entry.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		contents, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		contents = []byte(strings.ReplaceAll(string(contents), serverMarker, serverURL))
		return os.WriteFile(target, contents, 0o644)
	})
	if err != nil {
		t.Fatalf("copy fixture %s: %v", filepath.Base(source), err)
	}
}

func assertProductionCoverage(t *testing.T, ctx context.Context, repoRoot, coverageDir string, minimum float64) {
	t.Helper()
	profile := filepath.Join(t.TempDir(), "integration.cover")
	textfmt := exec.CommandContext(ctx, "go", "tool", "covdata", "textfmt", "-i="+coverageDir, "-o="+profile)
	textfmt.Dir = repoRoot
	if output, err := textfmt.CombinedOutput(); err != nil {
		t.Fatalf("format integration coverage: %v\n%s", err, output)
	}

	cover := exec.CommandContext(ctx, "go", "tool", "cover", "-func="+profile)
	cover.Dir = repoRoot
	output, err := cover.CombinedOutput()
	if err != nil {
		t.Fatalf("summarize integration coverage: %v\n%s", err, output)
	}
	percentage, err := totalCoverage(string(output))
	if err != nil {
		t.Fatal(err)
	}
	if percentage < minimum {
		t.Fatalf("integration production coverage = %.1f%%, want at least %.1f%%\n%s", percentage, minimum, output)
	}
}

func totalCoverage(output string) (float64, error) {
	pattern := regexp.MustCompile(`(?m)^total:\s+\(statements\)\s+([0-9]+(?:\.[0-9]+)?)%$`)
	match := pattern.FindStringSubmatch(output)
	if match == nil {
		return 0, fmt.Errorf("coverage summary has no total line: %q", output)
	}
	percentage, err := strconv.ParseFloat(match[1], 64)
	if err != nil {
		return 0, fmt.Errorf("parse total coverage %q: %w", match[1], err)
	}
	return percentage, nil
}
