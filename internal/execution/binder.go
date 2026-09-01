package execution

import (
	"context"
	"regexp"
	"slices"

	"github.com/divilla/apihydra/internal/domain"
	"github.com/divilla/apihydra/pkg/runner"
)

var variablePlaceholder = regexp.MustCompile(`\$\{[A-Za-z_][A-Za-z0-9_]*\}|\$[A-Za-z_][A-Za-z0-9_]*`)

// Binder loads, interpolates, and captures step variables through
// one shared KeyValueStore.
type Binder struct {
	kvs *KeyValueStore
}

// NewBinder returns a binder that reads from and writes to kvs.
func NewBinder(kvs *KeyValueStore) *Binder {
	return &Binder{
		kvs: kvs,
	}
}

// LoadVariables stores every variable declared in step.Vars in the binder's
// key-value store.
func (b *Binder) LoadVariables(
	ctx context.Context,
	step *domain.Step,
) (int, error) {
	names := make([]string, 0, len(step.Vars))
	for name := range step.Vars {
		names = append(names, name)
	}
	slices.Sort(names)

	for _, name := range names {
		if err := b.kvs.Set(name, string(step.Vars[name])); err != nil {
			return 0, err
		}
	}
	return 0, nil
}

// InterpolateRequestBody replaces $var and ${var} placeholders in
// step.Request.Body with values from the binder's key-value store.
func (b *Binder) InterpolateRequestBody(
	ctx context.Context,
	step *domain.Step,
) (int, error) {
	body, err := b.interpolate(step.Request.Body)
	if err != nil {
		return 0, err
	}
	step.Request.Body = body
	return 0, nil
}

// InterpolateResponseExpectedBody replaces $var and ${var} placeholders in
// step.Response.ExpectedBody with values from the binder's key-value store.
func (b *Binder) InterpolateResponseExpectedBody(
	ctx context.Context,
	step *domain.Step,
) (int, error) {
	body, err := b.interpolate(step.Response.ExpectedBody)
	if err != nil {
		return 0, err
	}
	step.Response.ExpectedBody = body
	return 0, nil
}

// CaptureResponseVariables evaluates every selector in step.Response.Capture
// against step.Response.ActualBody with runner.JQExtract and stores each result
// in the binder's key-value store under its corresponding capture name.
func (b *Binder) CaptureResponseVariables(
	ctx context.Context,
	step *domain.Step,
) (int, error) {
	names := make([]string, 0, len(step.Response.Capture))
	for name := range step.Response.Capture {
		names = append(names, name)
	}
	slices.Sort(names)

	for _, name := range names {
		value, exitCode, err := runner.JQExtract(ctx, string(step.Response.Capture[name]), string(step.Response.ActualBody))
		if err != nil {
			return exitCode, err
		}
		if err := b.kvs.Set(name, value); err != nil {
			return exitCode, err
		}
	}
	return 0, nil
}

func (b *Binder) interpolate(body domain.YAMLString) (domain.YAMLString, error) {
	var interpolationErr error
	interpolated := variablePlaceholder.ReplaceAllStringFunc(string(body), func(placeholder string) string {
		if interpolationErr != nil {
			return placeholder
		}

		name := placeholder[1:]
		if name[0] == '{' {
			name = name[1 : len(name)-1]
		}
		value, err := b.kvs.Get(name)
		if err != nil {
			interpolationErr = err
			return placeholder
		}
		return value
	})
	if interpolationErr != nil {
		return body, interpolationErr
	}
	return domain.YAMLString(interpolated), nil
}
