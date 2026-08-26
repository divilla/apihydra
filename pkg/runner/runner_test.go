package runner

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

func TestExportedContract(t *testing.T) {
	tests := map[string]struct {
		got  error
		want string
	}{
		"command":     {got: ErrCommand, want: "command error"},
		"curl":        {got: ErrCurl, want: "curl error"},
		"jq selector": {got: ErrJQSelector, want: "jq selector error"},
		"jq pretty":   {got: ErrJQPretty, want: "jq pretty error"},
		"git diff":    {got: ErrGitDiff, want: "git diff error"},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			if got := test.got.Error(); got != test.want {
				t.Fatalf("error text = %q, want %q", got, test.want)
			}
		})
	}

	var _ func(context.Context, string, string, map[string]string, int, int, string, string) (string, int, error) = Curl
	var _ func(context.Context, string, string) (string, int, error) = JQProject
	var _ func(context.Context, string, string) (string, int, error) = JQExtract
	var _ func(context.Context, string, string) (string, int, error) = JQFilter
	var _ func(context.Context, string) (string, int, error) = JQPretty
	var _ func(context.Context, string, string) (string, int, error) = GitDiff
}

func TestCurlPassesRequestDataAndReturnsResponse(t *testing.T) {
	argsPath, stdinPath := installCommand(t, "curl", `
printf '%s\n' "$@" > "$APIH_TEST_ARGS"
/bin/cat > "$APIH_TEST_STDIN"
printf '{"ok":true}' > "$(apih_curl_output "$@")"
printf '\napih-status:201'
`)
	rawCommand := ""
	ctx := context.WithValue(context.Background(), ErrCurl, &rawCommand)

	body, status, err := Curl(
		ctx,
		"POST",
		"https://example.test/resource?fixed=yes#section",
		map[string]string{
			"Accept":        "application/json",
			"Authorization": "Bearer secret-token",
			"Cookie":        "session=secret-cookie",
			"Z-Last":        "z",
		},
		12,
		3,
		"page=2",
		`{"name":"hydra"}`,
	)
	if err != nil {
		t.Fatalf("Curl() error = %v", err)
	}
	if body != `{"ok":true}` {
		t.Fatalf("Curl() body = %q, want response JSON", body)
	}
	if status != 201 {
		t.Fatalf("Curl() status = %d, want 201", status)
	}
	assertFileContents(t, stdinPath, "")
	assertCurlLines(t, argsPath, []string{
		"--disable", "--globoff", "--silent", "--show-error", "--request", "POST",
		"--url", "https://example.test/resource?fixed=yes&page=2#section",
		"--header", "Accept: application/json",
		"--header", "Authorization: Bearer secret-token",
		"--header", "Cookie: session=secret-cookie",
		"--header", "Z-Last: z",
		"--max-time", "12", "--retry", "3",
		"--data-binary", `{"name":"hydra"}`, "--output", "<response-file>", "--write-out", curlWriteOut,
	})
	for _, value := range []string{"Bearer secret-token", "session=secret-cookie", `{"name":"hydra"}`} {
		if !strings.Contains(rawCommand, value) {
			t.Fatalf("retained Curl command = %q, want unredacted %q", rawCommand, value)
		}
	}
	executedArgs := readLines(t, argsPath)
	if got, want := rawCommand, shellCommand("curl", executedArgs...); got != want {
		t.Fatalf("retained Curl command = %q, want exact executed command %q", got, want)
	}
	for index, argument := range executedArgs {
		if argument == "--output" && index+1 < len(executedArgs) {
			responsePath := executedArgs[index+1]
			if _, err := os.Stat(responsePath); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("retry response file still exists after Curl: %v", err)
			}
			t.Cleanup(func() { os.Remove(responsePath) })
		}
	}
	if output, err := exec.Command("/bin/sh", "-c", rawCommand).CombinedOutput(); err != nil {
		t.Fatalf("copy-pasted Curl command error = %v; output = %q", err, output)
	}
}

