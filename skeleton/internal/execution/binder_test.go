package execution

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/divilla/apihydra/skeleton/internal/domain"
)

func TestNewBinderRetainsStore(t *testing.T) {
	store := NewKeyValueStore()
	binder := NewBinder(store)
	if binder.kvs != store {
		t.Fatalf("binder.kvs = %p, want %p", binder.kvs, store)
	}
}

func TestRequestHeaderQueryAndPathVariableContract(t *testing.T) {
	tests := []struct {
		name, value, text, escaped string
	}{
		{"literal token", "123-some-token", "123-some-token", "123-some-token"},
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
			if step.Request.Defaults.BaseURL != "https://$token" || step.Response.ExpectedBody != "${token}" {
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

func TestBinderErrorClassifications(t *testing.T) {
	if got, want := ErrVariable.Error(), "variable error"; got != want {
		t.Fatalf("ErrVariable = %q, want %q", got, want)
	}
	if got, want := ErrCapture.Error(), "capture error"; got != want {
		t.Fatalf("ErrCapture = %q, want %q", got, want)
	}
}

func TestLoadVariablesNamesDuplicateVariable(t *testing.T) {
	store := NewKeyValueStore()
	if err := store.Set("token", "old"); err != nil {
		t.Fatal(err)
	}
	step := &domain.Step{Vars: map[string]domain.YAMLString{"token": "new"}}

	_, err := NewBinder(store).LoadVariables(context.Background(), step)
	if !errors.Is(err, ErrVariable) || !errors.Is(err, ErrKeyExists) {
		t.Fatalf("LoadVariables() error = %v, want variable and duplicate-key errors", err)
	}
	if !strings.Contains(err.Error(), "token") {
		t.Fatalf("LoadVariables() error = %q, want variable name", err)
	}
}

func TestInterpolationNamesMissingVariable(t *testing.T) {
	step := &domain.Step{}
	step.Request.Body = `${missing}`

	_, err := NewBinder(NewKeyValueStore()).InterpolateRequestBody(context.Background(), step)
	if !errors.Is(err, ErrVariable) || !errors.Is(err, ErrNotFound) {
		t.Fatalf("InterpolateRequestBody() error = %v, want variable and missing-key errors", err)
	}
	if !strings.Contains(err.Error(), "missing") {
		t.Fatalf("InterpolateRequestBody() error = %q, want variable name", err)
	}
	if got := string(step.Request.Body); got != `${missing}` {
		t.Fatalf("Request.Body = %q, want unchanged placeholder", got)
	}
}

func TestCaptureNamesDuplicateCapture(t *testing.T) {
	store := NewKeyValueStore()
	if err := store.Set("identifier", "old"); err != nil {
		t.Fatal(err)
	}
	step := &domain.Step{}
	step.Response.ActualBody = `{}`
	step.Response.Capture = map[string]domain.YAMLString{"identifier": ".id"}

	_, err := NewBinder(store).CaptureResponseVariables(context.Background(), step)
	if !errors.Is(err, ErrCapture) || !errors.Is(err, ErrKeyExists) {
		t.Fatalf("CaptureResponseVariables() error = %v, want capture and duplicate-key errors", err)
	}
	if !strings.Contains(err.Error(), "identifier") {
		t.Fatalf("CaptureResponseVariables() error = %q, want capture name", err)
	}
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
