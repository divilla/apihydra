package execution

import (
	"apih/internal/domain"
	"apih/pkg/runner"
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestNewBinderRetainsStoreAndMatchesReferenceContract(t *testing.T) {
	store := NewKeyValueStore()
	binder := NewBinder(store)
	if binder.kvs != store {
		t.Fatalf("binder.kvs = %p, want %p", binder.kvs, store)
	}

	var _ func(*KeyValueStore) *Binder = NewBinder
	var _ func(*Binder, context.Context, *domain.Step) (int, error) = (*Binder).LoadVariables
	var _ func(*Binder, context.Context, *domain.Step) (int, error) = (*Binder).InterpolateRequestBody
	var _ func(*Binder, context.Context, *domain.Step) (int, error) = (*Binder).InterpolateResponseExpectedBody
	var _ func(*Binder, context.Context, *domain.Step) (int, error) = (*Binder).CaptureResponseVariables
}

func TestLoadVariablesStoresDeclarationsWithoutMutatingStep(t *testing.T) {
	step := &domain.Step{Vars: map[string]domain.YAMLString{
		"name":  "Hydra",
		"count": "3",
	}}
	step.Request.Body = `{"unchanged":true}`
	wantVars := map[string]domain.YAMLString{"name": "Hydra", "count": "3"}
	binder := NewBinder(NewKeyValueStore())

	exitCode, err := binder.LoadVariables(context.Background(), step)
	if err != nil || exitCode != 0 {
		t.Fatalf("LoadVariables() = (%d, %v), want (0, nil)", exitCode, err)
	}
	for name, want := range map[string]string{"name": "Hydra", "count": "3"} {
		if got, err := binder.kvs.Get(name); err != nil || got != want {
			t.Fatalf("store.Get(%q) = (%q, %v), want (%q, nil)", name, got, err, want)
		}
	}
	if !reflect.DeepEqual(step.Vars, wantVars) || step.Request.Body != `{"unchanged":true}` {
		t.Fatalf("LoadVariables() mutated step = %+v", step)
	}
}

func TestLoadVariablesPreservesExistingValueOnDuplicate(t *testing.T) {
	store := NewKeyValueStore()
	if err := store.Set("name", "first"); err != nil {
		t.Fatalf("Set() error = %v", err)
	}
	step := &domain.Step{Vars: map[string]domain.YAMLString{"name": "second"}}

	exitCode, err := NewBinder(store).LoadVariables(context.Background(), step)
	if exitCode != 0 || !errors.Is(err, ErrKeyExists) {
		t.Fatalf("LoadVariables() = (%d, %v), want (0, ErrKeyExists)", exitCode, err)
	}
	if got, err := store.Get("name"); err != nil || got != "first" {
		t.Fatalf("store.Get(name) = (%q, %v), want (first, nil)", got, err)
	}
}

func TestInterpolateRequestBodySupportsBothPlaceholderFormsAndOnlyMutatesRequest(t *testing.T) {
	binder := binderWithValues(t, map[string]string{"name": "Hydra", "count": "3"})
	step := &domain.Step{Vars: map[string]domain.YAMLString{"name": "declaration"}}
	step.Request.Body = `{"name":"$name","count":${count},"again":"$name"}`
	step.Response.ExpectedBody = `{"name":"$name"}`

	exitCode, err := binder.InterpolateRequestBody(context.Background(), step)
	if err != nil || exitCode != 0 {
		t.Fatalf("InterpolateRequestBody() = (%d, %v), want (0, nil)", exitCode, err)
	}
	if got, want := step.Request.Body, domain.YAMLString(`{"name":"Hydra","count":3,"again":"Hydra"}`); got != want {
		t.Fatalf("Request.Body = %q, want %q", got, want)
	}
	if got := step.Response.ExpectedBody; got != `{"name":"$name"}` {
		t.Fatalf("Response.ExpectedBody = %q, want unchanged", got)
	}
	if got := step.Vars["name"]; got != "declaration" {
		t.Fatalf("Vars[name] = %q, want unchanged", got)
	}
}

func TestInterpolateResponseExpectedBodySupportsBothPlaceholderFormsAndOnlyMutatesResponse(t *testing.T) {
	binder := binderWithValues(t, map[string]string{"id": "7", "valid": "true"})
	step := &domain.Step{}
	step.Request.Body = `{"id":$id}`
	step.Response.ExpectedBody = `{"id":$id,"valid":${valid}}`
	step.Response.ActualBody = `{"id":8,"valid":false}`

	exitCode, err := binder.InterpolateResponseExpectedBody(context.Background(), step)
	if err != nil || exitCode != 0 {
		t.Fatalf("InterpolateResponseExpectedBody() = (%d, %v), want (0, nil)", exitCode, err)
	}
	if got, want := step.Response.ExpectedBody, domain.YAMLString(`{"id":7,"valid":true}`); got != want {
		t.Fatalf("Response.ExpectedBody = %q, want %q", got, want)
	}
	if got := step.Request.Body; got != `{"id":$id}` {
		t.Fatalf("Request.Body = %q, want unchanged", got)
	}
	if got := step.Response.ActualBody; got != `{"id":8,"valid":false}` {
		t.Fatalf("Response.ActualBody = %q, want unchanged", got)
	}
}

func TestInterpolationReturnsMissingVariableWithoutPartialMutation(t *testing.T) {
	tests := map[string]struct {
		setBody func(*domain.Step)
		call    func(*Binder, *domain.Step) (int, error)
		body    func(*domain.Step) domain.YAMLString
	}{
		"request": {
			setBody: func(step *domain.Step) { step.Request.Body = `$known ${missing} $known` },
			call: func(binder *Binder, step *domain.Step) (int, error) {
				return binder.InterpolateRequestBody(context.Background(), step)
			},
			body: func(step *domain.Step) domain.YAMLString { return step.Request.Body },
		},
		"expected response": {
			setBody: func(step *domain.Step) { step.Response.ExpectedBody = `$known ${missing} $known` },
			call: func(binder *Binder, step *domain.Step) (int, error) {
				return binder.InterpolateResponseExpectedBody(context.Background(), step)
			},
			body: func(step *domain.Step) domain.YAMLString { return step.Response.ExpectedBody },
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			binder := binderWithValues(t, map[string]string{"known": "value"})
			step := &domain.Step{}
			test.setBody(step)
			wantBody := test.body(step)

			exitCode, err := test.call(binder, step)
			if exitCode != 0 || !errors.Is(err, ErrNotFound) {
				t.Fatalf("interpolation = (%d, %v), want (0, ErrNotFound)", exitCode, err)
			}
			if got := test.body(step); got != wantBody {
				t.Fatalf("body = %q, want unchanged %q", got, wantBody)
			}
		})
	}
}

