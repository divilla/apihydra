package runner

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

func TestPublicContract(t *testing.T) {
	var curl func(context.Context, string, string, map[string]string, int, int, string, string) (string, int, error) = Curl
	var jqFilter func(context.Context, string, string) (string, int, error) = JQFilter
	var jqSelect func(context.Context, []string, string) (string, int, error) = JQSelect
	var jqPretty func(context.Context, string) (string, int, error) = JQPretty
	var gitDiff func(context.Context, string, string) (string, int, error) = GitDiff
	_, _, _, _, _ = curl, jqFilter, jqSelect, jqPretty, gitDiff

	tests := map[string]struct {
		err  error
		want string
	}{
		"command":     {ErrCommand, "command error"},
		"curl":        {ErrCurl, "curl error"},
		"jq selector": {ErrJQSelector, "jq selector error"},
		"jq pretty":   {ErrJQPretty, "jq pretty error"},
		"git diff":    {ErrGitDiff, "git diff error"},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			if got := test.err.Error(); got != test.want {
				t.Fatalf("error text = %q, want %q", got, test.want)
			}
		})
	}
}

func TestCurlExecutesRequestAndReturnsBodyAndStatus(t *testing.T) {
	requireCommand(t, "curl")
	requireCommand(t, "jq")

	type request struct {
		method string
		query  string
		header string
		body   string
	}
	requests := make(chan request, 1)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, incoming *http.Request) {
		body, err := io.ReadAll(incoming.Body)
		if err != nil {
			t.Errorf("read request body: %v", err)
		}
		requests <- request{
			method: incoming.Method,
			query:  incoming.URL.RawQuery,
			header: incoming.Header.Get("X-API-Key"),
			body:   string(body),
		}
		writer.WriteHeader(http.StatusCreated)
		_, _ = writer.Write([]byte("response body\n"))
	}))
	defer server.Close()

	output, status, err := Curl(
		context.Background(),
		http.MethodPatch,
		server.URL+"?first=one#fragment",
		map[string]string{"X-Zed": "last", "X-API-Key": "secret"},
		5,
		1,
		"second=two",
		"{\n  \"request\": \"body\"\n}",
	)
	if err != nil {
		t.Fatalf("Curl() error = %v", err)
	}
	if status != http.StatusCreated {
		t.Fatalf("Curl() status = %d, want %d", status, http.StatusCreated)
	}
	if output != "response body\n" {
		t.Fatalf("Curl() body = %q, want %q", output, "response body\n")
	}

	got := <-requests
	if got.method != http.MethodPatch {
		t.Errorf("request method = %q, want %q", got.method, http.MethodPatch)
	}
	if got.query != "first=one&second=two" {
		t.Errorf("request query = %q, want %q", got.query, "first=one&second=two")
	}
	if got.header != "secret" {
		t.Errorf("request header = %q, want %q", got.header, "secret")
	}
	if got.body != `{"request":"body"}` {
		t.Errorf("request body = %q, want compact color-free JSON", got.body)
	}
}

func TestCurlHEADReturnsStatusWithoutExpectingResponseBody(t *testing.T) {
	requireCommand(t, "curl")

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, incoming *http.Request) {
		if incoming.Method != http.MethodHead {
			t.Errorf("request method = %q, want %q", incoming.Method, http.MethodHead)
		}
		writer.Header().Set("Content-Length", "128")
		writer.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	output, status, err := Curl(context.Background(), http.MethodHead, server.URL, nil, 5, 0, "", "")
	if err != nil || status != http.StatusOK || output != "" {
		t.Fatalf("Curl() = (%q, %d, %v), want (empty, 200, nil)", output, status, err)
	}
}

func TestCurlStreamsLargeRequestBody(t *testing.T) {
	requireCommand(t, "curl")
	requireCommand(t, "jq")

	payload := strings.Repeat("x", 256*1024)
	body := "{\n  \"payload\": \"" + payload + "\"\n}"
	wantBody := `{"payload":"` + payload + `"}`
	receivedBody := make(chan string, 1)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, incoming *http.Request) {
		body, err := io.ReadAll(incoming.Body)
		if err != nil {
			t.Errorf("read request body: %v", err)
		}
		receivedBody <- string(body)
		writer.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	_, status, err := Curl(
		context.Background(),
		http.MethodPost,
		server.URL,
		nil,
		5,
		0,
		"",
		body,
	)
	if err != nil {
		t.Fatalf("Curl() error = %v", err)
	}
	if status != http.StatusNoContent {
		t.Fatalf("Curl() status = %d, want %d", status, http.StatusNoContent)
	}
	if gotBody := <-receivedBody; gotBody != wantBody {
		t.Fatalf("request body length = %d, want %d", len(gotBody), len(wantBody))
	}
}

