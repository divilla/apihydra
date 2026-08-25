package execution

import (
	"apih/internal/domain"
	"apih/pkg/errs"
	"apih/pkg/runner"
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestValidatorExportedContract(t *testing.T) {
	if validator := NewValidator(); validator == nil {
		t.Fatal("NewValidator() = nil")
	}
	if got, want := ErrValidation.Error(), "validation error"; got != want {
		t.Fatalf("ErrValidation = %q, want %q", got, want)
	}
	if got, want := ErrValidatorFatal.Error(), "fatal validator error"; got != want {
		t.Fatalf("ErrValidatorFatal = %q, want %q", got, want)
	}

	var _ func() *Validator = NewValidator
	var _ func(*Validator, context.Context, *domain.Step) error = (*Validator).ValidateStatus
	var _ func(*Validator, context.Context, *domain.Step) (string, error) = (*Validator).ValidateBody
	var _ func(*Validator, context.Context, *domain.Step) (string, error) = (*Validator).ValidateTypes
}

func TestValidateStatusSupportsWildcardAndExactComparison(t *testing.T) {
	tests := map[string]struct {
		expected int
		actual   int
		wantErr  bool
	}{
		"wildcard": {expected: 0, actual: 503},
		"equal":    {expected: 201, actual: 201},
		"mismatch": {expected: 201, actual: 200, wantErr: true},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			step := &domain.Step{}
			step.Response.ExpectedStatus = test.expected
			step.Response.ActualStatus = test.actual
			wantStep := *step

			err := NewValidator().ValidateStatus(context.Background(), step)
			if got := errors.Is(err, ErrValidation); got != test.wantErr {
				t.Fatalf("ValidateStatus() error = %v, ErrValidation = %t, want %t", err, got, test.wantErr)
			}
			if !reflect.DeepEqual(*step, wantStep) {
				t.Fatalf("ValidateStatus() mutated step = %+v", step)
			}
		})
	}
}

func TestBuildTypeFilterIsDeterministicAndSelectsMismatches(t *testing.T) {
	got := buildTypeFilter(map[string][]string{
		`.user.name`: {"string", "null"},
		`.active`:    {"boolean"},
	})
	want := `[` +
		`({selector:".active",expected:["boolean"],actual:((.active))} | select(.actual as $value | ((($value | type) == "boolean") | not)) | .actual |= type),` +
		`({selector:".user.name",expected:["string","null"],actual:((.user.name))} | select(.actual as $value | (((($value | type) == "string") or (($value | type) == "null")) | not)) | .actual |= type)` +
		`] | .[]`
	if got != want {
		t.Fatalf("buildTypeFilter() = %q, want %q", got, want)
	}
	if got := buildTypeFilter(nil); got != `[] | .[]` {
		t.Fatalf("buildTypeFilter(nil) = %q, want empty declaration filter", got)
	}
	if got := buildTypeFilter(map[string][]string{".value": nil}); !strings.Contains(got, `select(.actual as $value | (false | not))`) {
		t.Fatalf("buildTypeFilter(empty declaration) = %q, want always-failing predicate", got)
	}
}

func TestBuildTypeFilterSupportsIntegerAndZeroDeclarations(t *testing.T) {
	filter := buildTypeFilter(map[string][]string{".version": {"int", "zero"}})
	for _, want := range []string{
		`if ($value | type) == "number" then (($value | floor) == $value) else false end`,
		`($value == 0)`,
	} {
		if !strings.Contains(filter, want) {
			t.Fatalf("buildTypeFilter() = %q, want predicate %q", filter, want)
		}
	}
}

