package execution

import (
	"context"
	"testing"

	"apih/skeleton/internal/domain"
)

func TestValidatorSplitAPIPlaceholders(t *testing.T) {
	validator := NewValidator()
	step := &domain.Step{}

	tests := map[string]struct {
		call      func()
		wantPanic string
	}{
		"types": {
			call: func() {
				validator.ValidateTypes(context.Background(), step)
			},
			wantPanic: "Validator.ValidateTypes is not implemented",
		},
		"status": {
			call: func() {
				validator.ValidateStatus(context.Background(), step)
			},
			wantPanic: "Validator.ValidateStatus is not implemented",
		},
		"body": {
			call: func() {
				_, _ = validator.ValidateBody(context.Background(), step)
			},
			wantPanic: "Validator.ValidateBody is not implemented",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			defer func() {
				if got := recover(); got != test.wantPanic {
					t.Fatalf("panic = %v, want %q", got, test.wantPanic)
				}
			}()
			test.call()
		})
	}
}
