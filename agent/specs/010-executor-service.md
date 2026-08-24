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

## Deliberately unspecified

The skeleton does not define preparation traversal order or transactionality,
ordering between same-stage goroutines, sorting beyond the existing slice
structure, separate file or step goroutines, URL construction, success
reporting, debug selection or stopping, or precedence between multiple
nonfatal validation failures.

## Required implementation and tests

- Production output: `internal/execution/executor.go` retains the implemented
  tree validation and scheduler and replaces the `Prepare` and `processDir`
  TODO bodies with the binding deep-copy and eight-phase execution behavior.
- Test output: `internal/execution/executor_test.go` retains the reference tests
  and adds coverage for deep-copy isolation, every phase and mutation, all
  validation-reporting paths, collaborator failures, success/debug reporting
  chosen within the contract, and context cancellation.
- Each acceptance criterion is traced to at least one meaningful unit test, and
  Executor unit-test statement coverage remains greater than 95% under the
  race detector.

## Acceptance criteria

1. Public names, signatures, constructor state, and static error text match the
   reference.
2. `Prepare` deep-copies `ResolvedSteps` into `RuntimeSteps`, including all
   mutable slices and maps, preserves the original `Step.Definition` pointers,
   does not mutate `ResolvedSteps`, and does not load or interpolate variables.
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
   Curl response status and body to `Step.Response.ActualStatus` and
   `Step.Response.ActualBody`, interpolates expected values before Curl runs,
   and sends a non-empty `ValidateTypes` failed string to
   `Reporter.ValidationTypes`.
7. Completed validation mismatch traversal continues through remaining work
   and returns code `101` with a nil error.
8. Debug, presentation, sorting, and per-validator payload rules absent from
   the skeleton are not introduced here.
9. No TODO or zero-value placeholder remains in `Prepare` or `processDir`;
   package tests, race tests, and `git diff --check` pass.
