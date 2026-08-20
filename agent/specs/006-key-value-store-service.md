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

## Acceptance criteria

1. Names, signatures, sentinels, and storage types match the reference.
2. New stores are empty and independent.
3. `Get` distinguishes a missing key from a present empty value.
4. `Set` is atomically write-once and preserves the first value.
5. Concurrent access is race-free.
