package execution

import (
	"errors"
	"sync"
)

var NotFoundError = errors.New("key not found")
var KeyExistError = errors.New("key already exists")

type KeyValueStore struct {
	m  map[string]string
	mu sync.RWMutex
}

func NewKeyValueStore() *KeyValueStore {
	return &KeyValueStore{
		m: make(map[string]string),
	}
}

func (kvs *KeyValueStore) Get(key string) (string, error) {
	kvs.mu.RLock()
	defer kvs.mu.RUnlock()

	if val, ok := kvs.m[key]; ok {
		return val, nil
	}
	return "", NotFoundError
}

func (kvs *KeyValueStore) Set(key, val string) error {
	kvs.mu.Lock()
	defer kvs.mu.Unlock()

	if _, ok := kvs.m[key]; ok {
		return KeyExistError
	}
	kvs.m[key] = val
	return nil
}