func TestCurlUsesHeadTransferModeAndReturnsEmptyBody(t *testing.T) {
	argsPath, _ := installCommand(t, "curl", `
printf '%s\n' "$@" > "$APIH_TEST_ARGS"
printf 'HTTP/1.1 200 OK\r\nContent-Length: 12\r\n\r\napih-status:200'
`)

	body, status, err := Curl(context.Background(), "HEAD", "https://example.test/resource", nil, 0, 0, "", "")
	if err != nil {
		t.Fatalf("Curl() error = %v", err)
	}
	if body != "" || status != 200 {
		t.Fatalf("Curl() = (%q, %d), want empty body and status 200", body, status)
	}
	assertCurlLines(t, argsPath, []string{
		"--disable", "--globoff", "--silent", "--show-error", "--head",
		"--url", "https://example.test/resource", "--write-out", curlWriteOut,
	})
}

func TestCurlLetsCurlSelectMethodWhenMethodIsEmpty(t *testing.T) {
	tests := map[string]struct {
		body     string
		wantArgs []string
	}{
		"without body defaults to GET": {
			wantArgs: []string{
				"--disable", "--globoff", "--silent", "--show-error",
				"--url", "https://example.test/resource", "--write-out", curlWriteOut,
			},
		},
		"with body defaults to POST": {
			body: `{"name":"hydra"}`,
			wantArgs: []string{
				"--disable", "--globoff", "--silent", "--show-error",
				"--url", "https://example.test/resource", "--data-binary", `{"name":"hydra"}`, "--write-out", curlWriteOut,
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			argsPath, stdinPath := installCommand(t, "curl", `
printf '%s\n' "$@" > "$APIH_TEST_ARGS"
/bin/cat > "$APIH_TEST_STDIN"
printf '{"ok":true}\napih-status:200'
`)

			body, status, err := Curl(context.Background(), "", "https://example.test/resource", nil, 0, 0, "", test.body)
			if err != nil {
				t.Fatalf("Curl() error = %v", err)
			}
			if body != `{"ok":true}` || status != 200 {
				t.Fatalf("Curl() = (%q, %d), want response body and status 200", body, status)
			}
			assertFileContents(t, stdinPath, "")
			assertCurlLines(t, argsPath, test.wantArgs)
		})
	}
}

func TestCurlAddsQueryBeforeFragment(t *testing.T) {
	argsPath, _ := installCommand(t, "curl", `
printf '%s\n' "$@" > "$APIH_TEST_ARGS"
printf '\napih-status:204'
`)

	_, _, err := Curl(context.Background(), "GET", "https://example.test/resource#section", nil, 0, 0, "page=2", "")
	if err != nil {
		t.Fatalf("Curl() error = %v", err)
	}
	assertCurlLines(t, argsPath, []string{
		"--disable", "--globoff", "--silent", "--show-error", "--request", "GET",
		"--url", "https://example.test/resource?page=2#section", "--write-out", curlWriteOut,
	})
}

func TestCurlSelectsQueryDelimiter(t *testing.T) {
	tests := map[string]struct {
		url  string
		want string
	}{
		"question mark":   {url: "https://example.test/resource?", want: "https://example.test/resource?page=2"},
		"query ampersand": {url: "https://example.test/resource?fixed=yes&", want: "https://example.test/resource?fixed=yes&page=2"},
		"path ampersand":  {url: "https://example.test/resource&", want: "https://example.test/resource&?page=2"},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			argsPath, _ := installCommand(t, "curl", `
printf '%s\n' "$@" > "$APIH_TEST_ARGS"
printf '\napih-status:204'
`)

			_, _, err := Curl(context.Background(), "GET", test.url, nil, 0, 0, "page=2", "")
			if err != nil {
				t.Fatalf("Curl() error = %v", err)
			}
			assertCurlLines(t, argsPath, []string{
				"--disable", "--globoff", "--silent", "--show-error", "--request", "GET",
				"--url", test.want, "--write-out", curlWriteOut,
			})
		})
	}
}

