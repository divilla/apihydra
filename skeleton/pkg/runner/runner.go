package runner

import (
	"context"
	"errors"
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

// Curl builds and executes a curl HTTP request and returns its response body
// and status code. It is equivalent to calling CurlBuild followed by
// CurlExecute with the returned executable and arguments unchanged and the
// original body as standard input.
func Curl(ctx context.Context, method string, url string, headers map[string]string, timeout int, retries int, query string, body string) (string, int, error) {
	executable, args, err := CurlBuild(ctx, method, url, headers, timeout, retries, query, body)
	if err != nil {
		return "", 0, err
	}
	return CurlExecute(ctx, executable, args, body)
}

// CurlBuild constructs the curl executable and complete argument list for an
// HTTP request without executing it. It returns complete, unredacted values in
// deterministic argument order. A non-empty body of at most 1,024 Unicode
// characters is the final --data-binary argument value. A longer body uses @-
// as that final value. An empty body adds no --data-binary argument.
func CurlBuild(ctx context.Context, method string, url string, headers map[string]string, timeout int, retries int, query string, body string) (executable string, args []string, err error) {
	// TODO: implement
	return "", []string{}, nil
}

// CurlExecute executes executable directly with args exactly as received and
// supplies requestBody as the command's standard input. It does not add,
// remove, reorder, redact, or otherwise transform arguments. It returns the
// HTTP response body and status code.
func CurlExecute(ctx context.Context, executable string, args []string, requestBody string) (responseBody string, status int, err error) {
	// TODO: implement
	return "", 0, nil
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
	// TODO: implement
	return ""
}

// JQProject selects members from input and returns them as one recursively
// key-sorted, pretty JSON object.
func JQProject(ctx context.Context, selector, input string) (string, int, error) {
	// TODO: implement
	return "", 0, nil
}

// JQExtract evaluates selector against input and returns the selected JSON value
// without wrapping it in an object. Results include scalars such as 1 and
// "some text".
func JQExtract(ctx context.Context, selector string, input string) (string, int, error) {
	// TODO: implement
	return "", 0, nil
}

// JQFilter evaluates filter against input and returns the filtered output.
func JQFilter(ctx context.Context, filter, input string) (string, int, error) {
	// TODO: implement
	return "", 0, nil
}

// JQPretty returns input as recursively key-sorted, pretty JSON.
func JQPretty(ctx context.Context, input string) (string, int, error) {
	// TODO: implement
	return "", 0, nil
}

// GitDiff compares expected with actual and preserves Git's original color
// output in the returned headerless diff. It creates private operation
// directories only below tempRunDir/git-diff and removes each operation
// directory before returning. tempRunDir is the CLI-owned directory for the
// current application run; GitDiff never falls back to a system temporary
// directory.
func GitDiff(ctx context.Context, tempRunDir, expected, actual string) (string, int, error) {
	// TODO: implement
	return "", 0, nil
}