func TestCurlReturnsOnlyFinalRetryBody(t *testing.T) {
	requireCommand(t, "curl")

	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		if attempts.Add(1) == 1 {
			writer.WriteHeader(http.StatusServiceUnavailable)
			_, _ = writer.Write([]byte("transient response"))
			return
		}
		_, _ = writer.Write([]byte("final response"))
	}))
	defer server.Close()

	output, status, err := Curl(context.Background(), http.MethodGet, server.URL, nil, 5, 1, "", "")
	if err != nil {
		t.Fatalf("Curl() error = %v", err)
	}
	if status != http.StatusOK {
		t.Fatalf("Curl() status = %d, want %d", status, http.StatusOK)
	}
	if output != "final response" {
		t.Fatalf("Curl() body = %q, want only final response", output)
	}
	if got := attempts.Load(); got != 2 {
		t.Fatalf("request attempts = %d, want 2", got)
	}
}

func TestCurlDisablesImplicitConfiguration(t *testing.T) {
	useCommandScript(t, "curl", `
test "$1" = "--disable" || exit 9
output=
while [ "$#" -gt 0 ]; do
	if [ "$1" = "--output" ]; then
		shift
		output=$1
	fi
	shift
done
printf response > "$output"
printf '\n200'
`)

	output, status, err := Curl(context.Background(), http.MethodGet, "http://example.test", nil, 1, 0, "", "")
	if err != nil || status != http.StatusOK || output != "response" {
		t.Fatalf("Curl() = (%q, %d, %v), want (response, 200, nil)", output, status, err)
	}
}

func TestCurlDisablesURLGlobbing(t *testing.T) {
	const url = "http://example.test/items?filter[status]=open&value={a,b}"
	useCommandScript(t, "curl", `
output=
globoff_count=0
for argument do
	if [ "$argument" = "--globoff" ]; then
		globoff_count=$((globoff_count + 1))
	fi
	if [ "$argument" = "--output" ]; then
		want_output=true
	elif [ "${want_output:-false}" = true ]; then
		output=$argument
		want_output=false
	fi
	second_last=$last
	last=$argument
done
test "$globoff_count" -eq 1 || exit 9
test "$second_last" = "--" || exit 10
test "$last" = "http://example.test/items?filter[status]=open&value={a,b}" || exit 11
printf response > "$output"
printf '\n200'
`)

	output, status, err := Curl(context.Background(), http.MethodGet, url, nil, 1, 0, "", "")
	if err != nil || status != http.StatusOK || output != "response" {
		t.Fatalf("Curl() = (%q, %d, %v), want (response, 200, nil)", output, status, err)
	}
}

func TestCurlTerminatesOptionsBeforeURL(t *testing.T) {
	const url = "--config=/tmp/curlrc"
	useCommandScript(t, "curl", `
output=
for argument do
	if [ "$argument" = "--output" ]; then
		want_output=true
	elif [ "${want_output:-false}" = true ]; then
		output=$argument
		want_output=false
	fi
	second_last=$last
	last=$argument
done
test "$second_last" = "--" || exit 9
test "$last" = "--config=/tmp/curlrc" || exit 10
printf response > "$output"
printf '\n200'
`)

	output, status, err := Curl(context.Background(), http.MethodGet, url, nil, 1, 0, "", "")
	if err != nil || status != http.StatusOK || output != "response" {
		t.Fatalf("Curl() = (%q, %d, %v), want (response, 200, nil)", output, status, err)
	}
}

