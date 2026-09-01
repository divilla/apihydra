package errs

import (
	"errors"
	"fmt"
	"slices"
	"testing"

	"github.com/divilla/apihydra/internal/domain"
)

func TestExportedContractMatchesReference(t *testing.T) {
	var _ ExitCoder = (*ExitError)(nil)
	var _ func(int, error) error = WithExitCode
	var _ func(int, error, error, ...any) error = Build
	var _ func(error, int) int = Code
	var _ func(*domain.DefaultsDefinition, string, error, error) error = DefaultsDefinitionError
	var _ func(*domain.StepsDefinition, string, error, error) error = StepDefinitionError
	var _ func(*domain.Step, string, error, error) error = StepExecutionError

	want := []int{101, 102, 103}
	if got := []int{ExitValidation, ExitConfiguration, ExitInternal}; !slices.Equal(got, want) {
		t.Fatalf("exit codes = %v, want %v", got, want)
	}
}

func TestBuildPreservesCodeIdentityOrderAndOptionalDetails(t *testing.T) {
	staticErr := errors.New("static")
	originalErr := errors.New("original")

	tests := map[string]struct {
		original error
		details  []any
		wantText string
		wantWrap []error
	}{
		"static details and original": {
			original: originalErr,
			details:  []any{"field ", "name"},
			wantText: "static: field name: original",
			wantWrap: []error{staticErr, originalErr},
		},
		"static without optional components": {
			wantText: "static",
			wantWrap: []error{staticErr},
		},
		"empty details are omitted": {
			original: originalErr,
			details:  []any{""},
			wantText: "static: original",
			wantWrap: []error{staticErr, originalErr},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			built := Build(27, staticErr, test.original, test.details...)
			exitErr, ok := built.(*ExitError)
			if !ok {
				t.Fatalf("Build() type = %T, want *ExitError", built)
			}
			if got := exitErr.ExitCode(); got != 27 {
				t.Fatalf("ExitCode() = %d, want 27", got)
			}
			if got := exitErr.Error(); got != test.wantText {
				t.Fatalf("Error() = %q, want %q", got, test.wantText)
			}
			if got := exitErr.Unwrap(); !slices.Equal(got, test.wantWrap) {
				t.Fatalf("Unwrap() = %v, want %v", got, test.wantWrap)
			}
			if !errors.Is(built, staticErr) {
				t.Fatal("Build() did not preserve the static error")
			}
			if test.original != nil && !errors.Is(built, test.original) {
				t.Fatal("Build() did not preserve the original error")
			}
		})
	}
}

func TestCodeLookupFallbackAndNilHandling(t *testing.T) {
	cause := errors.New("cause")
	coded := WithExitCode(27, cause)

	if got := Code(coded, ExitInternal); got != 27 {
		t.Fatalf("Code() = %d, want 27", got)
	}
	if !errors.Is(coded, cause) {
		t.Fatal("WithExitCode() did not preserve the cause")
	}
	if got := Code(fmt.Errorf("wrapped: %w", coded), ExitInternal); got != 27 {
		t.Fatalf("Code() through wrapper = %d, want 27", got)
	}
	if got := Code(errors.New("uncoded"), ExitInternal); got != ExitInternal {
		t.Fatalf("Code() fallback = %d, want %d", got, ExitInternal)
	}
	if got := Code(nil, ExitInternal); got != 0 {
		t.Fatalf("Code(nil) = %d, want 0", got)
	}
	if got := Build(1, nil, cause); got != nil {
		t.Fatalf("Build() with nil static error = %v, want nil", got)
	}
	if got := WithExitCode(1, nil); got != nil {
		t.Fatalf("WithExitCode() with nil error = %v, want nil", got)
	}
}

