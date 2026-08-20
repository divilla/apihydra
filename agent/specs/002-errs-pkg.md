# `pkg/errs`

## Status and ownership

- Binding reference: `skeleton/pkg/errs/errors.go`
- Shared product contract: [`prd.md`](../prd.md)
- Status: skeleton-aligned specification

This specification owns contextual error construction. The PRD owns the
meaning of the four product exit codes; originating packages own their static
error classifications.

## Public API

```go
type ExitCoder interface {
    ExitCode() int
}

func Build(code int, errStatic, errOriginal error, details ...any) error
func WithExitCode(code int, err error) error
func Code(err error, fallback int) int
func DefaultsDefinitionError(defaults *domain.DefaultsDefinition, yamlPath string,
    errStatic, errOriginal error) error
func StepDefinitionError(step *domain.StepsDefinition, yamlPath string,
    errStatic, errOriginal error) error
func StepExecutionError(step *domain.Step, yamlPath string,
    errStatic, errOriginal error) error
```

`ExitError` is the concrete private-state implementation of `error` and
`ExitCoder` exposed by these constructors.

## Construction and lookup

`Build` returns nil when `errStatic` is nil. Otherwise it retains the supplied
code, static error, optional original error, and detail values.

`ExitError.Error` joins non-empty components with `": "` in this order:

1. static error text;
2. `fmt.Sprint(details...)`;
3. original error text.

`ExitError.Unwrap` returns the static error and, when present, the original
error. Both identities remain available to `errors.Is` and `errors.As`.
`ExitCode` returns the retained code.

`WithExitCode(code, err)` delegates to `Build(code, err, nil)`.

`Code` returns:

- `ExitSuccess` for nil;
- the first discoverable `ExitCoder.ExitCode()` for a coded error;
- `fallback` for a non-nil uncoded error.

## Provenance helpers

`DefaultsDefinitionError` and `StepDefinitionError` always use
`ExitConfiguration`. `StepExecutionError` uses the original error's attached
code with `ExitInternal` as fallback; an attached success code is replaced by
`ExitInternal`.

The helpers safely derive optional details through the reference provenance
chain. A source file contributes `file <path>`, a non-empty YAML path
contributes `yaml path <path>`, and both are joined with `, `. Missing
definitions, files, or step provenance omit unavailable details without
panicking.

## Boundary

This package does not own static errors from other packages, terminal output,
ANSI styling, source-line lookup, cancellation, or command execution. The
repository-wide contextual-error ownership rule is defined once in the PRD.

## Acceptance criteria

1. All exported names and signatures match the reference.
2. Construction preserves codes, error identities, component order, and
   optional details exactly.
3. Nil static errors return nil, and nil lookup returns success.
4. Definition helpers use configuration code and safe provenance.
5. Step execution errors cannot expose a non-nil error as success.
6. No other package's static classification or presentation behavior is
   duplicated here.