func TestCurlClassifiesCommandAndOutputFailures(t *testing.T) {
	t.Run("invalid request body", func(t *testing.T) {
		requireCommand(t, "jq")

		var requestReceived atomic.Bool
		server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			requestReceived.Store(true)
		}))
		defer server.Close()

		output, code, err := Curl(context.Background(), http.MethodPost, server.URL, nil, 1, 0, "", `{`)
		if output != "" || code == 0 || !errors.Is(err, ErrCurl) {
			t.Fatalf("Curl() = (%q, %d, %v), want (empty, non-zero, ErrCurl)", output, code, err)
		}
		if requestReceived.Load() {
			t.Fatal("Curl() sent a request after jq rejected the body")
		}
	})

	t.Run("command failure", func(t *testing.T) {
		useCommandScript(t, "curl", "exit 7\n")

		output, code, err := Curl(context.Background(), "GET", "http://example.test", nil, 1, 0, "", "")
		if output != "" || code != 7 || !errors.Is(err, ErrCurl) {
			t.Fatalf("Curl() = (%q, %d, %v), want (empty, 7, ErrCurl)", output, code, err)
		}
	})

	t.Run("missing status", func(t *testing.T) {
		useCommandScript(t, "curl", curlOutputScript("printf response > \"$output\"\n"))

		output, code, err := Curl(context.Background(), "GET", "http://example.test", nil, 1, 0, "", "")
		if output != "response" || code != 0 || !errors.Is(err, ErrCurl) {
			t.Fatalf("Curl() = (%q, %d, %v), want (response, 0, ErrCurl)", output, code, err)
		}
	})

	t.Run("invalid status", func(t *testing.T) {
		useCommandScript(t, "curl", curlOutputScript("printf response > \"$output\"\nprintf '\\ninvalid'\n"))

		output, code, err := Curl(context.Background(), "GET", "http://example.test", nil, 1, 0, "", "")
		if output != "response" || code != 0 || !errors.Is(err, ErrCurl) {
			t.Fatalf("Curl() = (%q, %d, %v), want (response, 0, ErrCurl)", output, code, err)
		}
	})

	t.Run("missing response output", func(t *testing.T) {
		useCommandScript(t, "curl", "printf '\\n200'\n")

		output, code, err := Curl(context.Background(), "GET", "http://example.test", nil, 1, 0, "", "")
		if output != "" || code != 0 || !errors.Is(err, ErrCurl) {
			t.Fatalf("Curl() = (%q, %d, %v), want (empty, 0, ErrCurl)", output, code, err)
		}
	})

	t.Run("missing executable", func(t *testing.T) {
		t.Setenv("PATH", t.TempDir())

		_, code, err := Curl(context.Background(), "GET", "http://example.test", nil, 1, 0, "", "")
		if code != -1 || !errors.Is(err, ErrCommand) {
			t.Fatalf("Curl() = (_, %d, %v), want (_, -1, ErrCommand)", code, err)
		}
	})

	t.Run("canceled context", func(t *testing.T) {
		useCommandScript(t, "curl", "printf '\\n200'\n")
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, code, err := Curl(ctx, "GET", "http://example.test", nil, 1, 0, "", "")
		if code != -1 || !errors.Is(err, context.Canceled) {
			t.Fatalf("Curl() = (_, %d, %v), want (_, -1, context.Canceled)", code, err)
		}
	})

	t.Run("temporary directory failure", func(t *testing.T) {
		notDirectory := filepath.Join(t.TempDir(), "file")
		if err := os.WriteFile(notDirectory, []byte("not a directory"), 0o600); err != nil {
			t.Fatalf("write TMPDIR obstruction: %v", err)
		}
		t.Setenv("TMPDIR", notDirectory)

		output, code, err := Curl(context.Background(), "GET", "http://example.test", nil, 1, 0, "", "")
		if output != "" || code != -1 || !errors.Is(err, ErrCurl) {
			t.Fatalf("Curl() = (%q, %d, %v), want (empty, -1, ErrCurl)", output, code, err)
		}
	})
}

func TestAppendQuery(t *testing.T) {
	tests := map[string]struct {
		url   string
		query string
		want  string
	}{
		"empty":                 {"https://example.test/path", "", "https://example.test/path"},
		"new query":             {"https://example.test/path", "a=1", "https://example.test/path?a=1"},
		"existing query":        {"https://example.test/path?a=1", "b=2", "https://example.test/path?a=1&b=2"},
		"question-mark suffix":  {"https://example.test/path?", "a=1", "https://example.test/path?a=1"},
		"ampersand suffix":      {"https://example.test/path?a=1&", "b=2", "https://example.test/path?a=1&b=2"},
		"ampersand path suffix": {"https://example.test/resource&", "a=1", "https://example.test/resource&?a=1"},
		"fragment":              {"https://example.test/path#part", "a=1", "https://example.test/path?a=1#part"},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			if got := appendQuery(test.url, test.query); got != test.want {
				t.Fatalf("appendQuery(%q, %q) = %q, want %q", test.url, test.query, got, test.want)
			}
		})
	}
}

