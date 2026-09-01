package execution

import (
	"testing"

	"github.com/divilla/apihydra/skeleton/internal/domain"
)

func TestNewValidatorRetainsConfig(t *testing.T) {
	config := domain.Config{TempRunDir: "/cache/apih/run-1"}
	validator := NewValidator(config)
	if validator == nil {
		t.Fatal("NewValidator() = nil")
	}
	if validator.config != config {
		t.Fatalf("NewValidator() config = %#v, want %#v", validator.config, config)
	}
}
