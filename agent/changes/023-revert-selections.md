# Selections with ancestor execution

## Status and ownership

- Binding selection reference: [`selection.go`](../../skeleton/internal/definition/selection.go)
- Binding discovery and resolution references: [`loader.go`](../../skeleton/internal/definition/loader.go), [`resolver.go`](../../skeleton/internal/definition/resolver.go)
- Binding execution and CLI references: [`executor.go`](../../skeleton/internal/execution/executor.go), [`main.go`](../../skeleton/cmd/apih/main.go)
- Binding metadata reference: [`suite.go`](../../skeleton/internal/domain/suite.go)
- Reference tests: [`selection_test.go`](../../skeleton/internal/definition/selection_test.go), [`suite_test.go`](../../skeleton/internal/domain/suite_test.go)
- Shared product contract: [`prd.md`](../prd.md)
- Shared domain types: [`000-domain-types.md`](../specs/000-domain-types.md)
- Related guides: [Loader](../specs/003-loader-service.md), [Resolver](../specs/005-resolver-service.md), [Executor](../specs/010-executor-service.md), [CLI](../specs/011-main-app.md)
- Status: skeleton-aligned implementation guide

## Reference contract

The binding skeleton defines selection scope, nearest-root discovery, source
indices, staged execution, and definition metadata. This guide summarizes those
responsibilities without reproducing declarations or algorithms. It supersedes
change [`020-select-tests.md`](020-select-tests.md) where that change excluded
ancestor steps from execution and validation.

The invocation remains:

```text
apih [flags] [selection ...]
```

Every selection includes all steps in its strict ancestor directories through
the discovered suite root. For a file target, ancestry starts above the file's
containing directory; other files in that containing directory remain excluded
unless another selection includes them. No work above the suite root is included.

| Selection | Executed work |
| --- | --- |
| `directory` | All ancestor steps through the suite root, then all steps in that directory and its descendants. |
| `steps.yaml` | All ancestor steps through the suite root, then all steps in that file; `.yml` also works. |
| `steps.yaml:N` | All ancestor steps through the suite root, then source step `N`; `0` is the first entry in `spec.steps`. |
| `steps.yaml:N-M` | All ancestor steps through the suite root, then source steps `N` through `M`, inclusive; `N-N` selects one step. |

No arguments select the invocation directory under the same ancestry rules.
All targets still share one nearest qualifying root. Selections form a union,
overlaps execute each source step once, and argument order does not schedule
execution. Explicit targets and indices are validated even when ancestor work
or another selection subsumes them. Selected source files are validated in full.
Required ancestor directories include all supported definition files in their
validation scope; unrelated sibling subtrees remain excluded.

Stages execute from root stage `0` downward, with a full join before the next
depth starts. In parallelism `1`, the default, each directory runs its steps
files serially in ascending filename order, and each file runs its steps
serially in source order. Directories in the same stage run concurrently.
Ancestor captures, variables, and cookies reach descendants through the existing
shared-store and cookie-inheritance contracts. Unselected predecessors in a
partially selected file do not execute to satisfy dependencies.

Scenario descriptions use optional `metadata.description`, matching the
existing `metadata` envelope. The field is a string in YAML and JSON and is
empty when omitted. It describes the scenario without changing selection or
execution behavior.

## Boundaries

Loader owns target normalization, root discovery, directory scope, and filename
ordering. Resolver owns explicit source-range validation and the resolved step
union. Executor owns stage barriers, serial file execution within mode `1`,
same-stage directory concurrency, and existing runtime state propagation.
The CLI owns flags, help, diagnostics, and run storage. Domain owns the metadata
schema; documentation does not introduce an alternative `meta` envelope.

The [user manual](../../docs/user-manual/apih.md) explains default scheduling
and suite design: place authentication in root steps when descendants need its
captures; group API areas in directories such as `shop/products`, `admin`,
`user`, and `unauthenticated`; keep a create/get/update/get/delete scenario
together in one steps file and describe it with `metadata.description`.
These are layout recommendations, not required names or inferred dependencies.
Authentication-free areas can use a separate suite root when they must avoid
root authentication work.

