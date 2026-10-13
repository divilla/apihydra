# Refactor assessment and implementation

The assessment covers the entire local `master` codebase. The user authorized
implementing the findings one by one with the change-code skill. The tables
below retain the assessed evidence and parity requirements; the implementation
record at the end reports the completed passes on `change/021-refactor`.

The examined baseline is `27abf1e298c227ed97c623aa8595c1a8387096d5`.
At assessment time, `master`, `origin/master`, and `change/021-refactor` pointed
to that commit and the working tree was clean. Remote freshness was not checked.

## Scope and authority

The assessment covered application packages, unit and architecture tests,
integration infrastructure and fixtures, repository automation, and the
documentation and reference contracts that constrain them. User-maintained
`work/` suites and documented examples are inputs, not dead-code candidates;
external services were not contacted. Protected skeleton sources and tests
are reference material, not refactor targets.

[`AGENTS.md`](../../AGENTS.md), [`skeleton/`](../../skeleton/),
[`agent/prd.md`](../prd.md), and [`agent/architecture.md`](../architecture.md)
remain authoritative. Preserve exported declarations, package boundaries,
supported toolchains, dependency versions, and observable behavior. No proposed
application pass needs a new shared/public API or a skeleton edit.

Size is evidence of mixed responsibilities only when accompanied by a concrete
seam below. Test-only callers, reference implementations, platform variants,
and externally callable operations are not proof of dead code.

## Baseline verification

All completed checks passed:

- `go test -short -cover ./cmd/... ./internal/... ./pkg/... .`
- `go vet ./cmd/... ./internal/... ./pkg/... .`
- `go test -race ./cmd/... ./internal/... ./pkg/... .`
- `go test -tags=integration ./int-tests -count=1`
- `STATICCHECK_CACHE=/tmp/apih-refactor-staticcheck staticcheck ./cmd/... ./internal/... ./pkg/... .`
- `goimports -l cmd internal pkg` (no files listed)
- `golint ./cmd/... ./internal/... ./pkg/...` (no diagnostics)
- `make check-scripts`
- `git diff --check`

| Production package | Unit statement coverage |
| --- | ---: |
| `cmd/apih` | 96.3% |
| `internal/definition` | 97.1% |
| `internal/domain` | 100.0% |
| `internal/execution` | 97.4% |
| `internal/reporting` | 96.8% |
| `pkg/errs` | 100.0% |
| `pkg/runner` | 96.6% |

The initial staticcheck invocation could not write its default cache under the
filesystem sandbox. Redirecting only that cache to `/tmp` resolved the failure.
No source failure was found. `make check` itself was not run: its formatting
step writes files. Its lint, vet, race, and integration components were checked
using the commands above. Tests ran on this host; other operating systems and
the Docker toolchain matrix remain unverified.

## Prerequisites and parity inventory

Use this document as the task record and reuse the existing specs, tests, and
`int-tests/input/` fixtures. No replacement PRD or architecture document is
needed. Update implementation-location notes in affected guides when helpers
move, without promoting implementation choices into new public requirements.

Before implementing each pass, bind its acceptance checks to the existing tests
named below. Add characterization only for an uncovered behavior affected by
that pass; every new acceptance criterion must have a unit test. Maintain unit
coverage greater than 95% and retain the platform-specific integration coverage
thresholds. Statement coverage alone does not prove parity.

