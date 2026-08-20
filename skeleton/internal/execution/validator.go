package execution

import (
	"apih/skeleton/internal/domain"
	"context"
	"errors"
)

// ErrValidation reports that ValidateTypes or ValidateStatus found at least
// one nonfatal mismatch. StepRunner reports these mismatches and converts them
// to validation exit status instead of returning them as its final error.
var ErrValidation = errors.New("validation error")
var ErrValidatorFatal = errors.New("fatal validator error")

type Validator struct{}

func NewValidator() *Validator {
	return &Validator{}
}

// ValidateStatus validates ActualStatus against ExpectedStatus.
// ExpectedStatus 0 accepts any ActualStatus; otherwise ActualStatus must equal
// ExpectedStatus.
func (v *Validator) ValidateStatus(
	ctx context.Context,
	step *domain.Step,
) error {
	// TODO: implement
	panic("Validator.ValidateStatus is not implemented")
}

// ValidateBody validates ActualBody against ExpectedBody. ActualBody parsed
// with runner.JQProject must equal ExpectedBody parsed with runner.JQPretty.
// If they differ, it returns the diff calculated by runner.GitDiff. If they are
// equal, it returns "", nil.
func (v *Validator) ValidateBody(
	ctx context.Context,
	step *domain.Step,
) (string, error) {
	// TODO: implement
	panic("Validator.ValidateBody is not implemented")
}

// ValidateTypes validates step.Response.ActualBody against the type declarations
// in step.Response.ExpectedTypes. Its result may contain multiple validation
// errors so callers can report every detected type mismatch.
func (v *Validator) ValidateTypes(
	ctx context.Context,
	step *domain.Step,
) []error {
	// TODO: implement
	panic("Validator.ValidateTypes is not implemented")
}
