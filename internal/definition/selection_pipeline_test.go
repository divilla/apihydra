package definition

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/divilla/apihydra/internal/domain"
	"github.com/divilla/apihydra/pkg/errs"
)

func loadSelectionPipeline(ctx context.Context, suite *domain.Suite, targets []string) error {
	loader := NewLoader()
	decoder := NewDecoder()
	resolver := NewResolver()
	if err := loader.Select(ctx, suite, targets); err != nil {
		return err
	}
	for _, phase := range []func(context.Context, *domain.Suite) error{loader.LoadDirectoryStructure, loader.LoadDirectoryFiles, loader.DecodeBaseDefinitions, decoder.DecodeFiles, decoder.ValidateDefaultsDefinitions, decoder.ValidateStepsDefinitions, resolver.ResolveDefaults, resolver.ResolveSteps} {
		if err := phase(ctx, suite); err != nil {
			return err
		}
	}
	return nil
}

func TestSelectionPipelinePreservesAncestorsAndValidatesOnlyApplicableFiles(t *testing.T) {
	root := selectionFixture(t, map[string]string{
		"root.yaml":           "app: apihydra\nkind: root\nspec:\n  base_url: http://example.test\n  headers: {Root: inherited}\n",
		"unselected.yaml":     "app: apihydra\nkind: bad\n",
		"sibling/broken.yaml": "[",
		"a/defaults.yaml":     "app: apihydra\nkind: defaults\nspec:\n  base_path: /ancestor\n  disable_cookies: true\n",
		"a/unselected.yaml":   "app: apihydra\nkind: steps\nspec: [",
		"a/b/steps.yml":       "app: apihydra\nkind: steps\nspec:\n  defaults:\n    headers: {File: inherited}\n  steps:\n    - request: {path: /zero}\n    - request: {path: /one, defaults: {disable_cookies: false}}\n    - request: {path: /two}\n",
	})
	suite := &domain.Suite{WorkDir: filepath.Join(root, "a/b")}
	if err := loadSelectionPipeline(context.Background(), suite, []string{"steps.yml:1-2", "steps.yml:1"}); err != nil {
		t.Fatal(err)
	}
	if suite.WorkDir != root || suite.Root.Path != "/" || len(suite.Root.Children) != 1 || len(suite.Root.StepsFiles) != 0 {
		t.Fatalf("root = %+v", suite.Root)
	}
	a := suite.Root.Children[0]
	b := a.Children[0]
	if a.Stage != 1 || b.Stage != 2 || b.Parent != a || b.Path != "/a/b" || len(a.StepsFiles) != 0 {
		t.Fatal("ancestor structure or scope")
	}
	steps := b.ResolvedSteps[0]
	if len(steps) != 2 || steps[0].Index != 1 || steps[1].Index != 2 || len(b.StepsDefinitions[0].Spec.Steps) != 3 {
		t.Fatalf("selected = %+v", steps)
	}
	defaults := steps[0].Request.Defaults
	if defaults.BaseURL != "http://example.test" || defaults.BasePath != "/ancestor" || defaults.Headers["Root"] != "inherited" || defaults.Headers["File"] != "inherited" || defaults.DisableCookies == nil || *defaults.DisableCookies {
		t.Fatalf("defaults = %+v", defaults)
	}
	if steps[1].Request.Defaults.DisableCookies == nil || !*steps[1].Request.Defaults.DisableCookies {
		t.Fatal("ancestor cookie setting lost")
	}
	// A directory selection includes malformed files within that subtree.
	if err := loadSelectionPipeline(context.Background(), &domain.Suite{WorkDir: root}, []string{"a"}); !errors.Is(err, ErrInvalidDefinition) {
		t.Fatalf("directory validation = %v", err)
	}
	// Full selected files are decoded before filtering out individual steps.
	path := filepath.Join(root, "a/b/steps.yml")
	if err := os.WriteFile(path, []byte("app: apihydra\nkind: steps\nspec:\n  steps:\n    - request: {defaults: {timeout: invalid}}\n    - {}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := loadSelectionPipeline(context.Background(), &domain.Suite{WorkDir: root}, []string{"a/b/steps.yml:1"}); !errors.Is(err, ErrInvalidDefinition) || !strings.Contains(err.Error(), "steps.yml") {
		t.Fatalf("unselected source step validation = %v", err)
	}
}

func TestSelectionPipelineRejectsInvalidDefaultsAndNonStepsTargets(t *testing.T) {
	for _, contents := range []string{
		"{app: apihydra, kind: defaults, spec: [}",
		"{\"kind\": \"defaults\", 'app': 'apihydra', spec: [}",
		"  app: apihydra\n  kind: defaults\n  spec: [",
		"--- # defaults\n  app: apihydra\n  kind: defaults\n  spec: [",
		"---\t# defaults\n  app: apihydra\n  kind: defaults\n  spec: [",
		"  app: apihydra\n  kind: defaults\n  spec:\nbase_url: broken",
		"---\n# comment\n  spec:\nbase_url: broken\n  kind: defaults\n  app: apihydra",
		"  app: >-\n    apihydra\n  spec:\nbase_url: broken\n  kind: |-\n    defaults",
		"---\n# comment\n  spec: [\n  kind: defaults\n  app: apihydra",
		"app: apihydra\nkind: defaults\nspec: [",
		"app: apihydra\rkind: defaults\rspec: [\r",
		"spec: [\nkind: defaults\napp: apihydra",
		"app: apihydra\nkind: defaults\nspec: {timeout: invalid}",
		"app: >-\n  apihydra\nkind: defaults\nspec: [",
		"app: |-\n  apihydra\nkind: >-\n  defaults\nspec: [",
		"---\n  spec: [\n  kind: |-\n    defaults\n  app: >-\n    apihydra\n",
	} {
		root := selectionFixture(t, map[string]string{"root.yaml": "app: apihydra\nkind: root\nspec: {}", "a/defaults.yml": contents, "a/steps.yaml": "app: apihydra\nkind: steps\nspec: {steps: []}"})
		if err := loadSelectionPipeline(context.Background(), &domain.Suite{WorkDir: root}, []string{"a/steps.yaml"}); !errors.Is(err, ErrInvalidDefinition) || !strings.Contains(err.Error(), "defaults.yml") || errs.Code(err, 0) != 102 {
			t.Fatalf("required defaults validation = %v", err)
		}
	}
	root := selectionFixture(t, map[string]string{"root.yaml": "app: apihydra\nkind: root\nspec: {}", "other.yaml": "app: other\nkind: steps\nspec: {steps: []}"})
	for _, target := range []string{"root.yaml", "other.yaml"} {
		if err := loadSelectionPipeline(context.Background(), &domain.Suite{WorkDir: root}, []string{target}); !errors.Is(err, ErrInvalidSelection) {
			t.Fatalf("target %s = %v", target, err)
		}
	}
}

func TestSelectionPipelineExecutesSymlinkDirectoryTargets(t *testing.T) {
	root := selectionFixture(t, map[string]string{
		"root.yaml":       "app: apihydra\nkind: root\nspec: {}",
		"real/steps.yaml": "app: apihydra\nkind: steps\nspec: {steps: [{}]}",
	})
	if err := os.Symlink("real", filepath.Join(root, "alias")); err != nil {
		t.Fatal(err)
	}
	for _, targets := range [][]string{{"alias"}, {"alias/steps.yaml"}, {"alias", "real", "alias/steps.yaml:0"}} {
		suite := &domain.Suite{WorkDir: root}
		if err := loadSelectionPipeline(context.Background(), suite, targets); err != nil {
			t.Fatal(err)
		}
		if len(suite.Root.Children) != 1 {
			t.Fatalf("children = %+v", suite.Root.Children)
		}
		child := suite.Root.Children[0]
		if child.Path != "/real" || len(child.ResolvedSteps) != 1 || len(child.ResolvedSteps[0]) != 1 || child.ResolvedSteps[0][0].Index != 0 {
			t.Fatalf("selected work = %+v", child)
		}
	}
}

func TestSelectionPipelineMatchesFilesystemCasing(t *testing.T) {
	root := selectionFixture(t, map[string]string{
		"root.yaml":      "app: apihydra\nkind: root\nspec: {}",
		"api/steps.yaml": "app: apihydra\nkind: steps\nspec: {steps: [{}, {}]}",
		"other/bad.yaml": "[",
	})
	if _, err := os.Stat(filepath.Join(root, "API")); os.IsNotExist(err) {
		t.Skip("test filesystem is case-sensitive")
	} else if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		targets []string
		indices []int
	}{
		{[]string{"API"}, []int{0, 1}},
		{[]string{"API/STEPS.YAML"}, []int{0, 1}},
		{[]string{"API/STEPS.YAML:1", "api/steps.yaml:1"}, []int{1}},
		{[]string{"API", "api/steps.yaml:1"}, []int{0, 1}},
	} {
		suite := &domain.Suite{WorkDir: root}
		if err := loadSelectionPipeline(context.Background(), suite, test.targets); err != nil {
			t.Fatal(err)
		}
		if len(suite.Root.Children) != 1 || suite.Root.Children[0].Path != "/api" {
			t.Fatalf("selected tree = %+v", suite.Root)
		}
		groups := suite.Root.Children[0].ResolvedSteps
		if len(groups) != 1 || len(groups[0]) != len(test.indices) {
			t.Fatalf("selected steps = %+v", groups)
		}
		for i, index := range test.indices {
			if groups[0][i].Index != index {
				t.Fatalf("step index = %d, want %d", groups[0][i].Index, index)
			}
		}
	}
}
