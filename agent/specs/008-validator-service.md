# `internal/execution` Validator

## Status and ownership

- Binding reference: `skeleton/internal/execution/validator.go`
- Shared step model and exit codes: [`prd.md`](../prd.md)
- Phase orchestration: [`10-step-runner.md`](010-step-runner-service.md)
- Status: skeleton-aligned specification

This specification owns the Validator API. StepRunner owns invocation order
and suite-level treatment of detected validation failures.

## Public API

```go
var ErrValidation = errors.New("validation error")
var ErrValidatorFatal = errors.New("fatal validator error")

type Validator struct{}

func (v *Validator) ValidateTypes(ctx context.Context, step *domain.Step) []error
func (v *Validator) ValidateStatus(ctx context.Context, step *domain.Step) error
func (v *Validator) ValidateBody(ctx context.Context, step *domain.Step) (string, error)
```

The zero-value Validator has no retained state in the reference. All three
skeleton method bodies panic with an explicit not-implemented message; those
placeholders define no runtime result.

## Validation boundaries

`ValidateTypes` validates `Step.Response.ActualBody` against
`Step.Response.ExpectedTypes`. Its slice result permits reporting more than one
error.

`ValidateStatus` validates `ActualStatus` against `ExpectedStatus`.
`ExpectedStatus == 0` accepts every `ActualStatus`; otherwise `ActualStatus`
must equal `ExpectedStatus`.

`ValidateBody` validates `ActualBody` against `ExpectedBody`. It projects
`ActualBody` with `runner.JQProject`, formats `ExpectedBody` with
`runner.JQPretty`, and compares the normalized documents with `runner.GitDiff`.
Equal bodies return `"", nil`; unequal bodies return the calculated diff string.

`ErrValidation` classifies one or more nonfatal mismatches found by
`ValidateTypes` or `ValidateStatus`. A non-empty `ValidateBody` diff is also a
nonfatal validation mismatch. StepRunner reports those mismatches and converts
them to validation exit status rather than returning them as its final error.
`ErrValidatorFatal` exists as a separate static classification, but the
skeleton does not define which conditions must use it.

## Deliberately unspecified

The reference does not define:

- expected-type declaration tokens, modifiers, zero values, or optionality;
- selector syntax, ordering, stream behavior, or aggregation format;
- how `ValidateBody` constructs the `runner.JQProject` selector;
- interpretation of malformed or non-object expected responses;
- the relationship between validator errors and Reporter-owned static errors;
- status rules beyond the `ExpectedStatus == 0` wildcard and exact non-zero
  comparison;
- malformed-input, command-failure, or cancellation classification.

Those behaviors must not appear as requirements or required test matrices
until they are added to the protected skeleton.

## Acceptance criteria

1. Exported names, signatures, static error text, and explicit not-implemented
   placeholders match the reference.
2. Type validation compares `ActualBody` with `ExpectedTypes`. Status validation
   accepts every `ActualStatus` when `ExpectedStatus` is zero and requires exact
   equality otherwise. Body validation returns an empty diff for equal
   normalized bodies and the `GitDiff` result for unequal bodies.
3. Validator does not print, schedule work, or choose the process exit code.
4. No validation language, comparison algorithm, external-tool dependency, or
   failure payload absent from the skeleton is specified here.