func TestCurlKeepsOnlyFinalRetryBody(t *testing.T) {
	argsPath, _ := installCommand(t, "curl", `
printf '%s\n' "$@" > "$APIH_TEST_ARGS"
printf '{"retry":true}'
printf '{"ok":true}' > "$(apih_curl_output "$@")"
printf '\napih-status:200'
`)

	body, status, err := Curl(context.Background(), "GET", "https://example.test", nil, 0, 1, "", "")
	if err != nil {
		t.Fatalf("Curl() error = %v", err)
	}
	if got, want := body, `{"ok":true}`; got != want {
		t.Fatalf("Curl() body = %q, want final retry body %q", got, want)
	}
	if status != 200 {
		t.Fatalf("Curl() status = %d, want 200", status)
	}
	contents, err := os.ReadFile(argsPath)
	if err != nil || !strings.Contains(string(contents), "--retry\n1\n") {
		t.Fatalf("Curl() retry arguments = %q, %v, want --retry 1", contents, err)
	}
	if !strings.Contains(string(contents), "--output\n") {
		t.Fatalf("Curl() retry arguments = %q, want response output file", contents)
	}
}

func TestCurlReportsRetryResponseFileFailures(t *testing.T) {
	t.Run("create", func(t *testing.T) {
		t.Setenv("TMPDIR", filepath.Join(t.TempDir(), "missing"))

		body, status, err := Curl(context.Background(), "GET", "https://example.test", nil, 0, 1, "", "")
		assertCommandError(t, err, ErrCurl)
		if body != "" || status != 0 {
			t.Fatalf("Curl() = (%q, %d), want empty response", body, status)
		}
	})

	t.Run("read", func(t *testing.T) {
		installCommand(t, "curl", `
response_path=$(apih_curl_output "$@")
/bin/rm "$response_path"
printf '\napih-status:200'
`)

		body, status, err := Curl(context.Background(), "GET", "https://example.test", nil, 0, 1, "", "")
		assertCommandError(t, err, ErrCurl)
		if body != "" || status != 0 {
			t.Fatalf("Curl() = (%q, %d), want empty response", body, status)
		}
	})
}

func TestCurlTreatsLeadingAtBodyAsLiteralData(t *testing.T) {
	argsPath, stdinPath := installCommand(t, "curl", `
printf '%s\n' "$@" > "$APIH_TEST_ARGS"
/bin/cat > "$APIH_TEST_STDIN"
printf '{"ok":true}\napih-status:200'
`)
	wantBody := "@/etc/passwd"

	body, status, err := Curl(context.Background(), "POST", "https://example.test", nil, 0, 0, "", wantBody)
	if err != nil {
		t.Fatalf("Curl() error = %v", err)
	}
	if body != `{"ok":true}` || status != 200 {
		t.Fatalf("Curl() = (%q, %d), want literal-data response", body, status)
	}
	assertFileContents(t, stdinPath, "")
	assertCurlLines(t, argsPath, []string{
		"--disable", "--globoff", "--silent", "--show-error", "--request", "POST",
		"--url", "https://example.test", "--data-raw", wantBody, "--write-out", curlWriteOut,
	})
}

func TestCurlCommandQuotesApostrophesForPOSIXShell(t *testing.T) {
	argsPath, _ := installCommand(t, "curl", `
printf '%s\n' "$@" > "$APIH_TEST_ARGS"
printf '{"ok":true}\napih-status:200'
`)
	rawCommand := ""
	ctx := context.WithValue(context.Background(), ErrCurl, &rawCommand)

	_, _, err := Curl(
		ctx,
		"POST",
		"https://example.test/O'Brien",
		map[string]string{"X-Name": "D'Angelo"},
		0,
		0,
		"",
		`{"name":"O'Brien"}`,
	)
	if err != nil {
		t.Fatalf("Curl() error = %v", err)
	}
	if got, want := shellQuote("O'Brien"), `'O'"'"'Brien'`; got != want {
		t.Fatalf("shellQuote() = %q, want POSIX single-quote escape %q", got, want)
	}
	if output, err := exec.Command("/bin/sh", "-c", rawCommand).CombinedOutput(); err != nil {
		t.Fatalf("copy-pasted Curl command error = %v; output = %q; command = %q", err, output, rawCommand)
	}
	assertCurlLines(t, argsPath, []string{
		"--disable", "--globoff", "--silent", "--show-error", "--request", "POST",
		"--url", "https://example.test/O'Brien", "--header", "X-Name: D'Angelo",
		"--data-binary", `{"name":"O'Brien"}`, "--write-out", curlWriteOut,
	})
}

