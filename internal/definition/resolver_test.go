package definition

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/divilla/apihydra/internal/domain"
)

func TestResolverContractAndConstructor(t *testing.T) {
	var _ func() *Resolver = NewResolver
	var _ func(*Resolver, context.Context, *domain.Suite) error = (*Resolver).ResolveDefaults
	var _ func(*Resolver, context.Context, *domain.Suite) error = (*Resolver).ResolveSteps
	var _ func(*Resolver, context.Context, *domain.Suite) error = (*Resolver).ValidateStepsDefinitions

	if resolver := NewResolver(); resolver == nil {
		t.Fatal("NewResolver() = nil")
	}
	if got := reflect.TypeOf(Resolver{}).NumField(); got != 0 {
		t.Fatalf("Resolver fields = %d, want stateless Resolver", got)
	}
}

func TestResolveDefaultsMergesRootAndNestedDefinitions(t *testing.T) {
	root := &domain.Directory{Stage: 0, Path: "/"}
	child := &domain.Directory{Stage: 1, Path: "/child", Parent: root}
	grandchild := &domain.Directory{Stage: 2, Path: "/child/grandchild", Parent: child}
	sibling := &domain.Directory{Stage: 1, Path: "/sibling", Parent: root}
	root.Children = []*domain.Directory{child, sibling}
	child.Children = []*domain.Directory{grandchild}

	root.DefaultsDefinition = defaultsDefinition(root, domain.Defaults{
		BaseURL:  "https://root.example",
		BasePath: "/v1",
		Headers:  map[string]string{"Accept": "root", "X-Root": "yes"},
		Timeout:  10,
		Retries:  2,
	})
	child.DefaultsDefinition = defaultsDefinition(child, domain.Defaults{
		BasePath: "/v2",
		Headers:  map[string]string{"Accept": "child", "X-Child": "yes"},
		Timeout:  4,
	})
	sibling.DefaultsDefinition = defaultsDefinition(sibling, domain.Defaults{
		BaseURL: "https://sibling.example",
		Headers: map[string]string{"X-Sibling": "yes"},
		Retries: 5,
	})
	root.ResolvedSteps = [][]domain.Step{{{Debug: true}}}
	root.RuntimeSteps = [][]domain.Step{{{Debug: true}}}

	if err := NewResolver().ResolveDefaults(context.Background(), &domain.Suite{Root: root}); err != nil {
		t.Fatalf("ResolveDefaults() error = %v", err)
	}

	assertDefaults(t, root.ResolvedDefaults, domain.Defaults{
		BaseURL:  "https://root.example",
		BasePath: "/v1",
		Headers:  map[string]string{"Accept": "root", "X-Root": "yes"},
		Timeout:  10,
		Retries:  2,
	})
	assertDefaults(t, child.ResolvedDefaults, domain.Defaults{
		BaseURL:  "https://root.example",
		BasePath: "/v2",
		Headers:  map[string]string{"Accept": "child", "X-Root": "yes", "X-Child": "yes"},
		Timeout:  4,
		Retries:  2,
	})
	assertDefaults(t, grandchild.ResolvedDefaults, child.ResolvedDefaults)
	assertDefaults(t, sibling.ResolvedDefaults, domain.Defaults{
		BaseURL:  "https://sibling.example",
		BasePath: "/v1",
		Headers:  map[string]string{"Accept": "root", "X-Root": "yes", "X-Sibling": "yes"},
		Timeout:  10,
		Retries:  5,
	})

	child.ResolvedDefaults.Headers["Accept"] = "mutated"
	if root.ResolvedDefaults.Headers["Accept"] != "root" {
		t.Fatal("child resolved headers alias root resolved headers")
	}
	if grandchild.ResolvedDefaults.Headers["Accept"] != "child" {
		t.Fatal("child resolved headers alias grandchild resolved headers")
	}
	if child.DefaultsDefinition.Spec.Headers["Accept"] != "child" {
		t.Fatal("resolved headers alias the decoded defaults definition")
	}
	if !root.ResolvedSteps[0][0].Debug || !root.RuntimeSteps[0][0].Debug {
		t.Fatal("ResolveDefaults() mutated resolved or runtime steps")
	}
}