func TestJQOperations(t *testing.T) {
	requireCommand(t, "jq")
	input := `{"z":0,"nested":{"z":1,"a":2},"name":"hydra","a":3}`

	t.Run("filter", func(t *testing.T) {
		output, code, err := JQFilter(context.Background(), ".name", input)
		if err != nil || code != 0 || output != "hydra\n" {
			t.Fatalf("JQFilter() = (%q, %d, %v), want (%q, 0, nil)", output, code, err, "hydra\n")
		}
	})

	t.Run("option-like filter", func(t *testing.T) {
		output, code, err := JQFilter(context.Background(), "-length", `[1,2,3]`)
		if err != nil || code != 0 || output != "-3\n" {
			t.Fatalf("JQFilter() = (%q, %d, %v), want (%q, 0, nil)", output, code, err, "-3\n")
		}
	})

	t.Run("select", func(t *testing.T) {
		output, code, err := JQSelect(context.Background(), []string{"nested", "a"}, input)
		if err != nil || code != 0 {
			t.Fatalf("JQSelect() = (_, %d, %v), want (_, 0, nil)", code, err)
		}
		if strings.Contains(output, "\x1b[") {
			t.Fatal("JQSelect() returned presentation color in its JSON document")
		}
		if strings.Index(output, `"a"`) > strings.Index(output, `"nested"`) {
			t.Fatalf("JQSelect() output is not key sorted: %q", output)
		}
		if strings.Index(output, `"a": 2`) > strings.Index(output, `"z": 1`) {
			t.Fatalf("JQSelect() nested output is not recursively key sorted: %q", output)
		}
		if strings.Contains(output, `"name"`) || strings.Contains(output, `"z": 0`) {
			t.Fatalf("JQSelect() included an unrequested member: %q", output)
		}
	})

	t.Run("select member names requiring quotes", func(t *testing.T) {
		output, code, err := JQSelect(
			context.Background(),
			[]string{"foo-bar", "space name", "", `quote"name`},
			`{"foo-bar":1,"space name":2,"":3,"quote\"name":4,"other":5}`,
		)
		want := "{\n" +
			"  \"\": 3,\n" +
			"  \"foo-bar\": 1,\n" +
			"  \"quote\\\"name\": 4,\n" +
			"  \"space name\": 2\n" +
			"}\n"
		if err != nil || code != 0 || output != want {
			t.Fatalf("JQSelect() = (%q, %d, %v), want (%q, 0, nil)", output, code, err, want)
		}
	})

	t.Run("empty selection", func(t *testing.T) {
		output, code, err := JQSelect(context.Background(), nil, input)
		if err != nil || code != 0 || output != "{}\n" {
			t.Fatalf("JQSelect() = (%q, %d, %v), want empty object", output, code, err)
		}
	})

	t.Run("pretty", func(t *testing.T) {
		output, code, err := JQPretty(context.Background(), input)
		if err != nil || code != 0 {
			t.Fatalf("JQPretty() = (_, %d, %v), want (_, 0, nil)", code, err)
		}
		if !strings.Contains(output, "\x1b[") {
			t.Fatal("JQPretty() did not preserve jq color output")
		}
		plain := removeANSI(output)
		if strings.Index(plain, `"a"`) > strings.Index(plain, `"name"`) ||
			strings.Index(plain, `"a": 2`) > strings.Index(plain, `"z": 1`) {
			t.Fatalf("JQPretty() output is not recursively key sorted: %q", plain)
		}
	})
}

