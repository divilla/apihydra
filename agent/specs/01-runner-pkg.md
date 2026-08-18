# `pkg/runner`

## Status and ownership

- Binding reference: `skeleton/pkg/runner/runner.go`
- Package-boundary test: `skeleton/architecture_test.go`
- Shared product contract: [`prd.md`](../prd.md)
- Status: skeleton-aligned specification

This specification owns Runner's exported command-operation API. The PRD owns
the repository-wide rule that external command execution remains in this
package.

## Public API

```go
var CommandError = errors.New("command error")
var CurlError = errors.New("curl error")
var JQSelectorError = errors.New("jq selector error")
var JQPrettyError = errors.New("jq pretty error")
var GitDiffError = errors.New("git diff error")

func Curl(ctx context.Context, method, url string, headers map[string]string,
    timeout, retries int, query, body string) (string, int, error)
func JQFilter(ctx context.Context, selector, input string) (string, int, error)
func JQSelect(ctx context.Context, selectors []string, input string) (string, int, error)
func JQPretty(ctx context.Context, input string) (string, int, error)
func GitDiff(ctx context.Context, expected, actual string) (string, int, error)
```

Every operation receives `context.Context` and returns text, an integer code,
and an error. The skeleton declares the five static classifications above; it
does not define which failure paths match each one.

## Operation boundaries

- `Curl` is the request operation with the exact request inputs represented by
  its parameters.
- `JQFilter` selects one JSON member or value from `input`.
- `JQSelect` returns one recursively key-sorted, pretty JSON document
  containing the requested members.
- `JQPretty` returns recursively key-sorted, pretty JSON with jq's original
  color output preserved.
- `GitDiff` compares `expected` with `actual` and returns a headerless diff
  preserving Git's original color output.

`JQFilter` and `JQSelect` are distinct APIs. `GitDiff` replaces any legacy
`BatDiff`/`bat` boundary.

## Deliberately unspecified

The reference functions are stubs except for their signatures and comments.
It therefore does not fix:

- executable names or argument vectors;
- use of stdin, temporary files, or environment variables;
- sorting beyond the `JQSelect` and `JQPretty` result descriptions;
- treatment of empty inputs, jq streams, or semantic non-zero statuses;
- stdout/stderr inclusion in errors;
- startup, cancellation, and operational exit-code normalization;
- header/query/body encoding or curl behavior.

Those details may be chosen during implementation but cannot be asserted as
product requirements until represented by the skeleton.

## Acceptance criteria

1. Exported names, signatures, and static error text match the reference.
2. Each function stays within the operation boundary stated by its reference
   name or comment.
3. JQ and Git presentation output preserves the color behavior explicitly
   required by the comments.
4. No external command is invoked by another production package.
5. No `BatDiff`, `bat` dependency, shell policy, or command-line contract is
   invented here.