func TestResolveDefaultsReplacesOnlyResolvedDefaultsAndAllowsEmptyTree(t *testing.T) {
	root := &domain.Directory{
		Path:             "/",
		ResolvedDefaults: domain.Defaults{BaseURL: "replace-me"},
	}
	child := &domain.Directory{
		Path:             "/child",
		Parent:           root,
		ResolvedDefaults: domain.Defaults{BaseURL: "replace-me-too"},
	}
	root.Children = []*domain.Directory{child}

	resolver := NewResolver()
	if err := resolver.ResolveDefaults(context.Background(), &domain.Suite{Root: root}); err != nil {
		t.Fatalf("ResolveDefaults() error = %v", err)
	}
	assertDefaults(t, root.ResolvedDefaults, domain.Defaults{Timeout: 10, Retries: 3})
	assertDefaults(t, child.ResolvedDefaults, domain.Defaults{Timeout: 10, Retries: 3})
	if err := resolver.ResolveDefaults(context.Background(), &domain.Suite{}); err != nil {
		t.Fatalf("ResolveDefaults() empty-tree error = %v", err)
	}
}

func TestResolveDefaultsAppliesProductTimeoutAndRetryFallbacks(t *testing.T) {
	root := &domain.Directory{Path: "/"}
	child := &domain.Directory{Path: "/child", Parent: root}
	root.Children = []*domain.Directory{child}
	child.DefaultsDefinition = defaultsDefinition(child, domain.Defaults{Timeout: 4})

	resolver := NewResolver()
	if err := resolver.ResolveDefaults(context.Background(), &domain.Suite{Root: root}); err != nil {
		t.Fatalf("ResolveDefaults() error = %v", err)
	}

	assertDefaults(t, root.ResolvedDefaults, domain.Defaults{Timeout: 10, Retries: 3})
	assertDefaults(t, child.ResolvedDefaults, domain.Defaults{Timeout: 4, Retries: 3})

	definition := stepsDefinition(root, "steps.yaml", 1)
	definition.Spec.Defaults.Timeout = 7
	definition.Spec.Steps[0].Request.Defaults.Retries = 9
	root.StepsDefinitions = []*domain.StepsDefinition{definition}
	if err := resolver.ResolveSteps(context.Background(), &domain.Suite{Root: root}); err != nil {
		t.Fatalf("ResolveSteps() error = %v", err)
	}
	request := root.ResolvedSteps[0][0].Request
	if request.Defaults.Timeout != 7 || request.Defaults.Retries != 9 {
		t.Fatalf("resolved request timeout/retries = %d/%d, want 7/9", request.Defaults.Timeout, request.Defaults.Retries)
	}
}

func TestResolveDefaultsAndStepsOverlayDisableCookiesByPresence(t *testing.T) {
	root := &domain.Directory{Path: "/"}
	child := &domain.Directory{Path: "/child", Parent: root}
	grandchild := &domain.Directory{Path: "/child/grandchild", Parent: child}
	root.Children = []*domain.Directory{child}
	child.Children = []*domain.Directory{grandchild}
	root.DefaultsDefinition = defaultsDefinition(root, domain.Defaults{DisableCookies: boolPointer(true)})
	child.DefaultsDefinition = defaultsDefinition(child, domain.Defaults{DisableCookies: boolPointer(false)})

	resolver := NewResolver()
	if err := resolver.ResolveDefaults(context.Background(), &domain.Suite{Root: root}); err != nil {
		t.Fatalf("ResolveDefaults() error = %v", err)
	}
	assertDisableCookies(t, root.ResolvedDefaults.DisableCookies, true)
	assertDisableCookies(t, child.ResolvedDefaults.DisableCookies, false)
	assertDisableCookies(t, grandchild.ResolvedDefaults.DisableCookies, false)
	if root.ResolvedDefaults.DisableCookies == root.DefaultsDefinition.Spec.DisableCookies ||
		child.ResolvedDefaults.DisableCookies == child.DefaultsDefinition.Spec.DisableCookies {
		t.Fatal("resolved DisableCookies pointers alias a decoded scope")
	}
	if grandchild.ResolvedDefaults.DisableCookies == child.ResolvedDefaults.DisableCookies {
		t.Fatal("inherited DisableCookies pointer aliases its parent")
	}
	*grandchild.ResolvedDefaults.DisableCookies = true
	assertDisableCookies(t, child.ResolvedDefaults.DisableCookies, false)

	definition := stepsDefinition(child, "steps.yaml", 3)
	definition.Spec.Defaults.DisableCookies = boolPointer(true)
	definition.Spec.Steps[0].Request.Defaults.DisableCookies = nil
	definition.Spec.Steps[1].Request.Defaults.DisableCookies = boolPointer(false)
	definition.Spec.Steps[2].Request.Defaults.DisableCookies = boolPointer(true)
	child.StepsDefinitions = []*domain.StepsDefinition{definition}
	if err := resolver.ResolveSteps(context.Background(), &domain.Suite{Root: root}); err != nil {
		t.Fatalf("ResolveSteps() error = %v", err)
	}
	assertDisableCookies(t, child.ResolvedSteps[0][0].Request.Defaults.DisableCookies, true)
	assertDisableCookies(t, child.ResolvedSteps[0][1].Request.Defaults.DisableCookies, false)
	assertDisableCookies(t, child.ResolvedSteps[0][2].Request.Defaults.DisableCookies, true)

	plain := &domain.Directory{Path: "/"}
	plainDefinition := stepsDefinition(plain, "plain.yaml", 1)
	plain.StepsDefinitions = []*domain.StepsDefinition{plainDefinition}
	if err := resolver.ResolveDefaults(context.Background(), &domain.Suite{Root: plain}); err != nil {
		t.Fatal(err)
	}
	if err := resolver.ResolveSteps(context.Background(), &domain.Suite{Root: plain}); err != nil {
		t.Fatal(err)
	}
	if plain.ResolvedDefaults.DisableCookies != nil || plain.ResolvedSteps[0][0].Request.Defaults.DisableCookies != nil {
		t.Fatal("all-nil DisableCookies chain did not remain nil")
	}
}