func TestJQOperationsClassifyFailures(t *testing.T) {
	t.Run("filter", func(t *testing.T) {
		useCommandScript(t, "jq", "exit 3\n")
		_, code, err := JQFilter(context.Background(), ".name", `{}`)
		if code != 3 || !errors.Is(err, ErrJQSelector) {
			t.Fatalf("JQFilter() = (_, %d, %v), want (_, 3, ErrJQSelector)", code, err)
		}
	})

	t.Run("select", func(t *testing.T) {
		useCommandScript(t, "jq", "exit 4\n")
		_, code, err := JQSelect(context.Background(), []string{"name"}, `{}`)
		if code != 4 || !errors.Is(err, ErrJQSelector) {
			t.Fatalf("JQSelect() = (_, %d, %v), want (_, 4, ErrJQSelector)", code, err)
		}
	})

	t.Run("pretty", func(t *testing.T) {
		useCommandScript(t, "jq", "exit 5\n")
		_, code, err := JQPretty(context.Background(), `{`)
		if code != 5 || !errors.Is(err, ErrJQPretty) {
			t.Fatalf("JQPretty() = (_, %d, %v), want (_, 5, ErrJQPretty)", code, err)
		}
	})
}

func TestGitDiffReturnsHeaderlessColoredDiff(t *testing.T) {
	requireCommand(t, "git")

	diff, code, err := GitDiff(context.Background(), "same\nold\n", "same\nnew\n")
	if err != nil {
		t.Fatalf("GitDiff() error = %v", err)
	}
	if code != 1 {
		t.Fatalf("GitDiff() code = %d, want 1", code)
	}
	if !strings.Contains(diff, "\x1b[") {
		t.Fatal("GitDiff() did not preserve Git color output")
	}
	plain := removeANSI(diff)
	if !strings.HasPrefix(plain, "@@") {
		t.Fatalf("GitDiff() output retained its header: %q", plain)
	}
	if !strings.Contains(plain, "-old") || !strings.Contains(plain, "+new") {
		t.Fatalf("GitDiff() output = %q, want changed lines", plain)
	}

	diff, code, err = GitDiff(context.Background(), "same\n", "same\n")
	if err != nil || code != 0 || diff != "" {
		t.Fatalf("GitDiff() for equal input = (%q, %d, %v), want (empty, 0, nil)", diff, code, err)
	}
}

func TestGitDiffUsesStableNamesForBinaryInput(t *testing.T) {
	requireCommand(t, "git")

	want := "Binary files expected and actual differ\n"
	for range 2 {
		diff, code, err := GitDiff(context.Background(), "expected\x00", "actual\x00")
		if err != nil || code != 1 || diff != want {
			t.Fatalf("GitDiff() = (%q, %d, %v), want (%q, 1, nil)", diff, code, err, want)
		}
	}
}

func TestGitDiffIgnoresInheritedBinaryAttributes(t *testing.T) {
	requireCommand(t, "git")

	directory := t.TempDir()
	attributesPath := filepath.Join(directory, "attributes")
	if err := os.WriteFile(attributesPath, []byte("* -diff\n"), 0o600); err != nil {
		t.Fatalf("write Git attributes: %v", err)
	}

	configPath := filepath.Join(directory, "gitconfig")
	command := exec.Command("git", "config", "--file", configPath, "core.attributesFile", attributesPath)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("configure Git attributes: %v: %s", err, output)
	}

	t.Setenv("GIT_CONFIG_GLOBAL", configPath)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")

	diff, code, err := GitDiff(context.Background(), "expected\n", "actual\n")
	if err != nil || code != 1 {
		t.Fatalf("GitDiff() = (_, %d, %v), want (_, 1, nil)", code, err)
	}
	plain := removeANSI(diff)
	if !strings.Contains(plain, "-expected") || !strings.Contains(plain, "+actual") {
		t.Fatalf("GitDiff() output = %q, want textual changed lines", plain)
	}
}

