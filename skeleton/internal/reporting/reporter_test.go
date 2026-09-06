package reporting

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/divilla/apihydra/skeleton/internal/domain"
)

func TestWorkingDirectoryUsesInjectedWriter(t *testing.T) {
	var output bytes.Buffer
	report := NewReporter(&output, false)

	if err := report.WorkingDirectory("/work"); err != nil {
		t.Fatalf("WorkingDirectory() error = %v", err)
	}
	if got, want := output.String(), "Working Directory: /work\n\n"; got != want {
		t.Fatalf("WorkingDirectory() output = %q, want %q", got, want)
	}
}

func TestValidationMethodsUseNonfatalReportingContract(t *testing.T) {
	var output bytes.Buffer
	report := NewReporter(&output, false)

	if err := report.ValidationTypes(context.Background(), &domain.Step{}, `{"type":"string"}`); err != nil {
		t.Fatalf("ValidationTypes() error = %v", err)
	}
	if err := report.ValidationStatus(context.Background(), &domain.Step{}, errors.New("status mismatch")); err != nil {
		t.Fatalf("ValidationStatus() error = %v", err)
	}
	if err := report.ValidationBody(context.Background(), &domain.Step{}, "body diff"); err != nil {
		t.Fatalf("ValidationBody() error = %v", err)
	}
}

func TestReporterStageAndSuccessSignatures(t *testing.T) {
	var _ func(*Reporter, context.Context, []*domain.Directory) error = (*Reporter).BeginStage
	var _ func(*Reporter, context.Context) error = (*Reporter).EndStage
	var _ func(*Reporter, context.Context, *domain.StepsDefinition) error = (*Reporter).Success
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

func TestValidationLineUsesOriginalExpectationKey(t *testing.T) {
	const source = `app: apihydra
kind: steps
spec:
  steps:
    - response: {expected_status: 200}
    - request:
        body: |
          expected_status: 999
      response:
        "expected_status":
          200
        expected_types:
          .id: [string]
        expected_body: |
          {"id": "expected"}
`
	file := &domain.File{Bytes: []byte(source)}
	step := &domain.Step{Index: 1, Definition: &domain.StepsDefinition{File: file}}
	for field, want := range map[string]string{
		"expected_status": "10", "expected_types": "12", "expected_body": "14", "missing": "6",
	} {
		if got := validationLine(step, field); got != want {
			t.Errorf("validationLine(%s) = %s, want %s", field, got, want)
		}
	}
	step.Index = 0
	if got := validationLine(step, "expected_status"); got != "5" {
		t.Errorf("flow mapping line = %s, want 5", got)
	}
}

func TestValidationLineHandlesUnavailableSources(t *testing.T) {
	for _, step := range []*domain.Step{
		nil,
		{},
		{Index: -1},
		{Definition: &domain.StepsDefinition{}},
		{Definition: &domain.StepsDefinition{File: &domain.File{}}},
		{Definition: &domain.StepsDefinition{File: &domain.File{Bytes: []byte("spec: [")}}},
		{Index: 2, Definition: &domain.StepsDefinition{File: &domain.File{Bytes: []byte("spec: {steps: []}")}}},
	} {
		if got := validationLine(step, "expected_status"); got != "unknown" {
			t.Errorf("validationLine(%+v) = %s, want unknown", step, got)
		}
	}
}