func TestValidateTypesEvaluatesActualBodyAndReturnsRunnerOutput(t *testing.T) {
	commandDir := newValidatorCommandDir(t)
	inputPath := filepath.Join(commandDir, "input")
	argsPath := filepath.Join(commandDir, "args")
	t.Setenv("APIH_VALIDATOR_INPUT", inputPath)
	t.Setenv("APIH_VALIDATOR_ARGS", argsPath)
	installValidatorCommand(t, commandDir, "jq", `
input=$(/bin/cat)
printf '%s' "$input" > "$APIH_VALIDATOR_INPUT"
printf '%s\n' "$@" > "$APIH_VALIDATOR_ARGS"
printf '%s\n' '{"selector":".active","expected":["boolean"],"actual":"string"}'
`)

	step := &domain.Step{}
	step.Response.ActualBody = `{"active":"yes"}`
	step.Response.ExpectedTypes = map[string][]string{".active": {"boolean"}}
	wantStep := cloneValidatorStep(step)

	failed, err := NewValidator().ValidateTypes(context.Background(), step)
	if err != nil {
		t.Fatalf("ValidateTypes() error = %v", err)
	}
	wantFailed := `{"selector":".active","expected":["boolean"],"actual":"string"}`
	if failed != wantFailed {
		t.Fatalf("ValidateTypes() failed = %q, want %q", failed, wantFailed)
	}
	if got := readValidatorFile(t, inputPath); got != string(step.Response.ActualBody) {
		t.Fatalf("jq input = %q, want %q", got, step.Response.ActualBody)
	}
	args := strings.Split(strings.TrimSpace(readValidatorFile(t, argsPath)), "\n")
	if got, want := args[len(args)-1], buildTypeFilter(step.Response.ExpectedTypes); got != want {
		t.Fatalf("jq filter = %q, want %q", got, want)
	}
	if !reflect.DeepEqual(step, wantStep) {
		t.Fatalf("ValidateTypes() mutated step = %+v", step)
	}
}

func TestValidateTypesReturnsEmptyResultWhenAllTypesMatch(t *testing.T) {
	commandDir := newValidatorCommandDir(t)
	installValidatorCommand(t, commandDir, "jq", `/bin/cat >/dev/null`)
	step := &domain.Step{}
	step.Response.ActualBody = `{"active":true}`

	failed, err := NewValidator().ValidateTypes(context.Background(), step)
	if err != nil || failed != "" {
		t.Fatalf("ValidateTypes() = (%q, %v), want empty success", failed, err)
	}
}

func TestValidateBodyReturnsEmptyDiffForEqualNormalizedBodies(t *testing.T) {
	commandDir := newValidatorCommandDir(t)
	logPath := filepath.Join(commandDir, "jq-log")
	t.Setenv("APIH_VALIDATOR_LOG", logPath)
	installValidatorCommand(t, commandDir, "jq", `
input=$(/bin/cat)
printf '%s|%s\n' "$*" "$input" >> "$APIH_VALIDATOR_LOG"
printf '%s\n' '{'
printf '%s\n' '  "a": 1,'
printf '%s\n' '  "b": 2'
printf '%s\n' '}'
`)
	installValidatorCommand(t, commandDir, "git", `printf 'git must not run' >&2; exit 99`)

	step := &domain.Step{}
	step.Response.ExpectedBody = `{"b":2,"a":1}`
	step.Response.ActualBody = `{"a":1,"b":2}`
	wantStep := cloneValidatorStep(step)

	diff, err := NewValidator().ValidateBody(context.Background(), step)
	if err != nil || diff != "" {
		t.Fatalf("ValidateBody() = (%q, %v), want empty success", diff, err)
	}
	log := readValidatorFile(t, logPath)
	if !strings.Contains(log, `--sort-keys .|{"b":2,"a":1}`) {
		t.Fatalf("jq log = %q, want expected body pretty invocation", log)
	}
	wantProjection := "--sort-keys -- " + buildBodySelector(string(step.Response.ExpectedBody)) + `|{"a":1,"b":2}`
	if !strings.Contains(log, wantProjection) {
		t.Fatalf("jq log = %q, want actual body projection invocation", log)
	}
	if !reflect.DeepEqual(step, wantStep) {
		t.Fatalf("ValidateBody() mutated step = %+v", step)
	}
}

