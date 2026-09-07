# Select directories, steps files, and individual steps

## Outcome

Allow one `apih` invocation to select directories, complete steps files, and
individual steps or inclusive step ranges within one suite. Discover the suite
root by searching upward from each selection, so users can run a subset from
inside a suite without copying its root definition or inherited defaults.

Selections restrict execution. Ancestor defaults still apply, but unselected
steps never run automatically to supply variables, captures, or cookies.

## Authority and scope

The user has approved revising the existing contract to support this change,
including ancestor root discovery, multiple selections within one suite,
zero-based step selectors, and the revised missing-root message. Preserve the
existing missing-root manual link.

This authorized revision supersedes the former CLI contract. Update the binding skeleton
contracts first, then align the PRD, specifications, documentation, tests,
mocks, scaffolding, and production code before the change is complete.
Relevant references include:

- `skeleton/cmd/apih/main.go` and its tests: argument parsing, root selection,
  invocation validation, composition, and diagnostics.
- `skeleton/internal/domain/config.go`, `suite.go`, and their tests: invocation
  inputs, suite-root identity, selected work, and source provenance.
- `skeleton/internal/definition/loader.go`, `decoder.go`, `resolver.go`, and
  their tests: discovery scope, inherited defaults, validation, and resolution.
- `skeleton/internal/execution/executor.go` and its tests: selected runtime
  steps, original indices, stage order, and cookie inheritance.

Declare any required shared or public API in the skeleton before introducing
it elsewhere. Preserve existing package ownership; this change specifies
behavior without inventing a second configuration carrier or service API.
`AGENTS.md` is outside this change's scope.

This change replaces the single-directory argument restriction, the requirement
that the selected directory itself contain a root document, and the old
`root defaults file missing` message. Existing flag parsing, parallelism modes,
execution phases, and fatal-diagnostic presentation continue to apply except
where explicitly revised below.

## Invocation forms

```text
apih [flags] [selection ...]
```

Each positional selection is one of:

| Selection | Executed work |
| --- | --- |
| `<directory>` | All steps in that directory and its descendants. |
| `<steps-file>` | All steps in that file. |
| `<steps-file>:N` | Only step `N` in that file. |
| `<steps-file>:N-M` | Only steps `N` through `M`, inclusive. |

With no positional arguments, select the current working directory and apply
the same root-discovery and subtree-selection rules as an explicit directory.
Running `apih` from an inner directory therefore selects that subtree, not the
entire discovered suite.

Resolve relative selection paths from the invocation's current working
directory. Accept absolute paths. Normalize equivalent paths before comparing
suite roots or combining overlapping selections. Use filesystem spelling for
all path components so alternate casing on case-insensitive filesystems
identifies the same work, while distinct case-sensitive names remain distinct.
If parent listing is denied, retain the accessible component spelling instead
of rejecting the target. Accessible suites require no additional ancestor
listing permission for canonicalization.
Parse step suffixes only in the final path component; colons in Unix parent directory names remain part
of the path.

A directory target must exist and be a directory. A file target must exist and
be a regular supported `.yaml` or `.yml` steps-definition file with
`app: apihydra` and `kind: steps`. A root or defaults document is not a valid
explicit steps-file target. A step suffix is valid only on a steps-file target.

Preserve native `pflag` behavior, including attached, equals, repeated,
interspersed, and `--` forms. Preserve `-p`/`--parallelism`, its default of `1`,
its accepted values `0`, `1`, and `2`, and last-repeated-value precedence. Help
continues to exit successfully without starting a run.

## Root discovery

For each selection:

1. Start at the selected directory, or at the containing directory of a
   selected steps file. Strip a step suffix before resolving its file path.
2. Look for a qualifying root document directly in that directory.
3. If none exists, inspect its parent and continue upward through the
   filesystem root.
4. Use the nearest directory containing a qualifying root document. Stop the
   upward search there; more distant roots do not participate in inheritance.
5. If no qualifying root exists, fail the invocation with the missing-root
   diagnostic below.

A qualifying root is a regular lowercase `.yaml` or `.yml` file whose top-level
YAML envelope decodes with the exact string values:

```yaml
app: apihydra
kind: root
```

The filename is arbitrary. Malformed YAML, other app values, invalid envelope
field types, and roots found only in descendants do not qualify. Qualification
identifies a candidate root; that root's full definition must still pass
validation before execution.

