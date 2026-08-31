package domain

import (
	"encoding/json"
	"errors"
	"reflect"
	"slices"
	"strings"
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

func TestDomainSchemaMatchesReference(t *testing.T) {
	if got, want := []DocumentKind{KindRoot, KindDefaults, KindSteps}, []DocumentKind{"root", "defaults", "steps"}; !slices.Equal(got, want) {
		t.Fatalf("document kinds = %v, want %v", got, want)
	}

	assertFields(t, reflect.TypeOf(Suite{}), []fieldSchema{
		{"WorkDir", reflect.TypeOf(""), ""},
		{"Root", reflect.TypeOf((*Directory)(nil)), ""},
	})
	assertFields(t, reflect.TypeOf(Directory{}), []fieldSchema{
		{"Stage", reflect.TypeOf(0), ""},
		{"Path", reflect.TypeOf(""), ""},
		{"Parent", reflect.TypeOf((*Directory)(nil)), ""},
		{"Children", reflect.TypeOf([]*Directory(nil)), ""},
		{"Files", reflect.TypeOf([]*File(nil)), ""},
		{"DefaultsFile", reflect.TypeOf((*File)(nil)), ""},
		{"StepsFiles", reflect.TypeOf([]*File(nil)), ""},
		{"DefaultsDefinition", reflect.TypeOf((*DefaultsDefinition)(nil)), ""},
		{"StepsDefinitions", reflect.TypeOf([]*StepsDefinition(nil)), ""},
		{"ResolvedDefaults", reflect.TypeOf(Defaults{}), ""},
		{"ResolvedSteps", reflect.TypeOf([][]Step(nil)), ""},
		{"RuntimeSteps", reflect.TypeOf([][]Step(nil)), ""},
	})
	assertFields(t, reflect.TypeOf(File{}), []fieldSchema{
		{"Stage", reflect.TypeOf(0), ""},
		{"Path", reflect.TypeOf(""), ""},
		{"Kind", reflect.TypeOf(DocumentKind("")), ""},
		{"Bytes", reflect.TypeOf([]byte(nil)), ""},
		{"Directory", reflect.TypeOf((*Directory)(nil)), ""},
	})
	assertFields(t, reflect.TypeOf(BaseDefinition{}), []fieldSchema{
		{"App", reflect.TypeOf(""), `yaml:"app" json:"app"`},
		{"Kind", reflect.TypeOf(""), `yaml:"kind" json:"kind"`},
		{"Spec", reflect.TypeOf(YAMLString("")), `yaml:"spec" json:"spec"`},
	})
	assertFields(t, reflect.TypeOf(DefaultsDefinition{}), []fieldSchema{
		{"App", reflect.TypeOf(""), `yaml:"app" json:"app"`},
		{"Kind", reflect.TypeOf(DocumentKind("")), `yaml:"kind" json:"kind"`},
		{"Metadata", reflect.TypeOf(Metadata{}), `yaml:"metadata" json:"metadata"`},
		{"Spec", reflect.TypeOf(Defaults{}), `yaml:"spec" json:"spec"`},
		{"File", reflect.TypeOf((*File)(nil)), `yaml:"-" json:"-"`},
	})
	assertFields(t, reflect.TypeOf(StepsDefinition{}), []fieldSchema{
		{"App", reflect.TypeOf(""), `yaml:"app" json:"app"`},
		{"Kind", reflect.TypeOf(DocumentKind("")), `yaml:"kind" json:"kind"`},
		{"Metadata", reflect.TypeOf(Metadata{}), `yaml:"metadata" json:"metadata"`},
		{"Spec", nil, `yaml:"spec" json:"spec"`},
		{"File", reflect.TypeOf((*File)(nil)), `yaml:"-" json:"-"`},
	})
	assertFields(t, reflect.TypeOf(StepsDefinition{}).Field(3).Type, []fieldSchema{
		{"Defaults", reflect.TypeOf(Defaults{}), `yaml:"defaults" json:"defaults"`},
		{"Steps", reflect.TypeOf([]Step(nil)), `yaml:"steps" json:"steps"`},
	})
	assertFields(t, reflect.TypeOf(Metadata{}), []fieldSchema{
		{"Name", reflect.TypeOf(""), `yaml:"name" json:"name"`},
		{"Labels", reflect.TypeOf([]string(nil)), `yaml:"labels" json:"labels"`},
	})
	assertFields(t, reflect.TypeOf(Defaults{}), []fieldSchema{
		{"BaseURL", reflect.TypeOf(""), `yaml:"base_url" json:"base_url"`},
		{"BasePath", reflect.TypeOf(""), `yaml:"base_path" json:"base_path"`},
		{"Headers", reflect.TypeOf(map[string]string(nil)), `yaml:"headers" json:"headers"`},
		{"DisableCookies", reflect.TypeOf((*bool)(nil)), `yaml:"disable_cookies" json:"disable_cookies"`},
		{"Timeout", reflect.TypeOf(0), `yaml:"timeout" json:"timeout"`},
		{"Retries", reflect.TypeOf(0), `yaml:"retries" json:"retries"`},
	})
	assertFields(t, reflect.TypeOf(Step{}), []fieldSchema{
		{"Index", reflect.TypeOf(0), `yaml:"-" json:"index"`},
		{"Vars", reflect.TypeOf(map[string]YAMLString(nil)), `yaml:"vars" json:"vars"`},
		{"Request", nil, `yaml:"request" json:"request"`},
		{"Response", nil, `yaml:"response" json:"response"`},
		{"Debug", reflect.TypeOf(false), `yaml:"debug" json:"debug"`},
		{"RawCurl", reflect.TypeOf(""), `yaml:"-" json:"-"`},
		{"Definition", reflect.TypeOf((*StepsDefinition)(nil)), `yaml:"-" json:"-"`},
	})
	assertFields(t, reflect.TypeOf(Step{}).Field(2).Type, []fieldSchema{
		{"Path", reflect.TypeOf(""), `yaml:"path" json:"path"`},
		{"Method", reflect.TypeOf(""), `yaml:"method" json:"method"`},
		{"Query", reflect.TypeOf(""), `yaml:"query" json:"query"`},
		{"Body", reflect.TypeOf(YAMLString("")), `yaml:"body" json:"body"`},
		{"Defaults", reflect.TypeOf(Defaults{}), `yaml:"defaults" json:"defaults"`},
	})
	assertFields(t, reflect.TypeOf(Step{}).Field(3).Type, []fieldSchema{
		{"ExpectedStatus", reflect.TypeOf(0), `yaml:"expected_status" json:"expected_status"`},
		{"ActualStatus", reflect.TypeOf(0), `yaml:"actual_status" json:"actual_status"`},
		{"ExpectedBody", reflect.TypeOf(YAMLString("")), `yaml:"expected_body" json:"expected_body"`},
		{"ActualBody", reflect.TypeOf(YAMLString("")), `yaml:"actual_body" json:"actual_body"`},
		{"ExpectedTypes", reflect.TypeOf(map[string][]string(nil)), `yaml:"expected_types" json:"expected_types"`},
		{"Capture", reflect.TypeOf(map[string]YAMLString(nil)), `yaml:"capture" json:"capture"`},
	})
}

func TestUnifiedDefaultsSchemaDecodesAndEncodesNestedValues(t *testing.T) {
	definitionYAML := []byte(`
app: apihydra
kind: steps
spec:
  defaults:
    base_path: /v1
    headers:
      Accept: application/json
    timeout: 8
    disable_cookies: true
  steps:
    - request:
        path: /items
        defaults:
          base_url: https://example.test
          disable_cookies: false
          retries: 2
      response:
        expected_body: '{"expected":true}'
        actual_body: '{"actual":true}'
`)

	var definition StepsDefinition
	if err := yaml.Unmarshal(definitionYAML, &definition); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if got, want := definition.Spec.Defaults, (Defaults{
		BasePath:       "/v1",
		Headers:        map[string]string{"Accept": "application/json"},
		DisableCookies: boolPointer(true),
		Timeout:        8,
	}); !reflect.DeepEqual(got, want) {
		t.Fatalf("steps defaults = %+v, want %+v", got, want)
	}
	step := definition.Spec.Steps[0]
	if got, want := step.Request.Defaults, (Defaults{BaseURL: "https://example.test", DisableCookies: boolPointer(false), Retries: 2}); !reflect.DeepEqual(got, want) {
		t.Fatalf("request defaults = %+v, want %+v", got, want)
	}
	if step.Response.ExpectedBody != `{"expected":true}` || step.Response.ActualBody != `{"actual":true}` {
		t.Fatalf("response bodies = %q/%q, want YAMLString values", step.Response.ExpectedBody, step.Response.ActualBody)
	}

	encodedYAML, err := yaml.Marshal(definition)
	if err != nil {
		t.Fatalf("Marshal YAML error = %v", err)
	}
	var roundTrip StepsDefinition
	if err := yaml.Unmarshal(encodedYAML, &roundTrip); err != nil {
		t.Fatalf("Unmarshal round-trip YAML error = %v", err)
	}
	if !reflect.DeepEqual(roundTrip.Spec.Defaults, definition.Spec.Defaults) ||
		roundTrip.Spec.Steps[0].Request.Defaults.BaseURL != step.Request.Defaults.BaseURL ||
		roundTrip.Spec.Steps[0].Request.Defaults.Retries != step.Request.Defaults.Retries ||
		roundTrip.Spec.Steps[0].Response.ActualBody != step.Response.ActualBody {
		t.Fatalf("YAML round trip = %+v, want nested defaults and YAMLString actual body", roundTrip)
	}

	step.Index = 4
	step.RawCurl = "curl --header Authorization: Bearer complete-secret"
	encodedJSON, err := json.Marshal(step)
	if err != nil {
		t.Fatalf("Marshal JSON error = %v", err)
	}
	var document map[string]any
	if err := json.Unmarshal(encodedJSON, &document); err != nil {
		t.Fatalf("Unmarshal JSON error = %v", err)
	}
	request, ok := document["request"].(map[string]any)
	if !ok {
		t.Fatalf("JSON request = %#v, want object", document["request"])
	}
	defaults, ok := request["defaults"].(map[string]any)
	if !ok {
		t.Fatalf("JSON request.defaults = %#v, want nested object", request["defaults"])
	}
	if got := defaults["base_url"]; got != "https://example.test" {
		t.Fatalf("JSON request.defaults.base_url = %#v, want https://example.test", got)
	}
	if got, exists := defaults["base_path"]; !exists || got != "" {
		t.Fatalf("JSON request.defaults.base_path = %#v (exists %t), want empty string", got, exists)
	}
	for _, direct := range []string{"base_url", "base_path", "headers", "disable_cookies", "timeout", "retries"} {
		if _, exists := request[direct]; exists {
			t.Fatalf("JSON request contains direct defaults field %q", direct)
		}
	}
	if got := document["index"]; got != float64(4) {
		t.Fatalf("JSON index = %#v, want 4", got)
	}
	for _, omitted := range []string{"RawCurl", "raw_curl", "Definition", "definition"} {
		if _, exists := document[omitted]; exists {
			t.Fatalf("JSON contains runtime-only field %q", omitted)
		}
	}
	if strings.Contains(string(encodedYAML), "curl --header") || strings.Contains(string(encodedJSON), "complete-secret") {
		t.Fatal("runtime-only RawCurl was serialized")
	}
}

func TestDefaultsDisableCookiesPreservesPresence(t *testing.T) {
	tests := map[string]struct {
		yaml string
		json string
		want *bool
	}{
		"absent inherits": {yaml: "{}", json: `{}`, want: nil},
		"true disables":   {yaml: "disable_cookies: true", json: `{"disable_cookies":true}`, want: boolPointer(true)},
		"false enables":   {yaml: "disable_cookies: false", json: `{"disable_cookies":false}`, want: boolPointer(false)},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			for format, decode := range map[string]func(*Defaults) error{
				"yaml": func(defaults *Defaults) error { return yaml.Unmarshal([]byte(test.yaml), defaults) },
				"json": func(defaults *Defaults) error { return json.Unmarshal([]byte(test.json), defaults) },
			} {
				t.Run(format, func(t *testing.T) {
					var defaults Defaults
					if err := decode(&defaults); err != nil {
						t.Fatalf("decode error = %v", err)
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
		})
	}
}

func boolPointer(value bool) *bool {
	return &value
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
	if got := definition.Spec.Steps[0].Response.ActualStatus; got != 0 {
		t.Fatalf("ActualStatus = %d, want zero before execution", got)
	}
	if got := definition.Spec.Steps[0].Response.ActualBody; got != "" {
		t.Fatalf("ActualBody = %q, want empty before execution", got)
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

func TestStepProvenanceHelpers(t *testing.T) {
	directory := &Directory{Stage: 7, Path: "/parent/child"}
	file := &File{Path: "/parent/child/steps.yaml", Directory: directory}
	definition := &StepsDefinition{File: file}
	step := &Step{Definition: definition}

	if got, want := step.DirectoryStage(), directory.Stage; got != want {
		t.Fatalf("DirectoryStage() = %d, want %d", got, want)
	}
	if got, want := step.DirectoryPath(), directory.Path; got != want {
		t.Fatalf("DirectoryPath() = %q, want %q", got, want)
	}
	if got, want := step.FilePath(), file.Path; got != want {
		t.Fatalf("FilePath() = %q, want %q", got, want)
	}
}

type fieldSchema struct {
	name   string
	typeOf reflect.Type
	tag    reflect.StructTag
}

func assertFields(t *testing.T, got reflect.Type, want []fieldSchema) {
	t.Helper()
	if got.Kind() != reflect.Struct {
		t.Fatalf("%s kind = %s, want struct", got, got.Kind())
	}
	if got.NumField() != len(want) {
		t.Fatalf("%s has %d fields, want %d", got, got.NumField(), len(want))
	}
	for index, expected := range want {
		field := got.Field(index)
		if field.Name != expected.name {
			t.Errorf("%s field %d name = %q, want %q", got, index, field.Name, expected.name)
		}
		if expected.typeOf != nil && field.Type != expected.typeOf {
			t.Errorf("%s.%s type = %s, want %s", got, field.Name, field.Type, expected.typeOf)
		}
		if field.Tag != expected.tag {
			t.Errorf("%s.%s tag = %q, want %q", got, field.Name, field.Tag, expected.tag)
		}
	}
}
