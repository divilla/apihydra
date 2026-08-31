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
goroutine start/completion order within a parallel task set, sorting beyond the
existing plan and slice structure, a concurrency limit, URL construction,
concurrent debug-winner selection, or precedence between multiple nonfatal
validation failures. Steps within a file are explicitly serial; no step-level
goroutines are permitted.

## Required implementation and tests

- Production output: `internal/execution/executor.go` retains the implemented
  tree validation and mode-aware directory/file schedulers and replaces the
  `Prepare` and `processFile` TODO bodies with the binding deep-copy and
  execution behavior, including the Debug-specific
  `CurlBuild`/`CurlRaw`/`CurlExecute` branch.
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
   mutable slices and maps such as each value-typed
   `Step.Request.Defaults.Headers`, preserves the original `Step.Definition`
   pointers and `Step.Index` values, does not mutate `ResolvedSteps`, and does
   not load or interpolate variables. It retains the unified `domain.Defaults`
   structure and does not introduce `*domain.Defaults` pointers.
3. `ValidateDirectories` returns configuration code and
   `ErrInvalidDirectoryTree` without panic for every invalid tree shape covered
   by the reference.
4. `PlanStages` groups a successfully validated tree by stage, and all stages
   execute serially with complete barriers. Config mode `0` runs directories
   and files serially; mode `1` overlaps same-stage directories but serializes
   each directory's files; mode `2` also overlaps files within each directory.
   Task sets are unbounded, file steps remain serial, and invalid modes return
   configuration `ErrInvalidParallelism`.
5. Executor brackets each stage with Reporter `BeginStage` and `EndStage`, using
   plan order and the existing definition/step slice order as canonical output
   order regardless of goroutine completion order.
6. A fatal stage error cancels and joins shared directory and file work,
   performs the final stage commit, and prevents later work. The first
   fatal result replaces provisional validation and retains its associated code
   and error without being replaced by later results. Terminal step errors gain
   `ErrStepExecution`, definition-file, and `spec.steps[index]` provenance; the
   later CLI stderr diagnostic is the final application output.
7. Non-debug per-step execution uses the eight phases in the reference order.
   A debug step replaces `runner.Curl` with `runner.CurlBuild`,
   `runner.CurlRaw`, assignment to `Step.RawCurl`, and `runner.CurlExecute`
   using the unchanged executable, arguments, and request body. `CurlRaw` alone
   owns its final-data compaction/fallback and POSIX header/data presentation;
   Executor does not copy, reorder, compact, quote, or otherwise transform the
   arguments. Both paths assign the Curl response status to
   `Step.Response.ActualStatus`, convert and assign the Curl response body to
   the `domain.YAMLString` `Step.Response.ActualBody`, interpolate expected
   values before Curl runs, and send a non-empty `ValidateTypes` failed string
   to `Reporter.ValidationTypes`.
8. Completed validation mismatch traversal continues through remaining work
   and returns code `101` with a nil error.
9. Immediately before a debug step finishes or returns a terminal error,
   Executor sends Reporter its latest mutated state. After Reporter
   successfully prints a successful debug step, Executor cancels concurrent
   work, skips all later steps and stages, suppresses directory success output,
   and converts its private stop signal into clean exit code `0` with no error.
   A terminal error is reported with the latest available state and then
   retains its original error and exit code.
10. One shared Binder/KeyValueStore is retained in every mode. The scheduler
    introduces no per-file or per-directory variable scope.
11. Presentation, terminal redraw mechanics, sorting, and per-validator payload
    rules remain with their
   owning packages.
12. No TODO or zero-value placeholder remains in `Prepare` or `processFile`;
   package tests, race tests, and `git diff --check` pass.
