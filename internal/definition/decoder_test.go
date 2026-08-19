package definition

import (
	"apih/internal/domain"
	"context"
	"errors"
	"reflect"
	"testing"
)

func TestDecoderPublicContract(t *testing.T) {
	var constructor func() *Decoder = NewDecoder
	var decode func(*Decoder, context.Context, *domain.Suite) error = (*Decoder).DecodeFiles
	var validateDefaults func(*Decoder, context.Context, *domain.Suite) error = (*Decoder).ValidateDefaultsDefinitions
	var validateSteps func(*Decoder, context.Context, *domain.Suite) error = (*Decoder).ValidateStepsDefinitions
	_, _, _, _ = constructor, decode, validateDefaults, validateSteps

	if got := *NewDecoder(); got != (Decoder{}) {
		t.Fatalf("NewDecoder() = %#v, want empty Decoder", got)
	}
}

func TestDecodeFilesDecodesClassifiedFilesAcrossDirectoryTree(t *testing.T) {
	defaultsFile := decoderFile("/defaults.yaml", `
app: apihydra
kind: defaults
metadata:
  name: shared
  labels: [root, defaults]
spec:
  baseUrl: https://example.test
  basePath: /api
  headers:
    X-Suite: decoder
  timeout: 12
  retries: 3
`)
	firstStepsFile := decoderFile("/child/first.yaml", `
app: apihydra
kind: steps
metadata:
  name: first
spec:
  steps:
    - vars:
        id: "42"
      request:
        method: POST
        path: /items
        body: '{"id": 42}'
      response:
        status: [200, 201]
        expected: '{"created": true}'
      debug: true
`)
	secondStepsFile := decoderFile("/child/second.yml", `
app: apihydra
kind: steps
metadata:
  name: second
  labels: [child]
spec:
  steps:
    - request:
        method: GET
        path: /first
    - request:
        method: DELETE
        path: /second
`)

	root := &domain.Directory{Stage: 0, Path: "/", DefaultsFile: defaultsFile}
	child := &domain.Directory{
		Stage:      1,
		Path:       "/child",
		Parent:     root,
		StepsFiles: []*domain.File{firstStepsFile, secondStepsFile},
	}
	root.Children = []*domain.Directory{child}
	defaultsFile.Directory = root
	firstStepsFile.Directory = child
	secondStepsFile.Directory = child

	if err := NewDecoder().DecodeFiles(context.Background(), &domain.Suite{Root: root}); err != nil {
		t.Fatalf("DecodeFiles() error = %v", err)
	}

	defaults := root.DefaultsDefinition
	if defaults == nil || defaults.File != defaultsFile || defaults.App != "apihydra" ||
		defaults.Kind != domain.KindDefaults || defaults.Metadata.Name != "shared" ||
		!reflect.DeepEqual(defaults.Metadata.Labels, []string{"root", "defaults"}) ||
		defaults.Spec.BaseURL != "https://example.test" || defaults.Spec.BasePath != "/api" ||
		!reflect.DeepEqual(defaults.Spec.Headers, map[string]string{"X-Suite": "decoder"}) ||
		defaults.Spec.Timeout != 12 || defaults.Spec.Retries != 3 {
		t.Fatalf("root DefaultsDefinition = %#v, want decoded defaults with source file", defaults)
	}
	if len(root.StepsDefinitions) != 0 {
		t.Fatalf("root StepsDefinitions = %#v, want none", root.StepsDefinitions)
	}
	if child.DefaultsDefinition != nil {
		t.Fatalf("child DefaultsDefinition = %#v, want nil", child.DefaultsDefinition)
	}
	if len(child.StepsDefinitions) != 2 {
		t.Fatalf("child StepsDefinitions length = %d, want 2", len(child.StepsDefinitions))
	}

	first := child.StepsDefinitions[0]
	if first.File != firstStepsFile || first.App != "apihydra" || first.Kind != domain.KindSteps ||
		first.Metadata.Name != "first" || len(first.Spec.Steps) != 1 {
		t.Fatalf("first StepsDefinition = %#v, want decoded first steps file", first)
	}
	firstStep := &first.Spec.Steps[0]
	if firstStep.Definition != first || firstStep.Index != 0 || firstStep.Request.Method != "POST" ||
		firstStep.Request.Path != "/items" || firstStep.Request.Body != `{"id": 42}` ||
		!reflect.DeepEqual(firstStep.Vars, map[string]domain.YAMLString{"id": "42"}) ||
		!reflect.DeepEqual(firstStep.Response.Status, []int{200, 201}) ||
		firstStep.Response.Expected != `{"created": true}` || !firstStep.Debug {
		t.Fatalf("first decoded step = %#v, want complete step with provenance", firstStep)
	}

	second := child.StepsDefinitions[1]
	if second.File != secondStepsFile || second.Metadata.Name != "second" ||
		!reflect.DeepEqual(second.Metadata.Labels, []string{"child"}) || len(second.Spec.Steps) != 2 {
		t.Fatalf("second StepsDefinition = %#v, want decoded second steps file", second)
	}
	for index := range second.Spec.Steps {
		step := &second.Spec.Steps[index]
		if step.Definition != second || step.Index != index {
			t.Fatalf("second step %d provenance = (%p, %d), want (%p, %d)",
				index, step.Definition, step.Index, second, index)
		}
	}
}

