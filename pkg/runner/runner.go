package runner

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"
)

// ErrCommand classifies an external-command failure.
var ErrCommand = errors.New("command error")

// ErrCurl classifies an HTTP request command failure.
var ErrCurl = errors.New("curl error")

// ErrJQSelector classifies a jq selector failure.
var ErrJQSelector = errors.New("jq selector error")

// ErrJQPretty classifies a jq formatting failure.
var ErrJQPretty = errors.New("jq pretty error")

// ErrGitDiff classifies a Git diff failure.
var ErrGitDiff = errors.New("git diff error")

const (
	curlStatusMarker       = "http-code:"
	maxInlineCurlBodyChars = 1024
)

// Curl builds and executes a curl HTTP request and returns its response body
// and status code. It is equivalent to calling CurlBuild followed by
// CurlExecute with the returned executable and arguments unchanged and the
// original body as standard input. A non-empty cookieJar enables curl's
// automatic cookie engine; an empty cookieJar omits automatic cookie options.
func Curl(ctx context.Context, method string, url string, headers map[string]string, cookieJar string, timeout int, retries int, query string, body string) (string, int, error) {
	executable, args, err := CurlBuild(ctx, method, url, headers, cookieJar, timeout, retries, query, body)
	if err != nil {
		return "", 0, err
	}
	return CurlExecute(ctx, executable, args, body)
}

// CurlBuild constructs the curl executable and complete argument list for an
// HTTP request without executing it. It returns complete, unredacted values in
// deterministic argument order. A non-empty cookieJar adds --cookie and
// --cookie-jar with that same path; an empty cookieJar adds neither. A
// non-empty body of at most 1,024 Unicode characters is the final
// --data-binary argument value. A longer body uses @- as that final value. An
// empty body adds no --data-binary argument.
func CurlBuild(_ context.Context, method string, url string, headers map[string]string, cookieJar string, timeout int, retries int, query string, body string) (executable string, args []string, err error) {
	baseURL, fragment, hasFragment := strings.Cut(url, "#")
	if query != "" {
		separator := "?"
		hasQuery := strings.Contains(baseURL, "?")
		if hasQuery {
			separator = "&"
		}
		if strings.HasSuffix(baseURL, "?") || hasQuery && strings.HasSuffix(baseURL, "&") {
			separator = ""
		}
		baseURL += separator + query
	}
	url = baseURL
	if hasFragment {
		url += "#" + fragment
	}

	args = []string{"--disable", "--globoff", "--silent", "--show-error"}
	isHead := method == "HEAD"
	if isHead {
		args = append(args, "--head")
	} else if method != "" {
		args = append(args, "--request", method)
	}
	args = append(args, "--url", url)
	headerNames := make([]string, 0, len(headers))
	for name := range headers {
		headerNames = append(headerNames, name)
	}
	slices.Sort(headerNames)
	for _, name := range headerNames {
		args = append(args, "--header", name+": "+headers[name])
	}
	if cookieJar != "" {
		args = append(args, "--cookie", cookieJar, "--cookie-jar", cookieJar)
	}
	if timeout > 0 {
		args = append(args, "--max-time", strconv.Itoa(timeout))
	}
	if retries > 0 {
		args = append(args, "--retry", strconv.Itoa(retries))
	}
	args = append(args, "--write-out", "%{stderr}"+curlStatusMarker+"%{http_code}")
	if body != "" {
		bodyArgument := "@-"
		if utf8.RuneCountInString(body) <= maxInlineCurlBodyChars {
			bodyArgument = body
		}
		args = append(args, "--data-binary", bodyArgument)
	}
	return "curl", args, nil
}

