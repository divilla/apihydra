package execution

import (
	"apih/skeleton/internal/domain"
	"context"
	"errors"
)

// ErrValidation reports that ValidateTypes or ValidateExpected found at least one mismatch.
var ErrValidation = errors.New("validation error")
var ErrValidatorFatal = errors.New("fatal validator error")

type Validator struct{}

func (v *Validator) ValidateTypes(
	ctx context.Context,
	step *domain.Step,
) []error {
	return nil
}

func (v *Validator) ValidateExpected(
	ctx context.Context,
	step *domain.Step,
) error {
	return nil
}