func TestDecodeFilesMutatesOnlyDecodedDefinitionFields(t *testing.T) {
	defaultsFile := decoderFile("/defaults.yaml", "app: apihydra\nkind: defaults\nspec: {}\n")
	stepsFile := decoderFile("/steps.yaml", "app: apihydra\nkind: steps\nspec:\n  steps: []\n")
	classifiedFiles := []*domain.File{defaultsFile, stepsFile}
	resolvedDefaults := domain.Defaults{BaseURL: "preserved"}
	resolvedSteps := [][]domain.Step{{{Debug: true}}}
	runtimeSteps := [][]domain.Step{{{Index: 9}}}
	directory := &domain.Directory{
		Stage:            4,
		Path:             "/preserved",
		Files:            classifiedFiles,
		DefaultsFile:     defaultsFile,
		StepsFiles:       []*domain.File{stepsFile},
		ResolvedDefaults: resolvedDefaults,
		ResolvedSteps:    resolvedSteps,
		RuntimeSteps:     runtimeSteps,
	}
	defaultsFile.Kind = domain.KindDefaults
	defaultsFile.Directory = directory
	stepsFile.Kind = domain.KindSteps
	stepsFile.Directory = directory

	if err := NewDecoder().DecodeFiles(context.Background(), &domain.Suite{WorkDir: "/work", Root: directory}); err != nil {
		t.Fatalf("DecodeFiles() error = %v", err)
	}

	if directory.Stage != 4 || directory.Path != "/preserved" || directory.Parent != nil ||
		len(directory.Children) != 0 || !sameFileSlice(directory.Files, classifiedFiles) ||
		directory.DefaultsFile != defaultsFile || !sameFileSlice(directory.StepsFiles, []*domain.File{stepsFile}) ||
		directory.ResolvedDefaults.BaseURL != "preserved" || !reflect.DeepEqual(directory.ResolvedSteps, resolvedSteps) ||
		!reflect.DeepEqual(directory.RuntimeSteps, runtimeSteps) || defaultsFile.Kind != domain.KindDefaults ||
		stepsFile.Kind != domain.KindSteps {
		t.Fatalf("DecodeFiles() mutated state outside decoded definition fields: %#v", directory)
	}
}

func TestDecodeFilesReturnsDecodeAndContextErrors(t *testing.T) {
	decoder := NewDecoder()

	t.Run("invalid defaults YAML", func(t *testing.T) {
		oldDefaults := &domain.DefaultsDefinition{App: "preserved"}
		oldSteps := []*domain.StepsDefinition{{App: "preserved"}}
		directory := &domain.Directory{
			DefaultsFile:       decoderFile("/defaults.yaml", "spec: [\n"),
			DefaultsDefinition: oldDefaults,
			StepsDefinitions:   oldSteps,
		}
		if err := decoder.DecodeFiles(context.Background(), &domain.Suite{Root: directory}); err == nil {
			t.Fatal("DecodeFiles() error = nil, want YAML decode error")
		}
		if directory.DefaultsDefinition != oldDefaults || !sameDefinitionSlice(directory.StepsDefinitions, oldSteps) {
			t.Fatal("DecodeFiles() replaced decoded fields after defaults decode failure")
		}
	})

	t.Run("invalid steps YAML", func(t *testing.T) {
		oldDefaults := &domain.DefaultsDefinition{App: "preserved"}
		oldSteps := []*domain.StepsDefinition{{App: "preserved"}}
		directory := &domain.Directory{
			DefaultsFile: decoderFile("/defaults.yaml", `
app: apihydra
kind: defaults
spec: {}
`),
			StepsFiles:         []*domain.File{decoderFile("/steps.yaml", "spec: [\n")},
			DefaultsDefinition: oldDefaults,
			StepsDefinitions:   oldSteps,
		}
		if err := decoder.DecodeFiles(context.Background(), &domain.Suite{Root: directory}); err == nil {
			t.Fatal("DecodeFiles() error = nil, want YAML decode error")
		}
		if directory.DefaultsDefinition != oldDefaults || !sameDefinitionSlice(directory.StepsDefinitions, oldSteps) {
			t.Fatal("DecodeFiles() replaced decoded fields after steps decode failure")
		}
	})

	t.Run("canceled context", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if err := decoder.DecodeFiles(ctx, &domain.Suite{Root: &domain.Directory{}}); !errors.Is(err, context.Canceled) {
			t.Fatalf("DecodeFiles() error = %v, want context.Canceled", err)
		}
	})

	t.Run("context canceled during child traversal", func(t *testing.T) {
		ctx := newCancelDuringTraversalContext()
		root := &domain.Directory{Children: []*domain.Directory{{}}}
		if err := decoder.DecodeFiles(ctx, &domain.Suite{Root: root}); !errors.Is(err, context.Canceled) {
			t.Fatalf("DecodeFiles() error = %v, want context.Canceled", err)
		}
	})
}