// CurlExecute executes executable directly with args exactly as received and
// supplies requestBody as the command's standard input. It does not add,
// remove, reorder, redact, or otherwise transform arguments. It returns the
// HTTP response body and status code.
func CurlExecute(ctx context.Context, executable string, args []string, requestBody string) (responseBody string, status int, err error) {
	output, stderr, _, err := execute(ctx, executable, requestBody, args...)
	marker := strings.LastIndex(stderr, curlStatusMarker)
	if err != nil {
		if marker >= 0 {
			stderr = stderr[:marker]
		}
		return "", 0, newCommandFailure(ErrCurl, err, stderr)
	}
	if marker < 0 {
		return "", 0, newCommandFailure(ErrCurl, errors.New("missing HTTP status"), "")
	}
	status, err = strconv.Atoi(strings.TrimSpace(stderr[marker+len(curlStatusMarker):]))
	if err != nil {
		return "", 0, newCommandFailure(ErrCurl, err, "")
	}
	if slices.Contains(args, "--head") {
		return "", status, nil
	}
	return output, status, nil
}

// CurlRaw returns the complete, unredacted textual curl statement represented
// by executable and args. The result is executable followed by each argument
// in order, separated by one ASCII space, with no trailing newline. When the
// final argument is the value following --data-binary and is not @-, CurlRaw
// attempts to transform it with jq --compact-output .; invalid JSON and jq
// failures retain the original value. Values following --header and
// --data-binary are wrapped in POSIX single quotes, with each embedded single
// quote encoded by closing the quoted string, writing a backslash-escaped
// single quote, and reopening the quoted string. No other value is transformed,
// and CurlRaw does not execute the curl request.
func CurlRaw(executable string, args []string) (raw string) {
	parts := make([]string, 1, len(args)+1)
	parts[0] = executable
	quoteNext := false
	for index, arg := range args {
		if quoteNext {
			if index == len(args)-1 && args[index-1] == "--data-binary" {
				arg = compactCurlRawBody(arg)
			}
			parts = append(parts, quoteShellArgument(arg))
			quoteNext = false
			continue
		}

		parts = append(parts, arg)
		quoteNext = arg == "--header" || arg == "--data-binary"
	}
	return strings.Join(parts, " ")
}

func compactCurlRawBody(body string) string {
	if body == "@-" {
		return body
	}
	compact, _, err := jq(context.Background(), ErrJQPretty, body, "--compact-output", ".")
	if err != nil {
		return body
	}
	return compact
}

func quoteShellArgument(arg string) string {
	return "'" + strings.ReplaceAll(arg, "'", "'\\''") + "'"
}

// JQProject selects members from input and returns them as one recursively
// key-sorted, pretty JSON object.
func JQProject(ctx context.Context, selector, input string) (string, int, error) {
	return jq(ctx, ErrJQSelector, input, "--sort-keys", "--", selector)
}

// JQExtract evaluates selector against input and returns the selected JSON value
// without wrapping it in an object. Results include scalars such as 1 and
// "some text".
func JQExtract(ctx context.Context, selector string, input string) (string, int, error) {
	return jq(ctx, ErrJQSelector, input, "--compact-output", "--", selector)
}

// JQFilter evaluates filter against input and returns the filtered output.
func JQFilter(ctx context.Context, filter, input string) (string, int, error) {
	return jq(ctx, ErrJQSelector, input, "--compact-output", "--", filter)
}

// JQPretty returns input as recursively key-sorted, pretty JSON.
func JQPretty(ctx context.Context, input string) (string, int, error) {
	return jq(ctx, ErrJQPretty, input, "--sort-keys", ".")
}

