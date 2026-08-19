// Package errs constructs contextual APIHydra errors and attaches exit codes.
package errs

import (
	"apih/internal/domain"
	"errors"
	"fmt"
	"strings"
)

const (
	// ExitSuccess indicates successful completion.
	ExitSuccess = 0
	// ExitValidation indicates completed execution with validation failures.
	ExitValidation = 101
	// ExitConfiguration indicates an invocation or configuration failure.
	ExitConfiguration = 102
	// ExitInternal indicates an internal failure.
	ExitInternal = 103
)

// ExitCoder exposes an error's process exit code.
type ExitCoder interface {
	ExitCode() int
}

// ExitError retains a static classification, an optional original error,
// contextual details, and a process exit code.
type ExitError struct {
	code        int
	errStatic   error
	errOriginal error
	details     []any
}

// WithExitCode attaches code to err.
func WithExitCode(code int, err error) error {
	return Build(code, err, nil)
}

// Build constructs a coded contextual error, or nil when errStatic is nil.
func Build(code int, errStatic, errOriginal error, details ...any) error {
	if errStatic == nil {
		return nil
	}
	return &ExitError{
		code:        code,
		errStatic:   errStatic,
		errOriginal: errOriginal,
		details:     details,
	}
}

// Error joins the non-empty error components in their contract order.
func (e *ExitError) Error() string {
	parts := []string{e.errStatic.Error()}
	if len(e.details) > 0 {
		if details := fmt.Sprint(e.details...); details != "" {
			parts = append(parts, details)
		}
	}
	if e.errOriginal != nil {
		parts = append(parts, e.errOriginal.Error())
	}
	return strings.Join(parts, ": ")
}

// Unwrap exposes both the static and original errors when available.
func (e *ExitError) Unwrap() []error {
	errs := []error{e.errStatic}
	if e.errOriginal != nil {
		errs = append(errs, e.errOriginal)
	}
	return errs
}

// ExitCode returns the code retained by the error.
func (e *ExitError) ExitCode() int {
	return e.code
}

// Code returns an attached code, ExitSuccess for nil, or fallback otherwise.
func Code(err error, fallback int) int {
	if err == nil {
		return ExitSuccess
	}

	var coder ExitCoder
	if errors.As(err, &coder) {
		return coder.ExitCode()
	}
	return fallback
}

// DefaultsDefinitionError constructs a configuration error with defaults provenance.
func DefaultsDefinitionError(
	defaults *domain.DefaultsDefinition,
	yamlPath string,
	errStatic, errOriginal error,
) error {
	return Build(ExitConfiguration, errStatic, errOriginal, definitionDetails(defaultsFile(defaults), yamlPath))
}

// StepDefinitionError constructs a configuration error with steps provenance.
func StepDefinitionError(
	step *domain.StepsDefinition,
	yamlPath string,
	errStatic, errOriginal error,
) error {
	return Build(ExitConfiguration, errStatic, errOriginal, definitionDetails(stepsFile(step), yamlPath))
}

// StepExecutionError constructs an error with step provenance and the original
// error's code, using ExitInternal when that code is absent or successful.
func StepExecutionError(
	step *domain.Step,
	yamlPath string,
	errStatic, errOriginal error,
) error {
	exitCode := Code(errOriginal, ExitInternal)
	if exitCode == ExitSuccess {
		exitCode = ExitInternal
	}
	return Build(exitCode, errStatic, errOriginal, definitionDetails(stepFile(step), yamlPath))
}

func defaultsFile(definition *domain.DefaultsDefinition) *domain.File {
	if definition == nil {
		return nil
	}
	return definition.File
}

func stepsFile(definition *domain.StepsDefinition) *domain.File {
	if definition == nil {
		return nil
	}
	return definition.File
}

func stepFile(step *domain.Step) *domain.File {
	if step == nil || step.Definition == nil {
		return nil
	}
	return step.Definition.File
}

func definitionDetails(file *domain.File, yamlPath string) string {
	parts := make([]string, 0, 2)
	if file != nil {
		parts = append(parts, "file "+file.Path)
	}
	if yamlPath != "" {
		parts = append(parts, "yaml path "+yamlPath)
	}
	return strings.Join(parts, ", ")
}
