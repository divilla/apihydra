package execution

import (
	"fmt"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
)

func TestKeyValueStorePublicContract(t *testing.T) {
	var constructor func() *KeyValueStore = NewKeyValueStore
	var get func(*KeyValueStore, string) (string, error) = (*KeyValueStore).Get
	var set func(*KeyValueStore, string, string) error = (*KeyValueStore).Set
	_, _, _ = constructor, get, set

	store := NewKeyValueStore()
	if got, want := reflect.TypeOf(store.m), reflect.TypeOf(map[string]string{}); got != want {
		t.Fatalf("KeyValueStore.m type = %v, want %v", got, want)
	}
	if got, want := reflect.TypeOf(&store.mu).Elem(), reflect.TypeOf((*sync.RWMutex)(nil)).Elem(); got != want {
		t.Fatalf("KeyValueStore.mu type = %v, want %v", got, want)
	}
	if got, want := ErrNotFound.Error(), "key not found"; got != want {
		t.Fatalf("ErrNotFound = %q, want %q", got, want)
	}
	if got, want := ErrKeyExists.Error(), "key already exists"; got != want {
		t.Fatalf("ErrKeyExists = %q, want %q", got, want)
	}
}

func TestNewKeyValueStoreReturnsEmptyIndependentStores(t *testing.T) {
	first := NewKeyValueStore()
	second := NewKeyValueStore()

	if first.m == nil || second.m == nil {
		t.Fatal("NewKeyValueStore() returned a store with a nil map")
	}
	if err := first.Set("key", "first"); err != nil {
		t.Fatalf("first.Set() error = %v", err)
	}
	if value, err := second.Get("key"); value != "" || err != ErrNotFound {
		t.Fatalf("second.Get() = %q, %v, want empty value and ErrNotFound", value, err)
	}
}

func TestGetDistinguishesMissingKeyFromStoredEmptyValue(t *testing.T) {
	store := NewKeyValueStore()

	if value, err := store.Get("missing"); value != "" || err != ErrNotFound {
		t.Fatalf("Get(missing) = %q, %v, want empty value and ErrNotFound", value, err)
	}
	if err := store.Set("empty", ""); err != nil {
		t.Fatalf("Set(empty) error = %v", err)
	}
	if value, err := store.Get("empty"); value != "" || err != nil {
		t.Fatalf("Get(empty) = %q, %v, want empty value and nil error", value, err)
	}
}

func TestSetIsWriteOnceAndPreservesFirstValue(t *testing.T) {
	store := NewKeyValueStore()
	if err := store.Set("key", "first"); err != nil {
		t.Fatalf("Set(first) error = %v", err)
	}
	if err := store.Set("key", "second"); err != ErrKeyExists {
		t.Fatalf("Set(second) error = %v, want ErrKeyExists", err)
	}
	if value, err := store.Get("key"); value != "first" || err != nil {
		t.Fatalf("Get(key) = %q, %v, want first and nil error", value, err)
	}
}

func TestKeyValueStoreConcurrentAccessAndAtomicFirstWrite(t *testing.T) {
	const goroutines = 64

	store := NewKeyValueStore()
	start := make(chan struct{})
	var writers sync.WaitGroup
	var successes atomic.Int64

	for index := range goroutines {
		writers.Add(1)
		go func() {
			defer writers.Done()
			<-start
			value := fmt.Sprintf("value-%d", index)
			uniqueKey := fmt.Sprintf("key-%d", index)
			err := store.Set("shared", value)
			switch err {
			case nil:
				successes.Add(1)
			case ErrKeyExists:
			default:
				t.Errorf("Set(shared) error = %v, want nil or ErrKeyExists", err)
			}
			if err := store.Set(uniqueKey, value); err != nil {
				t.Errorf("Set(%s) error = %v", uniqueKey, err)
			}
			if _, err := store.Get("shared"); err != nil {
				t.Errorf("Get(shared) error = %v", err)
			}
		}()
	}

	close(start)
	writers.Wait()

	if got := successes.Load(); got != 1 {
		t.Fatalf("successful first writes = %d, want 1", got)
	}
	if value, err := store.Get("shared"); value == "" || err != nil {
		t.Fatalf("Get(shared) = %q, %v, want a stored value and nil error", value, err)
	}
	for index := range goroutines {
		key := fmt.Sprintf("key-%d", index)
		want := fmt.Sprintf("value-%d", index)
		if value, err := store.Get(key); value != want || err != nil {
			t.Fatalf("Get(%s) = %q, %v, want %q and nil error", key, value, err, want)
		}
	}
}
