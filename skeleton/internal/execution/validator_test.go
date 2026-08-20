package execution

import "testing"

func TestNewValidatorReturnsValidator(t *testing.T) {
	if validator := NewValidator(); validator == nil {
		t.Fatal("NewValidator() = nil")
	}
}