func TestCurlOmitsOptionalArguments(t *testing.T) {
	argsPath, stdinPath := installCommand(t, "curl", `
printf '%s\n' "$@" > "$APIH_TEST_ARGS"
/bin/cat > "$APIH_TEST_STDIN"
printf 'plain response\napih-status:204'
`)

	body, status, err := Curl(context.Background(), "GET", "https://example.test", nil, 0, 0, "", "")
	if err != nil {
		t.Fatalf("Curl() error = %v", err)
	}
	if body != "plain response" || status != 204 {
		t.Fatalf("Curl() = (%q, %d), want (%q, 204)", body, status, "plain response")
	}
	assertFileContents(t, stdinPath, "")
	assertCurlLines(t, argsPath, []string{
		"--disable", "--globoff", "--silent", "--show-error", "--request", "GET",
		"--url", "https://example.test", "--write-out", curlWriteOut,
	})
}

func TestCurlRejectsMalformedCommandOutput(t *testing.T) {
	t.Run("missing status", func(t *testing.T) {
		installCommand(t, "curl", `printf 'response without status'`)
		_, status, err := Curl(context.Background(), "GET", "https://example.test", nil, 0, 0, "", "")
		assertCommandError(t, err, ErrCurl)
		if status != 0 {
			t.Fatalf("Curl() status = %d, want 0", status)
		}
	})

	t.Run("invalid status", func(t *testing.T) {
		installCommand(t, "curl", `printf 'response\napih-status:not-a-number'`)
		_, status, err := Curl(context.Background(), "GET", "https://example.test", nil, 0, 0, "", "")
		assertCommandError(t, err, ErrCurl)
		if status != 0 {
			t.Fatalf("Curl() status = %d, want 0", status)
		}
	})
}

func TestJQOperationsPassFiltersAndNormalizeResults(t *testing.T) {
	tests := map[string]struct {
		call     func(context.Context) (string, int, error)
		args     []string
		input    string
		response string
		want     string
	}{
		"project": {
			call:     func(ctx context.Context) (string, int, error) { return JQProject(ctx, "{b, a}", `{"b":2,"a":1}`) },
			args:     []string{"--sort-keys", "--", "{b, a}"},
			input:    `{"b":2,"a":1}`,
			response: "{\n  \"a\": 1,\n  \"b\": 2\n}\n",
			want:     "{\n  \"a\": 1,\n  \"b\": 2\n}",
		},
		"extract": {
			call:     func(ctx context.Context) (string, int, error) { return JQExtract(ctx, ".name", `{"name":"some text"}`) },
			args:     []string{"--compact-output", "--", ".name"},
			input:    `{"name":"some text"}`,
			response: "\"some text\"\n",
			want:     `"some text"`,
		},
		"filter": {
			call: func(ctx context.Context) (string, int, error) {
				return JQFilter(ctx, ".[] | select(.ok == false)", `[{"ok":true}]`)
			},
			args:     []string{"--compact-output", "--", ".[] | select(.ok == false)"},
			input:    `[{"ok":true}]`,
			response: "\n",
			want:     "",
		},
		"pretty": {
			call:     func(ctx context.Context) (string, int, error) { return JQPretty(ctx, `{"b":2,"a":1}`) },
			args:     []string{"--sort-keys", "."},
			input:    `{"b":2,"a":1}`,
			response: "{\n  \"a\": 1,\n  \"b\": 2\n}\r\n",
			want:     "{\n  \"a\": 1,\n  \"b\": 2\n}",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			argsPath, stdinPath := installCommand(t, "jq", "printf '%s\\n' \"$@\" > \"$APIH_TEST_ARGS\"\n/bin/cat > \"$APIH_TEST_STDIN\"\nprintf '%s' '"+test.response+"'\n")
			output, exitCode, err := test.call(context.Background())
			if err != nil {
				t.Fatalf("jq operation error = %v", err)
			}
			if exitCode != 0 {
				t.Fatalf("jq operation exit code = %d, want 0", exitCode)
			}
			if output != test.want {
				t.Fatalf("jq operation output = %q, want %q", output, test.want)
			}
			assertLines(t, argsPath, test.args)
			assertFileContents(t, stdinPath, test.input)
		})
	}
}

