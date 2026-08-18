package execution

import (
	"apih/skeleton/internal/domain"
	"context"
)

type VariableProcessor struct{}

func NewVariableProcessor() *VariableProcessor {
	return &VariableProcessor{}
}

func (p *VariableProcessor) Load(
	ctx context.Context,
	step *domain.Step,
) (int, error) {
	return 0, nil
}

func (p *VariableProcessor) ParseRequestBody(
	ctx context.Context,
	step *domain.Step,
) (int, error) {
	return 0, nil
}

func (p *VariableProcessor) ParseResponseExpected(
	ctx context.Context,
	step *domain.Step,
) (int, error) {
	return 0, nil
}

func (p *VariableProcessor) Capture(
	ctx context.Context,
	step *domain.Step,
) (int, error) {
	return 0, nil
}
