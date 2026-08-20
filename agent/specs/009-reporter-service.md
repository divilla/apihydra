# `internal/reporting` Reporter

## Status and ownership

- Binding reference: `skeleton/internal/reporting/reporter.go`
- Reference tests: `skeleton/internal/reporting/reporter_test.go`
- Execution-output boundary: [`prd.md`](../prd.md#package-ownership)
- Status: skeleton-aligned specification

This specification owns Reporter construction and the output behavior fixed by
the reference implementation or method comments. It does not define execution
scheduling or validation algorithms.

## Public API

```go
var ErrReporter = errors.New("reporting error")
var ErrTypeValidation = errors.New("type validation failed for")
var ErrExpectedValidation = errors.New("response does not match expected")

func NewReporter(output io.Writer) *Reporter
func (r *Reporter) WorkingDirectory(workDir string) error
func (r *Reporter) Success(ctx context.Context, directory *domain.Directory) error
func (r *Reporter) ValidationTypes(ctx context.Context, step *domain.Step, failure error) error
func (r *Reporter) ValidationExpected(ctx context.Context, step *domain.Step, failure error) error
func (r *Reporter) Debug(ctx context.Context, step *domain.Step) error
```

`ValidationTypes` and `ValidationExpected` mirror the corresponding Validator
operations. Reporter has no fatal-error or standard-error method.

## Construction and output ownership

`NewReporter` retains the supplied `io.Writer`. The CLI constructs one Reporter
for stdout. Reporter never writes directly to stderr and never terminates the
process. Every Reporter method returns a reporting failure to its caller.
Fatal diagnostic logging belongs to `cmd/cli`.

Reporter contains a mutex for serializing access to its writer. The exact state
transitions and output layouts for the remaining stub methods are not yet
specified.

## `WorkingDirectory`

For a non-nil Reporter and writer, `WorkingDirectory` writes exactly:

```text
Working Directory: <workDir>

```

A nil Reporter/writer or failed write returns a built internal error matching
`ErrReporter`; writer errors remain available through the error chain.

## Stubbed reporting operations

The remaining reference methods currently return nil. Their comments establish
only these boundaries:

- `Success` reports a directory whose execution completed without validation
  failures; its formatting is intentionally left to the implementation.
- `ValidationTypes` writes one nonfatal response-type validation failure to the
  injected stdout writer.
- `ValidationExpected` writes one nonfatal expected-response failure to the
  injected stdout writer and preserves any command colors carried by the
  failure when rendering the output block.
- `Debug` reports the final runtime state of a selected debug step.

These methods return only reporting failures. A validation mismatch is output,
not returned as a Reporter error, and does not terminate execution.

The skeleton does not specify their exact text, icons, spacing, path rendering,
JSON payloads, calls to Runner, failure-class validation, or error behavior.
It also does not define how a debug step is selected or scheduled.

## Acceptance criteria

1. Public names, signatures, static errors, and writer injection match the
   reference.
2. Working-directory output is byte-exact and write failures preserve their
   causes.
3. Reporter has no stderr or process-termination responsibility.
4. Stubbed methods remain within their documented reporting boundaries and do
   not acquire execution or validation responsibilities.
5. ValidationExpected preserves colored diff payloads as required by its
   reference comment.
6. No unimplemented layout, Runner integration, debug scheduling, or success
   policy is asserted as a requirement.
