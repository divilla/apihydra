package execution

import (
	"apih/internal/domain"
	"context"
)

// VariableProcessor provides the stateless variable-processing phase entry points.
type VariableProcessor struct{}

// NewVariableProcessor constructs an empty VariableProcessor.
func NewVariableProcessor() *VariableProcessor {
	return &VariableProcessor{}
}

// Load is the variable phase associated with Step.Vars.
func (p *VariableProcessor) Load(
	ctx context.Context,
	step *domain.Step,
) (int, error) {
	return 0, nil
}

// ParseRequestBody is the variable phase associated with Step.Request.Body.
func (p *VariableProcessor) ParseRequestBody(
	ctx context.Context,
	step *domain.Step,
) (int, error) {
	return 0, nil
}

// ParseResponseExpected is the variable phase associated with Step.Response.Expected.
func (p *VariableProcessor) ParseResponseExpected(
	ctx context.Context,
	step *domain.Step,
) (int, error) {
	return 0, nil
}

// Capture is the variable phase associated with Step.Response.Capture and the runtime response.
func (p *VariableProcessor) Capture(
	ctx context.Context,
	step *domain.Step,
) (int, error) {
	return 0, nil
}