func TestValidateBodySkipsUnspecifiedExpectedBody(t *testing.T) {
	commandDir := newValidatorCommandDir(t)
	installValidatorCommand(t, commandDir, "jq", `printf 'jq must not run' >&2; exit 99`)
	installValidatorCommand(t, commandDir, "git", `printf 'git must not run' >&2; exit 99`)

	step := &domain.Step{}
	step.Response.ExpectedBody = " \n\t"
	step.Response.ActualBody = `{"ignored":true}`
	wantStep := cloneValidatorStep(step)

	diff, err := NewValidator().ValidateBody(context.Background(), step)
	if err != nil || diff != "" {
		t.Fatalf("ValidateBody() = (%q, %v), want empty success", diff, err)
	}
	if !reflect.DeepEqual(step, wantStep) {
		t.Fatalf("ValidateBody() mutated step = %+v", step)
	}
}

func TestBuildBodySelectorDoesNotMaterializeMissingFieldsAsNull(t *testing.T) {
	selector := buildBodySelector(`{"aaa":null}`)
	if !strings.Contains(selector, `if ($actual | has($key)) then`) {
		t.Fatalf("buildBodySelector() = %q, want explicit actual-field presence check", selector)
	}
}

func TestValidateBodyReturnsGitDiffForUnequalNormalizedBodies(t *testing.T) {
	commandDir := newValidatorCommandDir(t)
	expectedPath := filepath.Join(commandDir, "expected")
	actualPath := filepath.Join(commandDir, "actual")
	t.Setenv("APIH_VALIDATOR_EXPECTED", expectedPath)
	t.Setenv("APIH_VALIDATOR_ACTUAL", actualPath)
	installValidatorCommand(t, commandDir, "jq", `
input=$(/bin/cat)
case "$input" in
  *expected*) printf '%s\n' '{"value":"expected"}' ;;
  *) printf '%s\n' '{"value":"actual"}' ;;
esac
`)
	installValidatorCommand(t, commandDir, "git", `
/bin/cp expected "$APIH_VALIDATOR_EXPECTED"
/bin/cp actual "$APIH_VALIDATOR_ACTUAL"
printf '%s\n' 'diff --git actual expected' '--- actual' '+++ expected' '@@ -1 +1 @@' '-{"value":"actual"}' '+{"value":"expected"}'
exit 1
`)

	step := &domain.Step{}
	step.Response.ExpectedBody = `{"value":"expected"}`
	step.Response.ActualBody = `{"value":"actual"}`
	diff, err := NewValidator().ValidateBody(context.Background(), step)
	if err != nil {
		t.Fatalf("ValidateBody() error = %v", err)
	}
	wantDiff := "-{\"value\":\"actual\"}\n+{\"value\":\"expected\"}\n"
	if diff != wantDiff {
		t.Fatalf("ValidateBody() diff = %q, want %q", diff, wantDiff)
	}
	if got := readValidatorFile(t, expectedPath); got != `{"value":"expected"}` {
		t.Fatalf("GitDiff expected = %q, want normalized expected body", got)
	}
	if got := readValidatorFile(t, actualPath); got != `{"value":"actual"}` {
		t.Fatalf("GitDiff actual = %q, want normalized actual body", got)
	}
}

