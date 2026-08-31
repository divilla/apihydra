package domain

import (
	"encoding/json"
	"errors"
	"slices"
	"testing"

	"github.com/goccy/go-yaml"
)

func TestYAMLStringPreservesValueThroughYAMLRoundTrip(t *testing.T) {
	var _ yaml.InterfaceMarshaler = YAMLString("")
	var _ yaml.InterfaceUnmarshaler = (*YAMLString)(nil)

	values := []YAMLString{
		`{"message":"quote \" slash \\ newline \n tab \t","template":"$value"}`,
		"  leading\nline\twith actual whitespace  ",
		"plain: value # not YAML syntax",
		"",
	}
	for _, want := range values {
		encoded, err := yaml.Marshal(want)
		if err != nil {
			t.Fatalf("Marshal(%q) error = %v", want, err)
		}
		var got YAMLString
		if err := yaml.Unmarshal(encoded, &got); err != nil {
			t.Fatalf("Unmarshal(%q) error = %v", encoded, err)
		}
		if got != want {
			t.Errorf("YAMLString round trip = %q, want %q", got, want)
		}
	}

	want := YAMLString("unchanged")
	got := want
	sentinel := errors.New("decode failed")
	if err := got.UnmarshalYAML(func(interface{}) error { return sentinel }); !errors.Is(err, sentinel) {
		t.Fatalf("UnmarshalYAML() error = %v, want %v", err, sentinel)
	}
	if got != want {
		t.Fatalf("UnmarshalYAML() after error = %q, want %q", got, want)
	}
}

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

func TestDefaultsDisableCookiesPreservesPresence(t *testing.T) {
	tests := map[string]struct {
		yaml string
		want *bool
	}{
		"absent inherits": {yaml: "{}", want: nil},
		"true disables":   {yaml: "disable_cookies: true", want: boolPointer(true)},
		"false enables":   {yaml: "disable_cookies: false", want: boolPointer(false)},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			var defaults Defaults
			if err := yaml.Unmarshal([]byte(test.yaml), &defaults); err != nil {
				t.Fatalf("Unmarshal() error = %v", err)
			}
			if test.want == nil {
				if defaults.DisableCookies != nil {
					t.Fatalf("DisableCookies = %v, want nil", *defaults.DisableCookies)
				}
				return
			}
			if defaults.DisableCookies == nil || *defaults.DisableCookies != *test.want {
				t.Fatalf("DisableCookies = %v, want %v", defaults.DisableCookies, *test.want)
			}
		})
	}
}

func TestDefaultsDisableCookiesJSONPreservesPresence(t *testing.T) {
	tests := map[string]struct {
		value string
		want  *bool
	}{
		"absent inherits": {value: `{}`, want: nil},
		"true disables":   {value: `{"disable_cookies":true}`, want: boolPointer(true)},
		"false enables":   {value: `{"disable_cookies":false}`, want: boolPointer(false)},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			var defaults Defaults
			if err := json.Unmarshal([]byte(test.value), &defaults); err != nil {
				t.Fatalf("Unmarshal() error = %v", err)
			}
			if test.want == nil {
				if defaults.DisableCookies != nil {
					t.Fatalf("DisableCookies = %v, want nil", *defaults.DisableCookies)
				}
				return
			}
			if defaults.DisableCookies == nil || *defaults.DisableCookies != *test.want {
				t.Fatalf("DisableCookies = %v, want %v", defaults.DisableCookies, *test.want)
			}
		})
	}
}

func boolPointer(value bool) *bool {
	return &value
}
