package errs

import (
	"apih/skeleton/internal/domain"
	"errors"
	"fmt"
	"strings"
)

const (
	ExitSuccess       = 0
	ExitValidation    = 101
	ExitConfiguration = 102
	ExitInternal      = 103
)

type ExitCoder interface {
	ExitCode() int
}

type ExitError struct {
	code        int
	errStatic   error
	errOriginal error
	details     []any
}

func WithExitCode(code int, err error) error {
	return Build(code, err, nil)
}

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

func (e *ExitError) ExitCode() int {
	return e.code
}

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

func DefaultsDefinitionError(defaults *domain.DefaultsDefinition, yamlPath string, errStatic, errOriginal error) error {
	return Build(ExitConfiguration, errStatic, errOriginal, definitionDetails(defaultsFile(defaults), yamlPath))
}

func StepDefinitionError(step *domain.StepsDefinition, yamlPath string, errStatic, errOriginal error) error {
	return Build(ExitConfiguration, errStatic, errOriginal, definitionDetails(stepsFile(step), yamlPath))
}

func StepExecutionError(step *domain.Step, yamlPath string, errStatic, errOriginal error) error {
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
