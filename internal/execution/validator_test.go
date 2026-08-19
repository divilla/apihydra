package execution

import (
	"apih/internal/domain"
	"context"
	"encoding/json"
	"reflect"
	"testing"
)

func TestValidatorPublicContract(t *testing.T) {
	var validateTypes func(*Validator, context.Context, *domain.Step) []error = (*Validator).ValidateTypes
	var validateExpected func(*Validator, context.Context, *domain.Step) error = (*Validator).ValidateExpected
	_, _ = validateTypes, validateExpected

	validator := &Validator{}
	if got := reflect.TypeOf(*validator).NumField(); got != 0 {
		t.Fatalf("Validator field count = %d, want 0", got)
	}

	tests := map[string]struct {
		err  error
		want string
	}{
		"validation": {ErrValidation, "validation error"},
		"fatal":      {ErrValidatorFatal, "fatal validator error"},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			if got := test.err.Error(); got != test.want {
				t.Fatalf("error text = %q, want %q", got, test.want)
			}
		})
	}
}

func TestValidatorMethodsRemainIndependentNoOpPhases(t *testing.T) {
	validator := &Validator{}
	step := populatedValidatorStep()
	before := marshalValidatorStep(t, step)

	if err := validator.ValidateExpected(context.Background(), step); err != nil {
		t.Fatalf("ValidateExpected() error = %v, want nil", err)
	}
	if failures := validator.ValidateTypes(context.Background(), step); failures != nil {
		t.Fatalf("ValidateTypes() = %v, want nil", failures)
	}
	if after := marshalValidatorStep(t, step); after != before {
		t.Fatalf("validation mutated Step:\nbefore: %s\n after: %s", before, after)
	}
}

func populatedValidatorStep() *domain.Step {
	step := &domain.Step{
		Vars:  map[string]domain.YAMLString{"account": `{"id":42}`},
		Debug: true,
		Index: 3,
	}
	step.Request.Method = "POST"
	step.Request.BaseURL = "https://example.test"
	step.Request.BasePath = "/api"
	step.Request.Path = "/accounts"
	step.Request.Headers = map[string]string{"Content-Type": "application/json"}
	step.Request.Timeout = 10
	step.Request.Retries = 2
	step.Request.Query = "active=true"
	step.Request.Body = `{"id":42}`
	step.Response.Status = []int{200, 201}
	step.Response.Body = `{"id":42,"active":true}`
	step.Response.Expected = `{"id":42}`
	step.Response.Types = map[string][]string{"id": {"number"}}
	step.Response.Capture = map[string]domain.YAMLString{"accountID": ".id"}
	return step
}

func marshalValidatorStep(t *testing.T, step *domain.Step) string {
	t.Helper()

	encoded, err := json.Marshal(step)
	if err != nil {
		t.Fatalf("marshal Step: %v", err)
	}
	return string(encoded)
}
