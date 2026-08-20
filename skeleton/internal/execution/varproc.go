package execution

import (
	"context"

	"apih/skeleton/internal/domain"
)

// VariableProcessor loads, interpolates, and captures step variables through
// one shared KeyValueStore.
type VariableProcessor struct {
	kvs *KeyValueStore
}

// NewVariableProcessor returns a processor that reads from and writes to kvs.
func NewVariableProcessor(kvs *KeyValueStore) *VariableProcessor {
	return &VariableProcessor{
		kvs: kvs,
	}
}

// LoadVariables stores every variable declared in step.Vars in the processor's
// key-value store.
func (p *VariableProcessor) LoadVariables(
	ctx context.Context,
	step *domain.Step,
) (int, error) {
	// TODO: implement
	return 0, nil
}

// InterpolateRequestBody replaces $var and ${var} placeholders in
// step.Request.Body with values from the processor's key-value store.
func (p *VariableProcessor) InterpolateRequestBody(
	ctx context.Context,
	step *domain.Step,
) (int, error) {
	// TODO: implement
	return 0, nil
}

// InterpolateResponseExpectedBody replaces $var and ${var} placeholders in
// step.Response.ExpectedBody with values from the processor's key-value store.
func (p *VariableProcessor) InterpolateResponseExpectedBody(
	ctx context.Context,
	step *domain.Step,
) (int, error) {
	// TODO: implement
	return 0, nil
}

// CaptureResponseVariables evaluates every selector in step.Response.Capture
// against step.Response.ActualBody with runner.JQExtract and stores each result
// in the processor's key-value store under its corresponding capture name.
func (p *VariableProcessor) CaptureResponseVariables(
	ctx context.Context,
	step *domain.Step,
) (int, error) {
	// TODO: implement
	return 0, nil
}