| Area | Behavior to preserve and existing evidence |
| --- | --- |
| Definitions | Nearest-root selection; lowercase YAML extensions; symlinks before `..`; filesystem spelling and non-listable ancestors; malformed inherited defaults; union order and original source indices; errors and atomic commits. Reuse `loader_test.go`, `kind_test.go`, `selection_test.go`, and `selection_pipeline_test.go`. |
| Resolution and preparation | Timeout/retry precedence; nil/true/false cookie defaults; provenance pointer identity; deep copies and current nil-versus-empty shapes. Reuse `resolver_test.go` and `TestPrepareDeepCopiesAllMutableStepStateAcrossTree`. Resolver currently collapses empty expected-type slices to nil, while Executor preserves non-nil empty slices; do not unify these cloning paths blindly. |
| Execution | Serial stage barriers, mode-specific overlap, unbounded task sets, serial steps, phase ordering, first-fatal result precedence, cancellation and joins, validation continuation, Debug stopping, and per-step cookie completion timing. Reuse `executor_test.go` and `cookie_test.go`. |
| Reporting | Byte-exact ANSI output, one trailing blank line per failed step, original YAML source lines, canonical buffered order, terminal resize/wrap accounting, short writes, cancellation checks around locks, and final Debug output. Reuse `reporter_test.go` and Linux PTY scenarios. |
| Commands and diagnostics | Curl argv/stdin and raw display; jq normalization; Git diff direction, colors, storage, and cleanup; error identities, exit codes, and final manual footer. Reuse Runner, errs, CLI, and black-box integration tests. |
| Tooling | Existing script arguments, messages, process exit statuses, command order, interruption handling, temporary-file cleanup, and Git lease checks. Reuse `make check-scripts`; never exercise publication or merge scenarios against the real remote as a test. |

## Iteration 1: Dead code

**No actionable findings.** Staticcheck reported no unused-code diagnostics in
the checked Go packages. That is supporting evidence, not an exhaustive proof
about runtime or external consumers.

Keep `Resolver.ValidateStepsDefinitions`: the
[`Resolver guide`](../specs/005-resolver-service.md) explicitly retains it even
though the CLI does not invoke it. Keep public error identities such as
`cmd/apih.ErrInvalidPath`, public Runner operations, and skeleton-backed
validation phases. The private `executeStages` adapter has numerous test
callers and a reference implementation; it is not unreachable code.
Platform-specific integration implementations and helper-process test entry
points also have intentional callers.

## Iteration 2: Duplicated paths

| Scope and evidence | Current behavior to preserve | Structural improvement | Validation proving parity |
| --- | --- | --- | --- |
| **D1 — verified:** `internal/definition/loader.go:292,361`: `isYAMLFile` and `isRootYAMLFile` have identical bodies and active callers in loading, root discovery, and selection. Two names imply a policy difference that does not exist. | Exact lowercase `.yaml` and `.yml` matching; regular-file checks stay at their existing callers. Root qualification and selected-file validation remain distinct. | Retain one package-private extension predicate and route existing callers through it. Do not consolidate the surrounding discovery/envelope logic. | `TestLoadDirectoryFilesLoadsSupportedRegularFilesWithSourceLinks`, `TestLoadDirectoryStructureRequiresQualifyingTopLevelRoot`, `TestParseSelectionRejectsInvalidForms`, and the definition package suite. Confirm the existing cases cover `.yaml`, `.yml`, uppercase suffixes, unrelated suffixes, directories, and symlinks at each affected boundary; add only missing cases. |
| **D2 — verified duplication, conditional pass:** `scripts/commit-user.pl` and `scripts/commit-agent.pl` are 46 lines each and differ only in usage text and default commit message. | Both entry paths, their distinct usage/default messages, custom argument bytes, repository-root resolution from any working directory, detached-HEAD rejection, and `add -A` → commit → push order with fail-fast behavior. | Prefer making the user entry point a small argument-validation wrapper delegating to the existing agent entry point with an explicit message. Keep both commands. Do not add a new shared module API merely for this pass. | Missing prerequisite: an isolated test harness for both entry points. Test default/custom/empty/excess arguments, spaces/quotes/newlines in messages, detached HEAD, invocation outside the repository, and failures at add/commit/push; compare argv, diagnostics, statuses, and which later commands did not run. Then `make check-scripts`. |

Other verified duplication is deferred: `codex-code-spec.pl` and
`codex-review-loop.pl` duplicate shell quoting, subprocess capture, temporary-root
selection, and process monitoring. Their input forwarding, session-event
handling, diagnostics, return values, and lifecycle state differ. Likewise,
`change-merge.pl` and `change-merge-direct.pl` share Git mechanics but differ in
PR-base discovery and validation. These need a separate tooling design and
parity matrix before a shared helper extraction; do not merge whole workflows.

