# `internal/execution` KeyValueStore

## Status and ownership

- Binding reference: [`skeleton/internal/execution/kvs.go`](../../skeleton/internal/execution/kvs.go)
- Shared product contract: [`prd.md`](../prd.md)
- Status: skeleton-aligned implementation guide

## Reference contract

The binding skeleton implementation and comments define the complete
KeyValueStore API and behavior. This guide does not reproduce them. A
concurrent write-once store makes duplicate variable definitions explicit and
keeps readers from observing replacement semantics.

The two errors are returned directly. Contextual wrapping by a consumer, if
needed, follows [`002-errs-pkg.md`](002-errs-pkg.md).

## Required implementation and tests

- Production output: `internal/execution/kvs.go` mirrors the complete binding
  concurrent store implementation.
- Test output: `internal/execution/kvs_test.go` covers new-store isolation,
  present/missing/empty values, duplicate preservation, and concurrent readers
  and writers under the race detector.
- Each acceptance criterion is traced to at least one meaningful unit test, and
  KeyValueStore unit-test statement coverage remains greater than 95%.

## Acceptance criteria

1. Names, signatures, sentinels, and storage types match the reference.
2. New stores are empty and independent.
3. `Get` distinguishes a missing key from a present empty value.
4. `Set` is atomically write-once and preserves the first value.
5. Concurrent access is race-free.
6. Package tests, `go test -race ./internal/execution`, and `git diff --check`
   pass.
