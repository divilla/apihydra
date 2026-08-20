package reporter

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
	if err := report.ValidationExpected(context.Background(), &domain.Step{}, errors.New("expected mismatch")); err != nil {
		t.Fatalf("ValidationExpected() error = %v", err)
	}
}
