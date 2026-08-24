package execution

import (
	"apih/internal/domain"
	"apih/pkg/errs"
	"apih/pkg/runner"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
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
	if step.Response.ExpectedStatus != 0 && step.Response.ActualStatus != step.Response.ExpectedStatus {
		return ErrValidation
	}
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
	expected, _, err := runner.JQPretty(ctx, string(step.Response.ExpectedBody))
	if err != nil {
		return "", errs.StepExecutionError(step, "", ErrValidatorFatal, err)
	}
	actual, _, err := runner.JQProject(ctx, ".", step.Response.ActualBody)
	if err != nil {
		return "", errs.StepExecutionError(step, "", ErrValidatorFatal, err)
	}
	if actual == expected {
		return "", nil
	}

	diff, _, err := runner.GitDiff(ctx, expected, actual)
	if err != nil {
		return "", errs.StepExecutionError(step, "", ErrValidatorFatal, err)
	}
	return diff, nil
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
	failed, _, err := runner.JQFilter(ctx, buildTypeFilter(step.Response.ExpectedTypes), step.Response.ActualBody)
	if err != nil {
		return "", errs.StepExecutionError(step, "", ErrValidatorFatal, err)
	}
	return failed, nil
}

func buildTypeFilter(expectedTypes map[string][]string) string {
	selectors := make([]string, 0, len(expectedTypes))
	for selector := range expectedTypes {
		selectors = append(selectors, selector)
	}
	slices.Sort(selectors)

	declarations := make([]string, 0, len(selectors))
	for _, selector := range selectors {
		selectorJSON, _ := json.Marshal(selector)
		expectedJSON, _ := json.Marshal(expectedTypes[selector])
		declarations = append(declarations, fmt.Sprintf(
			`({selector:%s,expected:%s,actual:((%s) | type)} | select(.actual as $actual | (.expected | index($actual) | not)))`,
			selectorJSON,
			expectedJSON,
			selector,
		))
	}
	return "[" + strings.Join(declarations, ",") + "] | .[]"
}
