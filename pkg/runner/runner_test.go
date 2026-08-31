package runner

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"testing"
	"unicode/utf8"
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

	var _ func(context.Context, string, string, map[string]string, string, int, int, string, string) (string, int, error) = Curl
	var _ func(context.Context, string, string, map[string]string, string, int, int, string, string) (string, []string, error) = CurlBuild
	var _ func(context.Context, string, []string, string) (string, int, error) = CurlExecute
	var _ func(string, []string) string = CurlRaw
	var _ func(context.Context, string, string) (string, int, error) = JQProject
	var _ func(context.Context, string, string) (string, int, error) = JQExtract
	var _ func(context.Context, string, string) (string, int, error) = JQFilter
	var _ func(context.Context, string) (string, int, error) = JQPretty
	var _ func(context.Context, string, string, string) (string, int, error) = GitDiff
}

func TestCurlBuildRawAndExecutePreserveCompleteUnredactedValues(t *testing.T) {
	argsPath, stdinPath := installCommand(t, "curl", `
printf '%s\n' "$@" > "$APIH_TEST_ARGS"
/bin/cat > "$APIH_TEST_STDIN"
printf '{"complete":true}'
printf 'http-code:202' >&2
`)

	executable, args, err := CurlBuild(
		context.Background(),
		"POST",
		"https://example.test/private",
		map[string]string{
			"Authorization": "Bearer complete secret",
			"Cookie":        "session=unredacted-cookie",
		},
		"/cache/apih/run-1/cookies/request.cookie.jar",
		10,
		3,
		"include=all",
		`{"security":"visible"}`,
	)
	if err != nil {
		t.Fatalf("CurlBuild() error = %v", err)
	}
	wantRaw := CurlRaw(executable, args)
	for _, sensitive := range []string{"Authorization: Bearer complete secret", "Cookie: session=unredacted-cookie"} {
		if !strings.Contains(wantRaw, sensitive) {
			t.Fatalf("CurlRaw() = %q, want complete sensitive value %q", wantRaw, sensitive)
		}
	}
	for _, cookieOption := range []string{"--cookie /cache/apih/run-1/cookies/request.cookie.jar", "--cookie-jar /cache/apih/run-1/cookies/request.cookie.jar"} {
		if !strings.Contains(wantRaw, cookieOption) {
			t.Fatalf("CurlRaw() = %q, want %q", wantRaw, cookieOption)
		}
	}
	if !strings.Contains(wantRaw, `--data-binary '{"security":"visible"}'`) {
		t.Fatalf("CurlRaw() = %q, want complete request body", wantRaw)
	}
	if _, err := os.Stat(argsPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("CurlBuild/CurlRaw executed command; args file error = %v", err)
	}
	for _, forbidden := range []string{"--output", os.TempDir()} {
		if strings.Contains(wantRaw, forbidden) {
			t.Fatalf("CurlRaw() = %q, contains response-file artifact %q", wantRaw, forbidden)
		}
	}

	body, status, err := CurlExecute(context.Background(), executable, args, `{"security":"visible"}`)
	if err != nil {
		t.Fatalf("CurlExecute() error = %v", err)
	}
	if body != `{"complete":true}` || status != 202 {
		t.Fatalf("CurlExecute() = (%q, %d), want complete response and 202", body, status)
	}
	argsPayload, err := os.ReadFile(argsPath)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(argsPayload), strings.Join(args, "\n")+"\n"; got != want {
		t.Fatalf("CurlExecute() arguments = %q, want unchanged %q", got, want)
	}
	assertFileContents(t, stdinPath, `{"security":"visible"}`)
}

