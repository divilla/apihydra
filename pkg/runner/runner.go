package runner

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// CommandError reports that an external command could not be started.
//
//lint:ignore ST1012 Name fixed by the binding skeleton API.
var CommandError = errors.New("command error")

// CurlError reports an unsuccessful curl operation.
//
//lint:ignore ST1012 Name fixed by the binding skeleton API.
var CurlError = errors.New("curl error")

// JQSelectorError reports an unsuccessful jq selection operation.
//
//lint:ignore ST1012 Name fixed by the binding skeleton API.
var JQSelectorError = errors.New("jq selector error")

// JQPrettyError reports an unsuccessful jq formatting operation.
//
//lint:ignore ST1012 Name fixed by the binding skeleton API.
var JQPrettyError = errors.New("jq pretty error")

// GitDiffError reports an unsuccessful Git diff operation.
//
//lint:ignore ST1012 Name fixed by the binding skeleton API.
var GitDiffError = errors.New("git diff error")

// Curl executes an HTTP request and returns its response body and status code.
func Curl(
	ctx context.Context,
	method string,
	url string,
	headers map[string]string,
	timeout int,
	retries int,
	query string,
	body string,
) (string, int, error) {
	tempDir, err := os.MkdirTemp("", "apih-curl-")
	if err != nil {
		return "", -1, CurlError
	}
	defer os.RemoveAll(tempDir)
	responsePath := filepath.Join(tempDir, "response")

	args := []string{
		"--disable",
		"--globoff",
		"--silent",
		"--show-error",
		"--request", method,
		"--max-time", strconv.Itoa(timeout),
		"--retry", strconv.Itoa(retries),
		"--output", responsePath,
		"--write-out", "\n%{http_code}",
	}
	headRequest := method == "HEAD"
	if headRequest {
		args = append(args, "--head")
	}

	headerNames := make([]string, 0, len(headers))
	for name := range headers {
		headerNames = append(headerNames, name)
	}
	sort.Strings(headerNames)
	for _, name := range headerNames {
		args = append(args, "--header", name+": "+headers[name])
	}
	input := ""
	if body != "" {
		compactBody, code, err := runCommand(
			ctx,
			"jq",
			[]string{"--compact-output", "--monochrome-output", "."},
			body,
			CurlError,
		)
		if err != nil {
			return "", code, err
		}
		args = append(args, "--data-binary", "@-")
		input = strings.TrimSuffix(compactBody, "\n")
	}
	args = append(args, "--", appendQuery(url, query))

	statusOutput, code, err := runCommand(ctx, "curl", args, input, CurlError)
	if err != nil {
		if headRequest {
			return "", code, err
		}
		response, readErr := os.ReadFile(responsePath)
		if readErr != nil {
			return "", code, err
		}
		return string(response), code, err
	}

	response, err := os.ReadFile(responsePath)
	if err != nil {
		return "", code, CurlError
	}

	separator := strings.LastIndexByte(statusOutput, '\n')
	if separator < 0 {
		return string(response), code, CurlError
	}
	status, err := strconv.Atoi(statusOutput[separator+1:])
	if err != nil {
		return string(response), code, CurlError
	}
	if headRequest {
		return "", status, nil
	}
	return string(response), status, nil
}

// JQFilter selects one JSON member or value from input.
func JQFilter(ctx context.Context, selector, input string) (string, int, error) {
	return runCommand(
		ctx,
		"jq",
		[]string{"--raw-output", "--", selector},
		input,
		JQSelectorError,
	)
}

// JQSelect returns one recursively key-sorted, pretty JSON document containing
// the requested members.
func JQSelect(ctx context.Context, selectors []string, input string) (string, int, error) {
	members := make([]string, len(selectors))
	for index, selector := range selectors {
		encodedSelector, err := json.Marshal(selector)
		if err != nil {
			return "", -1, JQSelectorError
		}
		quotedSelector := string(encodedSelector)
		members[index] = quotedSelector + ":.[" + quotedSelector + "]"
	}
	program := "{" + strings.Join(members, ",") + "}"
	return runCommand(
		ctx,
		"jq",
		[]string{"--sort-keys", "--", program},
		input,
		JQSelectorError,
	)
}

