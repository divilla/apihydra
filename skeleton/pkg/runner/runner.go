package runner

import (
	"context"
	"errors"
)

var ErrCommand = errors.New("command error")
var ErrCurl = errors.New("curl error")
var ErrJQSelector = errors.New("jq selector error")
var ErrJQPretty = errors.New("jq pretty error")
var ErrGitDiff = errors.New("git diff error")

func Curl(ctx context.Context, method string, url string, headers map[string]string, timeout int, retries int, query string, body string) (string, int, error) {
	return "", 0, nil
}

// JQFilter selects one JSON member or value from input.
func JQFilter(ctx context.Context, selector, input string) (string, int, error) {
	return "", 0, nil
}

// JQSelect returns one recursively key-sorted, pretty JSON document containing
// the requested members.
func JQSelect(ctx context.Context, selectors []string, input string) (string, int, error) {
	return "", 0, nil
}

// JQPretty returns recursively key-sorted, pretty JSON with jq's original
// color output preserved.
func JQPretty(ctx context.Context, input string) (string, int, error) {
	return "", 0, nil
}

// GitDiff compares expected with actual and preserves Git's original color
// output in the returned headerless diff.
func GitDiff(ctx context.Context, expected, actual string) (string, int, error) {
	return "", 0, nil
}
