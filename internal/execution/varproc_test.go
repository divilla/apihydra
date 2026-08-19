package execution

import (
	"apih/internal/domain"
	"context"
	"encoding/json"
	"reflect"
	"testing"
)

func TestVariableProcessorPublicContract(t *testing.T) {
	var constructor func() *VariableProcessor = NewVariableProcessor
	var load func(*VariableProcessor, context.Context, *domain.Step) (int, error) = (*VariableProcessor).Load
	var parseRequestBody func(*VariableProcessor, context.Context, *domain.Step) (int, error) = (*VariableProcessor).ParseRequestBody
	var parseResponseExpected func(*VariableProcessor, context.Context, *domain.Step) (int, error) = (*VariableProcessor).ParseResponseExpected
	var capture func(*VariableProcessor, context.Context, *domain.Step) (int, error) = (*VariableProcessor).Capture
	_, _, _, _, _ = constructor, load, parseRequestBody, parseResponseExpected, capture

	processor := NewVariableProcessor()
	if processor == nil {
		t.Fatal("NewVariableProcessor() = nil, want an empty processor")
	}
	if got := reflect.TypeOf(*processor).NumField(); got != 0 {
		t.Fatalf("VariableProcessor field count = %d, want 0", got)
	}
}

func TestVariableProcessorMethodsRemainIndependentNoOpPhases(t *testing.T) {
	type phase struct {
		name string
		run  func(*VariableProcessor, context.Context, *domain.Step) (int, error)
	}
	phases := []phase{
		{name: "load vars", run: (*VariableProcessor).Load},
		{name: "parse request body", run: (*VariableProcessor).ParseRequestBody},
		{name: "parse response expected", run: (*VariableProcessor).ParseResponseExpected},
		{name: "capture response", run: (*VariableProcessor).Capture},
	}

	for _, test := range phases {
		t.Run(test.name, func(t *testing.T) {
			processor := NewVariableProcessor()
			step := populatedVariableStep()
			before := marshalStep(t, step)

			exitCode, err := test.run(processor, context.Background(), step)
			if exitCode != 0 || err != nil {
				t.Fatalf("phase result = (%d, %v), want (0, nil)", exitCode, err)
			}
			if after := marshalStep(t, step); after != before {
				t.Fatalf("phase mutated Step:\nbefore: %s\n after: %s", before, after)
			}
		})
	}

	t.Run("does not impose phase ordering", func(t *testing.T) {
		processor := NewVariableProcessor()
		step := populatedVariableStep()
		before := marshalStep(t, step)

		for index := len(phases) - 1; index >= 0; index-- {
			exitCode, err := phases[index].run(processor, context.Background(), step)
			if exitCode != 0 || err != nil {
				t.Fatalf("%s result = (%d, %v), want (0, nil)", phases[index].name, exitCode, err)
			}
		}
		if after := marshalStep(t, step); after != before {
			t.Fatalf("reverse phase calls mutated Step:\nbefore: %s\n after: %s", before, after)
		}
	})
}

func populatedVariableStep() *domain.Step {
	step := &domain.Step{
		Vars: map[string]domain.YAMLString{
			"account": `{"id":42}`,
		},
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
	step.Request.Body = `{"id":"${account.id}"}`
	step.Response.Status = []int{200, 201}
	step.Response.Body = `{"id":42,"active":true}`
	step.Response.Expected = `{"id":"${account.id}"}`
	step.Response.Types = map[string][]string{"id": {"number"}}
	step.Response.Capture = map[string]domain.YAMLString{"accountID": ".id"}
	return step
}

func marshalStep(t *testing.T, step *domain.Step) string {
	t.Helper()

	encoded, err := json.Marshal(step)
	if err != nil {
		t.Fatalf("marshal Step: %v", err)
	}
	return string(encoded)
}
