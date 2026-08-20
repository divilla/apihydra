# `internal/execution` Validator

## Status and ownership

- Binding reference: [`skeleton/internal/execution/validator.go`](../../skeleton/internal/execution/validator.go)
- Shared step model and exit codes: [`prd.md`](../prd.md)
- Phase orchestration: [`010-step-runner-service.md`](010-step-runner-service.md)
- Status: skeleton-aligned implementation guide

## Reference contract

The binding skeleton defines Validator's API, comparison contracts, and error
classifications. This guide does not reproduce them. Separate type, status,
and body operations let StepRunner report every nonfatal mismatch through the
corresponding Reporter method. StepRunner defines invocation order and
suite-level result handling.

## Deliberately unspecified

The reference does not define:

- expected-type declaration tokens, modifiers, zero values, or optionality;
- selector syntax, ordering, stream behavior, or aggregation format;
- how `ValidateTypes` constructs its `runner.JQFilter` filter;
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
2. Type validation builds a filter from `ExpectedTypes`, evaluates it against
   `ActualBody` with `runner.JQFilter`, and returns `(failed string, error)`.
   Empty `failed` means all types validate; non-empty `failed` is a nonfatal
   mismatch; `error` means filtering failed.
3. Status validation accepts every `ActualStatus` when `ExpectedStatus` is zero
   and requires exact equality otherwise. Body validation returns an empty diff
   for equal normalized bodies and the `GitDiff` result for unequal bodies.
4. Validator does not print, schedule work, or choose the process exit code.
5. No validation language, comparison algorithm, external-tool dependency, or
   failure payload absent from the skeleton is specified here.