func TestJQCallerProgramsFollowOptionTerminator(t *testing.T) {
	tests := map[string]func(context.Context) (string, int, error){
		"project": func(ctx context.Context) (string, int, error) { return JQProject(ctx, "--1", "null") },
		"extract": func(ctx context.Context) (string, int, error) { return JQExtract(ctx, "--1", "null") },
		"filter":  func(ctx context.Context) (string, int, error) { return JQFilter(ctx, "--1", "null") },
	}

	for name, call := range tests {
		t.Run(name, func(t *testing.T) {
			argsPath, _ := installCommand(t, "jq", `
printf '%s\n' "$@" > "$APIH_TEST_ARGS"
/bin/cat >/dev/null
printf '1\n'
`)
			output, exitCode, err := call(context.Background())
			if err != nil || exitCode != 0 || output != "1" {
				t.Fatalf("jq operation = (%q, %d, %v), want (1, 0, nil)", output, exitCode, err)
			}
			contents, err := os.ReadFile(argsPath)
			if err != nil {
				t.Fatalf("ReadFile(%s) error = %v", argsPath, err)
			}
			args := strings.Split(strings.TrimSpace(string(contents)), "\n")
			if got, want := args[len(args)-2:], []string{"--", "--1"}; !reflect.DeepEqual(got, want) {
				t.Fatalf("jq final arguments = %#v, want %#v", got, want)
			}
		})
	}
}

func TestCommandStartupFailureAndCancellation(t *testing.T) {
	t.Run("startup", func(t *testing.T) {
		t.Setenv("PATH", t.TempDir())
		_, exitCode, err := JQPretty(context.Background(), `{}`)
		assertCommandError(t, err, ErrJQPretty)
		if exitCode != -1 {
			t.Fatalf("JQPretty() exit code = %d, want -1", exitCode)
		}
	})

	t.Run("failure", func(t *testing.T) {
		installCommand(t, "jq", `
/bin/cat >/dev/null
printf 'invalid filter' >&2
exit 7
`)
		_, exitCode, err := JQFilter(context.Background(), "bad filter", `{}`)
		assertCommandError(t, err, ErrJQSelector)
		if exitCode != 7 {
			t.Fatalf("JQFilter() exit code = %d, want 7", exitCode)
		}
		if !strings.Contains(err.Error(), "invalid filter") {
			t.Fatalf("JQFilter() error = %q, want stderr", err)
		}
	})

	t.Run("cancellation", func(t *testing.T) {
		installCommand(t, "jq", `printf 'should not run'`)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, exitCode, err := JQExtract(ctx, ".", `{}`)
		assertCommandError(t, err, ErrJQSelector)
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("JQExtract() error = %v, want context.Canceled", err)
		}
		if exitCode != -1 {
			t.Fatalf("JQExtract() exit code = %d, want -1", exitCode)
		}
	})
}

func TestCurlCommandFailure(t *testing.T) {
	installCommand(t, "curl", `
printf 'connection failed' >&2
exit 6
`)
	body, status, err := Curl(context.Background(), "GET", "https://example.test", nil, 0, 0, "", "")
	assertCommandError(t, err, ErrCurl)
	if body != "" || status != 0 {
		t.Fatalf("Curl() = (%q, %d), want empty response", body, status)
	}
}

