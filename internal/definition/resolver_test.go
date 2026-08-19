package definition

import (
	"apih/internal/domain"
	"context"
	"errors"
	"reflect"
	"testing"
)

func TestResolverPublicContract(t *testing.T) {
	var constructor func() *Resolver = NewResolver
	var resolveDefaults func(*Resolver, context.Context, *domain.Suite) error = (*Resolver).ResolveDefaults
	var resolveSteps func(*Resolver, context.Context, *domain.Suite) error = (*Resolver).ResolveSteps
	var validateSteps func(*Resolver, context.Context, *domain.Suite) error = (*Resolver).ValidateStepsDefinitions
	_, _, _, _ = constructor, resolveDefaults, resolveSteps, validateSteps

	if got := *NewResolver(); got != (Resolver{}) {
		t.Fatalf("NewResolver() = %#v, want empty Resolver", got)
	}
}

func TestResolveDefaultsTraversesTreeAndMergesInheritedDefinitions(t *testing.T) {
	rootHeaders := map[string]string{"Accept": "application/json", "X-Scope": "root"}
	rootDefinition := resolverDefaults(domain.Defaults{
		BaseURL:  "https://root.example",
		BasePath: "/v1",
		Headers:  rootHeaders,
		Timeout:  10,
		Retries:  2,
	})
	childHeaders := map[string]string{"X-Scope": "child", "X-Child": "true"}
	childDefinition := resolverDefaults(domain.Defaults{
		BasePath: "/v2",
		Headers:  childHeaders,
		Retries:  4,
	})

	root := &domain.Directory{Path: "/", DefaultsDefinition: rootDefinition}
	child := &domain.Directory{Path: "/child", Parent: root, DefaultsDefinition: childDefinition}
	grandchild := &domain.Directory{Path: "/child/grandchild", Parent: child}
	sibling := &domain.Directory{Path: "/sibling", Parent: root, DefaultsDefinition: resolverDefaults(domain.Defaults{Timeout: 20})}
	root.Children = []*domain.Directory{child, sibling}
	child.Children = []*domain.Directory{grandchild}

	if err := NewResolver().ResolveDefaults(context.Background(), &domain.Suite{Root: root}); err != nil {
		t.Fatalf("ResolveDefaults() error = %v", err)
	}

	assertDefaults(t, root.ResolvedDefaults, domain.Defaults{
		BaseURL: "https://root.example", BasePath: "/v1", Headers: rootHeaders, Timeout: 10, Retries: 2,
	})
	wantChild := domain.Defaults{
		BaseURL: "https://root.example", BasePath: "/v2",
		Headers: map[string]string{"Accept": "application/json", "X-Scope": "child", "X-Child": "true"},
		Timeout: 10, Retries: 4,
	}
	assertDefaults(t, child.ResolvedDefaults, wantChild)
	assertDefaults(t, grandchild.ResolvedDefaults, wantChild)
	assertDefaults(t, sibling.ResolvedDefaults, domain.Defaults{
		BaseURL: "https://root.example", BasePath: "/v1", Headers: rootHeaders, Timeout: 20, Retries: 2,
	})

	root.ResolvedDefaults.Headers["Accept"] = "changed"
	if rootHeaders["Accept"] != "application/json" || child.ResolvedDefaults.Headers["Accept"] != "application/json" {
		t.Fatal("ResolveDefaults() aliased definition or child header maps")
	}
}

