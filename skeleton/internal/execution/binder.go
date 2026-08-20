package execution

import (
	"context"

	"apih/skeleton/internal/domain"
)

// Binder loads, interpolates, and captures step variables through
// one shared KeyValueStore.
type Binder struct {
	kvs *KeyValueStore
}

// NewBinder returns a binder that reads from and writes to kvs.
func NewBinder(kvs *KeyValueStore) *Binder {
	return &Binder{
		kvs: kvs,
	}
}

// LoadVariables stores every variable declared in step.Vars in the binder's
// key-value store.
func (b *Binder) LoadVariables(
	ctx context.Context,
	step *domain.Step,
) (int, error) {
	// TODO: implement
	return 0, nil
}

// InterpolateRequestBody replaces $var and ${var} placeholders in
// step.Request.Body with values from the binder's key-value store.
func (b *Binder) InterpolateRequestBody(
	ctx context.Context,
	step *domain.Step,
) (int, error) {
	// TODO: implement
	return 0, nil
}

// InterpolateResponseExpectedBody replaces $var and ${var} placeholders in
// step.Response.ExpectedBody with values from the binder's key-value store.
func (b *Binder) InterpolateResponseExpectedBody(
	ctx context.Context,
	step *domain.Step,
) (int, error) {
	// TODO: implement
	return 0, nil
}

// CaptureResponseVariables evaluates every selector in step.Response.Capture
// against step.Response.ActualBody with runner.JQExtract and stores each result
// in the binder's key-value store under its corresponding capture name.
func (b *Binder) CaptureResponseVariables(
	ctx context.Context,
	step *domain.Step,
) (int, error) {
	// TODO: implement
	return 0, nil
}
