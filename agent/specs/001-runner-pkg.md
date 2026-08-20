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
var ErrCommand = errors.New("command error")
var ErrCurl = errors.New("curl error")
var ErrJQSelector = errors.New("jq selector error")
var ErrJQPretty = errors.New("jq pretty error")
var ErrGitDiff = errors.New("git diff error")

func Curl(ctx context.Context, method, url string, headers map[string]string,
    timeout, retries int, query, body string) (string, int, error)
func JQProject(ctx context.Context, selector, input string) (string, int, error)
func JQExtract(ctx context.Context, selector, input string) (string, int, error)
func JQPretty(ctx context.Context, input string) (string, int, error)
func GitDiff(ctx context.Context, expected, actual string) (string, int, error)
```

Every operation receives `context.Context` and returns text, an integer code,
and an error. The skeleton declares the five static classifications above; it
does not define which failure paths match each one.

## Operation boundaries

- `Curl` is the request operation with the exact request inputs represented by
  its parameters and returns the response body and HTTP status code.
- `JQProject` evaluates `selector` against `input` and returns the selected
  members as one recursively key-sorted, pretty JSON object.
- `JQExtract` evaluates `selector` against `input` and returns the selected JSON
  value without wrapping it in an object. Its result may be any JSON value,
  including a scalar such as `1` or `"some text"`.
- `JQPretty` returns `input` as recursively key-sorted, pretty JSON.
- `GitDiff` compares `expected` with `actual` and returns a headerless diff
  preserving Git's original color output.

`JQProject` and `JQExtract` are distinct operations: projection preserves a
selected object shape for document comparison, while extraction returns the
selected value for capture. `GitDiff` replaces any legacy `BatDiff`/`bat`
boundary.

## Deliberately unspecified

The reference functions are stubs except for their signatures and comments.
It therefore does not fix:

- executable names or argument vectors;
- use of stdin, temporary files, or environment variables;
- sorting beyond the `JQProject` and `JQPretty` result descriptions;
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
3. `JQProject` and `JQPretty` return comparable normalized JSON, while
   `GitDiff` preserves the color behavior required by its comment.
4. No external command is invoked by another production package.
5. No `BatDiff`, `bat` dependency, shell policy, or command-line contract is
   invented here.
