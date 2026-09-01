package errs

import (
	"errors"
	"fmt"
	"strings"

	"github.com/divilla/apihydra/internal/domain"
)

// Product process exit codes.
const (
	ExitValidation    = 101
	ExitConfiguration = 102
	ExitInternal      = 103
)

// ExitCoder exposes the process exit code attached to an error.
type ExitCoder interface {
	ExitCode() int
}

// ExitError retains a process code, static classification, optional cause, and
// contextual details.
type ExitError struct {
	code        int
	errStatic   error
	errOriginal error
	details     []any
}

// WithExitCode attaches code to err while preserving its identity.
func WithExitCode(code int, err error) error {
	return Build(code, err, nil)
}

// Build constructs a coded contextual error, or returns nil when errStatic is
// nil.
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

func (e *ExitError) Unwrap() []error {
	errs := []error{e.errStatic}
	if e.errOriginal != nil {
		errs = append(errs, e.errOriginal)
	}
	return errs
}

// ExitCode returns the process exit code attached to the error.
func (e *ExitError) ExitCode() int {
	return e.code
}

// Code returns an attached exit code, zero for nil, or fallback for an uncoded
// error.
func Code(err error, fallback int) int {
	if err == nil {
		return 0
	}

	var coder ExitCoder
	if errors.As(err, &coder) {
		return coder.ExitCode()
	}
	return fallback
}

// DefaultsDefinitionError adds defaults-file and YAML-path provenance to a
// configuration error.
func DefaultsDefinitionError(defaults *domain.DefaultsDefinition, yamlPath string, errStatic, errOriginal error) error {
	return Build(ExitConfiguration, errStatic, errOriginal, definitionDetails(defaultsFile(defaults), yamlPath))
}

// StepDefinitionError adds steps-file and YAML-path provenance to a
// configuration error.
func StepDefinitionError(step *domain.StepsDefinition, yamlPath string, errStatic, errOriginal error) error {
	return Build(ExitConfiguration, errStatic, errOriginal, definitionDetails(stepsFile(step), yamlPath))
}

// StepExecutionError adds step provenance and preserves a nonzero code from the
// original error, using ExitInternal otherwise.
func StepExecutionError(step *domain.Step, yamlPath string, errStatic, errOriginal error) error {
	exitCode := Code(errOriginal, ExitInternal)
	if exitCode == 0 {
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
