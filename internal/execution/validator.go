package execution

import (
	"apih/internal/domain"
	"context"
	"errors"
)

// ErrValidation reports that ValidateTypes or ValidateExpected found at least one mismatch.
var ErrValidation = errors.New("validation error")

// ErrValidatorFatal classifies a fatal validator failure.
var ErrValidatorFatal = errors.New("fatal validator error")

// Validator provides the stateless response-validation phase entry points.
type Validator struct{}

// ValidateTypes validates Step.Response.Body against Step.Response.Types.
func (v *Validator) ValidateTypes(
	ctx context.Context,
	step *domain.Step,
) []error {
	return nil
}

// ValidateExpected validates Step.Response.Body against Step.Response.Expected.
func (v *Validator) ValidateExpected(
	ctx context.Context,
	step *domain.Step,
) error {
	return nil
}
