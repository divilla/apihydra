package execution

import (
	"errors"
	"sync"
)

// ErrNotFound reports that a key is absent.
var ErrNotFound = errors.New("key not found")

// ErrKeyExists reports an attempt to replace an existing key.
var ErrKeyExists = errors.New("key already exists")

// KeyValueStore is a concurrent, write-once string store.
type KeyValueStore struct {
	m  map[string]string
	mu sync.RWMutex
}

// NewKeyValueStore returns an initialized empty store.
func NewKeyValueStore() *KeyValueStore {
	return &KeyValueStore{
		m: make(map[string]string),
	}
}

// Get returns a stored value or ErrNotFound when key is absent.
func (kvs *KeyValueStore) Get(key string) (string, error) {
	kvs.mu.RLock()
	defer kvs.mu.RUnlock()

	if val, ok := kvs.m[key]; ok {
		return val, nil
	}
	return "", ErrNotFound
}

// Set stores a new key and returns ErrKeyExists without replacing an old value.
func (kvs *KeyValueStore) Set(key, val string) error {
	kvs.mu.Lock()
	defer kvs.mu.Unlock()

	if _, ok := kvs.m[key]; ok {
		return ErrKeyExists
	}
	kvs.m[key] = val
	return nil
}
