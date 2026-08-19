// Package execution prepares and executes APIHydra request steps.
package execution

import (
	"errors"
	"sync"
)

// ErrNotFound reports that a key does not exist in a KeyValueStore.
var ErrNotFound = errors.New("key not found")

// ErrKeyExists reports that a key already exists in a KeyValueStore.
var ErrKeyExists = errors.New("key already exists")

// KeyValueStore is a concurrent, write-once string key-value store.
type KeyValueStore struct {
	m  map[string]string
	mu sync.RWMutex
}

// NewKeyValueStore constructs an empty KeyValueStore.
func NewKeyValueStore() *KeyValueStore {
	return &KeyValueStore{
		m: make(map[string]string),
	}
}

// Get returns the value stored under key.
func (kvs *KeyValueStore) Get(key string) (string, error) {
	kvs.mu.RLock()
	defer kvs.mu.RUnlock()

	if val, ok := kvs.m[key]; ok {
		return val, nil
	}
	return "", ErrNotFound
}

// Set stores val under key unless key already exists.
func (kvs *KeyValueStore) Set(key, val string) error {
	kvs.mu.Lock()
	defer kvs.mu.Unlock()

	if _, ok := kvs.m[key]; ok {
		return ErrKeyExists
	}
	kvs.m[key] = val
	return nil
}
