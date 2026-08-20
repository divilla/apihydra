package execution

import (
	"context"
	"testing"

	"apih/skeleton/internal/domain"
)

func TestInterpolateResponseExpectedBodyPlaceholder(t *testing.T) {
	processor := NewVariableProcessor(NewKeyValueStore())

	defer func() {
		if got, want := recover(), "VariableProcessor.InterpolateResponseExpectedBody is not implemented"; got != want {
			t.Fatalf("panic = %v, want %q", got, want)
		}
	}()

	_, _ = processor.InterpolateResponseExpectedBody(context.Background(), &domain.Step{})
}
