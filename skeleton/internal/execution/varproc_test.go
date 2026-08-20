package execution

import "testing"

func TestNewVariableProcessorRetainsStore(t *testing.T) {
	store := NewKeyValueStore()
	processor := NewVariableProcessor(store)
	if processor.kvs != store {
		t.Fatalf("processor.kvs = %p, want %p", processor.kvs, store)
	}
}