func TestGitDiffDisablesConfiguredTextconvDrivers(t *testing.T) {
	requireCommand(t, "git")

	directory := t.TempDir()
	attributesPath := filepath.Join(directory, "attributes")
	if err := os.WriteFile(attributesPath, []byte("* diff=apih-test\n"), 0o600); err != nil {
		t.Fatalf("write Git attributes: %v", err)
	}

	markerPath := filepath.Join(directory, "textconv-invoked")
	textconvPath := filepath.Join(directory, "textconv")
	textconv := "#!/bin/sh\nprintf invoked > \"$APIH_TEXTCONV_MARKER\"\nprintf transformed\n"
	if err := os.WriteFile(textconvPath, []byte(textconv), 0o700); err != nil {
		t.Fatalf("write textconv helper: %v", err)
	}

	configPath := filepath.Join(directory, "gitconfig")
	configureGit := func(key, value string) {
		t.Helper()
		command := exec.Command("git", "config", "--file", configPath, key, value)
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("configure Git %s: %v: %s", key, err, output)
		}
	}
	configureGit("core.attributesFile", attributesPath)
	configureGit("diff.apih-test.textconv", textconvPath)

	t.Setenv("GIT_CONFIG_GLOBAL", configPath)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("APIH_TEXTCONV_MARKER", markerPath)

	diff, code, err := GitDiff(context.Background(), "expected\n", "actual\n")
	if err != nil || code != 1 {
		t.Fatalf("GitDiff() = (_, %d, %v), want (_, 1, nil)", code, err)
	}
	plain := removeANSI(diff)
	if !strings.Contains(plain, "-expected") || !strings.Contains(plain, "+actual") {
		t.Fatalf("GitDiff() output = %q, want untransformed values", plain)
	}
	if _, err := os.Stat(markerPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("textconv helper was invoked: os.Stat() error = %v", err)
	}
}

func TestGitDiffClassifiesFailures(t *testing.T) {
	t.Run("command failure", func(t *testing.T) {
		useCommandScript(t, "git", "exit 2\n")

		output, code, err := GitDiff(context.Background(), "expected", "actual")
		if output != "" || code != 2 || !errors.Is(err, ErrGitDiff) {
			t.Fatalf("GitDiff() = (%q, %d, %v), want (empty, 2, ErrGitDiff)", output, code, err)
		}
	})

	t.Run("temporary directory failure", func(t *testing.T) {
		notDirectory := filepath.Join(t.TempDir(), "file")
		if err := os.WriteFile(notDirectory, []byte("not a directory"), 0o600); err != nil {
			t.Fatalf("write TMPDIR obstruction: %v", err)
		}
		t.Setenv("TMPDIR", notDirectory)

		output, code, err := GitDiff(context.Background(), "expected", "actual")
		if output != "" || code != -1 || !errors.Is(err, ErrGitDiff) {
			t.Fatalf("GitDiff() = (%q, %d, %v), want (empty, -1, ErrGitDiff)", output, code, err)
		}
	})
}

func TestRemoveDiffHeader(t *testing.T) {
	colored := "\x1b[1mdiff header\x1b[m\n\x1b[36m@@ -1 +1 @@\x1b[m\n-old\n+new\n"
	if got := removeDiffHeader(colored); got != "\x1b[36m@@ -1 +1 @@\x1b[m\n-old\n+new\n" {
		t.Fatalf("removeDiffHeader() = %q", got)
	}
	if got := removeDiffHeader("header\nBinary files expected and actual differ\n"); got != "Binary files expected and actual differ\n" {
		t.Fatalf("removeDiffHeader(binary) = %q", got)
	}
	if got := removeDiffHeader("header only\n"); got != "" {
		t.Fatalf("removeDiffHeader(no hunks) = %q, want empty", got)
	}
}

func TestRemoveANSILeavesIncompleteSequence(t *testing.T) {
	if got := removeANSI("plain"); got != "plain" {
		t.Fatalf("removeANSI(plain) = %q", got)
	}
	if got := removeANSI("before \x1b[31"); got != "before \x1b[31" {
		t.Fatalf("removeANSI(incomplete) = %q", got)
	}
}

func requireCommand(t *testing.T, name string) {
	t.Helper()
	if _, err := exec.LookPath(name); err != nil {
		t.Skipf("%s is required for this test: %v", name, err)
	}
}

func useCommandScript(t *testing.T, name, body string) {
	t.Helper()
	directory := t.TempDir()
	path := filepath.Join(directory, name)
	contents := "#!/bin/sh\n" + body
	if err := os.WriteFile(path, []byte(contents), 0o700); err != nil {
		t.Fatalf("write fake %s: %v", name, err)
	}
	t.Setenv("PATH", directory+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func curlOutputScript(body string) string {
	return `output=
while [ "$#" -gt 0 ]; do
	if [ "$1" = "--output" ]; then
		shift
		output=$1
	fi
	shift
done
` + body
}