// GitDiff compares expected with actual and preserves Git's original color
// output in the returned headerless diff. It creates private operation
// directories only below tempRunDir/git-diff and removes each operation
// directory before returning. tempRunDir is the CLI-owned directory for the
// current application run; GitDiff never falls back to a system temporary
// directory.
func GitDiff(ctx context.Context, tempRunDir, expected, actual string) (string, int, error) {
	operationsDir := filepath.Join(tempRunDir, "git-diff")
	if err := os.MkdirAll(operationsDir, 0o700); err != nil {
		return "", -1, newCommandFailure(ErrGitDiff, err, "")
	}
	tempDir, err := os.MkdirTemp(operationsDir, "operation-")
	if err != nil {
		return "", -1, newCommandFailure(ErrGitDiff, err, "")
	}
	defer os.RemoveAll(tempDir)

	if err := os.WriteFile(filepath.Join(tempDir, "expected"), []byte(expected), 0o600); err != nil {
		return "", -1, newCommandFailure(ErrGitDiff, err, "")
	}
	if err := os.WriteFile(filepath.Join(tempDir, "actual"), []byte(actual), 0o600); err != nil {
		return "", -1, newCommandFailure(ErrGitDiff, err, "")
	}

	output, stderr, exitCode, err := executeInDir(
		ctx,
		tempDir,
		"git",
		"",
		"-c", "color.diff.old=210",
		"-c", "color.diff.new=10",
		"-c", "color.diff.context=normal",
		"diff", "--no-index", "--color=always", "--no-color-moved", "--no-ext-diff", "--no-textconv", "--no-prefix", "--text", "--", "actual", "expected",
	)
	if err != nil && exitCode != 1 {
		return "", exitCode, newCommandFailure(ErrGitDiff, err, stderr)
	}
	if exitCode == 1 {
		return changedDiffLines(output), exitCode, nil
	}
	return "", 0, nil
}

func jq(ctx context.Context, operation error, input string, args ...string) (string, int, error) {
	output, stderr, exitCode, err := execute(ctx, "jq", input, args...)
	if err != nil {
		return "", exitCode, newCommandFailure(operation, err, stderr)
	}
	return strings.TrimRight(output, "\r\n"), 0, nil
}

func execute(ctx context.Context, name, input string, args ...string) (string, string, int, error) {
	return executeInDir(ctx, "", name, input, args...)
}

func executeInDir(ctx context.Context, dir, name, input string, args ...string) (string, string, int, error) {
	var stdout bytes.Buffer
	stderr, exitCode, err := runCommand(ctx, dir, name, input, &stdout, args...)
	return stdout.String(), stderr, exitCode, err
}

func runCommand(ctx context.Context, dir, name, input string, stdout io.Writer, args ...string) (string, int, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	cmd.Stdin = strings.NewReader(input)
	var stderr bytes.Buffer
	cmd.Stdout = stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err == nil {
		return stderr.String(), 0, nil
	}
	exitCode := -1
	if cmd.ProcessState != nil {
		exitCode = cmd.ProcessState.ExitCode()
	}
	if ctx.Err() != nil {
		err = ctx.Err()
	}
	return stderr.String(), exitCode, err
}

type commandFailure struct {
	operation error
	cause     error
	stderr    string
}

func newCommandFailure(operation, cause error, stderr string) error {
	return &commandFailure{operation: operation, cause: cause, stderr: strings.TrimSpace(stderr)}
}

func (e *commandFailure) Error() string {
	parts := []string{e.operation.Error(), ErrCommand.Error()}
	if e.stderr != "" {
		parts = append(parts, e.stderr)
	}
	if e.cause != nil {
		parts = append(parts, e.cause.Error())
	}
	return strings.Join(parts, ": ")
}

func (e *commandFailure) Unwrap() []error {
	return []error{e.operation, ErrCommand, e.cause}
}

func changedDiffLines(diff string) string {
	changed := make([]string, 0)
	inHunk := false
	for _, line := range strings.Split(diff, "\n") {
		visible := trimLeadingANSI(line)
		if strings.HasPrefix(visible, "@@") {
			inHunk = true
			continue
		}
		if inHunk && (strings.HasPrefix(visible, "-") || strings.HasPrefix(visible, "+")) {
			changed = append(changed, line)
		}
	}
	if len(changed) == 0 {
		return ""
	}
	return strings.Join(changed, "\n") + "\n"
}

func trimLeadingANSI(line string) string {
	for strings.HasPrefix(line, "\x1b[") {
		end := strings.IndexByte(line, 'm')
		if end < 0 {
			break
		}
		line = line[end+1:]
	}
	return line
}
