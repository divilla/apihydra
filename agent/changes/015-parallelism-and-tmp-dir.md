# Parallelism, Ordered Output, and Per-Run Temporary Storage

## Authority and scope

Implement this change against these binding references:

- `skeleton/internal/domain/config.go`
- `skeleton/cmd/apih/main.go`
- `skeleton/internal/execution/executor.go`
- `skeleton/internal/execution/validator.go`
- `skeleton/internal/reporting/reporter.go`
- `skeleton/pkg/runner/runner.go`

Their declarations, behavior comments, implemented schedulers, and reference
tests are authoritative. Production code, the PRD, component guides,
architecture documentation, and tests must all match them. Root and defaults
documents remain definition inputs whose values are propagated during
resolution. Only steps documents contain executable files and steps.

### User approval record

On 2026-08-31, the user explicitly approved the versions present at commit
`7c76be8` of all skeleton files changed by this change as the valid,
authoritative skeleton reference:

- `skeleton/cmd/apih/main.go`
- `skeleton/cmd/apih/main_test.go`
- `skeleton/internal/domain/config.go`
- `skeleton/internal/domain/config_test.go`
- `skeleton/internal/execution/executor.go`
- `skeleton/internal/execution/executor_test.go`
- `skeleton/internal/execution/validator.go`
- `skeleton/internal/execution/validator_test.go`
- `skeleton/internal/reporting/reporter.go`
- `skeleton/internal/reporting/reporter_test.go`
- `skeleton/pkg/runner/runner.go`

## CLI configuration

- Use `github.com/spf13/pflag` and retain its native flag behavior, including
  attached and equals forms, interspersed flags, repeated flags with the final
  value winning, and `--` termination.
- Both `-p` and `--parallelism` populate `domain.Config.Parallelism`.
- Parallelism defaults to `1`; only `0`, `1`, and `2` are valid.
- Accept at most one positional suite directory and store it in
  `domain.Config.Directory`.
- `-h` and `--help` print pflag-generated usage to stdout, exit successfully,
  and do not start an application run or create a run directory.
- Unknown flags, malformed flag values, unsupported parallelism values, and
  excess positional arguments return configuration exit code `102`, end with a
  CLI-owned stderr diagnostic, and produce no application stdout.
- Preserve the existing selected-directory validation and `ErrInvalidPath`
  behavior after parsing succeeds.

## Per-run temporary directory

- Every valid application run creates one unique, private `run-*` directory
  below `filepath.Join(os.UserCacheDir(), "apih")` and stores it in
  `domain.Config.TempRunDir` before constructing runtime services.
- Create the base and run directories lazily. The base is shared only by runs
  of the current user; every run directory and operation directory is private.
- Inject `domain.Config` by value into runtime consumers. Do not use package
  globals, mutate `TMPDIR` or another process-wide environment value, or fall
  back to the shared OS temporary directory.
- Temporary consumers create namespaced children below `Config.TempRunDir`.
  `runner.GitDiff` receives the run-directory path explicitly and creates
  unique operation directories only below `TempRunDir/git-diff`.
- `runner.GitDiff` attempts to remove each operation directory on every return.
  The CLI defers best-effort removal of the complete run directory on every
  controlled return, whether successful or failed.
- Suppress all operation- and run-directory cleanup errors. Cleanup never
  changes an exit code, replaces an existing result, or emits output.
- Abrupt process or machine termination may prevent cleanup; no recovery or
  startup scavenging requirement is introduced.
- The run directory is a general application workspace. Git diff must not
  assume it is the only consumer or place artifacts directly at its root.

## Execution modes

Stages are never parallel. Execute every stage in `PlanStages` order and fully
join it before starting the next stage.

- Mode `0`, no parallelism: process directories in stage-plan order, steps
  files in each directory's slice order, and steps in each file's slice order.
- Mode `1`, directory parallelism: process every directory in the active stage
  concurrently; within each directory, process steps files and their steps
  serially in slice order. This is the default.
- Mode `2`, file parallelism: process every directory in the active stage
  concurrently and every steps file within each directory concurrently; steps
  within one file remain serial in slice order.
- Directory and file task sets are unbounded; this change adds no worker limit.
- Invalid Config parallelism values are rejected defensively by Executor with
  configuration `ErrInvalidParallelism`, even though CLI validation normally
  prevents them.
- Retain one shared concurrent, write-once KeyValueStore and Binder in every
  mode. Do not introduce per-directory or per-file variable scopes. A suite
  with cross-file or cross-directory producer/consumer dependencies must use
  mode `0` when deterministic availability is required.
- Nonfatal validation mismatches do not cancel work. Complete all remaining
  eligible work and return exit code `101` with a nil error.
- A terminal error cancels and joins every active directory and file task,
  prevents all later work, and preserves the existing first-fatal-result and
  Debug precedence rules.