func TestResolveDefaultsReturnsCancellationWithoutPartialMutation(t *testing.T) {
	root := &domain.Directory{Path: "/", ResolvedDefaults: domain.Defaults{BaseURL: "old-root"}}
	child := &domain.Directory{Path: "/child", Parent: root, ResolvedDefaults: domain.Defaults{BaseURL: "old-child"}}
	root.Children = []*domain.Directory{child}
	root.DefaultsDefinition = defaultsDefinition(root, domain.Defaults{BaseURL: "new-root"})

	err := NewResolver().ResolveDefaults(&checkingContext{cancelAt: 3}, &domain.Suite{Root: root})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("ResolveDefaults() error = %v, want context.Canceled", err)
	}
	if root.ResolvedDefaults.BaseURL != "old-root" || child.ResolvedDefaults.BaseURL != "old-child" {
		t.Fatal("ResolveDefaults() partially mutated values after traversal cancellation")
	}

	err = NewResolver().ResolveDefaults(&checkingContext{cancelAt: 2}, &domain.Suite{Root: root})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("ResolveDefaults() directory error = %v, want context.Canceled", err)
	}
	if root.ResolvedDefaults.BaseURL != "old-root" || child.ResolvedDefaults.BaseURL != "old-child" {
		t.Fatal("ResolveDefaults() partially mutated values after directory cancellation")
	}

	single := &domain.Directory{Path: "/", ResolvedDefaults: domain.Defaults{BaseURL: "old"}}
	err = NewResolver().ResolveDefaults(&checkingContext{cancelAt: 3}, &domain.Suite{Root: single})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("ResolveDefaults() pre-commit error = %v, want context.Canceled", err)
	}
	if single.ResolvedDefaults.BaseURL != "old" {
		t.Fatal("ResolveDefaults() committed after cancellation")
	}
}