## Iteration 3: Oversized modules

Perform file moves as separate passes, without changing function bodies or
introducing packages. Keep public entry points in their existing files.

| Scope and evidence | Current behavior to preserve | Structural improvement | Validation proving parity |
| --- | --- | --- | --- |
| **O1 — verified:** `internal/execution/executor.go` is 920 lines. Cookie storage begins at `cookieJars` on line 730 and mixes filesystem lifecycle with scheduling and per-step orchestration. Preparation/cloning and stage scheduling form further existing seams. | Cookie ownership, locking, stage preparation before workers, byte-for-byte lineage copies, latest-completed-step selection including disabled steps, empty-directory inheritance, and fatal storage errors. Scheduling and clone semantics remain unchanged. | First move `cookieJars`, its helpers, and its error to `cookies.go`. In a separate pass, move preparation helpers to `prepare.go` and scheduling/tree helpers to focused files in `internal/execution`. Preserve receiver types, callbacks, state ownership, and initialization. | `TestCookieJarsModeZeroUsesOneRunJar`, `TestCookieJarsModeOneCopiesParentStateIntoIndependentDirectoryJars`, `TestCookieJarsModeTwoUsesLatestCompletionAndPreservesEmptyLineage`, `TestCookieStagePreparationFinishesBeforeWorkersStart`, all Executor tests, and `go test -race ./internal/execution`. No new behavior test needed for pure moves. |
| **O2 — verified:** `internal/reporting/reporter.go` is 747 lines and combines stage transactions, debug DTOs/JSON coloring, validation formatting/source lookup, and terminal geometry/redraws. | Exact output bytes, Debug projection and stop behavior, existing locking and context-check order, writer-error behavior, and terminal/non-terminal transaction semantics. | Move debug projection/color helpers and private DTOs to `debug.go`; move terminal geometry/redraw helpers to `terminal.go`; move validation formatting/source lookup to `validation.go`. Keep Reporter state and public methods together. Each extraction is its own diff. | `TestDebugProjectsValidJSONBodiesWithoutTransformingOtherValues`, `TestDebugPreservesInvalidAndEmptyBodiesAsStrings`, `TestReporterGroupsEveryValidationForOneStepUnderOneFailureHeader`, `TestNonTerminalStageCommitsOnceInCanonicalOrder`, `TestTerminalRedrawRefreshesDimensionsAndReflowsPreviousContent`, and the reporting race suite; run integration PTY scenarios too. |
| **O3 — verified:** `internal/definition/selection.go` is 419 lines. `inheritedDefinition` and `flowMappingFields` occupy lines 262–369 and implement malformed-YAML envelope recovery inside a path/selection module. | Recognition of required inherited root/default documents even with malformed bodies, flow/block forms, multiline scalars, comments/directives, and CR/LF/CRLF. Recovery must not broaden execution or full-validation scope. | Move these private recovery helpers together into `envelope.go` in the same package. Leave selection parsing, canonicalization, and step filtering unchanged. | `TestSelectionScopeAndMalformedDefaults`, `TestSelectionPipelinePreservesAncestorsAndValidatesOnlyApplicableFiles`, `TestSelectionPipelineRejectsInvalidDefaultsAndNonStepsTargets`, and the whole definition suite. Preserve existing recovery cases; no parser rewrite. |
| **O4 — candidate, lower priority:** `int-tests/integration_test.go` is 1,184 lines. `TestApplicationScenariosAndCoverage` contains server setup and hundreds of lines of scenario orchestration; command, environment, fixture, and coverage helpers follow it. Platform and selection scenarios already live in separate files. | Shared server lifetime, request observations, fixture substitution, scenario order, final aggregate coverage assertion, build tags, and platform behavior. | Start with moving fixture and command/coverage helpers into focused `_test.go` files with identical `integration` tags. Consider scenario extraction only where it yields a clear responsibility without a large parameter carrier or a new shared abstraction. | `go test -tags=integration ./int-tests -count=1` and its existing `TestTotalCoverage`, fixture, permission, and CLI-build assertions. Preserve platform tags; compile applicable non-host variants when implementing. No coverage-threshold reductions. |

