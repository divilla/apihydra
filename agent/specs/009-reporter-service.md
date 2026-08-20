# `internal/reporting` Reporter

## Status and ownership

- Binding reference: [`skeleton/internal/reporting/reporter.go`](../../skeleton/internal/reporting/reporter.go)
- Reference tests: [`skeleton/internal/reporting/reporter_test.go`](../../skeleton/internal/reporting/reporter_test.go)
- Execution-output boundary: [`prd.md`](../prd.md#package-ownership)
- Status: skeleton-aligned implementation guide

## Reference contract

The binding skeleton implementation, comments, and tests define Reporter's API
and output contract. This guide does not reproduce them. Reporter owns
human-readable execution output through its injected writer; fatal diagnostics
and process exit remain in `cmd/cli`. The validation reporting methods mirror
Validator's separate type, status, and body operations without taking ownership
of validation itself.

## Deliberately unspecified

For the stubbed methods, the skeleton does not specify exact text, icons,
spacing, path rendering, JSON payloads, calls to Runner, failure-class
validation, or error behavior. It also does not define how a debug step is
selected or scheduled.

## Acceptance criteria

1. Public names, signatures, static errors, and writer injection match the
   reference.
2. Working-directory output is byte-exact and write failures preserve their
   causes.
3. Reporter has no stderr or process-termination responsibility.
4. `ValidationTypes` accepts the failed string returned by type validation;
   stubbed methods remain within their documented reporting boundaries and do
   not acquire execution or validation responsibilities.
5. `ValidationBody` preserves colored diff payloads as required by the
   reference contract.
6. No unimplemented layout, Runner integration, debug scheduling, or success
   policy is asserted as a requirement.
