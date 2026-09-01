package definition

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/divilla/apihydra/internal/domain"
	"github.com/divilla/apihydra/pkg/errs"
)

func TestDecoderContractAndConstructor(t *testing.T) {
	var _ func() *Decoder = NewDecoder
	var _ func(*Decoder, context.Context, *domain.Suite) error = (*Decoder).DecodeFiles
	var _ func(*Decoder, context.Context, *domain.Suite) error = (*Decoder).ValidateDefaultsDefinitions
	var _ func(*Decoder, context.Context, *domain.Suite) error = (*Decoder).ValidateStepsDefinitions

	if decoder := NewDecoder(); decoder == nil {
		t.Fatal("NewDecoder() = nil")
	}
	if got := reflect.TypeOf(Decoder{}).NumField(); got != 0 {
		t.Fatalf("Decoder fields = %d, want stateless Decoder", got)
	}
}

func TestDecodeFilesDecodesTreeAndSetsSourceProvenance(t *testing.T) {
	root := &domain.Directory{Stage: 0, Path: "/"}
	child := &domain.Directory{Stage: 1, Path: "/child", Parent: root}
	root.Children = []*domain.Directory{child}
	root.DefaultsFile = definitionFile(root, "root.yaml", `
app: apihydra
kind: root
metadata:
  name: root defaults
  labels: [root, shared]
spec:
  base_url: https://example.test
  base_path: /v1
  headers:
    Accept: application/json
  disable_cookies: true
  timeout: 8
  retries: 2
`)
	root.StepsFiles = []*domain.File{
		definitionFile(root, "steps-a.yaml", `
app: apihydra
kind: steps
metadata:
  name: first steps
spec:
  defaults:
    base_path: /steps
    headers:
      X-Definition: yes
    disable_cookies: false
    timeout: 6
    retries: 2
  steps:
    - vars:
        account: primary
      request:
        method: POST
        path: /accounts
        body: '{"active":true}'
        defaults:
          base_url: https://step.example
          headers:
            X-Step: local
          disable_cookies: true
          timeout: 1
      response:
        expected_status: 201
        expected_body: '{"created":true}'
        expected_types:
          .created: [boolean]
        capture:
          account_id: .id
      debug: true
    - request:
        method: GET
        path: /accounts
`),
		definitionFile(root, "steps-b.yml", "app: apihydra\nkind: steps\nspec:\n  steps: []\n"),
	}
	child.DefaultsFile = definitionFile(child, "child/defaults.yaml", "app: apihydra\nkind: defaults\nspec:\n  base_path: /child\n")
	child.StepsFiles = []*domain.File{
		definitionFile(child, "child/steps.yaml", "app: apihydra\nkind: steps\nspec:\n  steps:\n    - request:\n        path: /child\n"),
	}

	suite := &domain.Suite{WorkDir: "/unused", Root: root}
	if err := NewDecoder().DecodeFiles(context.Background(), suite); err != nil {
		t.Fatalf("DecodeFiles() error = %v", err)
	}

	defaults := root.DefaultsDefinition
	if defaults == nil {
		t.Fatal("root.DefaultsDefinition = nil")
	}
	if defaults.File != root.DefaultsFile || defaults.App != "apihydra" || defaults.Kind != domain.KindRoot {
		t.Fatalf("root defaults provenance/header = {%p %q %q}, want {%p apihydra root}", defaults.File, defaults.App, defaults.Kind, root.DefaultsFile)
	}
	if defaults.Metadata.Name != "root defaults" || !slices.Equal(defaults.Metadata.Labels, []string{"root", "shared"}) {
		t.Fatalf("root defaults metadata = %+v", defaults.Metadata)
	}
	if defaults.Spec.BaseURL != "https://example.test" || defaults.Spec.BasePath != "/v1" || defaults.Spec.Headers["Accept"] != "application/json" || defaults.Spec.DisableCookies == nil || !*defaults.Spec.DisableCookies || defaults.Spec.Timeout != 8 || defaults.Spec.Retries != 2 {
		t.Fatalf("root defaults spec = %+v", defaults.Spec)
	}

	if got, want := len(root.StepsDefinitions), 2; got != want {
		t.Fatalf("len(root.StepsDefinitions) = %d, want %d", got, want)
	}
	stepsDefinition := root.StepsDefinitions[0]
	if stepsDefinition.File != root.StepsFiles[0] || stepsDefinition.Kind != domain.KindSteps || stepsDefinition.Metadata.Name != "first steps" {
		t.Fatalf("steps definition provenance/header = {%p %q %q}", stepsDefinition.File, stepsDefinition.Kind, stepsDefinition.Metadata.Name)
	}
	if got, want := len(stepsDefinition.Spec.Steps), 2; got != want {
		t.Fatalf("len(steps) = %d, want %d", got, want)
	}
	if got, want := stepsDefinition.Spec.Defaults, (domain.Defaults{
		BasePath:       "/steps",
		Headers:        map[string]string{"X-Definition": "yes"},
		DisableCookies: boolPointer(false),
		Timeout:        6,
		Retries:        2,
	}); !reflect.DeepEqual(got, want) {
		t.Fatalf("steps-file defaults = %+v, want %+v", got, want)
	}
	first := &stepsDefinition.Spec.Steps[0]
	if first.Definition != stepsDefinition || first.Index != 0 {
		t.Fatalf("first step provenance = {%p %d}, want {%p 0}", first.Definition, first.Index, stepsDefinition)
	}
	if first.Vars["account"] != "primary" || first.Request.Method != "POST" || first.Request.Path != "/accounts" || first.Request.Body != `{"active":true}` {
		t.Fatalf("first request = vars:%v request:%+v", first.Vars, first.Request)
	}
	if got, want := first.Request.Defaults, (domain.Defaults{
		BaseURL:        "https://step.example",
		Headers:        map[string]string{"X-Step": "local"},
		DisableCookies: boolPointer(true),
		Timeout:        1,
	}); !reflect.DeepEqual(got, want) {
		t.Fatalf("step request defaults = %+v, want %+v", got, want)
	}
	if first.Response.ExpectedStatus != 201 || first.Response.ExpectedBody != `{"created":true}` || !slices.Equal(first.Response.ExpectedTypes[".created"], []string{"boolean"}) || first.Response.Capture["account_id"] != ".id" || !first.Debug {
		t.Fatalf("first response/debug = response:%+v debug:%t", first.Response, first.Debug)
	}
	second := &stepsDefinition.Spec.Steps[1]
	if second.Definition != stepsDefinition || second.Index != 1 {
		t.Fatalf("second step provenance = {%p %d}, want {%p 1}", second.Definition, second.Index, stepsDefinition)
	}
	if root.StepsDefinitions[1].File != root.StepsFiles[1] || len(root.StepsDefinitions[1].Spec.Steps) != 0 {
		t.Fatal("second root steps file was not decoded")
	}
	if child.DefaultsDefinition == nil || child.DefaultsDefinition.File != child.DefaultsFile || child.DefaultsDefinition.Spec.BasePath != "/child" {
		t.Fatal("child defaults file was not decoded with provenance")
	}
	if len(child.StepsDefinitions) != 1 || child.StepsDefinitions[0].File != child.StepsFiles[0] || child.StepsDefinitions[0].Spec.Steps[0].Definition != child.StepsDefinitions[0] {
		t.Fatal("child steps file was not decoded with provenance")
	}
}

