package execution

import (
	"apih/skeleton/internal/domain"
	"context"
	"errors"
)

// ErrValidation classifies a nonfatal validation mismatch represented as an
// error. Executor reports these mismatches and converts them to validation
// exit status instead of returning them as its final error.
var ErrValidation = errors.New("validation error")

// ErrValidatorFatal classifies a failure that prevents validation from
// continuing.
var ErrValidatorFatal = errors.New("fatal validator error")

// Validator compares actual response values with a step's expectations.
type Validator struct{}

// NewValidator returns a stateless Validator.
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
	return nil
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
	return "", nil
}

// ValidateTypes builds a jq filter from step.Response.ExpectedTypes that selects
// declarations whose values in step.Response.ActualBody do not have an allowed
// type. It returns the output from runner.JQFilter. An empty failed result means
// every type validated; a non-empty result is a nonfatal validation mismatch.
// An error means filtering failed.
func (v *Validator) ValidateTypes(
	ctx context.Context,
	step *domain.Step,
) (string, error) {
	// TODO: implement
	return "", nil
}
