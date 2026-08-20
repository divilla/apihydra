# `internal/execution` KeyValueStore

## Status and ownership

- Binding reference: `skeleton/internal/execution/kvs.go`
- Shared product contract: [`prd.md`](../prd.md)
- Status: skeleton-aligned specification

This specification owns the complete KeyValueStore behavior because that
behavior is implemented in the reference skeleton.

## Public API

```go
var ErrNotFound = errors.New("key not found")
var ErrKeyExists = errors.New("key already exists")

func NewKeyValueStore() *KeyValueStore
func (kvs *KeyValueStore) Get(key string) (string, error)
func (kvs *KeyValueStore) Set(key, val string) error
```

The concrete store contains a `map[string]string` protected by
`sync.RWMutex`.

## Behavior

`NewKeyValueStore` returns an independent store with an initialized empty map.

`Get` performs an exact key lookup under a read lock. An existing key returns
its stored string and nil. A missing key returns `""` and `ErrNotFound`.
Empty stored strings remain distinguishable from absence.

`Set` holds the write lock across its existence check and insertion. A new key
stores the exact value and returns nil. An existing key returns
`ErrKeyExists` and does not replace the first value.

The map and lock make reads safe with writes and make concurrent first writes
atomic. The store does not normalize keys, validate values, interpolate text,
or support overwrites.

The two errors are returned directly. Contextual wrapping by a consumer, if
needed, follows `02-errs-pkg.md`.

## Acceptance criteria

1. Names, signatures, sentinels, and storage types match the reference.
2. New stores are empty and independent.
3. `Get` distinguishes a missing key from a present empty value.
4. `Set` is atomically write-once and preserves the first value.
5. Concurrent access is race-free.
