package execution

import (
    "fmt"
    "sync"
)

// CookieKeyValueStore is a concurrent string store.
type CookieKeyValueStore struct {
    m  map[string]string
    mu sync.RWMutex
}

// NewCookieKeyValueStore returns an initialized empty store.
func NewCookieKeyValueStore() *CookieKeyValueStore {
    return &CookieKeyValueStore{
        m: make(map[string]string),
    }
}

// Get returns a stored value or ErrNotFound when key is absent.
func (s *CookieKeyValueStore) Get(key string) (string, error) {
    s.mu.RLock()
    defer s.mu.RUnlock()

    if val, ok := s.m[key]; ok {
        return val, nil
    }
    return "", ErrNotFound
}

// GetAll returns all values as single string.
func (s *CookieKeyValueStore) GetAll() map[string]string {
    s.mu.RLock()
    defer s.mu.RUnlock()

    var m map[string]string
    for key, val := range s.m {
        m[key] = val
    }
    return m
}

// Set stores a new key and value pair.
func (s *CookieKeyValueStore) Set(key, val string) {
    s.mu.Lock()
    defer s.mu.Unlock()

    s.m[key] = val
}

// Del deletes kay value pair by key.
func (s *CookieKeyValueStore) Del(key string) {
    s.mu.Lock()
    defer s.mu.Unlock()

    delete(s.m, key)
}

func makeCookieKey(name, domain, path string) string {
    return fmt.Sprintf("%s:%s:%s", name, domain, path)
}

func contains(vals []string, val string) bool {
    for _, v := range vals {
        if v == val {
            return true
        }
    }
    return false
}