func TestCurlRetainsAttemptedCommandOnFailure(t *testing.T) {
	installCommand(t, "curl", `printf 'connection failed' >&2; exit 6`)
	rawCommand := ""
	ctx := context.WithValue(context.Background(), ErrCurl, &rawCommand)

	_, status, err := Curl(ctx, "POST", "https://example.test/fail", nil, 0, 0, "", "secret body")
	assertCommandError(t, err, ErrCurl)
	if status != 0 {
		t.Fatalf("Curl() status = %d, want 0", status)
	}
	if !strings.Contains(rawCommand, "https://example.test/fail") || !strings.Contains(rawCommand, "secret body") {
		t.Fatalf("retained attempted Curl command = %q, want complete request", rawCommand)
	}
}

func TestGitDiffPresentsActualToExpectedColoredChanges(t *testing.T) {
	argsPath, _ := installCommand(t, "git", `
printf '%s\n' "$@" > "$APIH_TEST_ARGS"
source=
target=
for argument do
    source=$target
    target=$argument
done
/bin/cat "$target" > "$APIH_TEST_EXPECTED"
/bin/cat "$source" > "$APIH_TEST_ACTUAL"
printf '\033[1mdiff --git actual expected\033[m\n'
printf '\033[1mindex 111..222 100644\033[m\n'
printf '\033[1m--- actual\033[m\n'
printf '\033[1m+++ expected\033[m\n'
printf '\033[36m@@ -1 +1 @@\033[m\n'
printf '\033[38;5;210m-new\033[m\n'
printf ' unchanged context\n'
printf '\033[92m+old\033[m\n'
printf '\033[36m@@ -4 +4 @@\033[m\n'
printf '\033[38;5;210m-new-again\033[m\n'
printf '\033[92m+old-again\033[m\n'
exit 1
`)
	expectedPath := filepath.Join(t.TempDir(), "expected-capture")
	actualPath := filepath.Join(t.TempDir(), "actual-capture")
	t.Setenv("APIH_TEST_EXPECTED", expectedPath)
	t.Setenv("APIH_TEST_ACTUAL", actualPath)

	diff, exitCode, err := GitDiff(context.Background(), "old\n", "new\n")
	if err != nil {
		t.Fatalf("GitDiff() error = %v", err)
	}
	if exitCode != 1 {
		t.Fatalf("GitDiff() exit code = %d, want 1", exitCode)
	}
	wantDiff := "\x1b[38;5;210m-new\x1b[m\n" +
		"\x1b[92m+old\x1b[m\n" +
		"\x1b[38;5;210m-new-again\x1b[m\n" +
		"\x1b[92m+old-again\x1b[m\n"
	if diff != wantDiff {
		t.Fatalf("GitDiff() diff = %q, want only colored changed lines %q", diff, wantDiff)
	}
	assertFileContents(t, expectedPath, "old\n")
	assertFileContents(t, actualPath, "new\n")
	assertLines(t, argsPath, []string{
		"-c", "color.diff.old=210", "-c", "color.diff.new=10", "-c", "color.diff.context=normal",
		"diff", "--no-index", "--color=always", "--no-color-moved", "--no-ext-diff", "--no-textconv", "--no-prefix", "--text", "--", "actual", "expected",
	})
}

func TestGitDiffNormalizesEqualAndFailedCommands(t *testing.T) {
	t.Run("equal", func(t *testing.T) {
		installCommand(t, "git", `exit 0`)
		diff, exitCode, err := GitDiff(context.Background(), "same", "same")
		if err != nil || diff != "" || exitCode != 0 {
			t.Fatalf("GitDiff() = (%q, %d, %v), want empty success", diff, exitCode, err)
		}
	})

	t.Run("failure", func(t *testing.T) {
		installCommand(t, "git", `
printf 'git failed' >&2
exit 3
`)
		_, exitCode, err := GitDiff(context.Background(), "old", "new")
		assertCommandError(t, err, ErrGitDiff)
		if exitCode != 3 {
			t.Fatalf("GitDiff() exit code = %d, want 3", exitCode)
		}
		if !strings.Contains(err.Error(), "git failed") {
			t.Fatalf("GitDiff() error = %q, want stderr", err)
		}
	})

	t.Run("no text hunk", func(t *testing.T) {
		installCommand(t, "git", `
printf 'binary files differ\n'
exit 1
`)
		diff, exitCode, err := GitDiff(context.Background(), "old", "new")
		if err != nil || diff != "" || exitCode != 1 {
			t.Fatalf("GitDiff() = (%q, %d, %v), want empty successful diff", diff, exitCode, err)
		}
	})
}