func TestCurlBuildUsesOneSelectedJarForBothCookieOptions(t *testing.T) {
	jar := "/cache/apih/run-1/cookies/selected.cookie.jar"
	_, args, err := CurlBuild(
		context.Background(),
		"GET",
		"https://example.test",
		map[string]string{"Cookie": "manual=preserved"},
		jar,
		0,
		0,
		"",
		"",
	)
	if err != nil {
		t.Fatal(err)
	}
	wantSequence := []string{"--header", "Cookie: manual=preserved", "--cookie", jar, "--cookie-jar", jar}
	if !containsArgumentSequence(args, wantSequence) {
		t.Fatalf("CurlBuild() args = %q, want sequence %q", args, wantSequence)
	}

	_, withoutCookies, err := CurlBuild(context.Background(), "GET", "https://example.test", map[string]string{"Cookie": "manual=preserved"}, "", 0, 0, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if containsArgument(withoutCookies, "--cookie") || containsArgument(withoutCookies, "--cookie-jar") || !containsArgumentSequence(withoutCookies, []string{"--header", "Cookie: manual=preserved"}) {
		t.Fatalf("CurlBuild(empty jar) args = %q, want explicit header without automatic cookie options", withoutCookies)
	}
}

func TestCurlRawCompactsBodyAndQuotesAndEscapesArguments(t *testing.T) {
	argsPath, stdinPath := installCommand(t, "jq", `
printf '%s\n' "$@" > "$APIH_TEST_ARGS"
/bin/cat > "$APIH_TEST_STDIN"
printf '%s' '{"message":"complete"}'
`)
	args := []string{
		"--header", "X-O'Brien: it's complete",
		"--url", "https://example.test/path?raw=value",
		"--data-binary", "{\n  \"message\": \"complete\"\n}",
	}

	got := CurlRaw("curl", args)
	want := `curl --header 'X-O'\''Brien: it'\''s complete' --url https://example.test/path?raw=value --data-binary '{"message":"complete"}'`
	if got != want {
		t.Fatalf("CurlRaw() = %q, want %q", got, want)
	}
	assertLines(t, argsPath, []string{"--compact-output", "."})
	assertFileContents(t, stdinPath, args[len(args)-1])
}

func TestCurlRawPreservesBodyWhenJQCannotCompactIt(t *testing.T) {
	installCommand(t, "jq", `exit 4`)

	got := CurlRaw("curl", []string{"--data-binary", "not 'json'"})
	want := `curl --data-binary 'not '\''json'\'''`
	if got != want {
		t.Fatalf("CurlRaw() = %q, want %q", got, want)
	}
}

func TestCurlPassesRequestDataAndReturnsResponse(t *testing.T) {
	argsPath, stdinPath := installCommand(t, "curl", `
printf '%s\n' "$@" > "$APIH_TEST_ARGS"
/bin/cat > "$APIH_TEST_STDIN"
printf '{"ok":true}'
printf 'http-code:201' >&2
`)

	body, status, err := Curl(
		context.Background(),
		"POST",
		"https://example.test/resource?fixed=yes#section",
		map[string]string{"Z-Last": "z", "Accept": "application/json"},
		"",
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
	assertFileContents(t, stdinPath, `{"name":"hydra"}`)
	assertCurlLines(t, argsPath, []string{
		"--disable", "--globoff", "--silent", "--show-error", "--request", "POST",
		"--url", "https://example.test/resource?fixed=yes&page=2#section",
		"--header", "Accept: application/json",
		"--header", "Z-Last: z",
		"--max-time", "12", "--retry", "3",
		"--write-out", "%{stderr}" + curlStatusMarker + "%{http_code}", "--data-binary", `{"name":"hydra"}`,
	})
}

func TestCurlBuildShowsBodiesUpTo1024CharactersInRawCommand(t *testing.T) {
	tests := map[string]struct {
		body             string
		wantBodyArgument string
	}{
		"1024 ASCII characters": {
			body:             strings.Repeat("a", 1024),
			wantBodyArgument: strings.Repeat("a", 1024),
		},
		"1024 multibyte characters": {
			body:             strings.Repeat("水", 1024),
			wantBodyArgument: strings.Repeat("水", 1024),
		},
		"more than 1024 characters": {
			body:             strings.Repeat("a", 1025),
			wantBodyArgument: "@-",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			executable, args, err := CurlBuild(
				context.Background(), "POST", "https://example.test", nil, "", 0, 0, "", test.body,
			)
			if err != nil {
				t.Fatalf("CurlBuild() error = %v", err)
			}
			if got := args[len(args)-1]; got != test.wantBodyArgument {
				t.Fatalf("CurlBuild() body argument length = %d, value prefix = %q, want length %d, prefix %q", utf8.RuneCountInString(got), got[:min(len(got), 16)], utf8.RuneCountInString(test.wantBodyArgument), test.wantBodyArgument[:min(len(test.wantBodyArgument), 16)])
			}
			raw := CurlRaw(executable, args)
			if !strings.Contains(raw, "--data-binary '"+test.wantBodyArgument+"'") {
				t.Fatalf("CurlRaw() does not contain expected body argument")
			}
		})
	}
}

func TestCurlUsesHeadTransferModeAndReturnsEmptyBody(t *testing.T) {
	argsPath, _ := installCommand(t, "curl", `
printf '%s\n' "$@" > "$APIH_TEST_ARGS"
printf 'HTTP/1.1 200 OK\r\nContent-Length: 12\r\n\r\n'
printf 'http-code:200' >&2
`)

	body, status, err := Curl(context.Background(), "HEAD", "https://example.test/resource", nil, "", 0, 0, "", "")
	if err != nil {
		t.Fatalf("Curl() error = %v", err)
	}
	if body != "" || status != 200 {
		t.Fatalf("Curl() = (%q, %d), want empty body and status 200", body, status)
	}
	assertCurlLines(t, argsPath, []string{
		"--disable", "--globoff", "--silent", "--show-error", "--head",
		"--url", "https://example.test/resource", "--write-out", "%{stderr}" + curlStatusMarker + "%{http_code}",
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
				"--url", "https://example.test/resource", "--write-out", "%{stderr}" + curlStatusMarker + "%{http_code}",
			},
		},
		"with body defaults to POST": {
			body: `{"name":"hydra"}`,
			wantArgs: []string{
				"--disable", "--globoff", "--silent", "--show-error",
				"--url", "https://example.test/resource", "--write-out", "%{stderr}" + curlStatusMarker + "%{http_code}", "--data-binary", `{"name":"hydra"}`,
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			argsPath, stdinPath := installCommand(t, "curl", `
printf '%s\n' "$@" > "$APIH_TEST_ARGS"
/bin/cat > "$APIH_TEST_STDIN"
printf '{"ok":true}'
printf 'http-code:200' >&2
`)

			body, status, err := Curl(context.Background(), "", "https://example.test/resource", nil, "", 0, 0, "", test.body)
			if err != nil {
				t.Fatalf("Curl() error = %v", err)
			}
			if body != `{"ok":true}` || status != 200 {
				t.Fatalf("Curl() = (%q, %d), want response body and status 200", body, status)
			}
			assertFileContents(t, stdinPath, test.body)
			assertCurlLines(t, argsPath, test.wantArgs)
		})
	}
}