## Required implementation and tests

- Production output: `internal/definition/selection.go`, `loader.go`, and
  `resolver.go` implement the binding scope, validation, and ordering;
  `internal/domain/suite.go` implements the metadata schema; CLI and Executor
  retain their binding responsibilities.
- Test output: `internal/definition/selection_test.go` and
  `selection_pipeline_test.go` cover scope and validation;
  `internal/domain/suite_test.go` covers metadata;
  `cmd/apih/selection_test.go` covers order and inherited state;
  `user_manual_test.go` verifies scheduling and layout guidance.
- Black-box output: `int-tests/selection_test.go` verifies actual CLI selections
  under modes `0`, `1`, and `2`, including ancestor requests and invalid targets.
- Align the PRD, owning guides, CLI help, manual, and examples with these contracts.
- Each numbered acceptance criterion is traced to at least one meaningful unit
  test. Component unit-test statement coverage remains greater than 95%; tests
  contribute to repository-wide coverage and assert observable behavior.
- Run `make check`, affected skeleton package tests, and `git diff --check`.

| Criteria | Unit-test trace |
| --- | --- |
| 1–4, 6 | `TestSelectionAncestorsRetainStepsAndExcludeFileSiblings`; range parsing in `TestParseSelectionRejectsInvalidForms`. |
| 5 | `TestSelectionPipelineDefaultScopeStopsAtNearestRoot`. |
| 7 | `TestSelectionPipelineRejectsInvalidAncestorDefinitions`, `TestSelectionPipelinePreservesAncestorsAndValidatesOnlyApplicableFiles`, and `TestRunInvalidLaterSelectionMakesNoRequests`. |
| 8 | `TestRunAncestorCapturesFileOrderAndConcurrentChildren`, `TestRunSelectionUnionDefaultsAndCookiesAcrossModes`, and `TestRunSelectedStepDoesNotLoadSkippedVariables`. |
| 9–10 | `TestRunAncestorCapturesFileOrderAndConcurrentChildren`. |
| 11 | `TestMetadataDescriptionRoundTrips`. |
| 12 | `TestUserManualSelectionLayoutGuidance`. |

## Deliberately unspecified

The skeleton does not define goroutine start or completion order among
same-stage directories, recursive symlink or hidden-directory discovery policy,
or precedence among multiple qualifying roots in one directory. Those choices
must not become additional requirements. Ascending filename order applies
within each directory; mode `1` does not serialize files across concurrent
directories.

## Acceptance criteria

1. A directory selection includes every ancestor's steps through the suite root,
   its own steps, and its descendants, excluding unrelated sibling subtrees.
2. A whole-file selection includes every ancestor's steps and that file's steps,
   excluding other files in the containing directory and its descendants.
3. A single-index selection includes ancestor steps and only the requested source
   step in the selected file, preserving its zero-based index and provenance.
4. An inclusive range includes ancestor steps and exactly the requested source
   positions in the selected file; an equal-bound range selects one step.
5. No arguments use the current directory's selection scope, and automatic
   ancestor execution stops at the nearest qualifying suite root.
6. Duplicate, overlapping, and automatically included ancestor selections execute
   each source step once without weakening explicit target or range validation.
7. Required ancestor definitions and complete selected files are validated before
   any request executes; invalid unrelated branches remain outside that scope.
8. Executed ancestor captures, variables, and cookies reach descendants under
   existing runtime contracts; skipped source predecessors supply none.
9. Parallelism `1` runs each directory's files serially in ascending filename
   order and completes each parent stage before its child stage starts.
10. Parallelism `1` permits same-stage directories to execute concurrently.
11. `metadata.description` decodes and encodes as a string in YAML and JSON,
    empty when omitted, while retaining existing name and label fields.
12. The manual documents default mode `1`, filename ordering, stage barriers,
    same-stage concurrency, root authentication, API-group directories, scenario
    files, and `metadata.description` without making layout advice mandatory.
