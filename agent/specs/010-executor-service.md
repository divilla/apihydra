# `internal/execution` Executor

## Status and ownership

- Binding reference: [`skeleton/internal/execution/executor.go`](../../skeleton/internal/execution/executor.go)
- Reference tests: [`skeleton/internal/execution/executor_test.go`](../../skeleton/internal/execution/executor_test.go)
- Shared domain and exit codes: [`prd.md`](../prd.md)
- Shared domain types: [`000-domain-types.md`](000-domain-types.md)
- Binder phases: [`007-binder-service.md`](007-binder-service.md)
- Reporter methods: [`009-reporter-service.md`](009-reporter-service.md)
- Status: skeleton-aligned implementation guide

## Reference contract

The binding skeleton implementation, comments, and tests define Executor's
API, preparation scope, tree validation, stage scheduling, execution order, and
result precedence. This guide does not reproduce them. Keeping validation,
planning, and preparation separate rejects malformed trees before scheduling
and preserves resolved definitions while runtime steps are mutated.

## Boundaries

Executor coordinates its collaborators. It does not own variable syntax,
validation algorithms, command implementation, output layout, or contextual
error formatting. Those contracts remain with their owning skeleton packages.
Runner owns the exact Curl command that is executed; Reporter owns rendering
that same unaltered command and the latest runtime step in a Debug dump.

## Deliberately unspecified

The skeleton does not define preparation traversal order or transactionality,
ordering between same-stage goroutines, sorting beyond the existing slice
structure, separate file or step goroutines, URL construction, success
reporting, concurrent debug-winner selection, or precedence between multiple
nonfatal validation failures.

## Required implementation and tests

- Production output: `internal/execution/executor.go` retains the implemented
  tree validation and scheduler and replaces the `Prepare` and `processDir`
  TODO bodies with the binding deep-copy and eight-phase execution behavior.
- Test output: `internal/execution/executor_test.go` retains the reference tests
  and adds coverage for deep-copy isolation, every phase and mutation, all
  validation-reporting paths, collaborator failures, success/debug reporting
  required by the enhanced Debug contract, latest-state reporting before
  normal completion and terminal-error return, and context cancellation.
- Each acceptance criterion is traced to at least one meaningful unit test, and
  Executor unit-test statement coverage remains greater than 95% under the
  race detector.

## Acceptance criteria

1. Public names, signatures, constructor state, and static error text match the
   reference.
2. `Prepare` deep-copies `ResolvedSteps` into `RuntimeSteps`, including all
   mutable slices and maps such as each value-typed
   `Step.Request.Defaults.Headers`, preserves the original `Step.Definition`
   pointers and `Step.Index` values, does not mutate `ResolvedSteps`, and does
   not load or interpolate variables. It retains the unified `domain.Defaults`
   structure and does not introduce `*domain.Defaults` pointers.
3. `ValidateDirectories` returns configuration code and
   `ErrInvalidDirectoryTree` without panic for every invalid tree shape covered
   by the reference.
4. `PlanStages` groups a successfully validated tree by stage, same-stage
   directories may overlap during `Execute`, all are joined, and later stages
   wait for the barrier.
5. A fatal stage error cancels shared work and prevents later stages. The first
   fatal result replaces provisional validation and retains its associated code
   and error without being replaced by later results.
6. Per-step execution uses the eight phases in the reference order, assigns the
   Curl response status to `Step.Response.ActualStatus`, converts and assigns
   the Curl response body to the `domain.YAMLString`
   `Step.Response.ActualBody`, interpolates expected values before Curl runs,
   retains the exact executed Curl command for Debug without reconstructing or
   redacting it, and sends a non-empty `ValidateTypes` failed string to
   `Reporter.ValidationTypes`.
7. Completed validation mismatch traversal continues through remaining work
   and returns code `101` with a nil error.
8. Executor requests a Debug dump at the latest possible point before a debug
   step finishes normally or its processing returns a terminal error. The dump
   therefore sees every runtime mutation completed before that point and the
   exact Curl command that was executed or attempted. A normally completed
   debug step cancels concurrent work, skips all later steps and stages,
   suppresses directory success output, and converts its private stop signal
   into clean exit code `0` with no error. A terminal execution error retains
   its error and exit outcome after the dump is attempted.
9. Presentation, sorting, and per-validator payload rules remain with their
   owning packages.
10. No TODO or zero-value placeholder remains in `Prepare` or `processDir`;
   package tests, race tests, and `git diff --check` pass.