All explicit selections must resolve to the same suite-root directory. Reject
an invocation that selects targets with different nearest roots, even when one
root is an ancestor of another. Resolve and check every selection before
executing any work.

The discovered root anchors `Suite.WorkDir`, the directory tree, relative
source paths, and working-directory reporting. Retain the ancestor directory
chain from that root to each selected target so defaults, parent links, stage
numbers, and cookie inheritance retain their existing meaning.

This change does not introduce a policy for multiple qualifying root documents
in the same directory or change recursive symlink and hidden-directory policy.
Selection targets resolve symlinks before parent components such as `..`,
root discovery, and scope matching;
aliases and real paths therefore identify the same selected work.

## Combining selections and retaining order

Combine all selections as a union. Execute each selected source step at most
once, regardless of repeated arguments, overlapping ranges, a file selected
both wholly and partially, or a directory overlapping a selected descendant.
The identity of a selected step is its normalized source file and its original
zero-based index.

Argument order does not reorder execution. Retain the existing suite stage,
directory, file, and within-file step order after filtering. Existing
parallelism modes still govern concurrency; selection does not impose a new
completion order on concurrent work.

Keep original `Step.Index` values and definition provenance. For example,
selecting `file.yaml:1-3` produces steps with indices `1`, `2`, and `3`, not
`0`, `1`, and `2`. Diagnostics and debug output must identify the original
`spec.steps[index]` location.

## Step selectors

`N` and `M` are non-negative decimal integers referring to positions in the
source file's `spec.steps` sequence. Index `0` is the first step. A range is
inclusive and requires `N <= M`; `N-N` selects one step.

Reject malformed selectors, negative indices, reversed ranges, integer
overflow, and any index outside the source sequence. Open-ended ranges and
comma-separated index lists are not supported by these forms; multiple
positional arguments can express separate selections from one file.

Validate every selector independently before forming the union. An invalid
selector remains an error even when another argument selects the whole file or
an overlapping directory. Do not silently clamp a range, ignore a bad argument,
or execute the valid portion of an invalid invocation.

## Defaults, validation, and runtime dependencies

For each selected steps file, load and validate the suite root and every
applicable defaults definition along its ancestor chain. Apply the existing
root, directory, steps-file, and individual-step defaults precedence,
including presence-sensitive cookie settings.

Definition validation covers selected steps files in full, including when only
some of their steps will execute, and the root/defaults definitions they
inherit. Invalid unselected steps files and unrelated directory branches must
not block execution. Discovery may inspect envelopes to locate roots and
applicable definitions; that inspection must not extend full validation to
unselected files or reinstate the former whole-directory kind-validation gate.

A directory selection includes all its supported definition files and its
subtree, so malformed or invalid definitions within that selected scope still
fail normally. Ancestor directories retained only for defaults inheritance do
not implicitly select their steps files or sibling subtrees.

Preserve app-scoped kind validation and definition provenance within the
applicable validation scope. A malformed or invalid required root/defaults
file or selected steps file remains a configuration error. Readable top-level
envelopes in flow mappings and multiline scalars still identify required
defaults when their bodies are malformed. Unrelated files
must not replace a missing-root diagnostic.

Execute only the selected steps. Do not run skipped predecessors, ancestor
steps, or other files to initialize variables, populate captures, establish
cookies, or satisfy inferred dependencies. If a selected step needs a value
that no executed step supplies, preserve the existing runtime error behavior.

Preserve run-local cookie ownership and stage-transition inheritance for all
parallelism modes. Ancestor directories with no executed steps preserve their
incoming state under the existing empty-directory contract; skipped requests
produce no cookie state. Preserve existing debug-stop and terminal-failure
behavior among the steps that are selected.

## Errors and output

Invalid paths, invalid file targets, malformed or out-of-bounds selectors, and
selections spanning multiple roots fail the entire invocation with
configuration exit code `102` before any selected step executes. Diagnostics
must identify the offending selection and explain the failure. Preserve the
existing category-specific manual footer and error-identity conventions;
record any required identity changes in the skeleton first.

When no qualifying root exists for a selection, return the existing
`ErrRootDefinitionMissing` identity and configuration exit code `102`, write
nothing to stdout, create no run-local cache directory, and write exactly:

```text
error: kind: root - file missing

please check user manual: https://github.com/divilla/apihydra/blob/master/docs/user-manual/apih.md#root-defaults-file-missing
```

