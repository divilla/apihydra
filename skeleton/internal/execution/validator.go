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

// ValidateTypes validates step.Response.Body against the type declarations in
// step.Response.Types. Its result may contain multiple validation errors so
// callers can report every detected type mismatch.
func (v *Validator) ValidateTypes(
	ctx context.Context,
	step *domain.Step,
) []error {
	panic("Validator.ValidateTypes is not implemented")
}

// ValidateExpected validates step.Response.Body against
// step.Response.Expected. It projects and orders the actual response with
// runner.JQProject, formats and orders the expected response with
// runner.JQPretty, and compares the normalized documents with runner.GitDiff.
// It returns at most one validation error.
func (v *Validator) ValidateExpected(
	ctx context.Context,
	step *domain.Step,
) error {
	panic("Validator.ValidateExpected is not implemented")
}