func TestCurlAddsQueryBeforeFragment(t *testing.T) {
	argsPath, _ := installCommand(t, "curl", `
printf '%s\n' "$@" > "$APIH_TEST_ARGS"
printf 'http-code:204' >&2
`)

	_, _, err := Curl(context.Background(), "GET", "https://example.test/resource#section", nil, "", 0, 0, "page=2", "")
	if err != nil {
		t.Fatalf("Curl() error = %v", err)
	}
	assertCurlLines(t, argsPath, []string{
		"--disable", "--globoff", "--silent", "--show-error", "--request", "GET",
		"--url", "https://example.test/resource?page=2#section", "--write-out", "%{stderr}" + curlStatusMarker + "%{http_code}",
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
printf 'http-code:204' >&2
`)

			_, _, err := Curl(context.Background(), "GET", test.url, nil, "", 0, 0, "page=2", "")
			if err != nil {
				t.Fatalf("Curl() error = %v", err)
			}
			assertCurlLines(t, argsPath, []string{
				"--disable", "--globoff", "--silent", "--show-error", "--request", "GET",
				"--url", test.want, "--write-out", "%{stderr}" + curlStatusMarker + "%{http_code}",
			})
		})
	}
}

func TestCurlKeepsStatusMetadataSeparateFromResponseBody(t *testing.T) {
	installCommand(t, "curl", `
printf '{"message":"http-code:599"}'
printf 'http-code:200' >&2
`)

	body, status, err := Curl(context.Background(), "GET", "https://example.test", nil, "", 0, 1, "", "")
	if err != nil {
		t.Fatalf("Curl() error = %v", err)
	}
	if got, want := body, `{"message":"http-code:599"}`; got != want {
		t.Fatalf("Curl() body = %q, want byte-exact response body %q", got, want)
	}
	if status != 200 {
		t.Fatalf("Curl() status = %d, want 200", status)
	}
}

func TestCurlOmitsOptionalArguments(t *testing.T) {
	argsPath, stdinPath := installCommand(t, "curl", `
printf '%s\n' "$@" > "$APIH_TEST_ARGS"
/bin/cat > "$APIH_TEST_STDIN"
printf 'plain response'
printf 'http-code:204' >&2
`)

	body, status, err := Curl(context.Background(), "GET", "https://example.test", nil, "", 0, 0, "", "")
	if err != nil {
		t.Fatalf("Curl() error = %v", err)
	}
	if body != "plain response" || status != 204 {
		t.Fatalf("Curl() = (%q, %d), want (%q, 204)", body, status, "plain response")
	}
	assertFileContents(t, stdinPath, "")
	assertCurlLines(t, argsPath, []string{
		"--disable", "--globoff", "--silent", "--show-error", "--request", "GET",
		"--url", "https://example.test", "--write-out", "%{stderr}" + curlStatusMarker + "%{http_code}",
	})
}

func TestCurlRejectsMalformedCommandOutput(t *testing.T) {
	t.Run("missing status", func(t *testing.T) {
		installCommand(t, "curl", `printf 'response without status'`)
		_, status, err := Curl(context.Background(), "GET", "https://example.test", nil, "", 0, 0, "", "")
		assertCommandError(t, err, ErrCurl)
		if status != 0 {
			t.Fatalf("Curl() status = %d, want 0", status)
		}
	})

	t.Run("invalid status", func(t *testing.T) {
		installCommand(t, "curl", `printf 'response'; printf 'http-code:not-a-number' >&2`)
		_, status, err := Curl(context.Background(), "GET", "https://example.test", nil, "", 0, 0, "", "")
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
printf 'http-code:000' >&2
exit 6
`)
	body, status, err := Curl(context.Background(), "GET", "https://example.test", nil, "", 0, 0, "", "")
	assertCommandError(t, err, ErrCurl)
	if body != "" || status != 0 {
		t.Fatalf("Curl() = (%q, %d), want empty response", body, status)
	}
	if strings.Contains(err.Error(), curlStatusMarker) {
		t.Fatalf("Curl() error = %q, contains internal status metadata", err)
	}
}

func TestCurlDoesNotUseTemporaryResponseFiles(t *testing.T) {
	argsPath, _ := installCommand(t, "curl", `
printf '%s\n' "$@" > "$APIH_TEST_ARGS"
printf 'in-memory response'
printf 'http-code:200' >&2
`)
	t.Setenv("TMPDIR", filepath.Join(t.TempDir(), "missing"))

	body, status, err := Curl(context.Background(), "GET", "https://example.test", nil, "", 0, 0, "", "")
	if err != nil || body != "in-memory response" || status != 200 {
		t.Fatalf("Curl() = (%q, %d, %v), want in-memory response and 200", body, status, err)
	}
	contents, err := os.ReadFile(argsPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(contents), "--output") || strings.Contains(string(contents), os.TempDir()) {
		t.Fatalf("Curl() arguments use temporary response storage: %q", contents)
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

	diff, exitCode, err := GitDiff(context.Background(), t.TempDir(), "old\n", "new\n")
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
		diff, exitCode, err := GitDiff(context.Background(), t.TempDir(), "same", "same")
		if err != nil || diff != "" || exitCode != 0 {
			t.Fatalf("GitDiff() = (%q, %d, %v), want empty success", diff, exitCode, err)
		}
	})

	t.Run("failure", func(t *testing.T) {
		installCommand(t, "git", `
printf 'git failed' >&2
exit 3
`)
		_, exitCode, err := GitDiff(context.Background(), t.TempDir(), "old", "new")
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
		diff, exitCode, err := GitDiff(context.Background(), t.TempDir(), "old", "new")
		if err != nil || diff != "" || exitCode != 1 {
			t.Fatalf("GitDiff() = (%q, %d, %v), want empty successful diff", diff, exitCode, err)
		}
	})
}