// JQPretty returns recursively key-sorted, pretty JSON with jq's original
// color output preserved.
func JQPretty(ctx context.Context, input string) (string, int, error) {
	return runCommand(
		ctx,
		"jq",
		[]string{"--sort-keys", "--color-output", "."},
		input,
		JQPrettyError,
	)
}

// GitDiff compares expected with actual and preserves Git's original color
// output in the returned headerless diff.
func GitDiff(ctx context.Context, expected, actual string) (string, int, error) {
	tempDir, err := os.MkdirTemp("", "apih-git-diff-")
	if err != nil {
		return "", -1, GitDiffError
	}
	defer os.RemoveAll(tempDir)

	expectedPath := filepath.Join(tempDir, "expected")
	actualPath := filepath.Join(tempDir, "actual")
	if err := os.WriteFile(expectedPath, []byte(expected), 0o600); err != nil {
		return "", -1, GitDiffError
	}
	if err := os.WriteFile(actualPath, []byte(actual), 0o600); err != nil {
		return "", -1, GitDiffError
	}

	output, code, err := runCommandWithEnv(
		ctx,
		"git",
		[]string{
			"-c",
			"core.attributesFile=" + os.DevNull,
			"-C",
			tempDir,
			"diff",
			"--no-index",
			"--no-ext-diff",
			"--no-textconv",
			"--no-prefix",
			"--color=always",
			"--",
			"expected",
			"actual",
		},
		"",
		GitDiffError,
		[]string{"GIT_ATTR_NOSYSTEM=1"},
		1,
	)
	if err != nil {
		return output, code, err
	}
	return removeDiffHeader(output), code, nil
}

func appendQuery(rawURL, query string) string {
	if query == "" {
		return rawURL
	}

	base, fragment, found := strings.Cut(rawURL, "#")
	separator := "?"
	if strings.Contains(base, "?") {
		separator = "&"
		if strings.HasSuffix(base, "?") || strings.HasSuffix(base, "&") {
			separator = ""
		}
	}
	base += separator + query
	if found {
		return base + "#" + fragment
	}
	return base
}

func runCommand(
	ctx context.Context,
	name string,
	args []string,
	input string,
	operationError error,
	acceptedExitCodes ...int,
) (string, int, error) {
	return runCommandWithEnv(
		ctx,
		name,
		args,
		input,
		operationError,
		nil,
		acceptedExitCodes...,
	)
}

func runCommandWithEnv(
	ctx context.Context,
	name string,
	args []string,
	input string,
	operationError error,
	environment []string,
	acceptedExitCodes ...int,
) (string, int, error) {
	var stdout bytes.Buffer
	cmd := exec.CommandContext(ctx, name, args...)
	if len(environment) > 0 {
		cmd.Env = append(os.Environ(), environment...)
	}
	cmd.Stdin = strings.NewReader(input)
	cmd.Stdout = &stdout

	err := cmd.Run()
	if err == nil {
		return stdout.String(), 0, nil
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return stdout.String(), -1, ctxErr
	}

	var exitError *exec.ExitError
	if !errors.As(err, &exitError) {
		return stdout.String(), -1, CommandError
	}
	exitCode := exitError.ExitCode()
	for _, accepted := range acceptedExitCodes {
		if exitCode == accepted {
			return stdout.String(), exitCode, nil
		}
	}
	return stdout.String(), exitCode, operationError
}

func removeDiffHeader(output string) string {
	lines := strings.SplitAfter(output, "\n")
	for index, line := range lines {
		plain := removeANSI(line)
		if strings.HasPrefix(plain, "@@") || strings.HasPrefix(plain, "Binary files ") {
			return strings.Join(lines[index:], "")
		}
	}
	return ""
}

func removeANSI(value string) string {
	for {
		start := strings.Index(value, "\x1b[")
		if start < 0 {
			return value
		}
		end := strings.IndexByte(value[start+2:], 'm')
		if end < 0 {
			return value
		}
		value = value[:start] + value[start+2+end+1:]
	}
}
