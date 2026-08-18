# `internal/reporter` Reporter

## Status and ownership

- Binding reference: `skeleton/internal/reporter/reporter.go`
- Reference tests: `skeleton/internal/reporter/reporter_test.go`
- Terminal-output boundary: [`prd.md`](../prd.md#package-ownership)
- Status: skeleton-aligned specification

This specification owns Reporter construction and the output behavior fixed by
the reference implementation or method comments. It does not define execution
scheduling or validation algorithms.

## Public API

```go
var ReporterError = errors.New("reporter error")
var TypeValidationError = errors.New("type validation failed for")
var ExpectedValidationError = errors.New("response does not match expected")

func NewReporter(output io.Writer) *Reporter
func (r *Reporter) WorkingDirectory(workDir string) error
func (r *Reporter) Error(failure error) error
func (r *Reporter) Success(ctx context.Context, directory *domain.Directory) error
func (r *Reporter) FailureTypes(ctx context.Context, step *domain.Step, failure error) error
func (r *Reporter) FailureDiff(ctx context.Context, step *domain.Step, failure error) error
func (r *Reporter) Debug(ctx context.Context, step *domain.Step) error
```

`FailureDiff` is the exact reference name; there is no `FailureExpected`
method.

## Construction and output ownership

`NewReporter` retains the supplied `io.Writer`. The CLI normally constructs
one Reporter for stdout and another for stderr. The repository-wide rule that
human-readable terminal writes remain in this package is owned by the PRD and
enforced by the architecture test.

Reporter contains a mutex and execution-output state. The two implemented
write methods serialize access to their writer. The exact state transitions
for the remaining stub methods are not yet specified.

## `WorkingDirectory`

For a non-nil Reporter and writer, `WorkingDirectory` writes exactly:

```text
Working Directory: <workDir>

```

A nil Reporter/writer or failed write returns a built internal error matching
`ReporterError`; writer errors remain available through the error chain.

## `Error`

`Error` writes one fatal diagnostic. It removes ANSI SGR sequences matched by
the reference expression, wraps the complete remaining message in red, and
adds one newline:

```text
ESC[31m<diagnostic>ESC[0m\n
```

A nil Reporter/writer, nil failure, or failed write returns a built internal
error matching `ReporterError`. This method does not choose or change the
process exit code.

## Stubbed reporting operations

The remaining reference methods currently return nil. Their comments establish
only these boundaries:

- `Success` reports a directory whose execution completed without validation
  failures; its formatting is intentionally left to the implementation.
- `FailureTypes` reports one nonfatal response-type validation failure.
- `FailureDiff` reports one nonfatal expected-response diff and preserves any
  command colors carried by the failure when rendering the output block.
- `Debug` reports the final runtime state of a selected debug step.

The skeleton does not specify their exact text, icons, spacing, path rendering,
JSON payloads, calls to Runner, failure-class validation, or error behavior.
It also does not define how a debug step is selected or scheduled.

## Acceptance criteria

1. Public names, signatures, static errors, and writer injection match the
   reference.
2. Working-directory output is byte-exact and write failures preserve their
   causes.
3. Fatal diagnostics remove embedded SGR styling and use one red wrapper.
4. Stubbed methods remain within their documented reporting boundaries and do
   not acquire execution or validation responsibilities.
5. FailureDiff preserves colored diff payloads as required by its reference
   comment.
6. No unimplemented layout, Runner integration, debug scheduling, or success
   policy is asserted as a requirement.
