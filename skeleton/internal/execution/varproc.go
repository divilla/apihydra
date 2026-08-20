package execution

import (
	"apih/skeleton/internal/domain"
	"context"
)

type VariableProcessor struct{}

func NewVariableProcessor() *VariableProcessor {
	return &VariableProcessor{}
}

// Load iterates over *domain.Step.Vars and triggers execution.KeyValueStore.Set
// for each key: value pair
func (p *VariableProcessor) Load(
	ctx context.Context,
	step *domain.Step,
) (int, error) {
	return 0, nil
}

// ParseRequestBody replaces `$var` and `${var}` placeholders with
// execution.KeyValueStore.Get(var) value in *domain.Step.Request.Body
func (p *VariableProcessor) ParseRequestBody(
	ctx context.Context,
	step *domain.Step,
) (int, error) {
	return 0, nil
}

// ParseResponseExpected replaces `$var` and `${var}` placeholders with
// execution.KeyValueStore.Get(var) value in *domain.Step.Response.Expected
func (p *VariableProcessor) ParseResponseExpected(
	ctx context.Context,
	step *domain.Step,
) (int, error) {
	return 0, nil
}

// Capture iterates over *domain.Step.Response.Capture key: value pairs
// executing <value> using runner.JQSelect and saving to store with
// execution.KeyValueStore.Set
func (p *VariableProcessor) Capture(
	ctx context.Context,
	step *domain.Step,
) (int, error) {
	return 0, nil
}
