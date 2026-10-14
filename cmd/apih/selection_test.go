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
	"time"

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
				if r.URL.Path == "/api/ancestor" {
					http.SetCookie(w, &http.Cookie{Name: "session", Value: "ancestor", Path: "/"})
				}
				if r.URL.Path == "/api/one" {
					if cookie == nil || cookie.Value != "ancestor" {
						t.Error("ancestor steps did not supply inherited cookies")
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
			writeSelectionFile(t, root, "a/b/unselected.yaml", "[invalid")
			writeSelectionFile(t, root, "a/defaults.yaml", "app: apihydra\nkind: defaults\nspec:\n  base_path: /api\n  headers: {Ancestor: inherited}\n")
			writeSelectionFile(t, root, "a/setup.yaml", "app: apihydra\nkind: steps\nspec:\n  steps:\n    - request: {method: GET, path: /ancestor}\n")
			writeSelectionFile(t, root, "a/b/file.yaml", "app: apihydra\nkind: steps\nspec:\n  steps:\n    - request: {method: GET, path: /zero}\n    - request: {method: GET, path: /one}\n      response: {expected_status: 200}\n    - request: {method: GET, path: /two}\n      response: {expected_status: 200}\n")
			var output bytes.Buffer
			code, err := run(context.Background(), domain.Config{Parallelism: mode, Selections: []string{"a/b/file.yaml:2", "a/b/file.yaml:1-2", "a/b/file.yaml:1"}}, reporting.NewReporter(&output, false))
			if code != 0 || err != nil {
				t.Fatalf("run = %d, %v, %s", code, err, output.String())
			}
			mu.Lock()
			got := slices.Clone(paths)
			mu.Unlock()
			if !reflect.DeepEqual(got, []string{"/api/ancestor", "/api/one", "/api/two"}) {
				t.Fatalf("requests = %v", got)
			}
			if !strings.HasPrefix(output.String(), "Working Directory: "+root+"\n") {
				t.Fatalf("root output = %s", output.String())
			}
		})
	}
}

func TestRunAncestorCapturesFileOrderAndConcurrentChildren(t *testing.T) {
	setTestUserCacheDir(t, t.TempDir())
	root := t.TempDir()
	t.Chdir(root)
	var mu sync.Mutex
	var paths []string
	childCount := 0
	childrenReady := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		paths = append(paths, r.URL.Path)
		if r.URL.Path == "/products" || r.URL.Path == "/users" {
			childCount++
			if childCount == 2 {
				close(childrenReady)
			}
		}
		mu.Unlock()
		if r.URL.Path == "/auth" {
			http.SetCookie(w, &http.Cookie{Name: "session", Value: "root", Path: "/"})
		} else {
			if r.Header.Get("Authorization") != "secret" {
				t.Error("ancestor capture was not available")
			}
			if r.URL.Path == "/parent" && r.Header.Get("Root-Value") != "ready" {
				t.Error("ancestor variable was not available")
			}
			wantCookie := "root"
			if r.URL.Path == "/products" || r.URL.Path == "/users" {
				wantCookie = "parent"
				select {
				case <-childrenReady:
				case <-r.Context().Done():
					return
				case <-time.After(3 * time.Second):
					t.Error("same-stage children did not overlap")
				}
			}
			cookie, err := r.Cookie("session")
			if err != nil || cookie.Value != wantCookie {
				t.Errorf("%s inherited cookie = %v, %v", r.URL.Path, cookie, err)
			}
			if r.URL.Path == "/parent" {
				http.SetCookie(w, &http.Cookie{Name: "session", Value: "parent", Path: "/"})
			}
		}
		_, _ = w.Write([]byte(`{"token":"secret"}`))
	}))
	defer server.Close()
	writeSelectionFile(t, root, "root.yaml", "app: apihydra\nkind: root\nspec: {base_url: "+server.URL+"}\n")
	// Create the later file first to prove that creation order does not schedule files.
	writeSelectionFile(t, root, "99-check.yaml", "app: apihydra\nkind: steps\nspec:\n  steps:\n    - request: {path: /check, defaults: {headers: {Authorization: '$token'}}}\n")
	writeSelectionFile(t, root, "00-auth.yml", "app: apihydra\nkind: steps\nspec:\n  steps:\n    - vars: {root_value: ready}\n      request: {path: /auth}\n      response: {capture: {token: .token}}\n")
	writeSelectionFile(t, root, "shop/setup.yaml", "app: apihydra\nkind: steps\nspec:\n  steps:\n    - request: {path: /parent, defaults: {headers: {Authorization: '$token', Root-Value: '$root_value'}}}\n")
	writeSelectionFile(t, root, "shop/products/scenario.yaml", "app: apihydra\nkind: steps\nmetadata: {description: Get a product after authentication.}\nspec:\n  steps:\n    - request: {path: /must-not-run}\n    - request: {path: /products, defaults: {headers: {Authorization: '$token'}}}\n")
	writeSelectionFile(t, root, "shop/products/unselected.yaml", "[")
	writeSelectionFile(t, root, "shop/users/steps.yml", "app: apihydra\nkind: steps\nspec:\n  steps:\n    - request: {path: /users, defaults: {headers: {Authorization: '$token'}}}\n")
	writeSelectionFile(t, root, "unrelated/broken.yaml", "[")
	var output bytes.Buffer
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	code, err := run(ctx, domain.Config{Parallelism: 1, Selections: []string{"shop/products/scenario.yaml:1-1", "shop/users", "shop/products/scenario.yaml:1"}}, reporting.NewReporter(&output, false))
	if code != 0 || err != nil {
		t.Fatalf("run = %d, %v, %s", code, err, output.String())
	}
	mu.Lock()
	got := slices.Clone(paths)
	mu.Unlock()
	if len(got) != 5 || !slices.Equal(got[:3], []string{"/auth", "/check", "/parent"}) {
		t.Fatalf("stage and filename order = %v", got)
	}
	slices.Sort(got[3:])
	if !slices.Equal(got[3:], []string{"/products", "/users"}) {
		t.Fatalf("selected children = %v", got[3:])
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
	writeSelectionFile(t, root, "a/b/target.yaml", "app: apihydra\nkind: steps\nspec:\n  steps:\n    - request: {method: GET, path: /}\n")
	for _, target := range []string{"steps.yaml:1", "steps.yaml:3-1", "missing.yaml", "root.yaml", "a/b/target.yaml:1"} {
		var output bytes.Buffer
		code, err := run(context.Background(), domain.Config{Parallelism: 1, Selections: []string{"steps.yaml", target}}, reporting.NewReporter(&output, false))
		if code != 102 || !errors.Is(err, definition.ErrInvalidSelection) {
			t.Fatalf("target %s: %d, %v", target, code, err)
		}
		if !strings.HasSuffix(fatalDiagnostic(err), "#invalid-arguments\n") {
			t.Fatalf("footer = %s", fatalDiagnostic(err))
		}
	}
	writeSelectionFile(t, root, "a/required.yaml", "app: apihydra\nkind: steps\nspec: [")
	var output bytes.Buffer
	code, err := run(context.Background(), domain.Config{Parallelism: 1, Selections: []string{"a/b/target.yaml:0"}}, reporting.NewReporter(&output, false))
	if code != 102 || !errors.Is(err, definition.ErrInvalidDefinition) {
		t.Fatalf("invalid ancestor definition: %d, %v", code, err)
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
