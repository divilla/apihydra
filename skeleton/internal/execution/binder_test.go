package execution

import (
	"context"
	"errors"
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
