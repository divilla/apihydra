package execution

import (
    "fmt"
    "strings"
    "sync"
)

// CookiesKeyValueStore is a concurrent, write-once string store.
type CookiesKeyValueStore struct {
    m  map[string]string
    mu sync.RWMutex
}

// NewCookiesKeyValueStore returns an initialized empty store.
func NewCookiesKeyValueStore() *CookiesKeyValueStore {
    return &CookiesKeyValueStore{
        m: make(map[string]string),
    }
}

// Get returns a stored value or ErrNotFound when key is absent.
func (s *CookiesKeyValueStore) Get(key string) (string, error) {
    s.mu.RLock()
    defer s.mu.RUnlock()

    if val, ok := s.m[key]; ok {
        return val, nil
    }
    return "", ErrNotFound
}

// GetAll returns all values as single string.
func (s *CookiesKeyValueStore) GetAll() string {
    s.mu.RLock()
    defer s.mu.RUnlock()

    var values string
    for _, val := range s.m {
        values += val + "\n"
    }
    return values
}

// GetIncluded returns all values having listed key.
func (s *CookiesKeyValueStore) GetIncluded(keys []string) string {
    s.mu.RLock()
    defer s.mu.RUnlock()

    var values string
    for key, val := range s.m {
        if contains(keys, key) {
            values += val + "\n"
        }
    }
    return values
}

// GetExcluded returns all values but having listed key.
func (s *CookiesKeyValueStore) GetExcluded(keys []string) string {
    s.mu.RLock()
    defer s.mu.RUnlock()

    var values string
    for key, val := range s.m {
        if !contains(keys, key) {
            values += val + "\n"
        }
    }
    return values
}

// Set stores a new key and value pair.
func (s *CookiesKeyValueStore) Set(key, val string) {
    s.mu.Lock()
    defer s.mu.Unlock()

    s.m[key] = val
}

// SetAll stores a new key and value pair.
func (s *CookiesKeyValueStore) SetAll(values string) {
    s.mu.Lock()
    defer s.mu.Unlock()

    for line := range strings.Lines(values) {
        vals := strings.Fields(line)
        s.Set(join(vals[5], vals[0], vals[1]), line)
    }
}

// Del deletes kay value pair by key.
func (s *CookiesKeyValueStore) Del(key string) {
    s.mu.Lock()
    defer s.mu.Unlock()

    delete(s.m, key)
}

func join(name, domain, path string) string {
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