func TestDecodeFilesMutatesOnlyDecodedDefinitionFields(t *testing.T) {
	root := &domain.Directory{Stage: 4, Path: "/keep"}
	defaultsFile := definitionFile(root, "defaults.yaml", "kind: defaults\nspec:\n  base_path: /new\n")
	stepsFile := definitionFile(root, "steps.yaml", "kind: steps\nspec:\n  steps: []\n")
	defaultsFile.Kind = domain.KindDefaults
	stepsFile.Kind = domain.KindSteps
	root.Files = []*domain.File{defaultsFile, stepsFile}
	root.DefaultsFile = defaultsFile
	root.StepsFiles = []*domain.File{stepsFile}
	root.DefaultsDefinition = &domain.DefaultsDefinition{App: "old"}
	root.StepsDefinitions = []*domain.StepsDefinition{{App: "old"}}
	root.ResolvedDefaults.BaseURL = "keep-resolved"
	root.ResolvedSteps = [][]domain.Step{{{Debug: true}}}
	root.RuntimeSteps = [][]domain.Step{{{Debug: true}}}

	if err := NewDecoder().DecodeFiles(context.Background(), &domain.Suite{WorkDir: "/keep", Root: root}); err != nil {
		t.Fatalf("DecodeFiles() error = %v", err)
	}
	if root.DefaultsDefinition.App != "" || root.DefaultsDefinition.Spec.BasePath != "/new" || len(root.StepsDefinitions) != 1 || root.StepsDefinitions[0].App != "" {
		t.Fatal("DecodeFiles() did not replace decoded definitions")
	}
	if root.Stage != 4 || root.Path != "/keep" || root.Files[0] != defaultsFile || root.DefaultsFile != defaultsFile || root.StepsFiles[0] != stepsFile {
		t.Fatal("DecodeFiles() mutated an earlier-phase field")
	}
	if root.ResolvedDefaults.BaseURL != "keep-resolved" || !root.ResolvedSteps[0][0].Debug || !root.RuntimeSteps[0][0].Debug {
		t.Fatal("DecodeFiles() mutated a later-phase field")
	}
	if defaultsFile.Kind != domain.KindDefaults || stepsFile.Kind != domain.KindSteps {
		t.Fatal("DecodeFiles() mutated file classification")
	}

	firstDefaults := root.DefaultsDefinition
	firstSteps := root.StepsDefinitions
	if err := NewDecoder().DecodeFiles(context.Background(), &domain.Suite{Root: root}); err != nil {
		t.Fatalf("second DecodeFiles() error = %v", err)
	}
	if root.DefaultsDefinition == firstDefaults || len(root.StepsDefinitions) != 1 || root.StepsDefinitions[0] == firstSteps[0] {
		t.Fatal("second DecodeFiles() did not replace definitions without duplicates")
	}
}