func TestGitDiffReportsTemporaryDirectoryFailure(t *testing.T) {
	runPath := filepath.Join(t.TempDir(), "run-file")
	if err := os.WriteFile(runPath, []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, exitCode, err := GitDiff(context.Background(), runPath, "old", "new")
	assertCommandError(t, err, ErrGitDiff)
	if exitCode != -1 {
		t.Fatalf("GitDiff() exit code = %d, want -1", exitCode)
	}
}

func TestGitDiffUsesDistinctRunScopedOperationsAndCleansThem(t *testing.T) {
	installCommand(t, "git", `
pwd >> "$APIH_TEST_GIT_DIRS"
printf '%s\n' '@@ -1 +1 @@' '-actual' '+expected'
exit 1
`)
	runDir := t.TempDir()
	captured := filepath.Join(t.TempDir(), "git-dirs")
	t.Setenv("APIH_TEST_GIT_DIRS", captured)
	t.Setenv("TMPDIR", filepath.Join(t.TempDir(), "must-not-be-used"))

	const operations = 16
	var wait sync.WaitGroup
	errorsSeen := make(chan error, operations)
	for range operations {
		wait.Add(1)
		go func() {
			defer wait.Done()
			_, exitCode, err := GitDiff(context.Background(), runDir, "expected", "actual")
			if err != nil {
				errorsSeen <- err
				return
			}
			if exitCode != 1 {
				errorsSeen <- errors.New("unexpected GitDiff exit code")
			}
		}()
	}
	wait.Wait()
	close(errorsSeen)
	for err := range errorsSeen {
		t.Fatal(err)
	}

	contents, err := os.ReadFile(captured)
	if err != nil {
		t.Fatal(err)
	}
	directories := strings.Fields(string(contents))
	if len(directories) != operations {
		t.Fatalf("captured operation directories = %d, want %d: %q", len(directories), operations, contents)
	}
	unique := make(map[string]struct{}, operations)
	wantParent := filepath.Join(runDir, "git-diff")
	for _, directory := range directories {
		if filepath.Dir(directory) != wantParent {
			t.Fatalf("operation directory = %q, want child of %q", directory, wantParent)
		}
		unique[directory] = struct{}{}
		if _, err := os.Stat(directory); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("operation directory cleanup error = %v for %q", err, directory)
		}
	}
	if len(unique) != operations {
		t.Fatalf("unique operation directories = %d, want %d", len(unique), operations)
	}
	entries, err := os.ReadDir(wantParent)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("git-diff namespace retained operation artifacts: %v", entries)
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
	command := "#!/bin/sh\nset -eu\n" + body + "\n"
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
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", path, err)
	}
	got := strings.Split(strings.TrimSuffix(string(contents), "\n"), "\n")
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("lines = %#v, want %#v", got, want)
	}
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

func containsArgument(args []string, want string) bool {
	for _, arg := range args {
		if arg == want {
			return true
		}
	}
	return false
}

func containsArgumentSequence(args, want []string) bool {
	for start := 0; start+len(want) <= len(args); start++ {
		if reflect.DeepEqual(args[start:start+len(want)], want) {
			return true
		}
	}
	return false
}
