# `internal/execution` Validator

## Status and ownership

- Binding reference: [`skeleton/internal/execution/validator.go`](../../skeleton/internal/execution/validator.go)
- Reference tests: [`skeleton/internal/execution/validator_test.go`](../../skeleton/internal/execution/validator_test.go)
- Shared step model and exit codes: [`prd.md`](../prd.md)
- Shared domain types: [`000-domain-types.md`](000-domain-types.md)
- Phase orchestration: [`010-executor-service.md`](010-executor-service.md)
- Status: skeleton-aligned implementation guide

## Reference contract

The binding skeleton defines Validator's API, comparison contracts, and error
classifications. This guide does not reproduce them. Separate type, status,
and body operations let Executor report every nonfatal mismatch through the
corresponding Reporter method. Executor defines invocation order and
suite-level result handling.

## Deliberately unspecified

The reference does not define:

- expected-type declaration tokens beyond the current implementation choices,
  modifiers, nonnumeric zero values, or optionality;
- selector syntax, ordering, stream behavior, or aggregation format;
- how `ValidateTypes` constructs its `runner.JQFilter` filter;
- interpretation of malformed or non-object expected responses;
- the relationship between validator errors and Reporter-owned static errors;
- status rules beyond the `ExpectedStatus == 0` wildcard and exact non-zero
  comparison;
- malformed-input, command-failure, or cancellation classification.

Those behaviors must not appear as requirements or required test matrices
until they are added to the protected skeleton.

## Current type-declaration behavior

Expected type declarations are alternatives: a selected response value is
valid when any listed declaration matches. Native jq type names such as
`number`, `string`, `boolean`, `array`, `object`, and `null` match jq's reported
type. The `int` declaration matches any integer-valued JSON number, including
zero. The `zero` declaration matches numeric `0`. Consequently,
`.version: [int, zero]` accepts an actual numeric version of `0` and is omitted
from failed type output.

## Body projection contract

`ValidateBody` passes `ExpectedBody` through `runner.JQPretty` without changing
its shape. It filters `ActualBody` recursively to fields declared by the
expected body, but only when those fields are present in the actual response.
Projection must never synthesize a missing field as `null`. Consequently, an
expected `"aaa": null` fails validation when the actual response omits `aaa`,
while an actual `"aaa": null` satisfies that field. On inequality, Validator
passes the unchanged normalized expected body and filtered normalized actual
body to `runner.GitDiff`. Runner presents that mismatch as the correction from
actual to expected: red values come from actual and green values come from
expected.

## Required implementation and tests

- Production output: `internal/execution/validator.go` replaces all canonical
  zero-value TODO bodies with the three binding validation operations.
- Test output: `internal/execution/validator_test.go` covers the status wildcard
  and equality/mismatch, empty/non-empty type results, normalized body
  equality/diff, runner failures, context cancellation, and mutation/output
  boundaries using the implementation choices permitted above.
- Each acceptance criterion is traced to at least one meaningful unit test, and
  Validator unit-test statement coverage remains greater than 95%.

## Acceptance criteria

1. Exported names, signatures, and static error text match the reference, and
   production methods do not retain zero-value TODO bodies.
2. Type validation builds a filter from `ExpectedTypes`, evaluates it against
   `ActualBody` with `runner.JQFilter`, and returns `(failed string, error)`.
   Empty `failed` means all types validate; non-empty `failed` is a nonfatal
   mismatch; `error` means filtering failed. Current filter behavior treats
   declarations as alternatives, supports native jq types, matches `int` to
   integer-valued numbers, and matches `zero` to numeric `0`.
3. Status validation accepts every `ActualStatus` when `ExpectedStatus` is zero
   and requires exact equality otherwise. Body validation prettifies the
   expected body unchanged, projects only present actual fields selected by the
   expected shape, does not equate a missing field with an expected null, returns
   an empty diff for equal normalized bodies, and returns the `GitDiff` result
   for unequal bodies without swapping the normalized expected and actual
   arguments.
4. Validator does not print, schedule work, or choose the process exit code.
5. No validation language, comparison algorithm, external-tool dependency, or
   failure payload absent from the skeleton is specified here.
6. Package tests and `git diff --check` pass.