func TestResolveStepsAppliesDefaultsAndPreservesOrderAndProvenance(t *testing.T) {
	root := &domain.Directory{
		Stage: 0,
		Path:  "/",
		ResolvedDefaults: domain.Defaults{
			BaseURL:  "https://root.example",
			BasePath: "/v1",
			Headers:  map[string]string{"Accept": "default", "X-Default": "yes"},
			Timeout:  10,
			Retries:  2,
		},
	}
	child := &domain.Directory{
		Stage:  1,
		Path:   "/child",
		Parent: root,
		ResolvedDefaults: domain.Defaults{
			BaseURL: "https://child.example",
			Headers: map[string]string{"X-Child": "yes"},
			Timeout: 3,
		},
	}
	root.Children = []*domain.Directory{child}

	first := stepsDefinition(root, "first.yaml", 2)
	first.Spec.Defaults = domain.Defaults{
		BasePath: "/definition",
		Headers:  map[string]string{"Accept": "definition", "X-Definition": "yes"},
		Timeout:  6,
	}
	first.Spec.Steps[0].Vars = map[string]domain.YAMLString{"name": "alice"}
	first.Spec.Steps[0].Request.Method = "POST"
	first.Spec.Steps[0].Request.Defaults.BasePath = "/local"
	first.Spec.Steps[0].Request.Path = "/users"
	first.Spec.Steps[0].Request.Defaults.Headers = map[string]string{"Accept": "step", "X-Step": "yes"}
	first.Spec.Steps[0].Request.Defaults.Retries = 7
	first.Spec.Steps[0].Request.Query = "active=true"
	first.Spec.Steps[0].Request.Body = `{"name":"$name"}`
	first.Spec.Steps[0].Response.ExpectedStatus = 201
	first.Spec.Steps[0].Response.ExpectedBody = `{"created":true}`
	first.Spec.Steps[0].Response.ExpectedTypes = map[string][]string{".created": {"boolean", "null"}}
	first.Spec.Steps[0].Response.Capture = map[string]domain.YAMLString{"id": ".id"}
	first.Spec.Steps[0].Debug = true

	second := stepsDefinition(root, "second.yaml", 1)
	second.Spec.Defaults = domain.Defaults{BaseURL: "https://definition.example", Retries: 5}
	second.Spec.Steps[0].Request.Defaults.BaseURL = "https://step.example"
	second.Spec.Steps[0].Request.Defaults.Timeout = 1
	root.StepsDefinitions = []*domain.StepsDefinition{first, second, nil}

	childDefinition := stepsDefinition(child, "child/steps.yaml", 1)
	child.StepsDefinitions = []*domain.StepsDefinition{childDefinition}
	root.RuntimeSteps = [][]domain.Step{{{Debug: true}}}
	root.DefaultsDefinition = defaultsDefinition(root, domain.Defaults{BaseURL: "decoded-must-not-be-used"})

	if err := NewResolver().ResolveSteps(context.Background(), &domain.Suite{Root: root}); err != nil {
		t.Fatalf("ResolveSteps() error = %v", err)
	}

	if got, want := len(root.ResolvedSteps), 3; got != want {
		t.Fatalf("len(root.ResolvedSteps) = %d, want %d", got, want)
	}
	if got, want := len(root.ResolvedSteps[0]), 2; got != want {
		t.Fatalf("len(root.ResolvedSteps[0]) = %d, want %d", got, want)
	}
	if got, want := len(root.ResolvedSteps[1]), 1; got != want {
		t.Fatalf("len(root.ResolvedSteps[1]) = %d, want %d", got, want)
	}
	if root.ResolvedSteps[2] != nil {
		t.Fatalf("root.ResolvedSteps[2] = %#v, want nil group for nil definition", root.ResolvedSteps[2])
	}

	resolved := &root.ResolvedSteps[0][0]
	if resolved.Definition != first || resolved.Index != 0 {
		t.Fatalf("resolved provenance = {%p %d}, want {%p 0}", resolved.Definition, resolved.Index, first)
	}
	if resolved.Request.Method != "POST" || resolved.Request.Defaults.BaseURL != "https://root.example" || resolved.Request.Defaults.BasePath != "/local" || resolved.Request.Path != "/users" {
		t.Fatalf("resolved request strings = %+v", resolved.Request)
	}
	if resolved.Request.Defaults.Timeout != 6 || resolved.Request.Defaults.Retries != 7 || resolved.Request.Query != "active=true" || resolved.Request.Body != `{"name":"$name"}` {
		t.Fatalf("resolved request values = %+v", resolved.Request)
	}
	if !reflect.DeepEqual(resolved.Request.Defaults.Headers, map[string]string{"Accept": "step", "X-Default": "yes", "X-Definition": "yes", "X-Step": "yes"}) {
		t.Fatalf("resolved headers = %v", resolved.Request.Defaults.Headers)
	}
	if resolved.Response.ExpectedStatus != 201 || resolved.Response.ExpectedBody != `{"created":true}` || !resolved.Debug {
		t.Fatalf("resolved response/debug = response:%+v debug:%t", resolved.Response, resolved.Debug)
	}

	inherited := root.ResolvedSteps[0][1]
	if inherited.Definition != first || inherited.Index != 1 || inherited.Request.Defaults.BaseURL != "https://root.example" || inherited.Request.Defaults.BasePath != "/definition" || inherited.Request.Defaults.Timeout != 6 || inherited.Request.Defaults.Retries != 2 || inherited.Request.Defaults.Headers["X-Definition"] != "yes" {
		t.Fatalf("second resolved step = %+v", inherited)
	}
	overridden := root.ResolvedSteps[1][0]
	if overridden.Definition != second || overridden.Request.Defaults.BaseURL != "https://step.example" || overridden.Request.Defaults.Timeout != 1 || overridden.Request.Defaults.BasePath != "/v1" || overridden.Request.Defaults.Retries != 5 {
		t.Fatalf("second definition resolved step = %+v", overridden)
	}
	childResolved := child.ResolvedSteps[0][0]
	if childResolved.Definition != childDefinition || childResolved.Request.Defaults.BaseURL != "https://child.example" || childResolved.Request.Defaults.Timeout != 3 || childResolved.Request.Defaults.BasePath != "" || childResolved.Request.Defaults.Retries != 0 || childResolved.Request.Defaults.Headers["X-Child"] != "yes" {
		t.Fatalf("child resolved step = %+v", childResolved)
	}
	if !root.RuntimeSteps[0][0].Debug {
		t.Fatal("ResolveSteps() mutated RuntimeSteps")
	}
	if root.DefaultsDefinition.Spec.BaseURL != "decoded-must-not-be-used" {
		t.Fatal("ResolveSteps() mutated DefaultsDefinition")
	}
}