func TestResolveStepsGroupsDefinitionsAndAppliesInheritedDefaults(t *testing.T) {
	root := &domain.Directory{
		Path: "/",
		DefaultsDefinition: resolverDefaults(domain.Defaults{
			BaseURL: "https://root.example", BasePath: "/api", Headers: map[string]string{"Accept": "json", "X-Scope": "root"},
			Timeout: 10, Retries: 2,
		}),
	}
	rootDefinition := resolverSteps(
		resolverStep("GET", "/first", "", "", nil, 0, 0),
		resolverStep("POST", "/second", "https://step.example", "/step", map[string]string{"X-Scope": "step"}, 30, 5),
	)
	root.StepsDefinitions = []*domain.StepsDefinition{rootDefinition}

	child := &domain.Directory{
		Path:   "/child",
		Parent: root,
		DefaultsDefinition: resolverDefaults(domain.Defaults{
			BasePath: "/child-api", Headers: map[string]string{"X-Scope": "child", "X-Child": "true"}, Retries: 4,
		}),
		StepsDefinitions: []*domain.StepsDefinition{
			resolverSteps(resolverStep("GET", "/child-first", "", "", map[string]string{"X-Step": "true"}, 0, 0)),
			nil,
			resolverSteps(resolverStep("DELETE", "/child-second", "", "", nil, 0, 0)),
		},
		RuntimeSteps: [][]domain.Step{{{Debug: true}}},
	}
	root.Children = []*domain.Directory{child}

	if err := NewResolver().ResolveSteps(context.Background(), &domain.Suite{Root: root}); err != nil {
		t.Fatalf("ResolveSteps() error = %v", err)
	}

	if len(root.ResolvedSteps) != 1 || len(root.ResolvedSteps[0]) != 2 {
		t.Fatalf("root ResolvedSteps shape = %#v, want one two-step group", root.ResolvedSteps)
	}
	first := root.ResolvedSteps[0][0]
	assertRequestDefaults(t, first, "https://root.example", "/api", map[string]string{"Accept": "json", "X-Scope": "root"}, 10, 2)
	second := root.ResolvedSteps[0][1]
	assertRequestDefaults(t, second, "https://step.example", "/step", map[string]string{"Accept": "json", "X-Scope": "step"}, 30, 5)

	if len(child.ResolvedSteps) != 3 || len(child.ResolvedSteps[0]) != 1 || len(child.ResolvedSteps[1]) != 0 || len(child.ResolvedSteps[2]) != 1 {
		t.Fatalf("child ResolvedSteps shape = %#v, want groups matching definitions", child.ResolvedSteps)
	}
	assertRequestDefaults(t, child.ResolvedSteps[0][0], "https://root.example", "/child-api", map[string]string{
		"Accept": "json", "X-Scope": "child", "X-Child": "true", "X-Step": "true",
	}, 10, 4)
	assertRequestDefaults(t, child.ResolvedSteps[2][0], "https://root.example", "/child-api", map[string]string{
		"Accept": "json", "X-Scope": "child", "X-Child": "true",
	}, 10, 4)

	if rootDefinition.Spec.Steps[0].Request.BaseURL != "" || rootDefinition.Spec.Steps[0].Request.Headers != nil {
		t.Fatal("ResolveSteps() mutated a decoded steps definition")
	}
	if len(child.RuntimeSteps) != 1 || !child.RuntimeSteps[0][0].Debug {
		t.Fatal("ResolveSteps() mutated RuntimeSteps")
	}
}

func TestResolverMethodsMutateOnlyTheirDeclaredOutput(t *testing.T) {
	definition := resolverDefaults(domain.Defaults{BaseURL: "https://example.test"})
	stepsDefinition := resolverSteps(resolverStep("GET", "/", "", "", nil, 0, 0))
	resolvedSteps := [][]domain.Step{{{Index: 7}}}
	runtimeSteps := [][]domain.Step{{{Debug: true}}}
	directory := &domain.Directory{
		Stage: 3, Path: "/preserved", Files: []*domain.File{{Path: "/preserved/file.yaml"}},
		DefaultsDefinition: definition, StepsDefinitions: []*domain.StepsDefinition{stepsDefinition},
		ResolvedSteps: resolvedSteps, RuntimeSteps: runtimeSteps,
	}
	suite := &domain.Suite{WorkDir: "/work", Root: directory}

	if err := NewResolver().ResolveDefaults(context.Background(), suite); err != nil {
		t.Fatalf("ResolveDefaults() error = %v", err)
	}
	if directory.Stage != 3 || directory.Path != "/preserved" || directory.DefaultsDefinition != definition ||
		directory.StepsDefinitions[0] != stepsDefinition || !reflect.DeepEqual(directory.ResolvedSteps, resolvedSteps) ||
		!reflect.DeepEqual(directory.RuntimeSteps, runtimeSteps) {
		t.Fatal("ResolveDefaults() mutated state outside ResolvedDefaults")
	}

	resolvedDefaults := directory.ResolvedDefaults
	if err := NewResolver().ResolveSteps(context.Background(), suite); err != nil {
		t.Fatalf("ResolveSteps() error = %v", err)
	}
	if !reflect.DeepEqual(directory.ResolvedDefaults, resolvedDefaults) || directory.DefaultsDefinition != definition ||
		directory.StepsDefinitions[0] != stepsDefinition || !reflect.DeepEqual(directory.RuntimeSteps, runtimeSteps) {
		t.Fatal("ResolveSteps() mutated state outside ResolvedSteps")
	}

	resolved := directory.ResolvedSteps
	if err := NewResolver().ValidateStepsDefinitions(context.Background(), suite); err != nil {
		t.Fatalf("ValidateStepsDefinitions() error = %v", err)
	}
	if !reflect.DeepEqual(directory.ResolvedDefaults, resolvedDefaults) || !reflect.DeepEqual(directory.ResolvedSteps, resolved) ||
		!reflect.DeepEqual(directory.RuntimeSteps, runtimeSteps) || directory.StepsDefinitions[0] != stepsDefinition {
		t.Fatal("ValidateStepsDefinitions() mutated suite state")
	}
}

