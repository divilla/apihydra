package execution

import "testing"

func TestNewBinderRetainsStore(t *testing.T) {
	store := NewKeyValueStore()
	binder := NewBinder(store)
	if binder.kvs != store {
		t.Fatalf("binder.kvs = %p, want %p", binder.kvs, store)
	}
}
