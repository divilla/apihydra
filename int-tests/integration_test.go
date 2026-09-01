//go:build integration

package inttests

import (
	"bytes"
	"context"
	"encoding/json"
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
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/goccy/go-yaml"
)

const (
	serverMarker       = "http://APIH_TEST_SERVER"
	integrationTimeout = 2 * time.Minute
	manualReference    = "https://github.com/divilla/apihydra/blob/master/docs/user-manual/apih.md"
)

type observedRequests struct {
	mu       sync.Mutex
	requests []observedRequest
}

type observedRequest struct {
	method   string
	path     string
	rawQuery string
	headers  http.Header
	body     string
}

func (o *observedRequests) add(request *http.Request, body string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.requests = append(o.requests, observedRequest{
		method:   request.Method,
		path:     request.URL.Path,
		rawQuery: request.URL.RawQuery,
		headers:  request.Header.Clone(),
		body:     body,
	})
}

func (o *observedRequests) contains(want observedRequest) bool {
	o.mu.Lock()
	defer o.mu.Unlock()
	for _, request := range o.requests {
		if request.method != want.method || request.path != want.path || request.rawQuery != want.rawQuery || request.body != want.body {
			continue
		}
		matches := true
		for name, values := range want.headers {
			if !slices.Equal(request.headers.Values(name), values) {
				matches = false
				break
			}
		}
		if matches {
			return true
		}
	}
	return false
}