func TestDefinitionHelpersUseConfigurationCodeAndProvenance(t *testing.T) {
	staticErr := errors.New("invalid definition")
	originalErr := errors.New("invalid value")

	tests := map[string]struct {
		build    func() error
		wantText string
	}{
		"defaults file and yaml path": {
			build: func() error {
				definition := &domain.DefaultsDefinition{File: &domain.File{Path: "config/defaults.yaml"}}
				return DefaultsDefinitionError(definition, ".spec.timeout", staticErr, originalErr)
			},
			wantText: "invalid definition: file config/defaults.yaml, yaml path .spec.timeout: invalid value",
		},
		"steps file only": {
			build: func() error {
				definition := &domain.StepsDefinition{File: &domain.File{Path: "steps.yaml"}}
				return StepDefinitionError(definition, "", staticErr, originalErr)
			},
			wantText: "invalid definition: file steps.yaml: invalid value",
		},
		"nil defaults definition": {
			build: func() error {
				return DefaultsDefinitionError(nil, ".spec", staticErr, originalErr)
			},
			wantText: "invalid definition: yaml path .spec: invalid value",
		},
		"defaults definition without file": {
			build: func() error {
				return DefaultsDefinitionError(&domain.DefaultsDefinition{}, "", staticErr, originalErr)
			},
			wantText: "invalid definition: invalid value",
		},
		"nil steps definition": {
			build: func() error {
				return StepDefinitionError(nil, ".spec.steps[0]", staticErr, originalErr)
			},
			wantText: "invalid definition: yaml path .spec.steps[0]: invalid value",
		},
		"steps definition without file": {
			build: func() error {
				return StepDefinitionError(&domain.StepsDefinition{}, "", staticErr, originalErr)
			},
			wantText: "invalid definition: invalid value",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			err := test.build()
			if got := Code(err, ExitInternal); got != ExitConfiguration {
				t.Fatalf("Code() = %d, want %d", got, ExitConfiguration)
			}
			if got := err.Error(); got != test.wantText {
				t.Fatalf("Error() = %q, want %q", got, test.wantText)
			}
			if !errors.Is(err, staticErr) || !errors.Is(err, originalErr) {
				t.Fatal("definition helper did not preserve both error identities")
			}
		})
	}
}

func TestStepExecutionErrorAlwaysUsesNonzeroCodeAndSafeProvenance(t *testing.T) {
	staticErr := errors.New("execution failed")
	cause := errors.New("tool failed")
	stepWithFile := &domain.Step{
		Definition: &domain.StepsDefinition{File: &domain.File{Path: "suite/steps.yaml"}},
	}

	tests := map[string]struct {
		step     *domain.Step
		yamlPath string
		original error
		wantCode int
		wantText string
	}{
		"preserves coded cause and full provenance": {
			step:     stepWithFile,
			yamlPath: ".spec.steps[2].request",
			original: WithExitCode(19, cause),
			wantCode: 19,
			wantText: "execution failed: file suite/steps.yaml, yaml path .spec.steps[2].request: tool failed",
		},
		"uncoded cause uses internal code": {
			step:     nil,
			original: cause,
			wantCode: ExitInternal,
			wantText: "execution failed: tool failed",
		},
		"nil cause cannot produce success": {
			step:     &domain.Step{},
			yamlPath: ".request",
			wantCode: ExitInternal,
			wantText: "execution failed: yaml path .request",
		},
		"zero coded cause cannot produce success": {
			step:     &domain.Step{Definition: &domain.StepsDefinition{}},
			original: WithExitCode(0, cause),
			wantCode: ExitInternal,
			wantText: "execution failed: tool failed",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			err := StepExecutionError(test.step, test.yamlPath, staticErr, test.original)
			if err == nil {
				t.Fatal("StepExecutionError() = nil")
			}
			if got := Code(err, 1); got != test.wantCode {
				t.Fatalf("Code() = %d, want %d", got, test.wantCode)
			}
			if got := err.Error(); got != test.wantText {
				t.Fatalf("Error() = %q, want %q", got, test.wantText)
			}
			if !errors.Is(err, staticErr) {
				t.Fatal("StepExecutionError() did not preserve the static error")
			}
			if test.original != nil && !errors.Is(err, cause) {
				t.Fatal("StepExecutionError() did not preserve the original cause")
			}
		})
	}
}
