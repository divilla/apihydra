package execution

import (
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
)

func TestNewKeyValueStoreReturnsEmptyIndependentStores(t *testing.T) {
	first := NewKeyValueStore()
	second := NewKeyValueStore()

	if first == nil || second == nil {
		t.Fatal("NewKeyValueStore() returned nil")
	}
	for name, store := range map[string]*KeyValueStore{"first": first, "second": second} {
		if value, err := store.Get("key"); value != "" || !errors.Is(err, ErrNotFound) {
			t.Fatalf("%s.Get() = %q, %v, want empty value and ErrNotFound", name, value, err)
		}
	}
	if err := first.Set("key", "first"); err != nil {
		t.Fatalf("first.Set() error = %v", err)
	}
	if value, err := second.Get("key"); value != "" || !errors.Is(err, ErrNotFound) {
		t.Fatalf("second.Get() = %q, %v, want empty value and ErrNotFound", value, err)
	}
}

func TestKeyValueStoreGetDistinguishesMissingFromPresentEmptyValue(t *testing.T) {
	store := NewKeyValueStore()

	if value, err := store.Get("missing"); value != "" || !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get(missing) = %q, %v, want empty value and ErrNotFound", value, err)
	}
	if err := store.Set("empty", ""); err != nil {
		t.Fatalf("Set(empty) error = %v", err)
	}
	if value, err := store.Get("empty"); value != "" || err != nil {
		t.Fatalf("Get(empty) = %q, %v, want empty value and nil error", value, err)
	}
}

func TestKeyValueStoreSetPreservesFirstValue(t *testing.T) {
	store := NewKeyValueStore()

	if err := store.Set("key", "first"); err != nil {
		t.Fatalf("first Set() error = %v", err)
	}
	if err := store.Set("key", "second"); !errors.Is(err, ErrKeyExists) {
		t.Fatalf("second Set() error = %v, want ErrKeyExists", err)
	}
	if value, err := store.Get("key"); value != "first" || err != nil {
		t.Fatalf("Get() = %q, %v, want first value and nil error", value, err)
	}
}

func TestKeyValueStoreConcurrentReadersAndWriters(t *testing.T) {
	const goroutines = 64

	store := NewKeyValueStore()
	if err := store.Set("readable", "value"); err != nil {
		t.Fatalf("Set(readable) error = %v", err)
	}

	start := make(chan struct{})
	errorsCh := make(chan error, goroutines*3)
	var successfulSharedWrites atomic.Int32
	var wg sync.WaitGroup

	for index := range goroutines {
		wg.Add(2)

		go func() {
			defer wg.Done()
			<-start
			for range goroutines {
				value, err := store.Get("readable")
				if err != nil || value != "value" {
					errorsCh <- fmt.Errorf("Get(readable) = %q, %v", value, err)
					return
				}
			}
		}()

		go func() {
			defer wg.Done()
			<-start
			if err := store.Set("shared", fmt.Sprintf("value-%d", index)); err == nil {
				successfulSharedWrites.Add(1)
			} else if !errors.Is(err, ErrKeyExists) {
				errorsCh <- fmt.Errorf("Set(shared) error = %w", err)
			}
			if err := store.Set(fmt.Sprintf("key-%d", index), "value"); err != nil {
				errorsCh <- fmt.Errorf("Set(unique) error = %w", err)
			}
		}()
	}

	close(start)
	wg.Wait()
	close(errorsCh)

	for err := range errorsCh {
		t.Error(err)
	}
	if got := successfulSharedWrites.Load(); got != 1 {
		t.Fatalf("successful shared writes = %d, want 1", got)
	}
	if _, err := store.Get("shared"); err != nil {
		t.Fatalf("Get(shared) error = %v", err)
	}
	for index := range goroutines {
		if value, err := store.Get(fmt.Sprintf("key-%d", index)); value != "value" || err != nil {
			t.Fatalf("Get(key-%d) = %q, %v, want value and nil error", index, value, err)
		}
	}
}