func TestGitDiffReportsTemporaryDirectoryFailure(t *testing.T) {
	t.Setenv("TMPDIR", filepath.Join(t.TempDir(), "missing"))
	_, exitCode, err := GitDiff(context.Background(), "old", "new")
	assertCommandError(t, err, ErrGitDiff)
	if exitCode != -1 {
		t.Fatalf("GitDiff() exit code = %d, want -1", exitCode)
	}
}

func TestProductionRunnerHasNoPlaceholderOrLegacyBoundary(t *testing.T) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller() did not return the test filename")
	}
	contents, err := os.ReadFile(filepath.Join(filepath.Dir(filename), "runner.go"))
	if err != nil {
		t.Fatalf("ReadFile(runner.go) error = %v", err)
	}
	for _, forbidden := range []string{"TODO: implement", "Bat" + "Diff", `exec.Command("bat"`} {
		if strings.Contains(string(contents), forbidden) {
			t.Fatalf("runner.go contains forbidden production text %q", forbidden)
		}
	}
}

func TestCommandFailureWithoutCause(t *testing.T) {
	err := newCommandFailure(ErrCurl, nil, "  details  ")
	if got, want := err.Error(), "curl error: command error: details"; got != want {
		t.Fatalf("command failure = %q, want %q", got, want)
	}
	assertCommandError(t, err, ErrCurl)
}

func installCommand(t *testing.T, name, body string) (string, string) {
	t.Helper()
	dir := t.TempDir()
	commandPath := filepath.Join(dir, name)
	command := `#!/bin/sh
set -eu
apih_curl_output() {
    while [ "$#" -gt 0 ]; do
        if [ "$1" = "--output" ]; then
            printf '%s' "$2"
            return 0
        fi
        shift
    done
    return 0
}
` + body + "\n"
	if err := os.WriteFile(commandPath, []byte(command), 0o700); err != nil {
		t.Fatalf("WriteFile(%s) error = %v", name, err)
	}
	argsPath := filepath.Join(t.TempDir(), "args")
	stdinPath := filepath.Join(t.TempDir(), "stdin")
	t.Setenv("PATH", dir)
	t.Setenv("APIH_TEST_ARGS", argsPath)
	t.Setenv("APIH_TEST_STDIN", stdinPath)
	return argsPath, stdinPath
}

func assertCurlLines(t *testing.T, path string, want []string) {
	t.Helper()
	got := readLines(t, path)
	for index, argument := range got {
		if argument == "--output" && index+1 < len(got) {
			got[index+1] = "<response-file>"
		}
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("lines = %#v, want %#v", got, want)
	}
}

func readLines(t *testing.T, path string) []string {
	t.Helper()
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", path, err)
	}
	return strings.Split(strings.TrimSuffix(string(contents), "\n"), "\n")
}

func assertCommandError(t *testing.T, err, operation error) {
	t.Helper()
	if !errors.Is(err, operation) {
		t.Fatalf("error = %v, want %v", err, operation)
	}
	if !errors.Is(err, ErrCommand) {
		t.Fatalf("error = %v, want ErrCommand", err)
	}
}

func assertFileContents(t *testing.T, path, want string) {
	t.Helper()
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", path, err)
	}
	if got := string(contents); got != want {
		t.Fatalf("file contents = %q, want %q", got, want)
	}
}

func assertLines(t *testing.T, path string, want []string) {
	t.Helper()
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", path, err)
	}
	got := strings.Split(strings.TrimSuffix(string(contents), "\n"), "\n")
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("lines = %#v, want %#v", got, want)
	}
}
