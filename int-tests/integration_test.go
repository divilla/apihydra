//go:build integration

package inttests

import (
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
	cliPackage := filepath.Join(repoRoot, "cmd", "cli")
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
	if !strings.Contains(nondirectory.stderr, "invalid path") {
		t.Fatalf("file path stderr = %q, want invalid-path diagnostic", nondirectory.stderr)
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
	header, rawCurlCommand, rawDebugJSON := parseDebugDump(t, debugDefaults.stdout)
	if got, want := header, "stage: 0\ndir-path: /\nfile-path: steps.yaml"; got != want {
		t.Fatalf("debug provenance = %q, want %q", got, want)
	}
	for _, want := range []string{
		"curl --disable --globoff --silent --show-error",
		"Bearer integration-secret",
		"session=integration-cookie",
		server.URL + "/zero-types",
	} {
		if !strings.Contains(rawCurlCommand, want) {
			t.Fatalf("raw Curl command = %q, want complete value %q", rawCurlCommand, want)
		}
	}
	if strings.Contains(rawDebugJSON, "\x1b[") || !strings.HasPrefix(rawDebugJSON, `{"index":0,"vars":`) {
		t.Fatalf("raw Debug JSON = %q, want unsorted, uncolored Step encoding", rawDebugJSON)
	}
	var debugStep map[string]any
	if err := json.Unmarshal([]byte(rawDebugJSON), &debugStep); err != nil {
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
	for _, direct := range []string{"base_url", "base_path", "headers", "timeout", "retries"} {
		if _, exists := request[direct]; exists {
			t.Fatalf("debug request contains direct defaults field %q: %#v", direct, request)
		}
	}
	if debugStep["index"] != float64(0) {
		t.Fatalf("debug index = %#v, want 0", debugStep["index"])
	}
	response, ok := debugStep["response"].(map[string]any)
	if !ok {
		t.Fatalf("debug response = %#v, want object", debugStep["response"])
	}
	if got, ok := response["actual_body"].(string); !ok || got != `{"version":0}` {
		t.Fatalf("debug actual_body = %#v, want raw YAMLString JSON string", response["actual_body"])
	}
	if strings.Contains(debugDefaults.stdout, "[\x1b[38;5;10m✓\x1b[0m]") || !strings.HasSuffix(debugDefaults.stdout, rawDebugJSON+"\n") {
		t.Fatalf("debug defaults stdout = %q, want raw Debug dump as final output", debugDefaults.stdout)
	}
	copyPaste := exec.CommandContext(ctx, "/bin/sh", "-c", rawCurlCommand)
	if output, err := copyPaste.CombinedOutput(); err != nil {
		t.Fatalf("copy-pasted Debug Curl command error = %v; output = %q; command = %q", err, output, rawCurlCommand)
	}

	terminalDebugSuite := filepath.Join("scenarios", "invalid-debug-request-body")
	terminalDebug := runCLI(t, ctx, binary, runRoot, coverageDir, terminalDebugSuite)
	if terminalDebug.exitCode == 0 || terminalDebug.exitCode == 101 || terminalDebug.stderr == "" {
		t.Fatalf("terminal Debug = code %d, stdout %q, stderr %q, want fatal outcome", terminalDebug.exitCode, terminalDebug.stdout, terminalDebug.stderr)
	}
	terminalHeader, terminalCommand, terminalJSON := parseDebugDump(t, terminalDebug.stdout)
	if got, want := terminalHeader, "stage: 0\ndir-path: /\nfile-path: steps.yaml"; got != want {
		t.Fatalf("terminal Debug provenance = %q, want %q", got, want)
	}
	for _, want := range []string{"/debug-invalid-request", "not-json"} {
		if !strings.Contains(terminalCommand, want) {
			t.Fatalf("terminal Debug Curl command = %q, want attempted value %q", terminalCommand, want)
		}
	}
	var terminalStep map[string]any
	if err := json.Unmarshal([]byte(terminalJSON), &terminalStep); err != nil {
		t.Fatalf("decode terminal Debug JSON: %v; JSON = %q", err, terminalJSON)
	}
	terminalResponse, _ := terminalStep["response"].(map[string]any)
	if terminalResponse["actual_status"] != float64(http.StatusOK) || terminalResponse["actual_body"] != `{"id":7,"ok":true}` {
		t.Fatalf("terminal Debug response = %#v, want latest Curl runtime values", terminalResponse)
	}
	if !strings.HasSuffix(terminalDebug.stdout, terminalJSON+"\n") {
		t.Fatalf("terminal Debug stdout = %q, want dump as final execution output", terminalDebug.stdout)
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
		{
			method: http.MethodGet,
			path:   "/zero-types",
			headers: http.Header{
				"Authorization": []string{"Bearer integration-secret"},
				"Cookie":        []string{"session=integration-cookie"},
			},
		},
	}
	for _, want := range wantRequests {
		if !requests.contains(want) {
			t.Fatalf("HTTP requests do not contain method %s path %s query %q headers %v body %q", want.method, want.path, want.rawQuery, want.headers, want.body)
		}
	}
	if requests.contains(observedRequest{method: http.MethodGet, path: "/after-debug"}) {
		t.Fatal("HTTP requests contain /after-debug, want execution stopped at debug step")
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
	}

	server.Close()
	commandFailure := runCLI(t, ctx, binary, runRoot, coverageDir, successSuite)
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

func parseDebugDump(t *testing.T, output string) (string, string, string) {
	t.Helper()

	start := strings.Index(output, "stage: ")
	if start < 0 {
		t.Fatalf("Debug output = %q, want stage header", output)
	}
	dump := output[start:]
	const commandMarker = "\n\ncurl-command:\n"
	commandStart := strings.Index(dump, commandMarker)
	if commandStart < 0 {
		t.Fatalf("Debug output = %q, want curl-command block", output)
	}
	header := dump[:commandStart]
	remainder := dump[commandStart+len(commandMarker):]
	commandEnd := strings.Index(remainder, "\n\n")
	if commandEnd < 0 {
		t.Fatalf("Debug output = %q, want raw command separator", output)
	}
	command := remainder[:commandEnd]
	payload := strings.TrimSuffix(remainder[commandEnd+2:], "\n")
	if command == "" || payload == "" {
		t.Fatalf("Debug output = %q, want command and JSON payload", output)
	}
	return header, command, payload
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
	return []string{"build", "-buildvcs=false", "-cover", "-covermode=atomic", "-coverpkg=apih/...", "-o", binary, "./cmd/cli"}
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
	cmd := exec.CommandContext(ctx, binary, suite)
	cmd.Dir = workDir
	cmd.Env = append(os.Environ(), "GOCOVERDIR="+coverageDir)
	cmd.Env = append(cmd.Env, env...)
	var stderr strings.Builder
	cmd.Stdout = output
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
	return cliResult{exitCode: exitCode, stderr: stderr.String()}
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
	cmd.Env = append(os.Environ(), "GOCOVERDIR="+coverageDir)
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