func TestValidationMethodsTraverseDefinitionsWithoutInventingRules(t *testing.T) {
	invalidByAnyLikelyRule := &domain.DefaultsDefinition{
		App:  "",
		Kind: domain.DocumentKind("unknown"),
	}
	steps := &domain.StepsDefinition{}
	steps.Spec.Steps = []domain.Step{{}}
	root := &domain.Directory{
		DefaultsDefinition: invalidByAnyLikelyRule,
		StepsDefinitions:   []*domain.StepsDefinition{steps},
	}
	child := &domain.Directory{
		Parent:             root,
		DefaultsDefinition: &domain.DefaultsDefinition{},
		StepsDefinitions:   []*domain.StepsDefinition{{}, {}},
	}
	root.Children = []*domain.Directory{child}
	suite := &domain.Suite{Root: root}
	wantDefaults := []*domain.DefaultsDefinition{root.DefaultsDefinition, child.DefaultsDefinition}
	wantSteps := [][]*domain.StepsDefinition{root.StepsDefinitions, child.StepsDefinitions}

	if err := NewDecoder().ValidateDefaultsDefinitions(context.Background(), suite); err != nil {
		t.Fatalf("ValidateDefaultsDefinitions() error = %v, want nil without declared field rules", err)
	}
	if err := NewDecoder().ValidateStepsDefinitions(context.Background(), suite); err != nil {
		t.Fatalf("ValidateStepsDefinitions() error = %v, want nil without declared field rules", err)
	}
	if root.DefaultsDefinition != wantDefaults[0] || child.DefaultsDefinition != wantDefaults[1] ||
		!sameDefinitionSlice(root.StepsDefinitions, wantSteps[0]) ||
		!sameDefinitionSlice(child.StepsDefinitions, wantSteps[1]) {
		t.Fatal("validation methods mutated decoded definitions")
	}
}

func TestValidationMethodsHonorContextWhileTraversingDefinitions(t *testing.T) {
	t.Run("defaults", func(t *testing.T) {
		ctx := newCancelDuringTraversalContext()
		root := &domain.Directory{DefaultsDefinition: &domain.DefaultsDefinition{}}
		root.Children = []*domain.Directory{{DefaultsDefinition: &domain.DefaultsDefinition{}}}
		if err := NewDecoder().ValidateDefaultsDefinitions(ctx, &domain.Suite{Root: root}); !errors.Is(err, context.Canceled) {
			t.Fatalf("ValidateDefaultsDefinitions() error = %v, want context.Canceled", err)
		}
	})

	t.Run("steps", func(t *testing.T) {
		ctx := newCancelDuringTraversalContext()
		root := &domain.Directory{StepsDefinitions: []*domain.StepsDefinition{{}, {}}}
		if err := NewDecoder().ValidateStepsDefinitions(ctx, &domain.Suite{Root: root}); !errors.Is(err, context.Canceled) {
			t.Fatalf("ValidateStepsDefinitions() error = %v, want context.Canceled", err)
		}
	})

	t.Run("already canceled", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		decoder := NewDecoder()
		if err := decoder.ValidateDefaultsDefinitions(ctx, &domain.Suite{}); !errors.Is(err, context.Canceled) {
			t.Fatalf("ValidateDefaultsDefinitions() error = %v, want context.Canceled", err)
		}
		if err := decoder.ValidateStepsDefinitions(ctx, &domain.Suite{}); !errors.Is(err, context.Canceled) {
			t.Fatalf("ValidateStepsDefinitions() error = %v, want context.Canceled", err)
		}
	})
}

func TestDecoderAcceptsEmptySuites(t *testing.T) {
	decoder := NewDecoder()
	for name, method := range map[string]func(context.Context, *domain.Suite) error{
		"decode":            decoder.DecodeFiles,
		"validate defaults": decoder.ValidateDefaultsDefinitions,
		"validate steps":    decoder.ValidateStepsDefinitions,
	} {
		t.Run(name, func(t *testing.T) {
			if err := method(context.Background(), nil); err != nil {
				t.Fatalf("method(nil) error = %v, want nil", err)
			}
			if err := method(context.Background(), &domain.Suite{}); err != nil {
				t.Fatalf("method(empty suite) error = %v, want nil", err)
			}
		})
	}
}

func decoderFile(path, contents string) *domain.File {
	return &domain.File{Path: path, Bytes: []byte(contents)}
}

func sameFileSlice(got, want []*domain.File) bool {
	if len(got) != len(want) {
		return false
	}
	for index := range got {
		if got[index] != want[index] {
			return false
		}
	}
	return true
}

func sameDefinitionSlice(got, want []*domain.StepsDefinition) bool {
	if len(got) != len(want) {
		return false
	}
	for index := range got {
		if got[index] != want[index] {
			return false
		}
	}
	return true
}
