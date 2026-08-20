# `internal/reporting` Reporter

## Status and ownership

- Binding reference: [`skeleton/internal/reporting/reporter.go`](../../skeleton/internal/reporting/reporter.go)
- Reference tests: [`skeleton/internal/reporting/reporter_test.go`](../../skeleton/internal/reporting/reporter_test.go)
- Execution-output boundary: [`prd.md`](../prd.md#package-ownership)
- Shared domain types: [`000-domain-types.md`](000-domain-types.md)
- Status: skeleton-aligned implementation guide

## Reference contract

The binding skeleton implementation, comments, and tests define Reporter's API
and output contract. This guide does not reproduce them. Reporter owns
human-readable execution output through its injected writer; fatal diagnostics
and process exit remain in `cmd/cli`. The validation reporting methods mirror
Validator's separate type, status, and body operations without taking ownership
of validation itself.

## Deliberately unspecified

For the TODO methods, the skeleton does not specify exact text, icons,
spacing, path rendering, JSON payloads, calls to Runner, failure-class
validation, or error behavior. It also does not define how a debug step is
selected or scheduled.

The methods' comments do require observable output. Implementations choose and
test a layout within those boundaries; canonical zero-value TODO bodies are not
acceptable production implementations.

## Required implementation and tests

- Production output: `internal/reporting/reporter.go` retains the complete
  working-directory implementation and replaces all remaining TODO bodies with
  serialized writes to the injected writer.
- Test output: `internal/reporting/reporter_test.go` covers every method,
  byte-exact working-directory output, chosen output blocks, colored diff
  preservation, nil/failing writers, cancellation policy, and concurrent
  writes under the race detector.
- Root `architecture_test.go` proves that production execution output remains
  in this package and fatal diagnostics remain in `cmd/cli`.
- Each acceptance criterion is traced to a meaningful unit or architecture
  test, and Reporter unit-test statement coverage remains greater than 95%.

## Acceptance criteria

1. Public names, signatures, static errors, and writer injection match the
   reference.
2. Working-directory output is byte-exact and write failures preserve their
   causes.
3. Reporter has no stderr or process-termination responsibility.
4. `ValidationTypes` accepts the failed string returned by type validation;
   TODO methods remain within their documented reporting boundaries and do
   not acquire execution or validation responsibilities.
5. `ValidationBody` preserves colored diff payloads as required by the
   reference contract.
6. Chosen layouts do not become cross-package contracts, and no Runner
   integration, debug scheduling, or success policy absent from the skeleton is
   asserted as a requirement.
7. No TODO or zero-value placeholder remains in a reporting method; package
   tests, race tests, the ownership test, and `git diff --check` pass.