func TestResolveStepsDeepCopiesMutableValues(t *testing.T) {
	root := &domain.Directory{
		Path: "/",
		ResolvedDefaults: domain.Defaults{
			Headers: map[string]string{"Default": "original"},
		},
	}
	definition := stepsDefinition(root, "steps.yaml", 2)
	definition.Spec.Defaults.Headers = map[string]string{"Definition": "original"}
	for index := range definition.Spec.Steps {
		definition.Spec.Steps[index].Vars = map[string]domain.YAMLString{"var": "original"}
		definition.Spec.Steps[index].Request.Defaults.Headers = map[string]string{"Step": "original"}
		definition.Spec.Steps[index].Response.ExpectedTypes = map[string][]string{".value": {"string", "null"}}
		definition.Spec.Steps[index].Response.Capture = map[string]domain.YAMLString{"captured": ".value"}
	}
	root.StepsDefinitions = []*domain.StepsDefinition{definition}

	if err := NewResolver().ResolveSteps(context.Background(), &domain.Suite{Root: root}); err != nil {
		t.Fatalf("ResolveSteps() error = %v", err)
	}
	resolved := &root.ResolvedSteps[0][0]
	resolved.Vars["var"] = "changed"
	resolved.Request.Defaults.Headers["Default"] = "changed"
	resolved.Request.Defaults.Headers["Step"] = "changed"
	resolved.Response.ExpectedTypes[".value"][0] = "changed"
	resolved.Response.Capture["captured"] = ".changed"
	resolved.Request.Defaults.Headers["Definition"] = "changed"

	source := definition.Spec.Steps[0]
	sibling := root.ResolvedSteps[0][1]
	if source.Vars["var"] != "original" || source.Request.Defaults.Headers["Step"] != "original" || source.Response.ExpectedTypes[".value"][0] != "string" || source.Response.Capture["captured"] != ".value" || definition.Spec.Defaults.Headers["Definition"] != "original" {
		t.Fatal("resolved mutable values alias the decoded step")
	}
	if root.ResolvedDefaults.Headers["Default"] != "original" {
		t.Fatal("resolved request headers alias ResolvedDefaults")
	}
	if sibling.Vars["var"] != "original" || sibling.Request.Defaults.Headers["Default"] != "original" || sibling.Request.Defaults.Headers["Definition"] != "original" || sibling.Request.Defaults.Headers["Step"] != "original" || sibling.Response.ExpectedTypes[".value"][0] != "string" || sibling.Response.Capture["captured"] != ".value" {
		t.Fatal("resolved mutable values alias a sibling step")
	}
}

