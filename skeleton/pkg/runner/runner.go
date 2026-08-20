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

// Curl executes an HTTP request and returns its response body and status code.
func Curl(ctx context.Context, method string, url string, headers map[string]string, timeout int, retries int, query string, body string) (string, int, error) {
	return "", 0, nil
}

// JQProject selects members from input and returns them as one recursively
// key-sorted, pretty JSON object.
func JQProject(ctx context.Context, selector, input string) (string, int, error) {
	return "", 0, nil
}

// JQExtract evaluates selector against input and returns the selected JSON value
// without wrapping it in an object. Results include scalars such as 1 and
// "some text".
func JQExtract(ctx context.Context, selector string, input string) (string, int, error) {
	return "", 0, nil
}

// JQPretty returns input as recursively key-sorted, pretty JSON.
func JQPretty(ctx context.Context, input string) (string, int, error) {
	return "", 0, nil
}

// GitDiff compares expected with actual and preserves Git's original color
// output in the returned headerless diff.
func GitDiff(ctx context.Context, expected, actual string) (string, int, error) {
	return "", 0, nil
}
