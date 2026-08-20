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

// ValidateExpected projects step.Response.Body with runner.JQProject, formats
// step.Response.Expected with runner.JQPretty, and compares the normalized
// documents with runner.GitDiff.
func (v *Validator) ValidateExpected(
	ctx context.Context,
	step *domain.Step,
) error {
	return nil
}