func TestResolverMethodsHonorContextCancellation(t *testing.T) {
	tests := map[string]func(context.Context, *domain.Suite) error{
		"resolve defaults": NewResolver().ResolveDefaults,
		"resolve steps":    NewResolver().ResolveSteps,
		"validate steps":   NewResolver().ValidateStepsDefinitions,
	}

	for name, method := range tests {
		t.Run(name+" before traversal", func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			if err := method(ctx, &domain.Suite{}); !errors.Is(err, context.Canceled) {
				t.Fatalf("method() error = %v, want context.Canceled", err)
			}
		})

		t.Run(name+" during traversal", func(t *testing.T) {
			ctx := newCancelDuringTraversalContext()
			root := &domain.Directory{StepsDefinitions: []*domain.StepsDefinition{{}}}
			root.Children = []*domain.Directory{{Parent: root}}
			if err := method(ctx, &domain.Suite{Root: root}); !errors.Is(err, context.Canceled) {
				t.Fatalf("method() error = %v, want context.Canceled", err)
			}
		})
	}
}

func TestResolverMethodsPropagateCancellationWithinTraversal(t *testing.T) {
	t.Run("defaults child", func(t *testing.T) {
		ctx := newCancelAfterChecksContext(3)
		root := &domain.Directory{}
		root.Children = []*domain.Directory{{Parent: root}}
		if err := NewResolver().ResolveDefaults(ctx, &domain.Suite{Root: root}); !errors.Is(err, context.Canceled) {
			t.Fatalf("ResolveDefaults() error = %v, want context.Canceled", err)
		}
	})

	t.Run("steps definition", func(t *testing.T) {
		ctx := newCancelAfterChecksContext(3)
		root := &domain.Directory{StepsDefinitions: []*domain.StepsDefinition{{}}}
		if err := NewResolver().ResolveSteps(ctx, &domain.Suite{Root: root}); !errors.Is(err, context.Canceled) {
			t.Fatalf("ResolveSteps() error = %v, want context.Canceled", err)
		}
	})

	t.Run("step", func(t *testing.T) {
		ctx := newCancelAfterChecksContext(4)
		root := &domain.Directory{StepsDefinitions: []*domain.StepsDefinition{resolverSteps(domain.Step{})}}
		if err := NewResolver().ResolveSteps(ctx, &domain.Suite{Root: root}); !errors.Is(err, context.Canceled) {
			t.Fatalf("ResolveSteps() error = %v, want context.Canceled", err)
		}
	})

	t.Run("steps child", func(t *testing.T) {
		ctx := newCancelAfterChecksContext(3)
		root := &domain.Directory{}
		root.Children = []*domain.Directory{{Parent: root}}
		if err := NewResolver().ResolveSteps(ctx, &domain.Suite{Root: root}); !errors.Is(err, context.Canceled) {
			t.Fatalf("ResolveSteps() error = %v, want context.Canceled", err)
		}
	})

	t.Run("validation definition", func(t *testing.T) {
		ctx := newCancelAfterChecksContext(3)
		root := &domain.Directory{StepsDefinitions: []*domain.StepsDefinition{{}}}
		if err := NewResolver().ValidateStepsDefinitions(ctx, &domain.Suite{Root: root}); !errors.Is(err, context.Canceled) {
			t.Fatalf("ValidateStepsDefinitions() error = %v, want context.Canceled", err)
		}
	})

	t.Run("validation child", func(t *testing.T) {
		ctx := newCancelAfterChecksContext(3)
		root := &domain.Directory{}
		root.Children = []*domain.Directory{{Parent: root}}
		if err := NewResolver().ValidateStepsDefinitions(ctx, &domain.Suite{Root: root}); !errors.Is(err, context.Canceled) {
			t.Fatalf("ValidateStepsDefinitions() error = %v, want context.Canceled", err)
		}
	})
}

