package domain

import (
	"reflect"
	"testing"
)

func TestConfigSchema(t *testing.T) {
	type field struct {
		name string
		typ  reflect.Type
		tag  reflect.StructTag
	}
	want := []field{
		{name: "Parallelism", typ: reflect.TypeOf(0)},
		{name: "Directory", typ: reflect.TypeOf("")},
		{name: "TempRunDir", typ: reflect.TypeOf("")},
	}

	typ := reflect.TypeOf(Config{})
	if typ.NumField() != len(want) {
		t.Fatalf("Config fields = %d, want %d", typ.NumField(), len(want))
	}
	for index, expected := range want {
		got := typ.Field(index)
		if got.Name != expected.name || got.Type != expected.typ || got.Tag != expected.tag {
			t.Fatalf("Config field %d = (%s, %v, %q), want (%s, %v, %q)", index, got.Name, got.Type, got.Tag, expected.name, expected.typ, expected.tag)
		}
	}
}
