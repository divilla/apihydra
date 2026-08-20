package domain

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/goccy/go-yaml"
)

func TestStepResponseExpectationSchema(t *testing.T) {
	definitionYAML := []byte(`
app: apihydra
kind: steps
spec:
  steps:
    - response:
        expected_status: 201
        expected_body: '{"created":true}'
        expected_types:
          .created: [boolean]
`)

	var definition StepsDefinition
	if err := yaml.Unmarshal(definitionYAML, &definition); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if got, want := definition.Spec.Steps[0].Response.ExpectedStatus, 201; got != want {
		t.Fatalf("ExpectedStatus = %d, want %d", got, want)
	}
	if got, want := definition.Spec.Steps[0].Response.ExpectedBody, YAMLString(`{"created":true}`); got != want {
		t.Fatalf("ExpectedBody = %q, want %q", got, want)
	}
	if got, want := definition.Spec.Steps[0].Response.ExpectedTypes[".created"], []string{"boolean"}; !slices.Equal(got, want) {
		t.Fatalf("ExpectedTypes[.created] = %v, want %v", got, want)
	}
	if got := definition.Spec.Steps[0].Response.ActualStatus; got != 0 {
		t.Fatalf("ActualStatus = %d, want zero before execution", got)
	}
	if got := definition.Spec.Steps[0].Response.ActualBody; got != "" {
		t.Fatalf("ActualBody = %q, want empty before execution", got)
	}

	encoded, err := json.Marshal(definition.Spec.Steps[0])
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	got := string(encoded)
	if !strings.Contains(got, `"expected_status":201`) {
		t.Fatalf("JSON = %s, want numeric expected_status", got)
	}
	if !strings.Contains(got, `"expected_body":"{\"created\":true}"`) {
		t.Fatalf("JSON = %s, want expected_body", got)
	}
	if !strings.Contains(got, `"expected_types":{".created":["boolean"]}`) {
		t.Fatalf("JSON = %s, want expected_types", got)
	}
	if !strings.Contains(got, `"actual_status":0`) {
		t.Fatalf("JSON = %s, want numeric actual_status", got)
	}
	if !strings.Contains(got, `"actual_body":""`) {
		t.Fatalf("JSON = %s, want actual_body", got)
	}
	if strings.Contains(got, `"status"`) || strings.Contains(got, `"expected"`) || strings.Contains(got, `"types"`) {
		t.Fatalf("JSON = %s, contains legacy response field", got)
	}
}

func TestStepResponseOmittedExpectedStatusUsesAnySentinel(t *testing.T) {
	definitionYAML := []byte(`
app: apihydra
kind: steps
spec:
  steps:
    - response:
        expected_body: '{}'
`)

	var definition StepsDefinition
	if err := yaml.Unmarshal(definitionYAML, &definition); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if got := definition.Spec.Steps[0].Response.ExpectedStatus; got != 0 {
		t.Fatalf("ExpectedStatus = %d, want <any> zero value", got)
	}
}

func TestStepResponseRejectsMultipleExpectedStatuses(t *testing.T) {
	definitionYAML := []byte(`
app: apihydra
kind: steps
spec:
  steps:
    - response:
        expected_status: [200, 201]
`)

	var definition StepsDefinition
	if err := yaml.Unmarshal(definitionYAML, &definition); err == nil {
		t.Fatal("Unmarshal() error = nil, want multiple expected statuses rejected")
	}
}
