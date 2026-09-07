package definition

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/divilla/apihydra/internal/domain"
	"github.com/divilla/apihydra/pkg/errs"
)

func selectionFixture(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, contents := range files {
		path := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(contents), 0600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestSelectNearestRootAndAtomicFailure(t *testing.T) {
	dir := selectionFixture(t, map[string]string{
		"outer.yml":                 "app: apihydra\nkind: root\n",
		"inner/root.yaml":           "app: apihydra\nkind: root\n",
		"inner/deep/file.yaml":      "app: apihydra\nkind: steps\n",
		"other/file.yml":            "app: apihydra\nkind: steps\n",
		"inner/deep/unrelated.yaml": "app: apihydra\nkind: invalid\n",
	})
	for _, targets := range [][]string{nil, {"."}, {"file.yaml:01-3"}, {"file.yaml", "../deep", "file.yaml:0"}} {
		suite := &domain.Suite{WorkDir: filepath.Join(dir, "inner/deep")}
		if err := NewLoader().Select(context.Background(), suite, targets); err != nil {
			t.Fatal(err)
		}
		if suite.WorkDir != filepath.Join(dir, "inner") {
			t.Fatalf("nearest root = %q", suite.WorkDir)
		}
		if len(targets) == 1 && strings.Contains(targets[0], ":") {
			got := suite.Selections[0]
			if got.First != 1 || got.Last != 3 || got.Directory {
				t.Fatalf("range = %+v", got)
			}
		}
	}
	suite := &domain.Suite{WorkDir: dir, Selections: []domain.Selection{{Path: "keep"}}}
	err := NewLoader().Select(context.Background(), suite, []string{"other/file.yml", "inner/deep/file.yaml"})
	if !errors.Is(err, ErrInvalidSelection) || !strings.Contains(err.Error(), "inner/deep/file.yaml") || errs.Code(err, 0) != 102 {
		t.Fatalf("different roots = %v", err)
	}
	if suite.WorkDir != dir || suite.Selections[0].Path != "keep" {
		t.Fatal("failed selection committed state")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := NewLoader().Select(ctx, suite, nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel = %v", err)
	}
	orphan := selectionFixture(t, map[string]string{"child/root.yml": "app: apihydra\nkind: root\n"})
	if err := NewLoader().Select(context.Background(), &domain.Suite{WorkDir: orphan}, nil); !errors.Is(err, ErrRootDefinitionMissing) {
		t.Fatalf("descendant root qualified: %v", err)
	}
	if _, err := nearestRoot(ctx, orphan); !errors.Is(err, context.Canceled) {
		t.Fatalf("root cancellation = %v", err)
	}
}

func TestParseSelectionRejectsInvalidForms(t *testing.T) {
	dir := selectionFixture(t, map[string]string{"steps.yml": "", "wrong.txt": ""})
	for _, target := range []string{"", ":0", "missing.yaml", "wrong.txt", ".:0", "steps.yml:", "steps.yml:-1", "steps.yml:3-1", "steps.yml:1-2-3", "steps.yml:1-", "steps.yml:1-x", "steps.yml:1,3", "steps.yml:+1", "steps.yml: 1", "steps.yml:99999999999999999999999999999999999999"} {
		t.Run(target, func(t *testing.T) {
			_, err := parseSelection(dir, target)
			if !errors.Is(err, ErrInvalidSelection) || errs.Code(err, 0) != 102 || !strings.Contains(err.Error(), "selection ") {
				t.Fatalf("parse = %v", err)
			}
		})
	}
	for _, target := range []string{"steps.yml", "./steps.yml:0", filepath.Join(dir, "steps.yml:2-2"), dir} {
		got, err := parseSelection(dir, target)
		if err != nil {
			t.Fatal(err)
		}
		if !filepath.IsAbs(got.Path) {
			t.Fatalf("path not absolute: %q", got.Path)
		}
	}
}

func TestSelectionScopeAndMalformedDefaults(t *testing.T) {
	root := filepath.Join(t.TempDir(), "suite")
	selections := []domain.Selection{{Path: filepath.Join(root, "a/b.yaml"), Last: 0}, {Path: filepath.Join(root, "c"), Directory: true, Last: -1}}
	for _, test := range []struct {
		path             string
		needed, selected bool
	}{{root, true, false}, {filepath.Join(root, "a"), true, false}, {filepath.Join(root, "c"), true, true}, {filepath.Join(root, "c/d"), true, true}, {filepath.Join(root, "cat"), false, false}} {
		if directoryNeeded(selections, test.path) != test.needed || directorySelected(selections, test.path) != test.selected {
			t.Fatalf("scope %s", test.path)
		}
	}
	if !fileSelected(selections, filepath.Join(root, "a/b.yaml")) || fileSelected(selections, filepath.Join(root, "a/other.yaml")) || !fileSelected(selections, filepath.Join(root, "c/any.yml")) {
		t.Fatal("file scope")
	}
	if !directoryNeeded(nil, root) || !fileSelected(nil, filepath.Join(root, "x")) {
		t.Fatal("empty selection should cover whole tree")
	}
	if !withinDirectory(filepath.Join(root, "x"), string(filepath.Separator)) {
		t.Fatal("filesystem root ancestry")
	}
	for _, test := range []struct {
		contents string
		want     bool
	}{
		{"{app: apihydra, kind: defaults, spec: [}", true},
		{"%YAML 1.2\n---\n{app: apihydra, kind: defaults, spec: [}", true},
		{"# defaults\n%YAML 1.2 # version\n%TAG !e! tag:example.com,2026:\n--- # document\n{app: apihydra, kind: defaults, spec: [}", true},
		{"%YAML 1.2\r\n---\r\n{app: apihydra, kind: root, spec: [}", true},
		{"%YAML 1.2\n---\n{app: other, kind: defaults, spec: [}", false},
		{"%YAML 1.2\n---\n{app: apihydra, kind: steps, spec: [}", false},
		{"%YAML 1.2\n---\n[{app: apihydra, kind: defaults}, [}", false},
		{"{kind: defaults, app: apihydra, spec: [}", true},
		{"---\n# defaults\n{\"app\": \"apihydra\", 'kind': 'defaults', spec: [}", true},
		{"{metadata: {name: 'a,b', labels: [one, two]}, app: apihydra, kind: defaults, spec: [}", true},
		{"{app: apihydra, # comment\n kind: defaults, spec: [}", true},
		{"{app: apihydra, kind: defaults", true},
		{"{app: apihydra, kind: root, spec: [}", true},
		{"{app: other, kind: defaults, spec: [}", false},
		{"{app: apihydra, kind: steps, spec: [}", false},
		{"{app: [apihydra], kind: defaults, spec: [}", false},
		{"{app: apihydra, metadata: {kind: defaults}, spec: [}", false},
		{"{metadata: {app: apihydra, kind: defaults}, spec: [}", false},
		{"{metadata: 'app: apihydra, kind: defaults', spec: [}", false},
		{"[{app: apihydra, kind: defaults}, [}", false},
		{"app: apihydra\nkind: defaults\nspec: [", true},
		{"app: apihydra\rkind: defaults\rspec: [\r", true},
		{"app: apihydra\r\nkind: defaults\r\nspec: [\r\n", true},
		{"app: apihydra\rkind: defaults\r\nspec: [\n", true},
		{"--- # defaults\r  app: |-\r    apihydra\r  kind: >-\r    defaults\r  spec: [\r", true},
		{"app: >\r\n  apihydra\r\nkind: defaults\r\nspec: [", false},
		{"app: apihydra\rkind: |+\r  defaults\r\rspec: [", false},
		{"app: other\rkind: defaults\rspec: [\r", false},
		{"app: apihydra\rspec: [\r  kind: defaults\r", false},
		{"app: >-\n  apihydra\nkind: defaults\nspec: [", true},
		{"app: |-\n  apihydra\nkind: >-\n  defaults\nspec: [", true},
		{"app:\n  apihydra\nkind:\n  defaults\nspec: [", true},
		{"app: \"api\\\n  hydra\"\nkind: defaults\nspec: [", true},
		{"---\n  app: >-\n    apihydra\n\n# comment\n  spec: [\n  kind: |-\n    defaults\n", true},
		{"spec: [\nkind: >-\n  defaults\napp: |-\n  apihydra", true},
		{"app: >\n  apihydra\nkind: defaults\nspec: [", false},
		{"app: apihydra\nspec: [\nkind: |+\n  defaults\n\n", false},
		{"app: >-\n  other\nkind: defaults\nspec: [", false},
		{"app: >-\n  apihydra\nkind: >-\n  steps\nspec: [", false},
		{"app: >-\n  apihydra\nspec: [\n  kind: defaults", false},
		{"spec: [\nkind: defaults\napp: apihydra", true},
		{"  app: apihydra\n  kind: defaults\n  spec: [", true},
		{"--- # defaults\n  app: apihydra\n  kind: defaults\n  spec: [", true},
		{"---\t# defaults\r\n  app: apihydra\r\n  kind: defaults\r\n  spec: [", true},
		{"... # end\n--- # defaults\n  app: apihydra\n  kind: defaults\n  spec: [", true},
		{"---#scalar\n  app: apihydra\n  kind: defaults\n  spec: [", false},
		{"---\n# defaults\n  spec: [\n  kind: defaults\n  app: apihydra", true},
		{"  app: apihydra\n  spec: [\n    kind: defaults", false},
		{"app: other\nspec: [\n  kind: defaults", false},
		{"app: apihydra\nkind: root\nspec: {}", true},
		{"app: apihydra\nkind: steps\nspec: [", false},
		{"app: []\nkind: defaults", false},
		{"[", false},
	} {
		if got := inheritedDefinition([]byte(test.contents)); got != test.want {
			t.Fatalf("envelope %q = %v", test.contents, got)
		}
	}
}

func TestFilterStepsUnionOriginalIndicesAndInvalidSubsumedRanges(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "steps.yaml")
	for _, test := range []struct {
		name       string
		selections []domain.Selection
		want       []int
		fail       bool
	}{
		{name: "whole suite", want: []int{0, 1, 2, 3}},
		{name: "whole file", selections: []domain.Selection{{Path: file, Last: -1}}, want: []int{0, 1, 2, 3}},
		{name: "single", selections: []domain.Selection{{Path: file, First: 0, Last: 0}}, want: []int{0}},
		{name: "range and duplicates", selections: []domain.Selection{{Path: file, First: 2, Last: 3}, {Path: file, First: 1, Last: 2}, {Path: file, First: 1, Last: 1}}, want: []int{1, 2, 3}},
		{name: "directory overlap", selections: []domain.Selection{{Path: file, First: 2, Last: 3}, {Path: root, Directory: true, Last: -1}}, want: []int{0, 1, 2, 3}},
		{name: "bad range subsumed by whole file", selections: []domain.Selection{{Path: file, Last: -1}, {Path: file, Last: 4}}, fail: true},
		{name: "wrong file target", selections: []domain.Selection{{Path: filepath.Join(root, "root.yaml"), Last: -1}}, fail: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			definition := &domain.StepsDefinition{App: "apihydra", Kind: domain.KindSteps, File: &domain.File{Path: "steps.yaml"}}
			for i := 0; i < 4; i++ {
				definition.Spec.Steps = append(definition.Spec.Steps, domain.Step{Index: i, Definition: definition})
			}
			dir := &domain.Directory{StepsDefinitions: []*domain.StepsDefinition{definition, nil}}
			groups := map[*domain.Directory][][]domain.Step{dir: {definition.Spec.Steps, nil}}
			err := filterSelectedSteps(&domain.Suite{WorkDir: root, Selections: test.selections}, groups)
			if test.fail {
				if !errors.Is(err, ErrInvalidSelection) {
					t.Fatalf("error = %v", err)
				}
				if len(groups[dir][0]) != 4 {
					t.Fatal("partial mutation")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			var indices []int
			for _, step := range groups[dir][0] {
				indices = append(indices, step.Index)
				if step.Definition != definition {
					t.Fatal("lost provenance")
				}
			}
			if !reflect.DeepEqual(indices, test.want) {
				t.Fatalf("indices = %v", indices)
			}
			if len(definition.Spec.Steps) != 4 {
				t.Fatal("mutated source")
			}
		})
	}
}

func TestSelectResolvesSymlinksBeforeRootDiscovery(t *testing.T) {
	dir := selectionFixture(t, map[string]string{
		"root.yaml":       "app: apihydra\nkind: root\n",
		"real/root.yaml":  "app: apihydra\nkind: root\n",
		"real/steps.yaml": "app: apihydra\nkind: steps\n",
	})
	if err := os.Symlink("real", filepath.Join(dir, "alias")); err != nil {
		t.Fatal(err)
	}
	for _, target := range []string{"alias", "alias/steps.yaml:0"} {
		suite := &domain.Suite{WorkDir: dir}
		if err := NewLoader().Select(context.Background(), suite, []string{target}); err != nil {
			t.Fatal(err)
		}
		if suite.WorkDir != filepath.Join(dir, "real") || strings.Contains(suite.Selections[0].Path, "alias") {
			t.Fatalf("symlink selection = %+v", suite)
		}
	}
}

func TestSelectResolvesSymlinksBeforeParentComponents(t *testing.T) {
	dir := selectionFixture(t, map[string]string{
		"root.yaml":              "app: apihydra\nkind: root\n",
		"steps.yaml":             "app: apihydra\nkind: steps\n",
		"nested/root.yaml":       "app: apihydra\nkind: root\n",
		"nested/steps.yaml":      "app: apihydra\nkind: steps\n",
		"nested/deep/empty.yaml": "",
	})
	if err := os.Symlink(filepath.Join("nested", "deep"), filepath.Join(dir, "alias")); err != nil {
		t.Fatal(err)
	}
	for _, target := range []string{"alias/..", "alias/../steps.yaml", "alias/../steps.yaml:0"} {
		for _, absolute := range []bool{false, true} {
			path := target
			if absolute {
				path = dir + string(filepath.Separator) + path
			}
			suite := &domain.Suite{WorkDir: dir}
			if err := NewLoader().Select(context.Background(), suite, []string{path}); err != nil {
				t.Fatal(err)
			}
			want := filepath.Join(dir, "nested")
			if suite.WorkDir != want {
				t.Fatalf("selection %q root = %q, want %q", path, suite.WorkDir, want)
			}
			if target != "alias/.." {
				want = filepath.Join(want, "steps.yaml")
			}
			if suite.Selections[0].Path != want {
				t.Fatalf("selection %q path = %q, want %q", path, suite.Selections[0].Path, want)
			}
		}
	}
}

func TestSelectPreservesColonsInParentDirectories(t *testing.T) {
	if os.PathSeparator != '/' {
		t.Skip("colons in directory names require Unix paths")
	}
	dir := selectionFixture(t, map[string]string{
		"suite:v1/root.yaml":        "app: apihydra\nkind: root\n",
		"suite:v1/child/steps.yaml": "app: apihydra\nkind: steps\n",
	})
	for _, test := range []struct {
		target string
		file   bool
		first  int
		last   int
	}{
		{"suite:v1/", false, 0, -1},
		{"suite:v1/child", false, 0, -1},
		{"suite:v1/child/steps.yaml", true, 0, -1},
		{"suite:v1/child/steps.yaml:0", true, 0, 0},
		{"suite:v1/child/steps.yaml:1-2", true, 1, 2},
	} {
		for _, absolute := range []bool{false, true} {
			target := test.target
			if absolute {
				target = dir + string(filepath.Separator) + target
			}
			t.Run(target, func(t *testing.T) {
				suite := &domain.Suite{WorkDir: dir}
				if err := NewLoader().Select(context.Background(), suite, []string{target}); err != nil {
					t.Fatal(err)
				}
				want := domain.Selection{
					Path:      filepath.Join(dir, test.target),
					Directory: !test.file, First: test.first, Last: test.last,
				}
				if test.file {
					want.Path = filepath.Join(dir, "suite:v1/child/steps.yaml")
				}
				if suite.WorkDir != filepath.Join(dir, "suite:v1") || len(suite.Selections) != 1 || suite.Selections[0] != want {
					t.Fatalf("root = %q, selections = %+v, want %+v", suite.WorkDir, suite.Selections, want)
				}
			})
		}
	}
}

func TestSelectFilesystemSpelling(t *testing.T) {
	dir := selectionFixture(t, map[string]string{
		"suite/root.yaml":      "app: apihydra\nkind: root\n",
		"suite/api/steps.yaml": "app: apihydra\nkind: steps\n",
	})
	// An unrelated hard link must never supply the canonical filename.
	if err := os.Link(filepath.Join(dir, "suite/api/steps.yaml"), filepath.Join(dir, "suite/api/aaa.txt")); err != nil {
		t.Fatal(err)
	}
	canonical, err := filesystemPath(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, prefix := range []string{"suite/api", "SUITE/API"} {
		t.Run(prefix, func(t *testing.T) {
			if _, err := os.Stat(filepath.Join(dir, prefix)); os.IsNotExist(err) {
				t.Skip("test filesystem is case-sensitive")
			} else if err != nil {
				t.Fatal(err)
			}
			file := "steps.yaml"
			if prefix == "SUITE/API" {
				file = "STEPS.YAML"
			}
			suite := &domain.Suite{WorkDir: dir}
			targets := []string{prefix, prefix + "/" + file + ":0", "suite/api/steps.yaml"}
			if err := NewLoader().Select(context.Background(), suite, targets); err != nil {
				t.Fatal(err)
			}
			root := filepath.Join(canonical, "suite")
			if suite.WorkDir != root {
				t.Fatalf("root = %q, want %q", suite.WorkDir, root)
			}
			for i, selection := range suite.Selections {
				want := filepath.Join(root, "api")
				if i > 0 {
					want = filepath.Join(want, "steps.yaml")
				}
				if selection.Path != want || selection.Directory != (i == 0) {
					t.Fatalf("selection = %+v, want %q", selection, want)
				}
			}
		})
	}
}

func TestFilesystemPathPreservesExactNamesAndErrors(t *testing.T) {
	dir := selectionFixture(t, map[string]string{"steps.yaml": ""})
	path := filepath.Join(dir, "steps.yaml")
	alias := filepath.Join(dir, "STEPS.yaml")
	if _, err := os.Stat(alias); os.IsNotExist(err) {
		if err := os.Link(path, alias); err != nil {
			t.Fatal(err)
		}
		got, err := filesystemPath(alias)
		if err != nil || filepath.Base(got) != "STEPS.yaml" {
			t.Fatalf("exact hard link name = %q, %v", got, err)
		}
	}
	for _, path := range []string{filepath.Join(dir, "missing"), filepath.Join(dir, "missing/child"), filepath.Join(path, "child")} {
		if _, err := filesystemPath(path); err == nil {
			t.Fatalf("expected filesystem error for %q", path)
		}
	}
}

func TestFilesystemNameUsesCasingAndIdentity(t *testing.T) {
	dir := selectionFixture(t, map[string]string{"steps.yaml": "", "aaa.txt": ""})
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(dir, "STEPS.YAML")
	if _, err := os.Stat(alias); os.IsNotExist(err) {
		if err := os.Link(filepath.Join(dir, "steps.yaml"), alias); err != nil {
			t.Fatal(err)
		}
	}
	// Supply the stored entries captured before creating the case alias. This
	// exercises case-insensitive directory enumeration on every test platform.
	if got, err := filesystemName(alias, entries); err != nil || got != "steps.yaml" {
		t.Fatalf("filesystem name = %q, %v", got, err)
	}
	if _, err := filesystemName(alias, entries[:1]); !os.IsNotExist(err) {
		t.Fatalf("unmatched identity = %v", err)
	}
	if _, err := filesystemName(filepath.Join(dir, "missing"), entries); !os.IsNotExist(err) {
		t.Fatalf("missing target = %v", err)
	}
	if err := os.Remove(filepath.Join(dir, "steps.yaml")); err != nil {
		t.Fatal(err)
	}
	if _, err := filesystemName(alias, entries); !os.IsNotExist(err) {
		t.Fatalf("removed entry = %v", err)
	}
}

func TestSelectBeneathNonListableAncestor(t *testing.T) {
	dir := selectionFixture(t, map[string]string{
		"parent/suite/root.yaml":      "app: apihydra\nkind: root\n",
		"parent/suite/api/steps.yaml": "app: apihydra\nkind: steps\n",
	})
	canonical, err := filesystemPath(dir)
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(canonical, "parent/suite")
	for _, ancestor := range []string{dir, filepath.Join(dir, "parent")} {
		t.Run(filepath.Base(ancestor), func(t *testing.T) {
			if err := os.Chmod(ancestor, 0100); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := os.Chmod(ancestor, 0700); err != nil {
					t.Error(err)
				}
			})
			if _, err := os.ReadDir(ancestor); err == nil {
				t.Skip("directory permissions are not enforced")
			} else if !errors.Is(err, os.ErrPermission) {
				t.Fatal(err)
			}
			for _, targets := range [][]string{nil, {"."}, {"api"}, {"api/steps.yaml"}, {"api/steps.yaml:0"}, {root}} {
				suite := &domain.Suite{WorkDir: root}
				if err := NewLoader().Select(context.Background(), suite, targets); err != nil {
					t.Fatalf("targets %v: %v", targets, err)
				}
				want := root
				directory, last := true, -1
				if len(targets) > 0 {
					switch targets[0] {
					case "api":
						want = filepath.Join(root, "api")
					case "api/steps.yaml", "api/steps.yaml:0":
						want, directory = filepath.Join(root, "api/steps.yaml"), false
						if targets[0] == "api/steps.yaml:0" {
							last = 0
						}
					}
				}
				expected := []domain.Selection{{Path: want, Directory: directory, Last: last}}
				if suite.WorkDir != root || !reflect.DeepEqual(suite.Selections, expected) {
					t.Fatalf("targets %v: root %q, selections %+v; want %q, %+v", targets, suite.WorkDir, suite.Selections, root, expected)
				}
			}
			// Search permission is still required to access the suite.
			if err := os.Chmod(ancestor, 0); err != nil {
				t.Fatal(err)
			}
			if _, err := filesystemPath(root); !errors.Is(err, os.ErrPermission) {
				t.Fatalf("inaccessible filesystem path = %v", err)
			}
			suite := &domain.Suite{WorkDir: root}
			if err := NewLoader().Select(context.Background(), suite, nil); !errors.Is(err, os.ErrPermission) || !errors.Is(err, ErrInvalidSelection) {
				t.Fatalf("inaccessible suite = %v", err)
			}
		})
	}
}