func TestResolveStepsReturnsCancellationWithoutPartialMutation(t *testing.T) {
	root := &domain.Directory{Path: "/", ResolvedSteps: [][]domain.Step{{{Debug: true}}}}
	definition := stepsDefinition(root, "steps.yaml", 2)
	root.StepsDefinitions = []*domain.StepsDefinition{definition}

	err := NewResolver().ResolveSteps(&checkingContext{cancelAt: 4}, &domain.Suite{Root: root})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("ResolveSteps() error = %v, want context.Canceled", err)
	}
	if len(root.ResolvedSteps) != 1 || !root.ResolvedSteps[0][0].Debug {
		t.Fatal("ResolveSteps() partially mutated values after cancellation")
	}

	err = NewResolver().ResolveSteps(&checkingContext{cancelAt: 2}, &domain.Suite{Root: root})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("ResolveSteps() definition error = %v, want context.Canceled", err)
	}
	if len(root.ResolvedSteps) != 1 || !root.ResolvedSteps[0][0].Debug {
		t.Fatal("ResolveSteps() partially mutated values after definition cancellation")
	}

	empty := &domain.Directory{Path: "/", ResolvedSteps: [][]domain.Step{{{Debug: true}}}}
	err = NewResolver().ResolveSteps(&checkingContext{cancelAt: 2}, &domain.Suite{Root: empty})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("ResolveSteps() pre-commit error = %v, want context.Canceled", err)
	}
	if len(empty.ResolvedSteps) != 1 || !empty.ResolvedSteps[0][0].Debug {
		t.Fatal("ResolveSteps() committed after cancellation")
	}

	if err := NewResolver().ResolveSteps(context.Background(), &domain.Suite{}); err != nil {
		t.Fatalf("ResolveSteps() empty-tree error = %v", err)
	}
}

func TestValidateStepsDefinitionsTraversesWithoutAddingFieldRules(t *testing.T) {
	root := &domain.Directory{Path: "/", StepsDefinitions: []*domain.StepsDefinition{nil}}
	child := &domain.Directory{
		Path:   "/child",
		Parent: root,
		StepsDefinitions: []*domain.StepsDefinition{
			{},
			{Kind: domain.DocumentKind("custom")},
		},
	}
	root.Children = []*domain.Directory{child}
	resolver := NewResolver()

	if err := resolver.ValidateStepsDefinitions(context.Background(), &domain.Suite{Root: root}); err != nil {
		t.Fatalf("ValidateStepsDefinitions() error = %v", err)
	}
	if err := resolver.ValidateStepsDefinitions(&checkingContext{cancelAt: 5}, &domain.Suite{Root: root}); !errors.Is(err, context.Canceled) {
		t.Fatalf("ValidateStepsDefinitions() traversal error = %v, want context.Canceled", err)
	}
	if err := resolver.ValidateStepsDefinitions(context.Background(), &domain.Suite{}); err != nil {
		t.Fatalf("ValidateStepsDefinitions() empty-tree error = %v", err)
	}
}

func defaultsDefinition(directory *domain.Directory, defaults domain.Defaults) *domain.DefaultsDefinition {
	return &domain.DefaultsDefinition{
		Spec: defaults,
		File: &domain.File{Path: directory.Path + "/defaults.yaml", Directory: directory},
	}
}

func stepsDefinition(directory *domain.Directory, path string, count int) *domain.StepsDefinition {
	definition := &domain.StepsDefinition{
		File: &domain.File{Path: path, Directory: directory},
	}
	definition.Spec.Steps = make([]domain.Step, count)
	for index := range definition.Spec.Steps {
		definition.Spec.Steps[index].Definition = definition
		definition.Spec.Steps[index].Index = index
	}
	return definition
}

func assertDefaults(t *testing.T, got, want domain.Defaults) {
	t.Helper()
	if got.BaseURL != want.BaseURL || got.BasePath != want.BasePath || got.Timeout != want.Timeout || got.Retries != want.Retries || !reflect.DeepEqual(got.Headers, want.Headers) || !reflect.DeepEqual(got.DisableCookies, want.DisableCookies) {
		t.Fatalf("defaults = %+v, want %+v", got, want)
	}
}

func assertDisableCookies(t *testing.T, got *bool, want bool) {
	t.Helper()
	if got == nil || *got != want {
		t.Fatalf("DisableCookies = %v, want %t", got, want)
	}
}

func boolPointer(value bool) *bool {
	return &value
}
