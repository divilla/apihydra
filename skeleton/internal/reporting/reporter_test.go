package reporting

import (
	"apih/skeleton/internal/domain"
	"bytes"
	"context"
	"errors"
	"testing"
)

func TestWorkingDirectoryUsesInjectedWriter(t *testing.T) {
	var output bytes.Buffer
	report := NewReporter(&output)

	if err := report.WorkingDirectory("/work"); err != nil {
		t.Fatalf("WorkingDirectory() error = %v", err)
	}
	if got, want := output.String(), "Working Directory: /work\n\n"; got != want {
		t.Fatalf("WorkingDirectory() output = %q, want %q", got, want)
	}
}

func TestValidationMethodsUseNonfatalReportingContract(t *testing.T) {
	var output bytes.Buffer
	report := NewReporter(&output)

	if err := report.ValidationTypes(context.Background(), &domain.Step{}, errors.New("type mismatch")); err != nil {
		t.Fatalf("ValidationTypes() error = %v", err)
	}
	if err := report.ValidationStatus(context.Background(), &domain.Step{}, errors.New("status mismatch")); err != nil {
		t.Fatalf("ValidationStatus() error = %v", err)
	}
	if err := report.ValidationBody(context.Background(), &domain.Step{}, "body diff"); err != nil {
		t.Fatalf("ValidationBody() error = %v", err)
	}
}

func TestValidationErrorLabelsMatchSplitResponseContract(t *testing.T) {
	tests := map[string]struct {
		got  error
		want string
	}{
		"types":  {got: ErrTypeValidation, want: "type validation failed for"},
		"status": {got: ErrStatusValidation, want: "response status does not match expected"},
		"body":   {got: ErrBodyValidation, want: "response body does not match expected"},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			if got := test.got.Error(); got != test.want {
				t.Fatalf("error = %q, want %q", got, test.want)
			}
		})
	}
}