## Iteration 4: Stale abstractions

| Scope and evidence | Current behavior to preserve | Structural improvement | Validation proving parity |
| --- | --- | --- | --- |
| **S1 — verified:** `pkg/runner/runner.go:260–289`: `executeInDir` is the only caller of private `runCommand` and always supplies a `bytes.Buffer`; its configurable stdout writer has no other consumer. | Direct command execution, cwd, unchanged args/stdin, separately captured stdout/stderr, partial stdout on failure, startup exit `-1`, actual process exit status, and context cancellation taking precedence over the process error. | Fold command setup/run/status handling into `executeInDir`, keeping stdout/stderr buffers there. Remove only `runCommand` and its unused `io` import. Keep public Curl/jq/Git wrappers and their distinct error classification. | `TestCommandStartupFailureAndCancellation`, `TestCurlCommandFailure`, `TestCurlKeepsStatusMetadataSeparateFromResponseBody`, `TestJQOperationsPassFiltersAndNormalizeResults`, `TestGitDiffNormalizesEqualAndFailedCommands`, and the Runner race suite. Before editing, check coverage of partial output with failure and nonempty cwd; add characterization if missing. |

Keep the stateless Loader/Decoder/Resolver service APIs, the separate jq
operations, and Reporter's existing synchronization. Debug DTOs repeat parts of
the domain schema, but replacing them would change serialization machinery and
needs stronger shape/number-preservation evidence; move them in O2 without
redesigning them.

## Iteration 5: Legacy patterns

**No actionable findings warrant a dedicated migration.** Production code
already uses contexts, modern `os`/`io` operations, generics, and `slices`.
The `interface{}` YAML callback spelling matches the reference signatures.
Keep dependencies and Go support unchanged. Replacing synchronization patterns,
YAML parsing, pflag, jq, or curl would increase compatibility scope without a
demonstrated refactor benefit.

## Iteration 6: Compact code

| Scope and evidence | Current behavior to preserve | Structural improvement | Validation proving parity |
| --- | --- | --- | --- |
| **C1 — verified:** `internal/execution/executor.go:156–166` forwards both results of `executeStagesPrepared` through an unnecessary error branch; `dirsValidator.validateRoot` similarly forwards `validateChildren` after establishing root state. | Exact returned code and error object, including validation status with nil error; root registration occurs before child validation; no extra calls or reordered side effects. | Use direct returns of the existing calls. After O1, apply at the functions' new locations where appropriate. | `TestExecuteReturnsSuccessWithoutStages`, `TestExecuteStagesContinuesAfterValidationStatus`, `TestExecuteStagesFatalErrorTakesPrecedenceOverValidationStatus`, `TestValidateDirectoriesRejectsInvalidTrees`, and Executor package tests. Existing tests suffice unless a concrete missing result case is found. |

Do not compress `processFile` into a generic pipeline or defer cookie completion
at file scope. Its repeated completion/error paths encode phase order and
completion timing. Also keep explicit clone logic where it expresses different
nil/empty policies across resolution and preparation.

## Sequencing, completion, and separate work

Use the six categories in order. D1 is independent. D2 is conditional on its
new characterization harness and can be omitted from the first implementation
batch. O1–O3 are independent file extractions; O4 is lower priority. S1 is
independent of the file moves. C1 follows O1 to avoid editing functions while
their location is changing. Empty categories require no manufactured edits.

For each implemented pass: rerun its named checks, inspect the diff, resolve
regressions, and record completed/deferred work. At each category boundary,
replace a scratch checkpoint outside tracked project files with scope,
invariants, changed files, actual results, remaining queue, and exact next
action. Include its path in the handoff.