The diagnostic ends with one newline. Preserve that manual URL and anchor.
Do not append a path to the exact missing-root line. Required file, selector,
root-consistency, and definition checks must finish before executing requests;
a later invalid argument must never allow earlier selections to run first.

## Examples

Assume `suite/` contains a qualifying root document, `suite/api/` has no nearer
root, and the example files contain enough steps for their selectors.

| Invocation | Result |
| --- | --- |
| `apih suite` | Run all steps in the suite tree. |
| `apih suite/api` | Discover `suite/`; run only `api/` and its descendants, inheriting suite defaults. |
| `apih` from `suite/api/` | Same selection as explicitly passing that inner directory. |
| `apih suite/api/users.yaml` | Run every step in `users.yaml`. |
| `apih suite/api/users.yaml:0` | Run only the first step. |
| `apih suite/api/users.yaml:1-3` | Skip step `0`; run steps `1`, `2`, and `3` with their original indices. |
| `apih suite/api/users.yaml:0 suite/api/orders.yaml:1-3 suite/health` | Run the union within one suite, in existing suite order. |
| `apih suite/api suite/api/users.yaml:0` | Run the directory selection; its first users step runs only once. |
| `apih suite/api/users.yaml:3-1` | Reject the reversed range with exit `102` before execution. |
| `apih suite-a suite-b` with different roots | Reject the invocation with exit `102` before execution. |
| `apih orphan/` with no root in it or any ancestor | Emit the exact missing-root diagnostic above and exit `102`. |

## Required verification

Implement and maintain at least one meaningful unit test for every individual
acceptance criterion. Keep unit-test coverage greater than 95%. Add black-box
integration cases that exercise argument parsing through actual selection,
root discovery, validation, request execution, and final output.

Use fixtures that record executed requests to prove that skipped steps,
overlaps, invalid later arguments, and unrelated invalid files have the required
behavior. Exercise selection under all three parallelism modes, including
ancestor directories retained for defaults and cookie inheritance without any
selected requests of their own.

Update `agent/prd.md`, affected guides under `agent/specs/`,
`docs/user-manual/apih.md`, CLI help, and relevant examples to describe the new
contract. Replace assertions that require one positional directory, a root
directly in the selected directory, or the former missing-root text. Historical
change `019` is superseded by this change for those behaviors.

Run `make check`, the affected skeleton package tests, and `git diff --check`.
Verify the canonical manual link still resolves to the documented
`root-defaults-file-missing` troubleshooting section.

## Acceptance criteria

1. No-argument and explicit-directory invocations search upward and choose the
   nearest qualifying root while executing only the selected subtree.
2. Root qualification uses the required regular-file extensions and exact
   `app`/`kind` envelope; a file target starts discovery in its containing
   directory, and descendant roots do not satisfy an ancestor's search.
3. A missing root produces exit `102`, empty stdout, no run cache, and the exact
   new diagnostic with the unchanged manual URL and anchor.
4. Complete steps-file selections execute only that file, and invalid paths,
   non-steps files, and directory targets with step suffixes fail before any
   request executes.
5. Single-index and inclusive-range selections use zero-based source positions,
   preserve original indices and provenance, and execute only selected steps.
6. Malformed, negative, overflowing, reversed, and out-of-bounds selectors
   reject the whole invocation with exit `102`, including when another valid
   argument subsumes the invalid selection.
7. Multiple arguments must resolve to one nearest root; different roots reject
   the whole invocation before any request executes.
8. Duplicate and overlapping selections execute each source step once in
   existing suite order, independently of argument order and within the
   concurrency rules of each parallelism mode.
9. Selected files inherit and validate all applicable root and ancestor
   defaults with existing precedence. Required invalid definitions fail, while
   invalid unselected files and unrelated branches do not block execution.
10. A partial-file selection validates that selected file in full, including
    unselected step definitions, before any selected request executes.
11. Skipped steps never supply variables, captures, or cookies; selected steps
    retain existing runtime failures, cookie ownership/inheritance, debug-stop,
    and terminal-failure behavior.
12. Suite paths, stages, and working-directory reporting remain anchored at the
    discovered root, including when all selected work is in descendants.
13. Native flag forms, parallelism validation, help, exit-code conventions, and
    single category-specific fatal footers retain their existing behavior.
14. Binding skeleton contracts are updated first and all affected artifacts
    align with them. Every acceptance criterion has unit-test coverage,
    unit-test coverage exceeds 95%, and required verification passes.
