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

const curlStatusMarker = "\n\x1eapih-status:"

// Curl executes an HTTP request and returns its response body and status code.
func Curl(ctx context.Context, method string, url string, headers map[string]string, timeout int, retries int, query string, body string) (string, int, error) {
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

	args := []string{"--disable", "--globoff", "--silent", "--show-error"}
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
	if timeout > 0 {
		args = append(args, "--max-time", strconv.Itoa(timeout))
	}
	if retries > 0 {
		args = append(args, "--retry", strconv.Itoa(retries))
	}
	if body != "" {
		args = append(args, "--data-binary", "@-")
	}
	responseDir, err := os.MkdirTemp("", "apih-curl-response-")
	if err != nil {
		return "", 0, newCommandFailure(ErrCurl, err, "")
	}
	defer os.RemoveAll(responseDir)
	responsePath := filepath.Join(responseDir, "response")
	args = append(args, "--output", responsePath, "--write-out", curlStatusMarker+"%{http_code}")

	output, stderr, _, err := execute(ctx, "curl", body, args...)
	if err != nil {
		return "", 0, newCommandFailure(ErrCurl, err, stderr)
	}
	marker := strings.LastIndex(output, curlStatusMarker)
	if marker < 0 {
		return "", 0, newCommandFailure(ErrCurl, errors.New("missing HTTP status"), "")
	}
	status, err := strconv.Atoi(strings.TrimSpace(output[marker+len(curlStatusMarker):]))
	if err != nil {
		return "", 0, newCommandFailure(ErrCurl, err, "")
	}
	if isHead {
		return "", status, nil
	}
	response, err := os.ReadFile(responsePath)
	if err != nil {
		return "", 0, newCommandFailure(ErrCurl, err, "")
	}
	return string(response), status, nil
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
// output in the returned headerless diff.
func GitDiff(ctx context.Context, expected, actual string) (string, int, error) {
	tempDir, err := os.MkdirTemp("", "apih-git-diff-")
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
