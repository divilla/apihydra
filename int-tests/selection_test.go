//go:build integration

package inttests

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
)

func runSelectionScenarios(t *testing.T, ctx context.Context, binary, runRoot, coverageDir, _ string) {
	t.Helper()
	var mu sync.Mutex
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		paths = append(paths, r.URL.Path)
		mu.Unlock()
		if r.Header.Get("Inherited") != "yes" {
			t.Error("selection lost ancestor defaults")
		}
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()
	root := filepath.Join(runRoot, "selection")
	files := map[string]string{
		"root.yml":                   "app: apihydra\nkind: root\nspec:\n  base_url: " + server.URL + "\n",
		"steps.yaml":                 "app: apihydra\nkind: steps\nspec: {defaults: {headers: {Inherited: yes}}, steps: [{request: {path: /wrong-source}}]}",
		"a/defaults.yaml":            "app: apihydra\nkind: defaults\nspec:\n  headers: {Inherited: yes}\n",
		"a/steps.yaml":               "app: apihydra\nkind: steps\nspec: {steps: [{request: {path: /parent}}]}",
		"a/b/steps.yaml":             "app: apihydra\nkind: steps\nspec:\n  steps:\n    - request: {method: GET, path: /zero}\n      vars: {previous: supplied}\n    - request: {method: GET, path: /one}\n    - request: {method: GET, path: /two}\n    - request: {method: GET, path: /three}\n",
		"a/unselected.yaml":          "app: apihydra\nkind: steps\nspec: [",
		"unrelated/bad.yaml":         "[",
		"nested/root.yaml":           "app: apihydra\nkind: root\nspec: {}",
		"bad-defaults/defaults.yaml": "---\n# indented envelope\n  app: apihydra\n  kind: defaults\n  spec: [\n    broken: value\n",
		"bad-defaults/steps.yaml":    "app: apihydra\nkind: steps\nspec: {steps: [{request: {path: /must-not-run}}]}",
		"multiline/defaults.yaml":    "app: >-\n  apihydra\nkind: defaults\nspec: [",
		"multiline/steps.yaml":       "app: apihydra\nkind: steps\nspec: {steps: [{request: {path: /must-not-run}}]}",
		"flow/defaults.yaml":         "{app: apihydra, kind: defaults, spec: [}",
		"flow/steps.yaml":            "app: apihydra\nkind: steps\nspec: {steps: [{request: {path: /must-not-run}}]}",
		"directive/defaults.yaml":    "%YAML 1.2\n---\n{app: apihydra, kind: defaults, spec: [}",
		"directive/steps.yaml":       "app: apihydra\nkind: steps\nspec: {steps: [{request: {path: /must-not-run}}]}",
		"dedented/defaults.yaml":     "  app: apihydra\n  kind: defaults\n  spec:\nbase_url: broken",
		"dedented/steps.yaml":        "app: apihydra\nkind: steps\nspec: {steps: [{request: {path: /must-not-run}}]}",
		"commented/defaults.yaml":    "--- # defaults\n  app: apihydra\n  kind: defaults\n  spec: [",
		"commented/steps.yaml":       "app: apihydra\nkind: steps\nspec: {steps: [{request: {path: /must-not-run}}]}",
		"cr/defaults.yaml":           "app: apihydra\rkind: defaults\rspec: [\r",
		"cr/steps.yaml":              "app: apihydra\nkind: steps\nspec: {steps: [{request: {path: /must-not-run}}]}",
	}
	if os.PathSeparator == '/' {
		files["suite:v1/defaults.yaml"] = files["a/defaults.yaml"]
		files["suite:v1/child/steps.yaml"] = files["a/b/steps.yaml"]
	}
	for name, contents := range files {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(contents), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(filepath.Join("a", "b"), filepath.Join(root, "alias")); err != nil {
		t.Fatal(err)
	}
	for mode := 0; mode <= 2; mode++ {
		for _, test := range []struct {
			name string
			cwd  string
			args []string
			want []string
			code int
		}{
			{"colon parent directory", root, []string{"suite:v1/child"}, []string{"/zero", "/one", "/two", "/three"}, 0},
			{"colon parent file", root, []string{"suite:v1/child/steps.yaml"}, []string{"/zero", "/one", "/two", "/three"}, 0},
			{"colon parent absolute range", root, []string{filepath.Join(root, "suite:v1/child/steps.yaml:1-2")}, []string{"/one", "/two"}, 0},
			{"symlink directory", root, []string{"alias"}, []string{"/zero", "/one", "/two", "/three"}, 0},
			{"symlink overlap", root, []string{"alias", "a/b/steps.yaml:1"}, []string{"/zero", "/one", "/two", "/three"}, 0},
			{"symlink parent file", root, []string{"alias/../steps.yaml"}, []string{"/parent"}, 0},
			{"symlink parent absolute step", root, []string{root + "/alias/../steps.yaml:0"}, []string{"/parent"}, 0},
			{"symlink parent overlap", root, []string{"alias/../steps.yaml:0", "a/steps.yaml"}, []string{"/parent"}, 0},
			{"malformed inherited defaults", root, []string{"bad-defaults/steps.yaml"}, nil, 102},
			{"cr defaults file", root, []string{"cr/steps.yaml"}, nil, 102},
			{"cr defaults step", root, []string{"cr/steps.yaml:0"}, nil, 102},
			{"cr defaults directory", root, []string{"cr"}, nil, 102},
			{"cr defaults later selection", root, []string{"a/b/steps.yaml", "cr/steps.yaml"}, nil, 102},
			{"commented defaults file", root, []string{"commented/steps.yaml"}, nil, 102},
			{"commented defaults step", root, []string{"commented/steps.yaml:0"}, nil, 102},
			{"commented defaults directory", root, []string{"commented"}, nil, 102},
			{"commented defaults later selection", root, []string{"a/b/steps.yaml", "commented/steps.yaml"}, nil, 102},
			{"multiline defaults file", root, []string{"multiline/steps.yaml"}, nil, 102},
			{"multiline defaults step", root, []string{"multiline/steps.yaml:0"}, nil, 102},
			{"multiline defaults directory", root, []string{"multiline"}, nil, 102},
			{"flow defaults file", root, []string{"flow/steps.yaml"}, nil, 102},
			{"flow defaults step", root, []string{"flow/steps.yaml:0"}, nil, 102},
			{"flow defaults directory", root, []string{"flow"}, nil, 102},
			{"flow defaults later selection", root, []string{"a/b/steps.yaml", "flow/steps.yaml"}, nil, 102},
			{"directive defaults file", root, []string{"directive/steps.yaml"}, nil, 102},
			{"directive defaults step", root, []string{"directive/steps.yaml:0"}, nil, 102},
			{"directive defaults later selection", root, []string{"a/b/steps.yaml", "directive/steps.yaml"}, nil, 102},
			{"dedented defaults file", root, []string{"dedented/steps.yaml"}, nil, 102},
			{"dedented defaults step", root, []string{"dedented/steps.yaml:0"}, nil, 102},
			{"dedented defaults directory", root, []string{"dedented"}, nil, 102},
			{"dedented defaults later selection", root, []string{"a/b/steps.yaml", "dedented/steps.yaml"}, nil, 102},
			{"filesystem casing directory", root, []string{"A/B"}, []string{"/zero", "/one", "/two", "/three"}, 0},
			{"filesystem casing file", root, []string{"A/B/STEPS.YAML"}, []string{"/zero", "/one", "/two", "/three"}, 0},
			{"filesystem casing range union", root, []string{"A/B/STEPS.YAML:1-2", "a/b/steps.yaml:1"}, []string{"/one", "/two"}, 0},
			{"empty target", root, []string{"a/b", ""}, nil, 102},
			{"directory suffix", root, []string{"a/b", "a/b:0"}, nil, 102},
			{"invalid range end", root, []string{"a/b", "a/b/steps.yaml:0-x"}, nil, 102},
			{"extra range bounds", root, []string{"a/b", "a/b/steps.yaml:0-1-2"}, nil, 102},
			{"subtree", root, []string{"a/b"}, []string{"/zero", "/one", "/two", "/three"}, 0},
			{"current inner", filepath.Join(root, "a/b"), nil, []string{"/zero", "/one", "/two", "/three"}, 0},
			{"file", root, []string{filepath.Join(root, "a/b/steps.yaml")}, []string{"/zero", "/one", "/two", "/three"}, 0},
			{"first", root, []string{"a/b/steps.yaml:0"}, []string{"/zero"}, 0},
			{"range union", root, []string{"a/b/steps.yaml:3", "a/b/steps.yaml:1-3", "a/b/steps.yaml:2"}, []string{"/one", "/two", "/three"}, 0},
			{"directory overlap", root, []string{"a/b/steps.yaml:1", "a/b"}, []string{"/zero", "/one", "/two", "/three"}, 0},
			{"out of bounds", root, []string{"a/b", "a/b/steps.yaml:4"}, nil, 102},
			{"reversed", root, []string{"a/b", "a/b/steps.yaml:3-1"}, nil, 102},
			{"different roots", root, []string{"a/b", "nested"}, nil, 102},
			{"invalid file", root, []string{"a/b", "root.yml"}, nil, 102},
			{"wrong scope includes invalid", root, []string{"a"}, nil, 102},
		} {
			t.Run(fmt.Sprintf("selection/%d/%s", mode, test.name), func(t *testing.T) {
				if strings.HasPrefix(test.name, "colon parent") && os.PathSeparator != '/' {
					t.Skip("colons in directory names require Unix paths")
				}
				if strings.HasPrefix(test.name, "filesystem casing") {
					if _, err := os.Stat(filepath.Join(root, "A/B")); os.IsNotExist(err) {
						t.Skip("test filesystem is case-sensitive")
					} else if err != nil {
						t.Fatal(err)
					}
				}
				mu.Lock()
				paths = nil
				mu.Unlock()
				args := append([]string{fmt.Sprintf("-p%d", mode)}, test.args...)
				var stdout strings.Builder
				result := runCLIArguments(t, ctx, binary, test.cwd, coverageDir, args, &stdout)
				if result.exitCode != test.code {
					t.Fatalf("code %d, want %d; %s", result.exitCode, test.code, result.stderr)
				}
				mu.Lock()
				got := slices.Clone(paths)
				mu.Unlock()
				if !reflect.DeepEqual(got, test.want) {
					t.Fatalf("requests = %v, want %v", got, test.want)
				}
				if test.code == 0 {
					if result.stderr != "" || !strings.HasPrefix(stdout.String(), "Working Directory: "+root+"\n") {
						t.Fatalf("output %q, %q", stdout.String(), result.stderr)
					}
				} else {
					assertFatalDiagnostic(t, result.stderr)
					if strings.HasPrefix(test.name, "cr defaults") && (!strings.Contains(result.stderr, "cr/defaults.yaml") || !strings.Contains(result.stderr, "#invalid-yaml-definition")) {
						t.Fatalf("defaults diagnostic = %q", result.stderr)
					}
					if strings.HasPrefix(test.name, "directive defaults") && (!strings.Contains(result.stderr, "directive/defaults.yaml") || !strings.Contains(result.stderr, "#invalid-yaml-definition")) {
						t.Fatalf("defaults diagnostic = %q", result.stderr)
					}
					if strings.HasPrefix(test.name, "commented defaults") && (!strings.Contains(result.stderr, "commented/defaults.yaml") || !strings.Contains(result.stderr, "#invalid-yaml-definition")) {
						t.Fatalf("defaults diagnostic = %q", result.stderr)
					}
					if strings.HasPrefix(test.name, "dedented defaults") && (!strings.Contains(result.stderr, "dedented/defaults.yaml") || !strings.Contains(result.stderr, "#invalid-yaml-definition")) {
						t.Fatalf("defaults diagnostic = %q", result.stderr)
					}
					if strings.HasPrefix(test.name, "multiline defaults") && (!strings.Contains(result.stderr, "multiline/defaults.yaml") || !strings.Contains(result.stderr, "#invalid-yaml-definition")) {
						t.Fatalf("defaults diagnostic = %q", result.stderr)
					}
					if strings.HasPrefix(test.name, "flow defaults") && (!strings.Contains(result.stderr, "flow/defaults.yaml") || !strings.Contains(result.stderr, "#invalid-yaml-definition")) {
						t.Fatalf("defaults diagnostic = %q", result.stderr)
					}
				}
			})
		}
	}
}
