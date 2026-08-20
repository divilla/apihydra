# `internal/execution` StepRunner

## Status and ownership

- Binding reference: [`skeleton/internal/execution/steprun.go`](../../skeleton/internal/execution/steprun.go)
- Reference tests: [`skeleton/internal/execution/steprun_test.go`](../../skeleton/internal/execution/steprun_test.go)
- Shared domain and exit codes: [`prd.md`](../prd.md)
- Reporter methods: [`009-reporter-service.md`](009-reporter-service.md)
- Status: skeleton-aligned implementation guide

## Reference contract

The binding skeleton implementation, comments, and tests define StepRunner's
API, preparation scope, tree validation, stage scheduling, execution order, and
result precedence. This guide does not reproduce them. Keeping validation,
planning, and preparation separate rejects malformed trees before scheduling
and preserves resolved definitions while runtime steps are mutated.

## Boundaries

StepRunner coordinates its collaborators. It does not own variable syntax,
validation algorithms, command implementation, output layout, or contextual
error formatting. Those contracts remain with their owning skeleton packages.

## Deliberately unspecified

The skeleton does not define preparation traversal order or transactionality,
ordering between same-stage goroutines, sorting beyond the existing slice
structure, separate file or step goroutines, URL construction, success
reporting, debug selection or stopping, or precedence between multiple
nonfatal validation failures.

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
