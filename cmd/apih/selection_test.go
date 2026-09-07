package main

import (
	"bytes"
	"context"
	"errors"
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

	"github.com/divilla/apihydra/internal/definition"
	"github.com/divilla/apihydra/internal/domain"
	"github.com/divilla/apihydra/internal/execution"
	"github.com/divilla/apihydra/internal/reporting"
)

func writeSelectionFile(t *testing.T, root, name, contents string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0600); err != nil {
		t.Fatal(err)
	}
}

func TestRunSelectionUnionDefaultsAndCookiesAcrossModes(t *testing.T) {
	for mode := 0; mode <= 2; mode++ {
		t.Run(fmt.Sprint(mode), func(t *testing.T) {
			setTestUserCacheDir(t, t.TempDir())
			root := t.TempDir()
			t.Chdir(root)
			var mu sync.Mutex
			var paths []string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				paths = append(paths, r.URL.Path)
				mu.Unlock()
				if r.Header.Get("Root") != "inherited" || r.Header.Get("Ancestor") != "inherited" {
					t.Error("lost inherited headers")
				}
				cookie, _ := r.Cookie("session")
				if r.URL.Path == "/api/one" {
					if cookie != nil {
						t.Error("skipped predecessor supplied cookies")
					}
					http.SetCookie(w, &http.Cookie{Name: "session", Value: "selected", Path: "/"})
				}
				if r.URL.Path == "/api/two" && (cookie == nil || cookie.Value != "selected") {
					t.Error("selected steps lost cookie state")
				}
				_, _ = w.Write([]byte(`{}`))
			}))
			defer server.Close()
			writeSelectionFile(t, root, "root.yaml", "app: apihydra\nkind: root\nspec:\n  base_url: "+server.URL+"\n  headers: {Root: inherited}\n")
			writeSelectionFile(t, root, "unselected.yaml", "[invalid")
			writeSelectionFile(t, root, "a/defaults.yaml", "app: apihydra\nkind: defaults\nspec:\n  base_path: /api\n  headers: {Ancestor: inherited}\n")
			writeSelectionFile(t, root, "a/skip.yaml", "app: apihydra\nkind: steps\nspec:\n  steps:\n    - request: {method: GET, path: /skipped-ancestor}\n")
			writeSelectionFile(t, root, "a/b/file.yaml", "app: apihydra\nkind: steps\nspec:\n  steps:\n    - request: {method: GET, path: /zero}\n    - request: {method: GET, path: /one}\n      response: {expected_status: 200}\n    - request: {method: GET, path: /two}\n      response: {expected_status: 200}\n")
			var output bytes.Buffer
			code, err := run(context.Background(), domain.Config{Parallelism: mode, Selections: []string{"a/b/file.yaml:2", "a/b/file.yaml:1-2", "a/b/file.yaml:1"}}, reporting.NewReporter(&output, false))
			if code != 0 || err != nil {
				t.Fatalf("run = %d, %v, %s", code, err, output.String())
			}
			mu.Lock()
			got := slices.Clone(paths)
			mu.Unlock()
			if !reflect.DeepEqual(got, []string{"/api/one", "/api/two"}) {
				t.Fatalf("requests = %v", got)
			}
			if !strings.HasPrefix(output.String(), "Working Directory: "+root+"\n") {
				t.Fatalf("root output = %s", output.String())
			}
		})
	}
}

func TestRunInvalidLaterSelectionMakesNoRequests(t *testing.T) {
	setTestUserCacheDir(t, t.TempDir())
	root := t.TempDir()
	t.Chdir(root)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { t.Error("invalid invocation executed a request") }))
	defer server.Close()
	writeSelectionFile(t, root, "root.yaml", "app: apihydra\nkind: root\nspec: {base_url: "+server.URL+"}")
	writeSelectionFile(t, root, "steps.yaml", "app: apihydra\nkind: steps\nspec:\n  steps:\n    - request: {method: GET, path: /}\n")
	for _, target := range []string{"steps.yaml:1", "steps.yaml:3-1", "missing.yaml", "root.yaml"} {
		var output bytes.Buffer
		code, err := run(context.Background(), domain.Config{Parallelism: 1, Selections: []string{"steps.yaml", target}}, reporting.NewReporter(&output, false))
		if code != 102 || !errors.Is(err, definition.ErrInvalidSelection) {
			t.Fatalf("target %s: %d, %v", target, code, err)
		}
		if !strings.HasSuffix(fatalDiagnostic(err), "#invalid-arguments\n") {
			t.Fatalf("footer = %s", fatalDiagnostic(err))
		}
	}
}

func TestRunSelectedStepDoesNotLoadSkippedVariables(t *testing.T) {
	setTestUserCacheDir(t, t.TempDir())
	root := t.TempDir()
	t.Chdir(root)
	writeRootDefinition(t, root, "root.yaml")
	writeSelectionFile(t, root, "steps.yaml", "app: apihydra\nkind: steps\nspec:\n  steps:\n    - vars: {from_previous: supplied}\n    - request: {body: '$from_previous'}\n")
	var output bytes.Buffer
	code, err := run(context.Background(), domain.Config{Selections: []string{"steps.yaml:1"}}, reporting.NewReporter(&output, false))
	if code == 0 || !errors.Is(err, execution.ErrVariable) || !strings.Contains(err.Error(), "spec.steps[1]") {
		t.Fatalf("skipped variable failure: %d, %v", code, err)
	}
}
