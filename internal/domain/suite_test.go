package domain

import (
	"errors"
	"reflect"
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
		{"App", reflect.TypeOf(""), `yaml:"app"`},
		{"Kind", reflect.TypeOf(""), `yaml:"kind"`},
		{"Spec", reflect.TypeOf(YAMLString("")), `yaml:"spec"`},
	})
	assertFields(t, reflect.TypeOf(DefaultsDefinition{}), []fieldSchema{
		{"App", reflect.TypeOf(""), `yaml:"app"`},
		{"Kind", reflect.TypeOf(DocumentKind("")), `yaml:"kind"`},
		{"Metadata", reflect.TypeOf(Metadata{}), `yaml:"metadata"`},
		{"Spec", reflect.TypeOf(Defaults{}), `yaml:"spec"`},
		{"File", reflect.TypeOf((*File)(nil)), `yaml:"-"`},
	})
	assertFields(t, reflect.TypeOf(StepsDefinition{}), []fieldSchema{
		{"App", reflect.TypeOf(""), `yaml:"app"`},
		{"Kind", reflect.TypeOf(DocumentKind("")), `yaml:"kind"`},
		{"Metadata", reflect.TypeOf(Metadata{}), `yaml:"metadata"`},
		{"Spec", nil, `yaml:"spec"`},
		{"File", reflect.TypeOf((*File)(nil)), `yaml:"-"`},
	})
	assertFields(t, reflect.TypeOf(StepsDefinition{}).Field(3).Type, []fieldSchema{
		{"Steps", reflect.TypeOf([]Step(nil)), `yaml:"steps"`},
	})
	assertFields(t, reflect.TypeOf(Metadata{}), []fieldSchema{
		{"Name", reflect.TypeOf(""), `yaml:"name"`},
		{"Labels", reflect.TypeOf([]string(nil)), `yaml:"labels"`},
	})
	assertFields(t, reflect.TypeOf(Defaults{}), []fieldSchema{
		{"BaseURL", reflect.TypeOf(""), `yaml:"baseUrl"`},
		{"BasePath", reflect.TypeOf(""), `yaml:"basePath"`},
		{"Headers", reflect.TypeOf(map[string]string(nil)), `yaml:"headers"`},
		{"Timeout", reflect.TypeOf(0), `yaml:"timeout"`},
		{"Retries", reflect.TypeOf(0), `yaml:"retries"`},
	})
	assertFields(t, reflect.TypeOf(Step{}), []fieldSchema{
		{"Vars", reflect.TypeOf(map[string]YAMLString(nil)), `yaml:"vars" json:"vars"`},
		{"Request", nil, `yaml:"request" json:"request"`},
		{"Response", nil, `yaml:"response" json:"response"`},
		{"Debug", reflect.TypeOf(false), `yaml:"debug" json:"debug"`},
		{"Definition", reflect.TypeOf((*StepsDefinition)(nil)), `yaml:"-" json:"-"`},
		{"Index", reflect.TypeOf(0), `yaml:"-" json:"index"`},
	})
	assertFields(t, reflect.TypeOf(Step{}).Field(1).Type, []fieldSchema{
		{"Method", reflect.TypeOf(""), `yaml:"method" json:"method"`},
		{"BaseURL", reflect.TypeOf(""), `yaml:"baseUrl" json:"baseUrl"`},
		{"BasePath", reflect.TypeOf(""), `yaml:"basePath" json:"basePath"`},
		{"Path", reflect.TypeOf(""), `yaml:"path" json:"path"`},
		{"Headers", reflect.TypeOf(map[string]string(nil)), `yaml:"headers" json:"headers"`},
		{"Timeout", reflect.TypeOf(0), `yaml:"timeout" json:"timeout"`},
		{"Retries", reflect.TypeOf(0), `yaml:"retries" json:"retries"`},
		{"Query", reflect.TypeOf(""), `yaml:"query" json:"query"`},
		{"Body", reflect.TypeOf(YAMLString("")), `yaml:"body" json:"body"`},
	})
	assertFields(t, reflect.TypeOf(Step{}).Field(2).Type, []fieldSchema{
		{"ExpectedStatus", reflect.TypeOf(0), `yaml:"expected_status" json:"expected_status"`},
		{"ActualStatus", reflect.TypeOf(0), `yaml:"actual_status" json:"actual_status"`},
		{"ExpectedBody", reflect.TypeOf(YAMLString("")), `yaml:"expected_body" json:"expected_body"`},
		{"ActualBody", reflect.TypeOf(YAMLString("")), `yaml:"actual_body" json:"actual_body"`},
		{"ExpectedTypes", reflect.TypeOf(map[string][]string(nil)), `yaml:"expected_types" json:"expected_types"`},
		{"Capture", reflect.TypeOf(map[string]YAMLString(nil)), `yaml:"capture" json:"capture"`},
	})
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
