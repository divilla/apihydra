package execution

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"regexp"
	"slices"
	"strings"

	"github.com/divilla/apihydra/skeleton/internal/domain"
	"github.com/divilla/apihydra/skeleton/pkg/errs"
	"github.com/divilla/apihydra/skeleton/pkg/runner"
)

// ErrVariable classifies a failure to load or interpolate a named variable.
var ErrVariable = errors.New("variable error")

// ErrCapture classifies a failure to evaluate or store a named capture.
var ErrCapture = errors.New("capture error")

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
// key-value store. A failure preserves ErrVariable, the store error, and the
// affected variable name.
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
			return 0, errs.Build(0, ErrVariable, err, name)
		}
	}
	return 0, nil
}

// InterpolateRequestBody interpolates the runtime request body, effective
// Defaults.Headers values, Query, and Path through the binder's key-value store.
// In the body, $var inserts a complete JSON value: valid stored JSON is
// preserved, and other values are JSON-encoded as strings. ${var} omits a JSON
// string's outer quotes, retaining escapes for use within a JSON string.
// In headers, Query, and Path, both forms insert text: stored JSON strings are
// decoded and other stored values are used verbatim. Query and Path replacements
// are escaped with url.QueryEscape; Path uses %20 for spaces instead of +.
// Surrounding query and path text is left unchanged.
// Header names and other request fields are not interpolated. Replacement is
// single-pass. A missing value preserves ErrVariable, ErrNotFound, and the
// affected variable name, leaving the body, headers, query, and path unchanged.
func (b *Binder) InterpolateRequestBody(
	ctx context.Context,
	step *domain.Step,
) (int, error) {
	body, err := b.interpolate(step.Request.Body)
	if err != nil {
		return 0, err
	}
	headers := make(map[string]string, len(step.Request.Defaults.Headers))
	for name, value := range step.Request.Defaults.Headers {
		headers[name], err = b.interpolateText(value, nil)
		if err != nil {
			return 0, err
		}
	}
	query, err := b.interpolateText(step.Request.Query, url.QueryEscape)
	if err != nil {
		return 0, err
	}
	path, err := b.interpolateText(step.Request.Path, func(value string) string {
		return strings.ReplaceAll(url.QueryEscape(value), "+", "%20")
	})
	if err != nil {
		return 0, err
	}
	step.Request.Body = body
	for name, value := range headers {
		step.Request.Defaults.Headers[name] = value
	}
	step.Request.Query = query
	step.Request.Path = path
	return 0, nil
}

// InterpolateResponseExpectedBody replaces $var and ${var} placeholders in
// step.Response.ExpectedBody with values from the binder's key-value store. A
// missing value preserves ErrVariable, ErrNotFound, and the affected variable
// name. Its replacement rules match the body rules of InterpolateRequestBody.
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
// in the binder's key-value store under its corresponding capture name. A
// failure preserves ErrCapture, its original selector or store error, and the
// affected capture name.
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
			return exitCode, errs.Build(exitCode, ErrCapture, err, name)
		}
		if err := b.kvs.Set(name, value); err != nil {
			return exitCode, errs.Build(exitCode, ErrCapture, err, name)
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
		text := name[0] == '{'
		if text {
			name = name[1 : len(name)-1]
		}
		value, err := b.kvs.Get(name)
		if err != nil {
			interpolationErr = errs.Build(0, ErrVariable, err, name)
			return placeholder
		}
		if !json.Valid([]byte(value)) {
			// Marshaling a string cannot fail.
			encoded, _ := json.Marshal(value)
			value = string(encoded)
		}
		if text {
			trimmed := strings.TrimSpace(value)
			if trimmed[0] == '"' {
				return trimmed[1 : len(trimmed)-1]
			}
		}
		return value
	})
	if interpolationErr != nil {
		return body, interpolationErr
	}
	return domain.YAMLString(interpolated), nil
}

func (b *Binder) interpolateText(input string, escape func(string) string) (string, error) {
	var interpolationErr error
	output := variablePlaceholder.ReplaceAllStringFunc(input, func(placeholder string) string {
		if interpolationErr != nil {
			return placeholder
		}
		name := strings.Trim(placeholder[1:], "{}")
		value, err := b.kvs.Get(name)
		if err != nil {
			interpolationErr = errs.Build(0, ErrVariable, err, name)
			return placeholder
		}
		var decoded string
		if json.Unmarshal([]byte(value), &decoded) == nil && strings.HasPrefix(strings.TrimSpace(value), `"`) {
			value = decoded
		}
		if escape != nil {
			return escape(value)
		}
		return value
	})
	if interpolationErr != nil {
		return input, interpolationErr
	}
	return output, nil
}
