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
	panic("VariableProcessor.LoadVariables is not implemented")
}

// InterpolateRequestBody replaces $var and ${var} placeholders in
// step.Request.Body with values from the processor's key-value store.
func (p *VariableProcessor) InterpolateRequestBody(
	ctx context.Context,
	step *domain.Step,
) (int, error) {
	// TODO: implement
	panic("VariableProcessor.InterpolateRequestBody is not implemented")
}

// InterpolateResponseExpected replaces $var and ${var} placeholders in
// step.Response.ExpectedBody with values from the processor's key-value store.
func (p *VariableProcessor) InterpolateResponseExpected(
	ctx context.Context,
	step *domain.Step,
) (int, error) {
	// TODO: implement
	panic("VariableProcessor.InterpolateResponseExpected is not implemented")
}

// CaptureResponseVariables evaluates every selector in step.Response.Capture
// against step.Response.ActualBody with runner.JQExtract and stores each result
// in the processor's key-value store under its corresponding capture name.
func (p *VariableProcessor) CaptureResponseVariables(
	ctx context.Context,
	step *domain.Step,
) (int, error) {
	// TODO: implement
	panic("VariableProcessor.CaptureResponseVariables is not implemented")
}
