# `pkg/errs`

## Status and ownership

- Binding reference: [`skeleton/pkg/errs/errors.go`](../../skeleton/pkg/errs/errors.go)
- Reference tests: [`skeleton/pkg/errs/errors_test.go`](../../skeleton/pkg/errs/errors_test.go)
- Shared product contract: [`prd.md`](../prd.md)
- Shared domain types: [`000-domain-types.md`](000-domain-types.md)
- Status: skeleton-aligned implementation guide

## Reference contract

The binding skeleton defines contextual error construction, lookup, formatting,
unwrapping, and provenance behavior. This guide does not reproduce those
declarations or algorithms. The PRD owns the meaning of the product exit codes;
originating packages own their static error classifications.

Centralizing contextual composition here keeps error identities and exit-code
metadata consistent without duplicating presentation logic in callers.

## Boundaries

This package does not own static errors from other packages, terminal output,
ANSI styling, source-line lookup, cancellation, or command execution. The
repository-wide contextual-error ownership rule is defined once in the PRD.

## Required implementation and tests

- Production output: `pkg/errs/errors.go` mirrors the complete binding
  implementation with production import paths.
- Test output: `pkg/errs/errors_test.go` covers construction, formatting,
  unwrapping, code lookup/fallback, nil handling, provenance, and safe partial
  domain values; root `architecture_test.go` enforces exclusive ownership of
  contextual error composition.
- Each acceptance criterion is traced to a meaningful unit or architecture
  test, and package unit-test statement coverage remains greater than 95%.

## Acceptance criteria

1. All exported names and signatures match the reference.
2. Construction preserves codes, error identities, component order, and
   optional details exactly.
3. Nil static errors return nil, and nil lookup returns success.
4. Definition helpers use configuration code and safe provenance.
5. Step execution errors cannot expose a non-nil error as success.
6. No other package's static classification or presentation behavior is
   duplicated here.
7. Package tests, the repository ownership test, and `git diff --check` pass.