## Ordered stage output

Logical stdout order is always:

```text
stage -> directory -> steps file -> step
```

Use the existing stage plan, `StepsDefinitions`/`RuntimeSteps` correspondence,
and step slice order. Goroutine start, event, or completion order must never
change final logical output order.

- Executor calls `Reporter.BeginStage` before scheduling a stage and
  `Reporter.EndStage` after all active work in that stage is joined.
- Reporter owns one buffer per steps file and routes every success, validation,
  and related execution-output event to that file's buffer.
- Success is reported per steps definition. Root and defaults files never
  receive executable success or failure blocks.
- For terminal stdout, every file-output event clears and redraws only the
  active-stage region using the latest contents of all file buffers in
  canonical directory/file/step order. The working-directory heading and all
  completed stages remain fixed and are never redrawn.
- For non-terminal stdout, emit no partial stage output. At `EndStage`, write
  the complete stage exactly once in canonical order.
- Concurrent writes remain serialized and race-free. Existing colors, success
  lines, grouped validation blocks, indentation, and Debug layout remain
  unchanged within each logical file block.
- A successful Debug breakpoint atomically suppresses later reporting and
  execution. Preserve every file block accumulated before cancellation in
  canonical order, then render the Debug dump separately as the final stdout
  block, regardless of the debug file's plan position. Concurrent Debug winner
  selection remains unspecified.

## Terminal errors and final output

- Wrap terminal step failures with `ErrStepExecution` and
  `errs.StepExecutionError` so the final diagnostic identifies the definition
  file, its directory through that path, and `spec.steps[index]`.
- On a terminal execution error, cancel and join active work, perform the final
  ordered `EndStage` render, and return the original coded failure.
- `cmd/apih` then writes the provenance-bearing fatal diagnostic to stderr.
  That diagnostic is the final application output: no later stdout, stderr,
  reporting event, cleanup diagnostic, step, file, directory, or stage is
  permitted.
- Retain existing exit-code precedence, validation semantics, and Debug
  behavior except where this change explicitly defines ordered buffering and
  final-output placement.

## Required implementation and tests

- Mirror every revised skeleton declaration in production, replacing only the
  `github.com/divilla/apihydra/skeleton/` import prefix with
  `github.com/divilla/apihydra/`.
- Implement Config-aware CLI, Validator, Executor, Reporter, and Runner
  behavior without retaining skeleton TODO placeholders in production.
- Update architecture ownership checks so `domain.Config` has one owner and
  execution output remains confined to Reporter.
- Add meaningful tests for every acceptance criterion below. Keep statement
  coverage above 95% for every changed production package and run all relevant
  tests under the race detector.
- Revise all conflicting PRD, architecture, specification, integration-test,
  and implementation documentation. Do not retain descriptions of mandatory
  same-stage directory concurrency, direct system-temporary storage,
  completion-order output, or the pre-pflag CLI.

## Acceptance criteria

1. Production public names, signatures, static errors, Config fields, and
   constructor state match every binding reference.
2. Native pflag forms, aliases, repetition, interspersed parsing, `--`, help,
   invalid values, positional arity, stdout/stderr placement, and exit codes
   match the CLI reference tests.
3. Every valid run receives a unique private
   `os.UserCacheDir()/apih/run-*` directory, all runtime temporary artifacts are
   namespaced beneath it, GitDiff never uses the system temporary directory,
   concurrent GitDiff calls use distinct children, and cleanup is attempted
   but every cleanup failure is silent.
4. Modes `0`, `1`, and `2` exhibit exactly the specified directory/file
   overlap while stages and per-file steps remain serial and ordered. Invalid
   modes fail defensively, and stage/file task sets are not artificially
   limited.
5. Every mode uses the same shared write-once KeyValueStore. Race tests pass,
   and no alternate variable scope or synchronization contract is introduced.
6. Reporter produces canonical stage/directory/file/step order when workers
   deliberately complete in reverse order. Terminal tests prove active-region
   redraw without rewriting completed output; non-terminal tests prove one
   complete write per stage with no partial stage output.
7. Success, every validation form, multiple files, multiple directories,
   empty steps files, and multiple stages retain their existing logical layouts
   inside the new ordered transaction model.
8. Fatal directory/file/step failures cancel and join active work, prevent all
   later work, finalize accumulated stdout, emit one provenance-bearing stderr
   diagnostic last, and produce no bytes or events afterward.
9. Validation-only runs complete eligible work and return `101` with nil error.
   Successful Debug stops cleanly, retains accumulated canonical file output,
   renders its complete unredacted dump last, and produces nothing afterward.
10. Changed package tests, black-box CLI and terminal tests, `go test ./...`,
    `go test -race ./...`, `make check`, coverage checks, and
    `git diff --check` all pass.