func TestValidatorClassifiesRunnerFailuresAsFatal(t *testing.T) {
	tests := map[string]struct {
		install  func(*testing.T, string)
		call     func(*Validator, *domain.Step) (string, error)
		want     error
		wantCode int
	}{
		"type filter": {
			install: func(t *testing.T, dir string) {
				installValidatorCommand(t, dir, "jq", `/bin/cat >/dev/null; printf 'bad filter' >&2; exit 7`)
			},
			call: func(validator *Validator, step *domain.Step) (string, error) {
				return validator.ValidateTypes(context.Background(), step)
			},
			want:     runner.ErrJQSelector,
			wantCode: 7,
		},
		"expected body": {
			install: func(t *testing.T, dir string) {
				installValidatorCommand(t, dir, "jq", `/bin/cat >/dev/null; printf 'bad expected body' >&2; exit 8`)
			},
			call: func(validator *Validator, step *domain.Step) (string, error) {
				return validator.ValidateBody(context.Background(), step)
			},
			want:     runner.ErrJQPretty,
			wantCode: 8,
		},
		"actual body": {
			install: func(t *testing.T, dir string) {
				installValidatorCommand(t, dir, "jq", `
input=$(/bin/cat)
case "$input" in
  expected) printf '{}\n' ;;
  *) printf 'bad actual body' >&2; exit 9 ;;
esac
`)
			},
			call: func(validator *Validator, step *domain.Step) (string, error) {
				return validator.ValidateBody(context.Background(), step)
			},
			want:     runner.ErrJQSelector,
			wantCode: 9,
		},
		"body diff": {
			install: func(t *testing.T, dir string) {
				installValidatorCommand(t, dir, "jq", `input=$(/bin/cat); printf '%s\n' "$input"`)
				installValidatorCommand(t, dir, "git", `printf 'diff failed' >&2; exit 10`)
			},
			call: func(validator *Validator, step *domain.Step) (string, error) {
				return validator.ValidateBody(context.Background(), step)
			},
			want:     runner.ErrGitDiff,
			wantCode: 10,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			commandDir := newValidatorCommandDir(t)
			test.install(t, commandDir)
			step := &domain.Step{}
			step.Response.ExpectedBody = "expected"
			step.Response.ActualBody = "actual"
			step.Response.ExpectedTypes = map[string][]string{".": {"object"}}
			wantStep := cloneValidatorStep(step)

			result, err := test.call(NewValidator(), step)
			if result != "" || !errors.Is(err, ErrValidatorFatal) || !errors.Is(err, test.want) {
				t.Fatalf("validator result = (%q, %v), want empty fatal %v", result, err, test.want)
			}
			if got := errs.Code(err, 0); got != test.wantCode {
				t.Fatalf("validator error code = %d, want preserved command code %d", got, test.wantCode)
			}
			if !reflect.DeepEqual(step, wantStep) {
				t.Fatalf("validator mutated step = %+v", step)
			}
		})
	}
}

func TestValidatorPropagatesContextCancellation(t *testing.T) {
	commandDir := newValidatorCommandDir(t)
	installValidatorCommand(t, commandDir, "jq", `printf 'must not run'`)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	step := &domain.Step{}
	step.Response.ExpectedBody = `{}`
	step.Response.ActualBody = `{}`

	failed, typeErr := NewValidator().ValidateTypes(ctx, step)
	if failed != "" || !errors.Is(typeErr, context.Canceled) || !errors.Is(typeErr, ErrValidatorFatal) {
		t.Fatalf("ValidateTypes(canceled) = (%q, %v), want canceled fatal error", failed, typeErr)
	}
	diff, bodyErr := NewValidator().ValidateBody(ctx, step)
	if diff != "" || !errors.Is(bodyErr, context.Canceled) || !errors.Is(bodyErr, ErrValidatorFatal) {
		t.Fatalf("ValidateBody(canceled) = (%q, %v), want canceled fatal error", diff, bodyErr)
	}
}

func cloneValidatorStep(step *domain.Step) *domain.Step {
	clone := *step
	if step.Response.ExpectedTypes != nil {
		clone.Response.ExpectedTypes = make(map[string][]string, len(step.Response.ExpectedTypes))
		for selector, expected := range step.Response.ExpectedTypes {
			clone.Response.ExpectedTypes[selector] = append([]string(nil), expected...)
		}
	}
	return &clone
}

func newValidatorCommandDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return dir
}

func installValidatorCommand(t *testing.T, dir, name, body string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("#!/bin/sh\nset -eu\n"+body+"\n"), 0o755); err != nil {
		t.Fatalf("WriteFile(%s) error = %v", path, err)
	}
}

func readValidatorFile(t *testing.T, path string) string {
	t.Helper()
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", path, err)
	}
	return string(contents)
}
