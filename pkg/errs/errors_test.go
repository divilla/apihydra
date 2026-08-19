package errs

import (
	"apih/internal/domain"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
)

type codedError struct {
	code int
	err  error
}

func (e codedError) Error() string {
	return "coded error"
}

func (e codedError) ExitCode() int {
	return e.code
}

func (e codedError) Unwrap() error {
	return e.err
}

func TestPublicContract(t *testing.T) {
	var exitCoder ExitCoder = (*ExitError)(nil)
	var build func(int, error, error, ...any) error = Build
	var withExitCode func(int, error) error = WithExitCode
	var code func(error, int) int = Code
	var defaultsError func(*domain.DefaultsDefinition, string, error, error) error = DefaultsDefinitionError
	var definitionError func(*domain.StepsDefinition, string, error, error) error = StepDefinitionError
	var executionError func(*domain.Step, string, error, error) error = StepExecutionError
	_, _, _, _, _, _, _ = exitCoder, build, withExitCode, code, defaultsError, definitionError, executionError

	tests := map[string]struct {
		got  int
		want int
	}{
		"success":       {ExitSuccess, 0},
		"validation":    {ExitValidation, 101},
		"configuration": {ExitConfiguration, 102},
		"internal":      {ExitInternal, 103},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			if test.got != test.want {
				t.Fatalf("exit code = %d, want %d", test.got, test.want)
			}
		})
	}
}

func TestBuildPreservesCodeIdentitiesAndComponentOrder(t *testing.T) {
	staticErr := errors.New("static")
	originalErr := errors.New("original")
	built := Build(27, staticErr, originalErr, "file ", "suite.yaml")

	if got, want := built.Error(), "static: file suite.yaml: original"; got != want {
		t.Fatalf("Build().Error() = %q, want %q", got, want)
	}
	if !errors.Is(built, staticErr) {
		t.Fatal("Build() did not preserve the static error")
	}
	if !errors.Is(built, originalErr) {
		t.Fatal("Build() did not preserve the original error")
	}
	var exitErr *ExitError
	if !errors.As(built, &exitErr) {
		t.Fatalf("Build() error type = %T, want *ExitError", built)
	}
	if got := exitErr.ExitCode(); got != 27 {
		t.Fatalf("ExitCode() = %d, want 27", got)
	}
}

func TestBuildOmitsAbsentAndEmptyComponents(t *testing.T) {
	staticErr := errors.New("static")
	originalErr := errors.New("original")
	tests := map[string]struct {
		err  error
		want string
	}{
		"static only":    {Build(1, staticErr, nil), "static"},
		"empty details":  {Build(1, staticErr, nil, "", ""), "static"},
		"original only":  {Build(1, staticErr, originalErr), "static: original"},
		"details only":   {Build(1, staticErr, nil, "detail"), "static: detail"},
		"with exit code": {WithExitCode(1, staticErr), "static"},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			if got := test.err.Error(); got != test.want {
				t.Fatalf("error text = %q, want %q", got, test.want)
			}
		})
	}
}

func TestBuildReturnsNilForNilStaticError(t *testing.T) {
	if err := Build(ExitInternal, nil, errors.New("original"), "detail"); err != nil {
		t.Fatalf("Build() = %v, want nil", err)
	}
	if err := WithExitCode(ExitInternal, nil); err != nil {
		t.Fatalf("WithExitCode() = %v, want nil", err)
	}
}

func TestCodeUsesAttachedCodeOrFallback(t *testing.T) {
	if got := Code(nil, 77); got != ExitSuccess {
		t.Fatalf("Code(nil) = %d, want %d", got, ExitSuccess)
	}
	if got := Code(errors.New("uncoded"), 77); got != 77 {
		t.Fatalf("Code(uncoded) = %d, want 77", got)
	}
	coded := WithExitCode(23, errors.New("cause"))
	if got := Code(fmt.Errorf("wrapped: %w", coded), 77); got != 23 {
		t.Fatalf("Code(wrapped coded) = %d, want 23", got)
	}
	outer := codedError{code: 29, err: coded}
	if got := Code(outer, 77); got != 29 {
		t.Fatalf("Code(nested coded) = %d, want outer code 29", got)
	}
}