func TestValidateStepsDefinitionsTraversesWithoutInventingRules(t *testing.T) {
	rootDefinitions := []*domain.StepsDefinition{nil, {App: "", Kind: domain.DocumentKind("unknown")}}
	childDefinitions := []*domain.StepsDefinition{{}}
	root := &domain.Directory{StepsDefinitions: rootDefinitions}
	child := &domain.Directory{Parent: root, StepsDefinitions: childDefinitions}
	root.Children = []*domain.Directory{child}

	if err := NewResolver().ValidateStepsDefinitions(context.Background(), &domain.Suite{Root: root}); err != nil {
		t.Fatalf("ValidateStepsDefinitions() error = %v, want nil without declared field rules", err)
	}
	if !sameDefinitionSlice(root.StepsDefinitions, rootDefinitions) || !sameDefinitionSlice(child.StepsDefinitions, childDefinitions) {
		t.Fatal("ValidateStepsDefinitions() mutated decoded definitions")
	}
}

func TestResolverAcceptsEmptySuites(t *testing.T) {
	resolver := NewResolver()
	for name, method := range map[string]func(context.Context, *domain.Suite) error{
		"resolve defaults": resolver.ResolveDefaults,
		"resolve steps":    resolver.ResolveSteps,
		"validate steps":   resolver.ValidateStepsDefinitions,
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

func resolverDefaults(defaults domain.Defaults) *domain.DefaultsDefinition {
	return &domain.DefaultsDefinition{Spec: defaults}
}

func resolverSteps(steps ...domain.Step) *domain.StepsDefinition {
	definition := &domain.StepsDefinition{}
	definition.Spec.Steps = steps
	for index := range definition.Spec.Steps {
		definition.Spec.Steps[index].Definition = definition
		definition.Spec.Steps[index].Index = index
	}
	return definition
}

func resolverStep(method, path, baseURL, basePath string, headers map[string]string, timeout, retries int) domain.Step {
	step := domain.Step{}
	step.Request.Method = method
	step.Request.Path = path
	step.Request.BaseURL = baseURL
	step.Request.BasePath = basePath
	step.Request.Headers = headers
	step.Request.Timeout = timeout
	step.Request.Retries = retries
	return step
}

func assertDefaults(t *testing.T, got, want domain.Defaults) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("resolved defaults = %#v, want %#v", got, want)
	}
}

func assertRequestDefaults(t *testing.T, got domain.Step, baseURL, basePath string, headers map[string]string, timeout, retries int) {
	t.Helper()
	if got.Request.BaseURL != baseURL || got.Request.BasePath != basePath || !reflect.DeepEqual(got.Request.Headers, headers) ||
		got.Request.Timeout != timeout || got.Request.Retries != retries {
		t.Fatalf("resolved request = %#v, want baseURL %q, basePath %q, headers %#v, timeout %d, retries %d",
			got.Request, baseURL, basePath, headers, timeout, retries)
	}
}

type cancelAfterChecksContext struct {
	context.Context
	cancel context.CancelFunc
	checks int
	after  int
}

func newCancelAfterChecksContext(after int) *cancelAfterChecksContext {
	ctx, cancel := context.WithCancel(context.Background())
	return &cancelAfterChecksContext{Context: ctx, cancel: cancel, after: after}
}

func (c *cancelAfterChecksContext) Err() error {
	c.checks++
	if c.checks == c.after {
		c.cancel()
	}
	return c.Context.Err()
}