func TestApplicationScenariosAndCoverage(t *testing.T) {
	if runApplicationScenariosAsUnprivilegedUser(t) {
		return
	}

	repoRoot := repositoryRoot(t)
	cliPackage := filepath.Join(repoRoot, "cmd", "apih")
	if _, err := os.Stat(cliPackage); errors.Is(err, os.ErrNotExist) {
		t.Skip("integration prerequisite missing: implement agent/specs/011-main-app.md")
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
		requests.add(r, requestBody)
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/cookies" {
			query := r.URL.Query()
			if delayText := query.Get("delay_ms"); delayText != "" {
				delay, parseErr := strconv.Atoi(delayText)
				if parseErr != nil {
					http.Error(w, parseErr.Error(), http.StatusBadRequest)
					return
				}
				time.Sleep(time.Duration(delay) * time.Millisecond)
			}
			gotCookie := ""
			if cookie, cookieErr := r.Cookie("session"); cookieErr == nil {
				gotCookie = cookie.Value
			}
			if wantCookie := query.Get("want"); gotCookie != wantCookie {
				w.WriteHeader(http.StatusConflict)
				_, _ = w.Write([]byte(`{"ok":false}`))
				return
			}
			if value := query.Get("set"); value != "" {
				http.SetCookie(w, &http.Cookie{Name: "session", Value: value, Path: "/"})
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"ok":true}`))
			return
		}
		if r.URL.Path == "/invalid-json" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("not-json"))
			return
		}
		if r.URL.Path == "/slow-invalid-json" {
			time.Sleep(100 * time.Millisecond)
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("not-json"))
			return
		}
		if r.URL.Path == "/slow-success" {
			time.Sleep(100 * time.Millisecond)
		}
		if r.URL.Path == "/hang" {
			<-r.Context().Done()
			return
		}
		if r.URL.Path == "/large" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"data":"` + strings.Repeat("a", 100000) + `"}`))
			return
		}
		if r.URL.Path == "/api/items" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"id":7,"ok":true,"metadata":{"state":"ready","ignored":true},"ignored":true}`))
			return
		}
		if r.URL.Path == "/zero-types" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"version":0}`))
			return
		}
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
	for _, fixture := range []string{"test1", "test2", "scenarios"} {
		source := filepath.Join(repoRoot, "int-tests", "input", fixture)
		destination := filepath.Join(runRoot, fixture)
		copyFixture(t, source, destination, server.URL)
	}

	wantMissingRoot := "error: root definition missing\n\nplease check user manual: " + manualReference + "#root-definition-missing\n"
	missingCurrentRoot := runCLIArguments(t, ctx, binary, runRoot, coverageDir, nil, io.Discard)
	if missingCurrentRoot.exitCode != 102 || missingCurrentRoot.stdout != "" || missingCurrentRoot.stderr != wantMissingRoot {
		t.Fatalf("current-directory missing root = code %d, stdout %q, stderr %q, want exact root diagnostic", missingCurrentRoot.exitCode, missingCurrentRoot.stdout, missingCurrentRoot.stderr)
	}
	missingSelectedDir := filepath.Join(runRoot, "missing-root")
	if err := os.MkdirAll(filepath.Join(missingSelectedDir, "nested"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(missingSelectedDir, "nested", "invalid.yaml"), []byte("app: []\nkind: root\nspec: {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for name, contents := range map[string]string{
		"malformed.yaml": "[",
		"wrong-app.yaml": "app: other\nkind: root\n",
		"wrong-kind.yml": "app: apihydra\nkind: []\n",
		"ignored.YAML":   "app: apihydra\nkind: root\n",
	} {
		if err := os.WriteFile(filepath.Join(missingSelectedDir, name), []byte(contents), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	missingSelectedRoot := runCLI(t, ctx, binary, runRoot, coverageDir, "missing-root")
	if missingSelectedRoot.exitCode != 102 || missingSelectedRoot.stdout != "" || missingSelectedRoot.stderr != wantMissingRoot {
		t.Fatalf("selected-directory missing root = code %d, stdout %q, stderr %q, want exact root diagnostic", missingSelectedRoot.exitCode, missingSelectedRoot.stdout, missingSelectedRoot.stderr)
	}
	arbitraryRootDir := filepath.Join(runRoot, "arbitrary-root")
	if err := os.Mkdir(arbitraryRootDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(arbitraryRootDir, "suite.yml"), []byte("app: apihydra\nkind: root\nspec: {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	arbitraryRoot := runCLI(t, ctx, binary, runRoot, coverageDir, "arbitrary-root")
	if arbitraryRoot.exitCode != 0 || arbitraryRoot.stderr != "" || !strings.Contains(arbitraryRoot.stdout, "Working Directory:") {
		t.Fatalf("arbitrary root filename = code %d, stdout %q, stderr %q, want success", arbitraryRoot.exitCode, arbitraryRoot.stdout, arbitraryRoot.stderr)
	}
	invalidNestedDir := filepath.Join(runRoot, "invalid-nested-definition")
	if err := os.MkdirAll(filepath.Join(invalidNestedDir, "child"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(invalidNestedDir, "root.yaml"), []byte("app: apihydra\nkind: root\nspec: {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(invalidNestedDir, "child", "broken.yaml"), []byte("app: []\nkind: root\nspec: {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	invalidNested := runCLI(t, ctx, binary, runRoot, coverageDir, "invalid-nested-definition")
	if invalidNested.exitCode != 102 || !strings.Contains(invalidNested.stdout, "Working Directory:") ||
		!strings.Contains(invalidNested.stderr, "invalid definition") || !strings.Contains(invalidNested.stderr, "file child/broken.yaml") ||
		!strings.HasSuffix(invalidNested.stderr, "#invalid-yaml-definition\n") {
		t.Fatalf("invalid nested definition = code %d, stdout %q, stderr %q, want contextual invalid-definition diagnostic", invalidNested.exitCode, invalidNested.stdout, invalidNested.stderr)
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
	assertFatalDiagnostic(t, invalid.stderr)
	nondirectory := runCLI(t, ctx, binary, runRoot, coverageDir, filepath.Join("test1", "root.yaml"))
	if nondirectory.exitCode != 102 {
		t.Fatalf("file path exit code = %d, want 102; stderr = %q", nondirectory.exitCode, nondirectory.stderr)
	}
	if nondirectory.stdout != "" {
		t.Fatalf("file path stdout = %q, want empty", nondirectory.stdout)
	}
	if !strings.Contains(nondirectory.stderr, "invalid path") {
		t.Fatalf("file path stderr = %q, want invalid-path diagnostic", nondirectory.stderr)
	}
	assertFatalDiagnostic(t, nondirectory.stderr)

	runArguments := func(args []string, env ...string) cliResult {
		var stdout strings.Builder
		result := runCLIArguments(t, ctx, binary, runRoot, coverageDir, args, &stdout, env...)
		result.stdout = stdout.String()
		return result
	}
	helpCache := filepath.Join(tempRoot, "help-cache")
	for _, helpFlag := range []string{"-h", "--help"} {
		help := runArguments([]string{helpFlag}, userCacheEnvironment(helpCache)...)
		if help.exitCode != 0 || help.stderr != "" || !strings.Contains(help.stdout, "--parallelism") {
			t.Fatalf("%s result = code %d, stdout %q, stderr %q", helpFlag, help.exitCode, help.stdout, help.stderr)
		}
	}
	if _, err := os.Stat(filepath.Join(userCacheDirectory(helpCache), "apih")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("help created application cache: %v", err)
	}
	helpOutputPath := filepath.Join(tempRoot, "help-output")
	if err := os.WriteFile(helpOutputPath, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	readOnlyHelpOutput, err := os.Open(helpOutputPath)
	if err != nil {
		t.Fatal(err)
	}
	helpWriteFailure := runCLIArguments(t, ctx, binary, runRoot, coverageDir, []string{"--help"}, readOnlyHelpOutput)
	_ = readOnlyHelpOutput.Close()
	if helpWriteFailure.exitCode != 103 || helpWriteFailure.stderr == "" {
		t.Fatalf("help write failure = code %d, stderr %q", helpWriteFailure.exitCode, helpWriteFailure.stderr)
	}
	for _, args := range [][]string{
		{"--unknown"},
		{"-p", "many"},
		{"--parallelism=3"},
		{"test1", "test2"},
	} {
		invalidArguments := runArguments(args)
		if invalidArguments.exitCode != 102 || invalidArguments.stdout != "" || invalidArguments.stderr == "" {
			t.Fatalf("invalid arguments %v = code %d, stdout %q, stderr %q", args, invalidArguments.exitCode, invalidArguments.stdout, invalidArguments.stderr)
		}
		assertFatalDiagnostic(t, invalidArguments.stderr)
	}
	for iteration := range 2 {
		cookiesModeZero := runArguments([]string{"--parallelism=0", filepath.Join("scenarios", "cookies-mode0")})
		if cookiesModeZero.exitCode != 0 || cookiesModeZero.stderr != "" {
			t.Fatalf("cookies mode 0 run %d = code %d, stdout %q, stderr %q", iteration, cookiesModeZero.exitCode, cookiesModeZero.stdout, cookiesModeZero.stderr)
		}
	}
	for mode, suite := range map[string]string{
		"1": filepath.Join("scenarios", "cookies-mode1"),
		"2": filepath.Join("scenarios", "cookies-mode2"),
	} {
		result := runArguments([]string{"--parallelism=" + mode, suite})
		if result.exitCode != 0 || result.stderr != "" {
			t.Fatalf("cookies mode %s = code %d, stdout %q, stderr %q", mode, result.exitCode, result.stdout, result.stderr)
		}
	}
	for _, suite := range []string{"cookies-mode2-empty-root", "cookies-mode2-empty-file"} {
		result := runArguments([]string{"--parallelism=2", filepath.Join("scenarios", suite)})
		if result.exitCode != 0 || result.stderr != "" {
			t.Fatalf("%s = code %d, stdout %q, stderr %q", suite, result.exitCode, result.stdout, result.stderr)
		}
	}
	orderedDirectories := runArguments([]string{filepath.Join("scenarios", "ordered-directories")})
	if orderedDirectories.exitCode != 0 || orderedDirectories.stderr != "" {
		t.Fatalf("ordered directories = code %d, stderr %q", orderedDirectories.exitCode, orderedDirectories.stderr)
	}
	if slow, fast := strings.Index(orderedDirectories.stdout, "/a-slow/steps\n"), strings.Index(orderedDirectories.stdout, "/b-fast/steps\n"); slow < 0 || fast < slow {
		t.Fatalf("ordered directory output = %q, want slow plan entry before fast completion", orderedDirectories.stdout)
	}
	orderedFiles := runArguments([]string{"--parallelism=2", filepath.Join("scenarios", "ordered-files")})
	if orderedFiles.exitCode != 0 || orderedFiles.stderr != "" {
		t.Fatalf("ordered files = code %d, stderr %q", orderedFiles.exitCode, orderedFiles.stderr)
	}
	if slow, fast := strings.Index(orderedFiles.stdout, "/a-slow\n"), strings.Index(orderedFiles.stdout, "/b-fast\n"); slow < 0 || fast < slow {
		t.Fatalf("ordered file output = %q, want slow plan entry before fast completion", orderedFiles.stdout)
	}
	modeSuite := filepath.Join("scenarios", "success-output")
	cacheUnavailable := runArguments([]string{modeSuite}, unavailableUserCacheEnvironment()...)
	if cacheUnavailable.exitCode != 103 || cacheUnavailable.stdout != "" || cacheUnavailable.stderr == "" {
		t.Fatalf("unavailable user cache = code %d, stdout %q, stderr %q", cacheUnavailable.exitCode, cacheUnavailable.stdout, cacheUnavailable.stderr)
	}
	cacheRoot := filepath.Join(tempRoot, "cache-file-root")
	cacheFile := userCacheDirectory(cacheRoot)
	if err := os.MkdirAll(filepath.Dir(cacheFile), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cacheFile, []byte("file"), 0o600); err != nil {
		t.Fatal(err)
	}
	cachePathFailure := runArguments([]string{modeSuite}, userCacheEnvironment(cacheRoot)...)
	if cachePathFailure.exitCode != 103 || cachePathFailure.stdout != "" || cachePathFailure.stderr == "" {
		t.Fatalf("cache path failure = code %d, stdout %q, stderr %q", cachePathFailure.exitCode, cachePathFailure.stdout, cachePathFailure.stderr)
	}
	for _, args := range [][]string{
		{"-p0", modeSuite},
		{modeSuite, "-p", "1"},
		{"--parallelism=2", modeSuite},
		{"-p", "0", "--parallelism", "2", modeSuite},
		{"--", modeSuite},
	} {
		modeResult := runArguments(args)
		if modeResult.exitCode != 0 || modeResult.stderr != "" || !strings.Contains(modeResult.stdout, "/steps\n") {
			t.Fatalf("parallelism arguments %v = code %d, stdout %q, stderr %q", args, modeResult.exitCode, modeResult.stdout, modeResult.stderr)
		}
	}

	realGit, err := exec.LookPath("git")
	if err != nil {
		t.Fatalf("locate git: %v", err)
	}
	operationLog := filepath.Join(tempRoot, "git-operation-dirs")
	gitProxy := fmt.Sprintf("#!/bin/sh\npwd >> %q\nexec %q \"$@\"\n", operationLog, realGit)
	tempTools := createToolDirectory(t, map[string]string{"git": gitProxy}, []string{"curl", "jq"})
	runCacheRoot := filepath.Join(tempRoot, "run-cache")
	runCache := userCacheDirectory(runCacheRoot)
	missingSystemTemp := filepath.Join(tempRoot, "missing-system-temp")
	for range 2 {
		overrides := append([]string{
			"PATH=" + tempTools,
			"TMPDIR=" + missingSystemTemp,
		}, userCacheEnvironment(runCacheRoot)...)
		result := runArguments(
			[]string{"--parallelism=2", filepath.Join("test2", "body-only")},
			overrides...,
		)
		if result.exitCode != 101 || result.stderr != "" {
			t.Fatalf("run-scoped GitDiff = code %d, stderr %q", result.exitCode, result.stderr)
		}
	}
	operationBytes, err := os.ReadFile(operationLog)
	if err != nil {
		t.Fatal(err)
	}
	operationDirs := strings.Fields(string(operationBytes))
	if len(operationDirs) != 2 || operationDirs[0] == operationDirs[1] {
		t.Fatalf("GitDiff operation directories = %v, want two unique paths", operationDirs)
	}
	for _, operationDir := range operationDirs {
		runDir := filepath.Dir(filepath.Dir(operationDir))
		if filepath.Dir(runDir) != filepath.Join(runCache, "apih") || !strings.HasPrefix(filepath.Base(runDir), "run-") {
			t.Fatalf("GitDiff operation directory = %q, want cache-scoped run path", operationDir)
		}
		if _, err := os.Stat(runDir); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("run directory cleanup error for %q: %v", runDir, err)
		}
	}

	successSuite := "test1"
	success := runCLI(t, ctx, binary, runRoot, coverageDir, successSuite)
	if success.exitCode != 0 {
		t.Fatalf("test1 exit code = %d, want 0; stdout = %q; stderr = %q", success.exitCode, success.stdout, success.stderr)
	}
	if success.stderr != "" {
		t.Fatalf("test1 stderr = %q, want empty", success.stderr)
	}
	if !strings.Contains(success.stdout, "Working Directory:") {
		t.Fatalf("test1 stdout = %q, want working-directory output", success.stdout)
	}
	if rootStage, childStage := strings.Index(success.stdout, "/steps\n"), strings.Index(success.stdout, "/child/steps\n"); rootStage < 0 || childStage < rootStage {
		t.Fatalf("test1 stdout = %q, want sequential stage output order", success.stdout)
	}
	// Exercise URL handling for coverage without making its unspecified
	// normalization behavior part of the black-box contract.
	runCLI(t, ctx, binary, runRoot, coverageDir, filepath.Join("scenarios", "curl-options"))

	validationSuite := "test2"
	validation := runCLI(t, ctx, binary, runRoot, coverageDir, validationSuite)
	if validation.exitCode != 101 {
		t.Fatalf("test2 exit code = %d, want 101; stdout = %q; stderr = %q", validation.exitCode, validation.stdout, validation.stderr)
	}
	if validation.stderr != "" {
		t.Fatalf("test2 stderr = %q, want empty for nonfatal validation", validation.stderr)
	}
	if !strings.Contains(validation.stdout, "Working Directory:") {
		t.Fatalf("test2 stdout = %q, want working-directory output", validation.stdout)
	}
	workingDirectoryOnly := fmt.Sprintf("Working Directory: %s\n\n", filepath.Join(runRoot, validationSuite))
	if strings.TrimSpace(strings.TrimPrefix(validation.stdout, workingDirectoryOnly)) == "" {
		t.Fatalf("test2 stdout = %q, want reported validation output", validation.stdout)
	}
	validationDetails := runCLI(t, ctx, binary, runRoot, coverageDir, filepath.Join("test2", "validation"))
	if validationDetails.exitCode != 101 || validationDetails.stderr != "" {
		t.Fatalf("validation detail result = code %d, stderr %q, want validation output", validationDetails.exitCode, validationDetails.stderr)
	}
	for _, want := range []string{
		"[\x1b[38;5;210m✗\x1b[0m] /steps\n",
		"[\x1b[38;5;210m✗\x1b[0m] /api/status-mismatch GET \x1b[36mstep-1\x1b[0m\n",
		"[\x1b[38;5;210m✗\x1b[0m] /api/implicit-post POST \x1b[36mstep-2\x1b[0m\n",
		"[\x1b[38;5;10m✓\x1b[0m] /update\n",
		"    expected_types:\n",
		"\x1b[38;5;210m[string]\x1b[0m",
		"    actual_status: \x1b[38;5;210m200\x1b[0m\n",
		"    expected_status: \x1b[38;5;10m201\x1b[0m\n",
		"    expected_body:\n",
		"\x1b[38;5;210m-  \"id\": 7,\x1b[m",
		"\x1b[92m+\x1b[m\x1b[92m  \"id\": 9,\x1b[m",
		"\x1b[92m+\x1b[m\x1b[92m  \"aaa\": null,\x1b[m",
	} {
		if !strings.Contains(validationDetails.stdout, want) {
			t.Fatalf("validation detail stdout = %q, want %q", validationDetails.stdout, want)
		}
	}
	if got := strings.Count(validationDetails.stdout, "[\x1b[38;5;210m✗\x1b[0m] /steps\n"); got != 1 {
		t.Fatalf("validation file header count = %d, want 1; stdout = %q", got, validationDetails.stdout)
	}
	if got := strings.Count(validationDetails.stdout, "[\x1b[38;5;210m✗\x1b[0m]"); got != 3 {
		t.Fatalf("validation cross count = %d, want file plus two step crosses; stdout = %q", got, validationDetails.stdout)
	}
	if got := strings.Count(validationDetails.stdout, "[\x1b[38;5;10m✓\x1b[0m] /update\n"); got != 1 {
		t.Fatalf("valid sibling definition success count = %d, want 1; stdout = %q", got, validationDetails.stdout)
	}
	if !strings.Contains(validationDetails.stdout, "\x1b[m\n\n[\x1b[38;5;210m✗\x1b[0m] /api/implicit-post") ||
		!strings.Contains(validationDetails.stdout, "\x1b[m\n\n[\x1b[38;5;10m✓\x1b[0m] /update\n") {
		t.Fatalf("validation detail stdout = %q, want one blank line below every failing step", validationDetails.stdout)
	}
	if strings.Contains(validationDetails.stdout, "\x1b[38;5;210m-  \"aaa\": null,\x1b[m") ||
		strings.Contains(validationDetails.stdout, "\x1b[36m@@") ||
		strings.Contains(validationDetails.stdout, `"ok": true`) ||
		strings.Contains(validationDetails.stdout, "\x1b[38;5;9m[string]\x1b[0m") {
		t.Fatalf("validation detail stdout = %q, want actual-to-expected changed values only", validationDetails.stdout)
	}

	successOutputSuite := filepath.Join("scenarios", "success-output")
	successOutput := runCLI(t, ctx, binary, runRoot, coverageDir, successOutputSuite)
	wantSuccessOutput := fmt.Sprintf(
		"Working Directory: %s\n\n[\x1b[38;5;10m✓\x1b[0m] /steps\n",
		filepath.Join(runRoot, successOutputSuite),
	)
	if successOutput.exitCode != 0 || successOutput.stderr != "" || successOutput.stdout != wantSuccessOutput {
		t.Fatalf("success output = code %d, stdout %q, stderr %q, want %q", successOutput.exitCode, successOutput.stdout, successOutput.stderr, wantSuccessOutput)
	}

	intZeroSuite := filepath.Join("scenarios", "int-zero-types")
	intZero := runCLI(t, ctx, binary, runRoot, coverageDir, intZeroSuite)
	wantIntZeroOutput := fmt.Sprintf(
		"Working Directory: %s\n\n[\x1b[38;5;10m✓\x1b[0m] /steps\n",
		filepath.Join(runRoot, intZeroSuite),
	)
	if intZero.exitCode != 0 || intZero.stderr != "" || intZero.stdout != wantIntZeroOutput {
		t.Fatalf("int/zero types = code %d, stdout %q, stderr %q, want %q", intZero.exitCode, intZero.stdout, intZero.stderr, wantIntZeroOutput)
	}

	debugDefaultsSuite := filepath.Join("scenarios", "debug-defaults")
	debugDefaults := runCLI(t, ctx, binary, runRoot, coverageDir, debugDefaultsSuite)
	if debugDefaults.exitCode != 0 || debugDefaults.stderr != "" {
		t.Fatalf("debug defaults = code %d, stdout %q, stderr %q, want success", debugDefaults.exitCode, debugDefaults.stdout, debugDefaults.stderr)
	}
	for _, want := range []string{
		"stage: 0\ndir-path: /\nfile-path: steps.yaml\n\ncurl-command:\ncurl ",
		`--write-out %{stderr}http-code:%{http_code} --data-binary '{"request":true}'`,
		"--header 'Authorization: Bearer complete-debug-secret'",
		"--header 'Cookie: session=unredacted-cookie'",
		"\x1b[1;34m\"defaults\"\x1b[0m",
		"\x1b[1;34m\"index\"\x1b[0m\x1b[1;39m:\x1b[0m \x1b[0;39m0\x1b[0m",
		"\x1b[1;34m\"actual_body\"\x1b[0m",
		"\x1b[1;34m\"timeout\"\x1b[0m\x1b[1;39m:\x1b[0m \x1b[0;39m10\x1b[0m",
		"\x1b[1;34m\"retries\"\x1b[0m\x1b[1;39m:\x1b[0m \x1b[0;39m3\x1b[0m",
	} {
		if !strings.Contains(debugDefaults.stdout, want) {
			t.Fatalf("debug defaults stdout = %q, want %q", debugDefaults.stdout, want)
		}
	}
	cookieArguments := regexp.MustCompile(`--cookie ([^ ]+\.cookie\.jar) --cookie-jar ([^ ]+\.cookie\.jar)`)
	cookieMatch := cookieArguments.FindStringSubmatch(debugDefaults.stdout)
	if len(cookieMatch) != 3 || cookieMatch[1] != cookieMatch[2] || !strings.Contains(cookieMatch[1], string(filepath.Separator)+"apih"+string(filepath.Separator)+"run-") {
		t.Fatalf("debug defaults cookie arguments = %v; stdout = %q", cookieMatch, debugDefaults.stdout)
	}
	if strings.Contains(debugDefaults.stdout, "--output") || strings.Contains(debugDefaults.stdout, "apih-curl-response-") {
		t.Fatalf("debug defaults stdout = %q, contains temporary curl response output", debugDefaults.stdout)
	}
	if strings.Contains(debugDefaults.stdout, "[\x1b[38;5;10m✓\x1b[0m]") || !strings.HasSuffix(debugDefaults.stdout, "\x1b[0m\n") {
		t.Fatalf("debug defaults stdout = %q, want debug JSON as final output", debugDefaults.stdout)
	}
	plainDebug := regexp.MustCompile(`\x1b\[[0-9;]*m`).ReplaceAllString(debugDefaults.stdout, "")
	jsonStart := strings.LastIndex(plainDebug, "\n{")
	if jsonStart < 0 {
		t.Fatalf("debug defaults stdout = %q, want JSON object", debugDefaults.stdout)
	}
	jsonStart++
	var debugStep map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(plainDebug[jsonStart:])), &debugStep); err != nil {
		t.Fatalf("decode debug defaults JSON: %v; stdout = %q", err, debugDefaults.stdout)
	}
	request, ok := debugStep["request"].(map[string]any)
	if !ok {
		t.Fatalf("debug request = %#v, want object", debugStep["request"])
	}
	defaults, ok := request["defaults"].(map[string]any)
	baseURL, hasBaseURL := defaults["base_url"].(string)
	if !ok || !hasBaseURL || baseURL == "" || defaults["timeout"] != float64(10) || defaults["retries"] != float64(3) {
		t.Fatalf("debug request.defaults = %#v, want base_url plus timeout 10 and retries 3", request["defaults"])
	}
	for _, direct := range []string{"base_url", "base_path", "headers", "disable_cookies", "timeout", "retries"} {
		if _, exists := request[direct]; exists {
			t.Fatalf("debug request contains direct defaults field %q: %#v", direct, request)
		}
	}
	if debugStep["index"] != float64(0) {
		t.Fatalf("debug index = %#v, want 0", debugStep["index"])
	}
	requestBody, ok := request["body"].(map[string]any)
	if !ok || requestBody["request"] != true {
		t.Fatalf("debug request.body = %#v, want prettified JSON object", request["body"])
	}
	response, ok := debugStep["response"].(map[string]any)
	if !ok {
		t.Fatalf("debug response = %#v, want object", debugStep["response"])
	}
	actualBody, ok := response["actual_body"].(map[string]any)
	if !ok || actualBody["version"] != float64(0) {
		t.Fatalf("debug actual_body = %#v, want prettified JSON object", response["actual_body"])
	}
	expectedBody, ok := response["expected_body"].(map[string]any)
	if !ok || expectedBody["version"] != float64(0) {
		t.Fatalf("debug expected_body = %#v, want prettified JSON object", response["expected_body"])
	}
	if _, exists := debugStep["raw_curl"]; exists {
		t.Fatalf("debug Step JSON contains runtime-only raw_curl: %#v", debugStep)
	}

	debugTerminalSuite := filepath.Join("scenarios", "debug-terminal-error")
	debugTerminal := runCLI(t, ctx, binary, runRoot, coverageDir, debugTerminalSuite)
	if debugTerminal.exitCode == 0 || debugTerminal.exitCode == 101 || debugTerminal.stderr == "" {
		t.Fatalf("terminal debug = code %d, stdout %q, stderr %q, want fatal diagnostic", debugTerminal.exitCode, debugTerminal.stdout, debugTerminal.stderr)
	}
	for _, want := range []string{
		"stage: 0\ndir-path: /\nfile-path: steps.yaml\n\ncurl-command:\ncurl ",
		"Authorization: Bearer terminal-debug-secret",
		`"actual_status"`,
		`"actual_body"`,
	} {
		if !strings.Contains(debugTerminal.stdout, want) {
			t.Fatalf("terminal debug stdout = %q, want %q", debugTerminal.stdout, want)
		}
	}
	stringBodyDebug := runCLI(t, ctx, binary, runRoot, coverageDir, filepath.Join("scenarios", "invalid-debug-request-body"))
	if stringBodyDebug.exitCode != 0 || stringBodyDebug.stderr != "" || !strings.Contains(stringBodyDebug.stdout, `"not-json"`) {
		t.Fatalf("string-body debug = code %d, stdout %q, stderr %q, want successful complete Step encoding", stringBodyDebug.exitCode, stringBodyDebug.stdout, stringBodyDebug.stderr)
	}
	nilExpectedTypes := runCLI(t, ctx, binary, runRoot, coverageDir, filepath.Join("scenarios", "nil-expected-types"))
	if nilExpectedTypes.exitCode != 101 || nilExpectedTypes.stderr != "" {
		t.Fatalf("nil expected-types result = code %d, stderr %q, want validation result", nilExpectedTypes.exitCode, nilExpectedTypes.stderr)
	}

	wantRequests := []observedRequest{
		{
			method:   http.MethodPost,
			path:     "/api/items",
			rawQuery: "source=integration",
			headers: http.Header{
				"Content-Type": []string{"application/json"},
				"X-Definition": []string{"inherited"},
				"X-Root":       []string{"inherited"},
				"X-Step":       []string{"local"},
			},
			body: `{"name":"alpha"}`,
		},
		{method: http.MethodPost, path: "/api/items/captured", body: `{"id":7}`},
		{method: http.MethodHead, path: "/api/head"},
		{method: http.MethodGet, path: "/child-api/inherited"},
	}
	for _, want := range wantRequests {
		if !requests.contains(want) {
			t.Fatalf("HTTP requests do not contain method %s path %s query %q headers %v body %q", want.method, want.path, want.rawQuery, want.headers, want.body)
		}
	}
	if requests.contains(observedRequest{method: http.MethodGet, path: "/after-debug"}) {
		t.Fatal("HTTP requests contain /after-debug, want execution stopped at debug step")
	}
	if requests.contains(observedRequest{method: http.MethodGet, path: "/after-terminal-error"}) {
		t.Fatal("HTTP requests contain /after-terminal-error, want terminal debug to stop execution")
	}

	fatalScenarios := []struct {
		suite    string
		exitCode int
	}{
		{filepath.Join("scenarios", "invalid-base"), 102},
		{filepath.Join("scenarios", "invalid-defaults"), 102},
		{filepath.Join("scenarios", "invalid-steps"), 102},
		{filepath.Join("scenarios", "duplicate-variable"), 103},
		{filepath.Join("scenarios", "missing-request-variable"), 103},
		{filepath.Join("scenarios", "missing-expected-variable"), 103},
		{filepath.Join("scenarios", "invalid-expected-body"), -1},
		{filepath.Join("scenarios", "invalid-actual-body"), -1},
		{filepath.Join("scenarios", "invalid-type-selector"), -1},
		{filepath.Join("scenarios", "invalid-capture-selector"), -1},
		{filepath.Join("scenarios", "duplicate-capture"), 103},
		{filepath.Join("scenarios", "concurrent-fatal"), -1},
	}

	runUnixSpecificScenarios(t, ctx, binary, runRoot, coverageDir, tempRoot, successSuite, validationSuite)
	runPlatformSpecificScenarios(t, ctx, binary, runRoot, coverageDir)

	manySiblings := filepath.Join(runRoot, "scenarios", "many-siblings")
	siblingSource := filepath.Join(manySiblings, "fatal", "steps.yaml")
	for index := range 200 {
		directory := filepath.Join(manySiblings, fmt.Sprintf("sibling-%03d", index))
		if err := os.Mkdir(directory, 0o755); err != nil {
			t.Fatalf("create sibling directory: %v", err)
		}
		if err := os.Link(siblingSource, filepath.Join(directory, "steps.yaml")); err != nil {
			t.Fatalf("create sibling fixture: %v", err)
		}
	}
	siblingCancellation := runCLI(t, ctx, binary, runRoot, coverageDir, filepath.Join("scenarios", "many-siblings"))
	if siblingCancellation.exitCode == 0 || siblingCancellation.exitCode == 101 || siblingCancellation.stderr == "" {
		t.Fatalf("many-siblings result = code %d, stderr %q, want fatal diagnostic", siblingCancellation.exitCode, siblingCancellation.stderr)
	}
	for index := range 200 {
		if err := os.Link(siblingSource, filepath.Join(manySiblings, "fatal", fmt.Sprintf("steps-%03d.yaml", index))); err != nil {
			t.Fatalf("create same-directory file fixture: %v", err)
		}
	}
	fileCancellation := runArguments([]string{"--parallelism=2", filepath.Join("scenarios", "many-siblings")})
	if fileCancellation.exitCode == 0 || fileCancellation.exitCode == 101 || fileCancellation.stderr == "" {
		t.Fatalf("many-files result = code %d, stderr %q, want fatal diagnostic", fileCancellation.exitCode, fileCancellation.stderr)
	}
	blockedOutputCancellation := runCLIWithDelayedOutput(t, ctx, binary, runRoot, coverageDir, filepath.Join("scenarios", "concurrent-fatal"), 300*time.Millisecond)
	if blockedOutputCancellation.exitCode == 0 || blockedOutputCancellation.exitCode == 101 || blockedOutputCancellation.stderr == "" {
		t.Fatalf("blocked-output cancellation result = code %d, stderr %q, want fatal diagnostic", blockedOutputCancellation.exitCode, blockedOutputCancellation.stderr)
	}

	for _, scenario := range fatalScenarios {
		result := runCLI(t, ctx, binary, runRoot, coverageDir, scenario.suite)
		if scenario.exitCode >= 0 && result.exitCode != scenario.exitCode {
			t.Fatalf("%s exit code = %d, want %d; stdout = %q; stderr = %q", scenario.suite, result.exitCode, scenario.exitCode, result.stdout, result.stderr)
		}
		if scenario.exitCode < 0 && (result.exitCode == 0 || result.exitCode == 101) {
			t.Fatalf("%s exit code = %d, want fatal nonzero result; stdout = %q; stderr = %q", scenario.suite, result.exitCode, result.stdout, result.stderr)
		}
		if result.stderr == "" {
			t.Fatalf("%s stderr is empty, want fatal diagnostic", scenario.suite)
		}
		assertFatalDiagnostic(t, result.stderr)
	}
	combinedFatal, combinedOutput := runCLICombined(
		t,
		ctx,
		binary,
		runRoot,
		coverageDir,
		filepath.Join("scenarios", "missing-request-variable"),
	)
	if combinedFatal.exitCode != 103 {
		t.Fatalf("combined fatal exit code = %d, want 103; output = %q", combinedFatal.exitCode, combinedOutput)
	}
	for _, provenance := range []string{"step execution error", "file steps.yaml", "yaml path spec.steps[0]"} {
		if !strings.Contains(combinedOutput, provenance) {
			t.Fatalf("combined fatal output = %q, want provenance %q", combinedOutput, provenance)
		}
	}
	if !strings.HasSuffix(combinedOutput, "#missing-or-duplicate-variable\n") {
		t.Fatalf("fatal diagnostic was not the final application output: %q", combinedOutput)
	}

	server.Close()
	commandFailure := runCLI(t, ctx, binary, runRoot, coverageDir, successSuite)
	if commandFailure.exitCode == 0 {
		t.Fatalf("unavailable HTTP server exit code = 0, want fatal failure; stdout = %q", commandFailure.stdout)
	}
	if commandFailure.stderr == "" {
		t.Fatal("unavailable HTTP server stderr is empty, want fatal diagnostic")
	}
	assertFatalDiagnostic(t, commandFailure.stderr)

	assertProductionCoverage(t, ctx, repoRoot, coverageDir, minimumCoverage)
}

func assertFatalDiagnostic(t *testing.T, stderr string) {
	t.Helper()
	if !strings.HasPrefix(stderr, "error: ") {
		t.Fatalf("fatal stderr = %q, want error prefix", stderr)
	}
	footer := "\n\nplease check user manual: " + manualReference + "#"
	if strings.Count(stderr, footer) != 1 || !strings.HasSuffix(stderr, "\n") {
		t.Fatalf("fatal stderr = %q, want one final anchored manual footer", stderr)
	}
}

func TestTotalCoverage(t *testing.T) {
	percentage, err := totalCoverage("github.com/divilla/apihydra/pkg\tcoverage: 91.2% of statements\ntotal:\t(statements)\t91.2%\n")
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

func TestCoveredCLIBuildDisablesVCSStamping(t *testing.T) {
	for _, argument := range coveredCLIBuildArguments("apih") {
		if argument == "-buildvcs=false" {
			return
		}
	}
	t.Fatal("covered CLI build arguments do not disable VCS stamping")
}

func TestPermissionDenialScenariosSupported(t *testing.T) {
	if permissionDenialScenariosSupported(0) {
		t.Fatal("permissionDenialScenariosSupported(0) = true, want false")
	}
	if !permissionDenialScenariosSupported(1) {
		t.Fatal("permissionDenialScenariosSupported(1) = false, want true")
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

func createToolDirectory(t *testing.T, scripts map[string]string, links []string) string {
	t.Helper()
	directory := t.TempDir()
	for name, contents := range scripts {
		if err := os.WriteFile(filepath.Join(directory, name), []byte(contents), 0o700); err != nil {
			t.Fatalf("write fake %s: %v", name, err)
		}
	}
	for _, name := range links {
		path, err := exec.LookPath(name)
		if err != nil {
			t.Fatalf("locate %s: %v", name, err)
		}
		if err := os.Symlink(path, filepath.Join(directory, name)); err != nil {
			t.Fatalf("link %s: %v", name, err)
		}
	}
	return directory
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