func TestCaptureResponseVariablesDelegatesExtractionAndStoresResults(t *testing.T) {
	argsPath := installBinderJQ(t, `
input=$(/bin/cat)
printf '%s' "$input" > "$APIH_BINDER_INPUT"
selector=
for arg do selector=$arg; done
printf '%s\n' "$selector" >> "$APIH_BINDER_SELECTORS"
case "$selector" in
    .count) printf '3\n' ;;
    .title) printf '"Hydra"\n' ;;
    *) exit 9 ;;
esac
`)
	inputPath := filepath.Join(filepath.Dir(argsPath), "input")
	selectorsPath := filepath.Join(filepath.Dir(argsPath), "selectors")
	t.Setenv("APIH_BINDER_INPUT", inputPath)
	t.Setenv("APIH_BINDER_SELECTORS", selectorsPath)

	step := &domain.Step{}
	step.Response.ActualBody = `{"title":"Hydra","count":3}`
	step.Response.ExpectedBody = `{"unchanged":true}`
	step.Response.Capture = map[string]domain.YAMLString{"title": ".title", "count": ".count"}
	wantCapture := map[string]domain.YAMLString{"title": ".title", "count": ".count"}
	binder := NewBinder(NewKeyValueStore())

	exitCode, err := binder.CaptureResponseVariables(context.Background(), step)
	if err != nil || exitCode != 0 {
		t.Fatalf("CaptureResponseVariables() = (%d, %v), want (0, nil)", exitCode, err)
	}
	for name, want := range map[string]string{"title": `"Hydra"`, "count": "3"} {
		if got, err := binder.kvs.Get(name); err != nil || got != want {
			t.Fatalf("store.Get(%q) = (%q, %v), want (%q, nil)", name, got, err, want)
		}
	}
	if got := readBinderFile(t, inputPath); got != string(step.Response.ActualBody) {
		t.Fatalf("jq input = %q, want %q", got, step.Response.ActualBody)
	}
	if got := strings.Fields(readBinderFile(t, selectorsPath)); !reflect.DeepEqual(got, []string{".count", ".title"}) {
		t.Fatalf("jq selectors = %v, want [.count .title]", got)
	}
	if step.Response.ExpectedBody != `{"unchanged":true}` || !reflect.DeepEqual(step.Response.Capture, wantCapture) {
		t.Fatalf("CaptureResponseVariables() mutated response declarations = %+v", step.Response)
	}
}

