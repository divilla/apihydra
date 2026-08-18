package errs

import (
	"errors"
	"fmt"
	"testing"
)

func TestCode(t *testing.T) {
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
	if got := Code(nil, ExitInternal); got != ExitSuccess {
		t.Fatalf("Code(nil) = %d, want %d", got, ExitSuccess)
	}
}

func TestErrorsUseProductExitCodes(t *testing.T) {
	definitionError := errors.New("invalid definition")
	executionError := errors.New("execution failed")
	if got := Code(DefaultsDefinitionError(nil, "", definitionError, nil), ExitInternal); got != ExitConfiguration {
		t.Fatalf("DefaultsDefinitionError exit code = %d, want %d", got, ExitConfiguration)
	}
	if got := Code(StepDefinitionError(nil, "", definitionError, nil), ExitInternal); got != ExitConfiguration {
		t.Fatalf("StepDefinitionError exit code = %d, want %d", got, ExitConfiguration)
	}
	if got := Code(StepExecutionError(nil, "", executionError, WithExitCode(19, errors.New("tool"))), ExitInternal); got != 19 {
		t.Fatalf("StepExecutionError exit code = %d, want 19", got)
	}
}

func TestBuildPreservesStaticAndOriginalErrors(t *testing.T) {
	staticErr := errors.New("static")
	originalErr := errors.New("original")
	built := Build(ExitConfiguration, staticErr, originalErr, "field ", "name")

	if !errors.Is(built, staticErr) {
		t.Fatal("Build() did not preserve the originating package's static error")
	}
	if !errors.Is(built, originalErr) {
		t.Fatal("Build() did not preserve the original error")
	}
	if got := Code(built, ExitInternal); got != ExitConfiguration {
		t.Fatalf("Build() exit code = %d, want %d", got, ExitConfiguration)
	}
}

func TestProductExitCodeContract(t *testing.T) {
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
