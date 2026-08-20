# `cmd/cli` main application

## Status and ownership

- Binding implementation reference: `skeleton/cmd/cli/main.go`
- Binding reference tests: `skeleton/cmd/cli/main_test.go`
- Shared CLI and exit-code contract: [`prd.md`](../prd.md)
- Reporter contract: [`09-reporter.md`](009-reporter-service.md)
- Definition collaborators: [`03-loader-service.md`](003-loader-service.md),
  [`04-decoder-service.md`](004-decoder-service.md), and
  [`05-resolver-service.md`](005-resolver-service.md)
- Execution collaborator: [`10-step-runner-service.md`](010-step-runner-service.md)
- Status: skeleton-aligned specification

This specification owns the production composition root in `cmd/cli` and its
package-local tests. It does not expand the public API or the pipeline
represented by the skeleton.

## Production contract

`cmd/cli/main.go` must reproduce `skeleton/cmd/cli/main.go` exactly, except that
imports use the production module path (`apih/...`) rather than the skeleton
module path (`apih/skeleton/...`).

The CLI package declares these exact package-level errors:

```go
var ErrInvalidPath = errors.New("invalid path")
var ErrWorkingDirectory = errors.New("working directory error")
```

It retains the reference entry points without adding flags, exported helpers,
configuration types, or alternate application constructors:

```go
func main()

func run(
    ctx context.Context,
    args []string,
    reporter *reporting.Reporter,
) (int, error)
```

`run` remains package-private. The application name is supplied through
`args[0]`; the reference does not inspect or validate its value.

## Process entry point

`main`:

1. configures the standard logger without timestamp flags and with `os.Stderr`;
2. constructs one Reporter backed by `os.Stdout`;
3. invokes `run(context.Background(), os.Args, outputReport)` exactly once;
4. passes a returned non-nil error to `log.Print`; and
5. calls `os.Exit` with the exact code returned by `run`.

`log.Fatal` and `log.Fatalf` are not used because both force exit code `1`.
Reporter never owns stderr. No other production package may own fatal
diagnostic logging or process exit.

## Working-directory selection

`run` begins with `os.Getwd()`.

- A working-directory lookup failure returns `errs.ExitInternal` and an error
  built with `ErrWorkingDirectory` that preserves the original failure.
- With no first positional argument, the current working directory is used.
- When `len(args) > 1`, only `args[1]` participates in selection. `run` joins it
  to the current working directory using `filepath.Join`, calls `os.Stat`, and
  requires the result to be a directory.
- A missing, inaccessible, or non-directory selected path returns
  `errs.ExitConfiguration` and an error built with `ErrInvalidPath`. Stat
  failures preserve the original error, and both invalid-path forms include the
  selected path as error context.
- Arguments after `args[1]` are not interpreted by the current reference.

No working-directory line is written before selection and validation succeed.
After successful selection, `run` calls
`reporter.WorkingDirectory(workDir)`. A reporting failure is returned with
`errs.ExitInternal` before any definition service is invoked.

## Definition pipeline

After reporting the selected directory, `run` derives a cancelable child
context, defers cancellation, and creates exactly one shared suite:

```go
suite := &domain.Suite{WorkDir: workDir}
```

It then invokes these eight phases, in this exact order, on the same context and
suite:

1. `Loader.LoadDirectoryStructure`
2. `Loader.LoadDirectoryFiles`
3. `Loader.DecodeBaseDefinitions`
4. `Decoder.DecodeFiles`
5. `Decoder.ValidateDefaultsDefinitions`
6. `Decoder.ValidateStepsDefinitions`
7. `Resolver.ResolveDefaults`
8. `Resolver.ResolveSteps`

The first phase error stops the pipeline and is returned unchanged with
`errs.ExitConfiguration`. Successful completion of all eight phases continues
into execution composition.

## Execution composition

After definition resolution, `run` constructs these execution collaborators in
order:

1. `execution.NewKeyValueStore()`
2. `execution.NewVariableProcessor(kvs)`
3. `execution.NewValidator()`
4. `execution.NewStepRunner(varProc, validator, reporter)`

The same Reporter that wrote the working directory is supplied to StepRunner.
`run` then calls `stepRunner.ValidateDirectories(suite)`. A validation failure
returns that method's exit code and error unchanged. On success, `run` calls
`stepRunner.Prepare(suite)`, obtains `stagesPlan` from
`stepRunner.PlanStages(suite)`, and returns
`stepRunner.Execute(ctx, stagesPlan)` unchanged.

The CLI does not add filtering or debug-selection behavior around the execution
services.

## Package tests

Tests remain in `cmd/cli/*_test.go` and follow
`skeleton/cmd/cli/main_test.go`, using production import paths. They verify
invalid-path rejection, successful main-flow completion, and output failure
handling without adding production test seams absent from the skeleton.

## Acceptance criteria

1. `cmd/cli/main.go` is identical to `skeleton/cmd/cli/main.go` after replacing
   `apih/skeleton/` import prefixes with `apih/`.
2. Invalid selected paths return `errs.ExitConfiguration` and no working
   directory output; a successful main flow returns `0` after
   reporting the working directory.
3. Reporter output failures return `errs.ExitInternal` and preserve the writer
   failure.
4. Fatal errors are logged to stderr and retain the exact product exit code
   returned by `run`.
5. After `Resolver.ResolveSteps`, the application constructs the execution
   collaborators, validates the directory tree, prepares runtime steps, plans
   stages, and executes the plan in the exact order represented by the
   skeleton, without adding behavior absent from that reference.
6. `go test ./cmd/cli`, `go test ./...`, `go test -race ./...`, and
   `git diff --check` pass.