After implementation, run `make check`, `make check-scripts` when tooling is
touched, unit coverage, and `git diff --check`. Use a writable staticcheck cache
if needed. Review the combined diff against the protected contracts and keep
`AGENTS.md` and `skeleton/` untouched. Publish implementation only from
`change/021-refactor`; merging into `master` is separate work.

No framework, dependency, or public-API migration is needed for the recommended
application passes. Removing reference-declared APIs, creating a shared
cross-package clone utility, replacing the Executor/Reporter validation-context
protocol with a new API, or introducing shared tooling APIs would be separately
scoped design work under the repository's skeleton-first authorization rules.
This plan does not authorize protected-path changes.

## Implementation record

All named implementation passes were completed sequentially. The dead-code and
legacy-pattern categories required no edits. The separately identified shared
tooling and API design candidates remain deferred as described above.

| Pass | Completed change | Acceptance evidence |
| --- | --- | --- |
| D1 | Removed duplicate `isYAMLFile`; loading reuses the skeleton-named `isRootYAMLFile`. | Existing loading/root/selection tests pass, covering extension rules and filesystem boundaries; definition race coverage 97.1%. |
| D2 | `commit-user.pl` validates its arguments and delegates an explicit message to the existing agent script. Added `scripts/commit_test.pl` to the tooling target. | All 72 isolated assertions pass before and after the change: default/custom/empty/excess arguments, message bytes, cwd, detached/empty branches, exact diagnostics/statuses, command order, and fail-fast behavior. |
| O1 | Extracted cookie storage, preparation, directory validation/planning, and stage scheduling into `cookies.go`, `prepare.go`, `directories.go`, and `stages.go`. | Executor race tests pass; declaration/body comparison confirmed pure moves before C1. |
| O2 | Extracted private debug projection/coloring, terminal geometry/redraw, and validation formatting/source lookup into `debug.go`, `terminal.go`, and `validation.go`. | Reporter race tests pass at 96.8% coverage; integration including PTY scenarios passes; declaration/body comparison confirmed pure moves. |
| O3 | Moved malformed-envelope recovery to `internal/definition/envelope.go`. | Definition race tests pass at 97.1% coverage; declaration/body comparison confirmed pure moves. |
| O4 | Moved integration CLI/environment, fixture, and coverage helpers to focused tagged test files. Kept the shared server and ordered scenario sequence together. | Integration suite passes; all 62 declarations compare unchanged; test binaries compile for Darwin and Windows. |
| S1 | Folded private `runCommand` into its sole caller `executeInDir`. | `TestExecuteInDirPreservesPartialOutputAndWorkingDirectoryOnFailure` passed before/after; Runner race suite passes at 96.5% coverage. |
| C1 | Simplified forwarding in `Executor.Execute` and `dirsValidator.validateRoot` to direct returns. | Existing success, validation-status, fatal-error, and invalid-tree tests pass under the race detector. |

Implementation-location notes are aligned in guides 003, 009, 010, and 012.
Public declarations, protected references, dependencies, and fixture contents
are unchanged. The scratch checkpoint is `/tmp/apih-refactor-checkpoint.md`.

Final verification passed:

- `STATICCHECK_CACHE=/tmp/apih-refactor-staticcheck make check` (formatting,
  staticcheck, golint, vet, race tests, and integration tests).
- `make check-scripts`, including the 72-assertion commit harness.
- `go test -short -cover ./cmd/... ./internal/... ./pkg/... .`: CLI 95.3%,
  definition 97.1%, domain 100%, execution 97.4%, reporting 96.8%, errs 100%,
  and Runner 96.5%; every production package remains above 95%.
- Darwin/amd64 and Windows/amd64 integration test binaries compile with
  `go test -tags=integration -c`; execution on those platforms remains unverified.
- Declaration/body comparison across affected packages confirms only the D1,
  S1, and C1 function changes plus the new Runner test. Reporting and integration
  declarations are identical to the implementation baseline.
- `git diff --check` and explicit checks that `AGENTS.md`, `skeleton/`,
  dependencies, and fixture contents remain unchanged.

No API, dependency, or framework migration was performed.