func TestDecodeFilesReturnsContextualErrorsWithoutPartialMutation(t *testing.T) {
	tests := map[string]struct {
		defaults string
		steps    string
		wantPath string
	}{
		"malformed defaults": {
			defaults: "kind: defaults\nspec: [\n",
			steps:    "kind: steps\nspec:\n  steps: []\n",
			wantPath: "defaults.yaml",
		},
		"malformed steps": {
			defaults: "kind: defaults\nspec:\n  base_path: /valid\n",
			steps:    "kind: steps\nspec:\n  steps: [\n",
			wantPath: "steps.yaml",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			root := &domain.Directory{Path: "/"}
			root.DefaultsFile = definitionFile(root, "defaults.yaml", test.defaults)
			root.StepsFiles = []*domain.File{definitionFile(root, "steps.yaml", test.steps)}
			oldDefaults := &domain.DefaultsDefinition{App: "keep"}
			oldSteps := []*domain.StepsDefinition{{App: "keep"}}
			root.DefaultsDefinition = oldDefaults
			root.StepsDefinitions = oldSteps

			err := NewDecoder().DecodeFiles(context.Background(), &domain.Suite{Root: root})
			if err == nil {
				t.Fatal("DecodeFiles() error = nil for malformed YAML")
			}
			if got := errs.Code(err, errs.ExitInternal); got != errs.ExitConfiguration {
				t.Fatalf("DecodeFiles() exit code = %d, want %d", got, errs.ExitConfiguration)
			}
			if !strings.Contains(err.Error(), "file "+test.wantPath) {
				t.Fatalf("DecodeFiles() error = %q, want file provenance", err)
			}
			if root.DefaultsDefinition != oldDefaults || !slices.Equal(root.StepsDefinitions, oldSteps) {
				t.Fatal("DecodeFiles() partially mutated definitions after an error")
			}
		})
	}
}

