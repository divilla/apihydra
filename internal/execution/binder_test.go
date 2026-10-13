package execution

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/divilla/apihydra/internal/domain"
	"github.com/divilla/apihydra/pkg/runner"
	"github.com/goccy/go-yaml"
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
	if got, want := ErrVariable.Error(), "variable error"; got != want {
		t.Fatalf("ErrVariable = %q, want %q", got, want)
	}
	if got, want := ErrCapture.Error(), "capture error"; got != want {
		t.Fatalf("ErrCapture = %q, want %q", got, want)
	}
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
	if exitCode != 0 || !errors.Is(err, ErrVariable) || !errors.Is(err, ErrKeyExists) {
		t.Fatalf("LoadVariables() = (%d, %v), want named ErrVariable and ErrKeyExists", exitCode, err)
	}
	if !strings.Contains(err.Error(), "name") {
		t.Fatalf("LoadVariables() error = %q, want variable name", err)
	}
	if got, err := store.Get("name"); err != nil || got != "first" {
		t.Fatalf("store.Get(name) = (%q, %v), want (first, nil)", got, err)
	}
}

func TestInterpolateRequestBodySupportsBothPlaceholderFormsAndOnlyMutatesRequest(t *testing.T) {
	binder := binderWithValues(t, map[string]string{"name": "Hydra", "count": "3"})
	step := &domain.Step{Vars: map[string]domain.YAMLString{"name": "declaration"}}
	step.Request.Body = `{"name":$name,"count":${count},"again":"${name}"}`
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
			if exitCode != 0 || !errors.Is(err, ErrVariable) || !errors.Is(err, ErrNotFound) {
				t.Fatalf("interpolation = (%d, %v), want named ErrVariable and ErrNotFound", exitCode, err)
			}
			if !strings.Contains(err.Error(), "missing") {
				t.Fatalf("interpolation error = %q, want missing variable name", err)
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
	if exitCode != 0 || !errors.Is(err, ErrCapture) || !errors.Is(err, ErrKeyExists) {
		t.Fatalf("CaptureResponseVariables() = (%d, %v), want named ErrCapture and ErrKeyExists", exitCode, err)
	}
	if !strings.Contains(err.Error(), "name") {
		t.Fatalf("CaptureResponseVariables() error = %q, want capture name", err)
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
	if exitCode != 7 || !errors.Is(err, ErrCapture) || !errors.Is(err, runner.ErrJQSelector) || !errors.Is(err, runner.ErrCommand) {
		t.Fatalf("CaptureResponseVariables() = (%d, %v), want jq command failure with code 7", exitCode, err)
	}
	if !strings.Contains(err.Error(), "invalid selector") {
		t.Fatalf("error = %q, want command stderr", err)
	}
	if !strings.Contains(err.Error(), "value") {
		t.Fatalf("error = %q, want capture name", err)
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
	if exitCode != -1 || !errors.Is(err, ErrCapture) || !errors.Is(err, context.Canceled) || !errors.Is(err, runner.ErrJQSelector) {
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

// Each case is part of the binding replacement contract for both body phases.
func TestBodyVariableReplacementContract(t *testing.T) {
	tests := []struct {
		name, value, body, want string
	}{
		{"literal string value", "123-some-token", `{"auth":$token}`, `{"auth":"123-some-token"}`},
		{"captured string value", `"123-some-token"`, `{"auth":$token}`, `{"auth":"123-some-token"}`},
		{"literal string content", "123-some-token", `{"auth":"bearer: ${token}"}`, `{"auth":"bearer: 123-some-token"}`},
		{"captured string content", `"123-some-token"`, `{"auth":"bearer: ${token}"}`, `{"auth":"bearer: 123-some-token"}`},
		{"literal escapes", "quote\" slash\\ newline\n tab\t", `{"auth":$token,"text":"prefix ${token}"}`, `{"auth":"quote\" slash\\ newline\n tab\t","text":"prefix quote\" slash\\ newline\n tab\t"}`},
		{"captured escapes", `"quote\" slash\\ newline\n tab\t"`, `{"auth":$token,"text":"prefix ${token}"}`, `{"auth":"quote\" slash\\ newline\n tab\t","text":"prefix quote\" slash\\ newline\n tab\t"}`},
		{"empty literal", "", `{"auth":$token,"text":"${token}"}`, `{"auth":"","text":""}`},
		{"empty captured", `""`, `{"auth":$token,"text":"${token}"}`, `{"auth":"","text":""}`},
		{"number", "7", `{"auth":$token,"other":${token}}`, `{"auth":7,"other":7}`},
		{"boolean", "true", `{"auth":$token,"other":${token}}`, `{"auth":true,"other":true}`},
		{"null", "null", `{"auth":$token,"other":${token}}`, `{"auth":null,"other":null}`},
		{"object", `{"id":7}`, `{"auth":$token,"other":${token}}`, `{"auth":{"id":7},"other":{"id":7}}`},
		{"array", `[1,"two"]`, `{"auth":$token,"other":${token}}`, `{"auth":[1,"two"],"other":[1,"two"]}`},
		{"quoted numeric string", `"7"`, `{"auth":$token,"text":"${token}"}`, `{"auth":"7","text":"7"}`},
		{"JSON string whitespace", " \"token\"\n", `{"auth":$token,"text":"${token}"}`, "{\"auth\": \"token\"\n,\"text\":\"token\"}"},
		{"literal whitespace", " token ", `{"auth":$token,"text":"${token}"}`, `{"auth":" token ","text":" token "}`},
		{"invalid JSON remains a string", `"unterminated`, `{"auth":$token,"text":"${token}"}`, `{"auth":"\"unterminated","text":"\"unterminated"}`},
		{"no recursive expansion", "$missing ${missing}", `{"auth":$token,"text":"${token}"}`, `{"auth":"$missing ${missing}","text":"$missing ${missing}"}`},
	}
	for _, test := range tests {
		for _, phase := range []string{"request", "expected response"} {
			t.Run(test.name+"/"+phase, func(t *testing.T) {
				store := NewKeyValueStore()
				if err := store.Set("token", test.value); err != nil {
					t.Fatal(err)
				}
				binder := NewBinder(store)
				step := &domain.Step{}
				var body domain.YAMLString
				var code int
				var err error
				if phase == "request" {
					step.Request.Body = domain.YAMLString(test.body)
					code, err = binder.InterpolateRequestBody(context.Background(), step)
					body = step.Request.Body
				} else {
					step.Response.ExpectedBody = domain.YAMLString(test.body)
					code, err = binder.InterpolateResponseExpectedBody(context.Background(), step)
					body = step.Response.ExpectedBody
				}
				if code != 0 || err != nil {
					t.Fatalf("interpolation = (%d, %v), want (0, nil)", code, err)
				}
				if string(body) != test.want || !json.Valid([]byte(body)) {
					t.Fatalf("body = %q, want valid JSON %q", body, test.want)
				}
			})
		}
	}
}

func TestRequestHeaderQueryAndPathVariableContract(t *testing.T) {
	tests := []struct {
		name, value, text, escaped string
	}{
		{"literal token", "123-some-token", "123-some-token", "123-some-token"},
		{"literal bearer", "Bearer: 123-some-token", "Bearer: 123-some-token", "Bearer%3A+123-some-token"},
		{"captured token", `"123-some-token"`, "123-some-token", "123-some-token"},
		{"query delimiters", "a b+c&d=e/?#%", "a b+c&d=e/?#%", "a+b%2Bc%26d%3De%2F%3F%23%25"},
		{"Unicode", "ž水", "ž水", "%C5%BE%E6%B0%B4"},
		{"literal whitespace", " token ", " token ", "+token+"},
		{"captured escapes", `"quote\" slash\\ newline\n tab\t"`, "quote\" slash\\ newline\n tab\t", "quote%22+slash%5C+newline%0A+tab%09"},
		{"literal escapes", "quote\" slash\\ newline\n tab\t", "quote\" slash\\ newline\n tab\t", "quote%22+slash%5C+newline%0A+tab%09"},
		{"empty literal", "", "", ""},
		{"empty captured", `""`, "", ""},
		{"captured string whitespace", " \"token\"\n", "token", "token"},
		{"number", "7", "7", "7"},
		{"Boolean", "true", "true", "true"},
		{"null", "null", "null", "null"},
		{"object", `{"id":7}`, `{"id":7}`, "%7B%22id%22%3A7%7D"},
		{"array", `[1,"two"]`, `[1,"two"]`, "%5B1%2C%22two%22%5D"},
		{"quoted numeric string", `"7"`, "7", "7"},
		{"invalid JSON", `"unterminated`, `"unterminated`, "%22unterminated"},
		{"single pass", "$missing ${missing}", "$missing ${missing}", "%24missing+%24%7Bmissing%7D"},
		{"existing percent escape", "%2F", "%2F", "%252F"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store := NewKeyValueStore()
			if err := store.Set("token", test.value); err != nil {
				t.Fatal(err)
			}
			step := &domain.Step{}
			step.Request.Defaults.Headers = map[string]string{
				"Authorization": "$token", "X-Text": "Bearer: ${token}",
				"$token": "literal", "X-Repeated": "$token|${token}",
			}
			step.Request.Query = "fixed=%2F&auth=${token}&again=$token"
			step.Request.Path = "api/v1/${token}/%2F/$token"
			step.Request.Defaults.BaseURL = "https://$token"
			step.Request.Defaults.BasePath = "/${token}"
			step.Request.Method = "$token"
			step.Response.ExpectedBody = "${token}"
			code, err := NewBinder(store).InterpolateRequestBody(context.Background(), step)
			if code != 0 || err != nil {
				t.Fatalf("interpolation = (%d, %v), want (0, nil)", code, err)
			}
			wantHeaders := map[string]string{
				"Authorization": test.text, "X-Text": "Bearer: " + test.text,
				"$token": "literal", "X-Repeated": test.text + "|" + test.text,
			}
			if !reflect.DeepEqual(step.Request.Defaults.Headers, wantHeaders) {
				t.Fatalf("headers = %#v, want %#v", step.Request.Defaults.Headers, wantHeaders)
			}
			if want := "fixed=%2F&auth=" + test.escaped + "&again=" + test.escaped; step.Request.Query != want {
				t.Fatalf("query = %q, want %q", step.Request.Query, want)
			}
			if want := "api/v1/" + strings.ReplaceAll(test.escaped, "+", "%20") + "/%2F/" + strings.ReplaceAll(test.escaped, "+", "%20"); step.Request.Path != want {
				t.Fatalf("path = %q, want %q", step.Request.Path, want)
			}
			if step.Request.Defaults.BaseURL != "https://$token" || step.Request.Defaults.BasePath != "/${token}" || step.Request.Method != "$token" || step.Response.ExpectedBody != "${token}" {
				t.Fatalf("interpolation mutated unrelated fields: %+v", step)
			}
			if got, err := store.Get("token"); err != nil || got != test.value {
				t.Fatalf("stored token = (%q, %v), want (%q, nil)", got, err, test.value)
			}
		})
	}
}

func TestRequestVariableFailureLeavesAllRequestFieldsUnchanged(t *testing.T) {
	for _, field := range []string{"body", "header", "query", "path"} {
		t.Run(field, func(t *testing.T) {
			store := NewKeyValueStore()
			if err := store.Set("known", "value"); err != nil {
				t.Fatal(err)
			}
			step := &domain.Step{}
			step.Request.Body = "$known"
			step.Request.Defaults.Headers = map[string]string{"A": "$known", "B": "$known"}
			step.Request.Query = "auth=$known"
			step.Request.Path = "/api/${known}"
			switch field {
			case "body":
				step.Request.Body = "$known ${missing} $also_missing"
			case "header":
				step.Request.Defaults.Headers["B"] = "$known ${missing} $also_missing"
			case "query":
				step.Request.Query = "auth=$known&other=${missing}&last=$also_missing"
			case "path":
				step.Request.Path = "/api/${known}/${missing}/$also_missing"
			}
			wantBody, wantQuery, wantPath := step.Request.Body, step.Request.Query, step.Request.Path
			wantHeaders := map[string]string{"A": step.Request.Defaults.Headers["A"], "B": step.Request.Defaults.Headers["B"]}
			code, err := NewBinder(store).InterpolateRequestBody(context.Background(), step)
			if code != 0 || !errors.Is(err, ErrVariable) || !errors.Is(err, ErrNotFound) || !strings.Contains(err.Error(), "missing") {
				t.Fatalf("interpolation = (%d, %v), want named variable and missing-key errors", code, err)
			}
			if step.Request.Body != wantBody || step.Request.Query != wantQuery || step.Request.Path != wantPath || !reflect.DeepEqual(step.Request.Defaults.Headers, wantHeaders) {
				t.Fatalf("failed interpolation mutated request: %+v", step.Request)
			}
		})
	}
}

func TestRequestVariableInterpolationPreservesEmptyHeadersQueryAndPath(t *testing.T) {
	for _, headers := range []map[string]string{nil, {}} {
		step := &domain.Step{}
		step.Request.Defaults.Headers = headers
		code, err := NewBinder(NewKeyValueStore()).InterpolateRequestBody(context.Background(), step)
		if code != 0 || err != nil || !reflect.DeepEqual(step.Request.Defaults.Headers, headers) || step.Request.Query != "" || step.Request.Path != "" {
			t.Fatalf("empty request interpolation = (%d, %v, %+v)", code, err, step.Request)
		}
	}
}

// Change 022 criteria 7–8: the manual and skill's serial authentication example
// uses the real schema and the same runnable fixture, including captured JSON.
func TestDocumentedAuthenticationExampleMatchesCLIFixture(t *testing.T) {
	manual, err := os.ReadFile("../../docs/user-manual/apih.md")
	if err != nil {
		t.Fatal(err)
	}
	fixture, err := os.ReadFile("../../int-tests/input/scenarios/request-header-vars/steps.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(manual), "```yaml\n"+string(fixture)+"```") {
		t.Fatal("manual authentication example differs from runnable fixture")
	}
	var definition domain.StepsDefinition
	if err := yaml.Unmarshal(fixture, &definition); err != nil {
		t.Fatal(err)
	}
	if len(definition.Spec.Steps) != 2 {
		t.Fatal("authentication example must have producer and consumer")
	}
	binder := NewBinder(NewKeyValueStore())
	producer := &definition.Spec.Steps[0]
	producer.Response.ActualBody = `{"token":"a b+c&d=e/?#%"}`
	if _, err := binder.CaptureResponseVariables(context.Background(), producer); err != nil {
		t.Fatal(err)
	}
	consumer := &definition.Spec.Steps[1]
	if _, err := binder.InterpolateRequestBody(context.Background(), consumer); err != nil {
		t.Fatal(err)
	}
	if consumer.Request.Defaults.Headers["Authorization"] != "Bearer a b+c&d=e/?#%" || consumer.Request.Defaults.Headers["X-Token"] != "a b+c&d=e/?#%" || consumer.Request.Query != "fixed=%2F&auth=a+b%2Bc%26d%3De%2F%3F%23%25&again=a+b%2Bc%26d%3De%2F%3F%23%25" {
		t.Fatalf("documented authentication request = %+v", consumer.Request)
	}
}

// The path follow-up's documented example uses the query text rules, with
// spaces percent-encoded so an HTTP server recovers the original path value.
func TestDocumentedPathVariableExample(t *testing.T) {
	const example = `vars:
  path: "a b+c/d?e#%ž"
request:
  method: GET
  path: "api/v1/${path}"
`
	manual, err := os.ReadFile("../../docs/user-manual/apih.md")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(manual), "```yaml\n"+example+"```") {
		t.Fatal("manual is missing the path example")
	}
	for _, captured := range []bool{false, true} {
		t.Run(strconv.FormatBool(captured), func(t *testing.T) {
			var step domain.Step
			if err := yaml.Unmarshal([]byte(example), &step); err != nil {
				t.Fatal(err)
			}
			binder := NewBinder(NewKeyValueStore())
			if captured {
				step.Response.ActualBody = `{"path":"a b+c/d?e#%ž"}`
				step.Response.Capture = map[string]domain.YAMLString{"path": ".path"}
				if _, err := binder.CaptureResponseVariables(context.Background(), &step); err != nil {
					t.Fatal(err)
				}
			} else if _, err := binder.LoadVariables(context.Background(), &step); err != nil {
				t.Fatal(err)
			}
			if _, err := binder.InterpolateRequestBody(context.Background(), &step); err != nil {
				t.Fatal(err)
			}
			if want := "api/v1/a%20b%2Bc%2Fd%3Fe%23%25%C5%BE"; step.Request.Path != want {
				t.Fatalf("path = %q, want %q", step.Request.Path, want)
			}
		})
	}
}