func TestCaptureResponseVariablesPreservesExistingValueOnDuplicate(t *testing.T) {
	installBinderJQ(t, `/bin/cat >/dev/null
printf '"second"\n'
`)
	store := NewKeyValueStore()
	if err := store.Set("name", `"first"`); err != nil {
		t.Fatalf("Set() error = %v", err)
	}
	step := &domain.Step{}
	step.Response.ActualBody = `{"name":"second"}`
	step.Response.Capture = map[string]domain.YAMLString{"name": ".name"}

	exitCode, err := NewBinder(store).CaptureResponseVariables(context.Background(), step)
	if exitCode != 0 || !errors.Is(err, ErrKeyExists) {
		t.Fatalf("CaptureResponseVariables() = (%d, %v), want (0, ErrKeyExists)", exitCode, err)
	}
	if got, err := store.Get("name"); err != nil || got != `"first"` {
		t.Fatalf("store.Get(name) = (%q, %v), want first value", got, err)
	}
}

func TestCaptureResponseVariablesPropagatesCommandFailure(t *testing.T) {
	installBinderJQ(t, `/bin/cat >/dev/null
printf 'invalid selector' >&2
exit 7
`)
	step := &domain.Step{}
	step.Response.ActualBody = `{}`
	step.Response.Capture = map[string]domain.YAMLString{"value": ".bad["}
	binder := NewBinder(NewKeyValueStore())

	exitCode, err := binder.CaptureResponseVariables(context.Background(), step)
	if exitCode != 7 || !errors.Is(err, runner.ErrJQSelector) || !errors.Is(err, runner.ErrCommand) {
		t.Fatalf("CaptureResponseVariables() = (%d, %v), want jq command failure with code 7", exitCode, err)
	}
	if !strings.Contains(err.Error(), "invalid selector") {
		t.Fatalf("error = %q, want command stderr", err)
	}
	if _, err := binder.kvs.Get("value"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("store.Get(value) error = %v, want ErrNotFound", err)
	}
}

func TestCaptureResponseVariablesPropagatesCanceledContext(t *testing.T) {
	installBinderJQ(t, `printf 'should not run'`)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	step := &domain.Step{}
	step.Response.ActualBody = `{}`
	step.Response.Capture = map[string]domain.YAMLString{"value": ".value"}

	exitCode, err := NewBinder(NewKeyValueStore()).CaptureResponseVariables(ctx, step)
	if exitCode != -1 || !errors.Is(err, context.Canceled) || !errors.Is(err, runner.ErrJQSelector) {
		t.Fatalf("CaptureResponseVariables() = (%d, %v), want canceled jq extraction", exitCode, err)
	}
}

func binderWithValues(t *testing.T, values map[string]string) *Binder {
	t.Helper()
	store := NewKeyValueStore()
	for name, value := range values {
		if err := store.Set(name, value); err != nil {
			t.Fatalf("Set(%q) error = %v", name, err)
		}
	}
	return NewBinder(store)
}

func installBinderJQ(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "jq")
	if err := os.WriteFile(path, []byte("#!/bin/sh\nset -eu\n"+body), 0o755); err != nil {
		t.Fatalf("WriteFile(%s) error = %v", path, err)
	}
	t.Setenv("PATH", dir)
	return path
}

func readBinderFile(t *testing.T, path string) string {
	t.Helper()
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", path, err)
	}
	return string(contents)
}
