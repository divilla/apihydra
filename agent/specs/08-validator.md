# `internal/execution` Validator

## Status and ownership

- Binding reference: `skeleton/internal/execution/validator.go`
- Shared step model and exit codes: [`prd.md`](../prd.md)
- Phase orchestration: [`09-step-runner.md`](10-step-runner.md)
- Status: skeleton-aligned specification

This specification owns the Validator API. StepRunner owns invocation order
and suite-level treatment of detected validation failures.

## Public API

```go
var ErrValidation = errors.New("validation error")
var ErrValidatorFatal = errors.New("fatal validator error")

type Validator struct{}

func (v *Validator) ValidateTypes(ctx context.Context, step *domain.Step) []error
func (v *Validator) ValidateExpected(ctx context.Context, step *domain.Step) error
```

The zero-value Validator has no retained state in the reference.

## Validation boundaries

`ValidateTypes` validates the runtime step's response body against
`Step.Response.Types`. Its slice result permits reporting more than one error.

`ValidateExpected` validates the runtime step's response body against
`Step.Response.Expected` and returns at most one error.

`ErrValidation` classifies one or more mismatches found by either validation
operation. `ErrValidatorFatal` exists as a separate static classification, but
the skeleton does not define which conditions must use it.

## Deliberately unspecified

The reference does not define:

- response-type declaration tokens, modifiers, zero values, or optionality;
- selector syntax, ordering, stream behavior, or aggregation format;
- expected-response projection, normalization, or comparison;
- use of `JQFilter`, `JQSelect`, or `GitDiff` by Validator;
- the relationship between validator errors and Reporter-owned static errors;
- HTTP-status validation;
- malformed-input, command-failure, or cancellation classification.

Those behaviors must not appear as requirements or required test matrices
until they are added to the protected skeleton.

## Acceptance criteria

1. Exported names, signatures, and static error text match the reference.
2. Type and expected validation consume only the corresponding shared Step
   response fields.
3. Validator does not print, schedule work, or choose the process exit code.
4. No validation language, comparison algorithm, external-tool dependency, or
   failure payload absent from the skeleton is specified here.
