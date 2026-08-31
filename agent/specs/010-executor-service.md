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

Executor does own selection, creation, inheritance, and completion tracking of
cookie jars. Runner owns only translation of the selected non-empty path into
curl arguments and omission of those arguments for an empty path.

## Cookie-jar lifecycle

Every jar is created in a cookie-specific namespace below
`Config.TempRunDir`, exists before it can be passed to Runner, and shares the
run directory's cleanup lifecycle. Executor eagerly creates mode-owned jars
even when all currently assigned steps have cookies disabled. A step with
effective `DisableCookies == true` passes an empty cookie-jar parameter and
leaves its owning jar unchanged. Nil or false enables cookies and passes the
owning jar. Explicit `Cookie` headers remain ordinary headers.

Parallelism strictly selects ownership and inheritance:

- Mode `0` creates one jar for the run, uses it for every enabled request, and
  creates no transition copies.
- Mode `1` creates one jar per directory. The root jar starts empty. After a
  stage joins, each direct child directory receives a distinct byte-for-byte
  copy of its parent's final jar before the next stage begins.
- Mode `2` creates one jar per steps file. Root file jars start empty. After
  every step finishes, including a cookie-disabled step, Executor records that
  file's jar as the directory's latest completed jar. After the stage joins,
  each steps file in every direct child receives a distinct copy of the parent
  jar whose step completion was observed last. Actual completion order and Go
  scheduling intentionally choose the source; file modification timestamps
  are not used.

In mode `2`, a directory with no executed steps preserves its incoming state.
Empty steps-file jars are unchanged copies of that state and may be copy
sources. A root with no steps-file jars creates one additional empty
inheritance jar. Copies flow only from parent to direct child; sibling state is
never exchanged, jars are never merged, and concurrent work never shares a
writable jar. Another run's jars are never discovered or reused. Required
run-directory, jar initialization, and copy failures return internal failure
without executing an enabled request cookie-less.

## Deliberately unspecified

The skeleton does not define preparation traversal order or transactionality,
goroutine start/completion order within a parallel task set beyond observing
the last completion for mode-2 cookie inheritance, sorting beyond the existing
plan and slice structure, a concurrency limit, URL construction,
concurrent debug-winner selection, or precedence between multiple nonfatal
validation failures. Steps within a file are explicitly serial; no step-level
goroutines are permitted.

## Required implementation and tests

- Production output: `internal/execution/executor.go` retains the implemented
  tree validation and mode-aware directory/file schedulers and replaces the
  `Prepare` and `processFile` TODO bodies with the binding deep-copy and
  execution behavior, including the Debug-specific
  `CurlBuild`/`CurlRaw`/`CurlExecute` branch and the Config-scoped cookie-jar
  ownership, inheritance, and completion tracking above.
- Test output: `internal/execution/executor_test.go` retains the reference tests
  and adds coverage for deep-copy isolation, every phase and mutation, all
  validation-reporting paths, collaborator failures, success/debug reporting
  chosen within the contract, context cancellation, cookie enable/disable and
  re-enable behavior, all three jar-ownership modes, controlled reverse file
  completion, empty directories and roots, run isolation, and jar filesystem
  failures.
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
   It passes the selected owning jar to `runner.Curl`, or an empty path when
   cookies are disabled. A debug step replaces `runner.Curl` with
   cookie-aware `runner.CurlBuild`,
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
13. Every cookie jar exists below the current `Config.TempRunDir` and is
    created even for disabled work. Modes `0`, `1`, and `2` use respectively
    one run jar, one jar per directory, and one jar per steps file; transition
    copies follow only parent-child lineage, mode-2 source selection follows
    the last observed completed step, directories without executed steps
    preserve incoming state, and concurrent workers never share or merge
    writable jars. Separate runs never exchange state, and storage failures
    return internal failure without silently omitting enabled cookies.