func TestDefinitionHelpersUseConfigurationCodeAndSafeProvenance(t *testing.T) {
	staticErr := errors.New("invalid definition")
	originalErr := errors.New("invalid value")
	file := &domain.File{Path: "/suite/defaults.yaml"}

	tests := map[string]struct {
		err  error
		want string
	}{
		"defaults file and yaml path": {
			DefaultsDefinitionError(&domain.DefaultsDefinition{File: file}, "spec.timeout", staticErr, originalErr),
			"invalid definition: file /suite/defaults.yaml, yaml path spec.timeout: invalid value",
		},
		"defaults missing definition": {
			DefaultsDefinitionError(nil, "spec.timeout", staticErr, nil),
			"invalid definition: yaml path spec.timeout",
		},
		"defaults missing file and path": {
			DefaultsDefinitionError(&domain.DefaultsDefinition{}, "", staticErr, nil),
			"invalid definition",
		},
		"steps file only": {
			StepDefinitionError(&domain.StepsDefinition{File: file}, "", staticErr, nil),
			"invalid definition: file /suite/defaults.yaml",
		},
		"steps missing definition": {
			StepDefinitionError(nil, "spec.steps[0]", staticErr, nil),
			"invalid definition: yaml path spec.steps[0]",
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			if got := test.err.Error(); got != test.want {
				t.Fatalf("error text = %q, want %q", got, test.want)
			}
			if got := Code(test.err, ExitInternal); got != ExitConfiguration {
				t.Fatalf("Code() = %d, want %d", got, ExitConfiguration)
			}
		})
	}
}

func TestStepExecutionErrorUsesOriginalCodeAndSafeProvenance(t *testing.T) {
	staticErr := errors.New("execution failed")
	file := &domain.File{Path: "/suite/steps.yaml"}
	step := &domain.Step{Definition: &domain.StepsDefinition{File: file}}

	tests := map[string]struct {
		step     *domain.Step
		original error
		wantCode int
		wantText string
	}{
		"coded original": {
			step,
			WithExitCode(19, errors.New("tool failed")),
			19,
			"execution failed: file /suite/steps.yaml, yaml path spec.steps[0]: tool failed",
		},
		"uncoded original": {
			&domain.Step{},
			errors.New("tool failed"),
			ExitInternal,
			"execution failed: tool failed",
		},
		"success-coded original": {
			nil,
			WithExitCode(ExitSuccess, errors.New("tool failed")),
			ExitInternal,
			"execution failed: tool failed",
		},
		"nil original": {
			nil,
			nil,
			ExitInternal,
			"execution failed",
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			yamlPath := ""
			if name == "coded original" {
				yamlPath = "spec.steps[0]"
			}
			err := StepExecutionError(test.step, yamlPath, staticErr, test.original)
			if got := Code(err, 77); got != test.wantCode {
				t.Fatalf("Code() = %d, want %d", got, test.wantCode)
			}
			if got := err.Error(); got != test.wantText {
				t.Fatalf("error text = %q, want %q", got, test.wantText)
			}
		})
	}
}

func TestHelpersReturnNilForNilStaticError(t *testing.T) {
	originalErr := WithExitCode(12, errors.New("original"))
	if err := DefaultsDefinitionError(nil, "path", nil, originalErr); err != nil {
		t.Fatalf("DefaultsDefinitionError() = %v, want nil", err)
	}
	if err := StepDefinitionError(nil, "path", nil, originalErr); err != nil {
		t.Fatalf("StepDefinitionError() = %v, want nil", err)
	}
	if err := StepExecutionError(nil, "path", nil, originalErr); err != nil {
		t.Fatalf("StepExecutionError() = %v, want nil", err)
	}
}

func TestPackageDoesNotOwnStaticClassificationsOrPresentation(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read package directory: %v", err)
	}

	forbidden := []string{
		"errors.New(",
		"errors.Join(",
		"fmt.Errorf(",
		"fmt.Print",
		"fmt.Fprint",
		"log.Print",
		`\x1b[`,
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		contents, err := os.ReadFile(entry.Name())
		if err != nil {
			t.Fatalf("read %s: %v", entry.Name(), err)
		}
		for _, pattern := range forbidden {
			if strings.Contains(string(contents), pattern) {
				t.Errorf("%s contains foreign classification or presentation pattern %q", entry.Name(), pattern)
			}
		}
	}
}
