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

// Validator compares actual response values with a step's expectations. Its
// Config supplies the private run directory used for body-diff artifacts.
type Validator struct {
	config domain.Config
}

// NewValidator retains config for validation operations.
func NewValidator(config domain.Config) *Validator {
	return &Validator{config: config}
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

// ValidateBody jq-prettifies ExpectedBody without changing its shape and
// projects ActualBody to expected fields that are present in the actual
// response. Missing fields are not materialized, including fields expected to
// be null. The recursively sorted results must be equal. If they differ, it
// returns the diff calculated by runner.GitDiff; otherwise it returns "", nil.
func (v *Validator) ValidateBody(
	ctx context.Context,
	step *domain.Step,
) (string, error) {
	if strings.TrimSpace(string(step.Response.ExpectedBody)) == "" {
		return "", nil
	}

	expected, _, err := runner.JQPretty(ctx, string(step.Response.ExpectedBody))
	if err != nil {
		return "", errs.StepExecutionError(step, "", ErrValidatorFatal, err)
	}
	actual, _, err := runner.JQProject(ctx, buildBodySelector(string(step.Response.ExpectedBody)), string(step.Response.ActualBody))
	if err != nil {
		return "", errs.StepExecutionError(step, "", ErrValidatorFatal, err)
	}
	if actual == expected {
		return "", nil
	}

	diff, _, err := runner.GitDiff(ctx, v.config.TempRunDir, expected, actual)
	if err != nil {
		return "", errs.StepExecutionError(step, "", ErrValidatorFatal, err)
	}
	return diff, nil
}

func buildBodySelector(expected string) string {
	return `def apih_project($expected):
  if (($expected | type) == "object" and type == "object") then
    . as $actual
    | reduce ($expected | keys_unsorted[]) as $key
        ({};
          if ($actual | has($key)) then
            . + {($key): ($actual[$key] | apih_project($expected[$key]))}
          else
            .
          end)
  elif (($expected | type) == "array" and type == "array" and ($expected | length) == length) then
    . as $actual
    | [range(0; length) as $index | ($actual[$index] | apih_project($expected[$index]))]
  else
    .
  end;
apih_project(` + expected + `)`
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
	failed, _, err := runner.JQFilter(ctx, buildTypeFilter(step.Response.ExpectedTypes), string(step.Response.ActualBody))
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
			`({selector:%s,expected:%s,actual:((%s))} | select(.actual as $value | (%s | not)) | .actual |= type)`,
			selectorJSON,
			expectedJSON,
			selector,
			expectedTypePredicate(expectedTypes[selector]),
		))
	}
	return "[" + strings.Join(declarations, ",") + "] | .[]"
}

func expectedTypePredicate(expected []string) string {
	predicates := make([]string, 0, len(expected))
	for _, declaration := range expected {
		switch declaration {
		case "int":
			predicates = append(predicates, `(if ($value | type) == "number" then (($value | floor) == $value) else false end)`)
		case "zero":
			predicates = append(predicates, `($value == 0)`)
		default:
			declarationJSON, _ := json.Marshal(declaration)
			predicates = append(predicates, fmt.Sprintf(`(($value | type) == %s)`, declarationJSON))
		}
	}
	if len(predicates) == 0 {
		return "false"
	}
	if len(predicates) == 1 {
		return predicates[0]
	}
	return "(" + strings.Join(predicates, " or ") + ")"
}