func TestDecodeFilesHonorsCancellationAndAllowsEmptyTree(t *testing.T) {
	decoder := NewDecoder()
	if err := decoder.DecodeFiles(context.Background(), &domain.Suite{}); err != nil {
		t.Fatalf("DecodeFiles() empty-tree error = %v", err)
	}
	if err := decoder.DecodeFiles(
		&checkingContext{cancelAt: 2},
		&domain.Suite{Root: &domain.Directory{Path: "/"}},
	); !errors.Is(err, context.Canceled) {
		t.Fatalf("DecodeFiles() pre-commit cancellation error = %v, want context.Canceled", err)
	}

	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	oldDefaults := &domain.DefaultsDefinition{App: "keep"}
	root := &domain.Directory{
		Path:               "/",
		DefaultsFile:       &domain.File{Path: "defaults.yaml", Bytes: []byte("kind: defaults\n")},
		DefaultsDefinition: oldDefaults,
	}
	err := decoder.DecodeFiles(canceled, &domain.Suite{Root: root})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("DecodeFiles() cancellation error = %v, want context.Canceled", err)
	}
	if root.DefaultsDefinition != oldDefaults {
		t.Fatal("DecodeFiles() mutated definitions after cancellation")
	}

	oldSteps := []*domain.StepsDefinition{{App: "keep"}}
	root = &domain.Directory{Path: "/", StepsDefinitions: oldSteps}
	root.StepsFiles = []*domain.File{
		definitionFile(root, "steps.yaml", "kind: steps\nspec:\n  steps: []\n"),
	}
	err = decoder.DecodeFiles(&checkingContext{cancelAt: 2}, &domain.Suite{Root: root})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("DecodeFiles() steps cancellation error = %v, want context.Canceled", err)
	}
	if !slices.Equal(root.StepsDefinitions, oldSteps) {
		t.Fatal("DecodeFiles() mutated steps definitions after cancellation")
	}
}

func TestValidationMethodsTraverseAllDecodedDefinitionsWithoutExtraFieldRules(t *testing.T) {
	root := &domain.Directory{Path: "/", DefaultsDefinition: &domain.DefaultsDefinition{}}
	child := &domain.Directory{
		Path:               "/child",
		Parent:             root,
		DefaultsDefinition: &domain.DefaultsDefinition{Kind: domain.DocumentKind("custom")},
		StepsDefinitions: []*domain.StepsDefinition{
			{},
			{Kind: domain.DocumentKind("custom")},
		},
	}
	root.Children = []*domain.Directory{child}
	suite := &domain.Suite{Root: root}
	decoder := NewDecoder()

	if err := decoder.ValidateDefaultsDefinitions(context.Background(), suite); err != nil {
		t.Fatalf("ValidateDefaultsDefinitions() error = %v", err)
	}
	if err := decoder.ValidateStepsDefinitions(context.Background(), suite); err != nil {
		t.Fatalf("ValidateStepsDefinitions() error = %v", err)
	}
	if err := decoder.ValidateDefaultsDefinitions(
		context.Background(),
		&domain.Suite{Root: &domain.Directory{Path: "/"}},
	); err != nil {
		t.Fatalf("ValidateDefaultsDefinitions() no-definition error = %v", err)
	}

	if err := decoder.ValidateDefaultsDefinitions(&checkingContext{cancelAt: 4}, suite); !errors.Is(err, context.Canceled) {
		t.Fatalf("ValidateDefaultsDefinitions() traversal error = %v, want context.Canceled", err)
	}
	if err := decoder.ValidateStepsDefinitions(&checkingContext{cancelAt: 4}, suite); !errors.Is(err, context.Canceled) {
		t.Fatalf("ValidateStepsDefinitions() traversal error = %v, want context.Canceled", err)
	}
}

func TestValidationMethodsAllowEmptyTree(t *testing.T) {
	decoder := NewDecoder()
	suite := &domain.Suite{}
	if err := decoder.ValidateDefaultsDefinitions(context.Background(), suite); err != nil {
		t.Fatalf("ValidateDefaultsDefinitions() empty-tree error = %v", err)
	}
	if err := decoder.ValidateStepsDefinitions(context.Background(), suite); err != nil {
		t.Fatalf("ValidateStepsDefinitions() empty-tree error = %v", err)
	}
}
