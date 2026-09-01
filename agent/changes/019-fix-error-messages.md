# Actionable error messages and root-suite validation

## Outcome

Make every user-visible fatal `apih` error concise, meaningful, and directly
actionable for coding agents and human users. Fatal diagnostics must identify
the failed operation and the most specific available source location, then link
to the applicable troubleshooting section in the APIHydra user manual.

The selected suite directory must contain a valid root document before `apih`
recursively discovers or decodes definitions below it. If it does not, `apih`
must return the exact root-missing diagnostic defined below instead of exposing
an error from an unrelated nested YAML file.

This change revises the current product contract. The existing PRD deliberately
leaves definition placement and cardinality unspecified, while this change
requires a root document in the selected suite directory and changes fatal CLI
presentation. Implementation therefore requires explicit authorization to
update the affected protected `skeleton/` files, followed by alignment of the
PRD, package specifications, user manual, tests, and production code. This
change file itself does not authorize or modify `AGENTS.md` or `skeleton/`.

## Selected suite directory

Support the two existing invocation forms:

```text
apih
apih <directory>
```

- With no positional argument, the selected suite directory is the current
  working directory.
- With one positional directory argument, the selected suite directory is that
  directory resolved according to the existing CLI path rules.
- Existing help, flag, positional-argument, and invalid-path behavior remains
  unchanged except for the common fatal-diagnostic presentation required by
  this change.

The phrase “suite root” in this change means the selected suite directory, not
an arbitrary nested directory and not the root of a Git repository unless that
directory was selected for the invocation.

## Required root document

Before recursively discovering or decoding definitions in descendants, inspect
only regular `.yaml` and `.yml` files directly inside the selected suite
directory. At least one of those files must have this common envelope:

```yaml
app: apihydra
kind: root
```

The filename is not significant: `root.yaml` is conventional but not required.
A document named `suite.yml`, for example, is a root document when its `app` and
`kind` fields have the required values.

A file does not satisfy the root requirement when it is absent, nested below
the selected directory, not a regular `.yaml` or `.yml` file, malformed, has a
non-string `app` or `kind`, has an `app` value other than `apihydra`, or has a
`kind` value other than `root`.

If no qualifying root document exists, stop before recursive definition
discovery, definition decoding, working-directory reporting, execution, or any
other application output. Return configuration exit code `102`, write nothing
to stdout, and write exactly this to stderr, substituting the final stable
manual reference:

```text
error: root definition missing

please check user manual: <user-manual-ref>#root-definition-missing
```

The diagnostic must end with one newline. No nested YAML file may replace,
precede, or follow it. In particular, running `apih` from the APIHydra repository
root must not expose the deliberately invalid nested integration fixture at
`int-tests/input/scenarios/invalid-base/definition.yaml` when the repository
root itself has no qualifying root document.

Once a qualifying root document is found, continue with recursive discovery
and report later malformed or invalid definitions using their own actionable
diagnostics and provenance. This change does not define selection or rejection
behavior when more than one qualifying root document exists; that policy must
not be invented without a separate contract decision.

## Fatal diagnostic presentation

Use lowercase error text, matching Go and the application's existing error
convention. Do not convert diagnostics to title case or all-uppercase text.

Every fatal CLI error must be presented once on stderr in this shape:

```text
error: <actionable error message>

please check user manual: <user-manual-ref>#<troubleshooting-anchor>
```

The first line must describe the user-visible failure rather than expose a Go
type, an implementation phase, or an unqualified dependency error. Include the
most specific safe context available, such as the selected path, definition
file, YAML field or path, step index, variable or capture name, external tool,
and underlying operating-system cause.

Preserve existing error identities, error-chain inspection, product exit codes,
and stdout/stderr ownership. Presentation must not flatten internal errors in a
way that prevents `errors.Is`, `errors.As`, or `errs.Code` from working. Fatal
diagnostics remain owned by `cmd/apih`; lower packages return classified errors
with context and do not write the manual footer themselves.

Write one footer for the final fatal diagnostic, not one footer for every
wrapped component in its error chain. Do not add the footer to:

- successful output;
- `--help` output;
- nonfatal validation mismatches reported to stdout with exit code `101`;
- individual rows or sections within a grouped validation report; or
- errors returned only to an internal caller and never presented by the CLI.

The footer is part of the final fatal diagnostic. Nothing may be written after
it, preserving the existing final-output ordering guarantee.

## Meaningful-error audit

Audit every fatal error reachable through the command. Do not replace all
errors with generic prose. Preserve useful parser and operating-system details
while adding the context needed to understand and repair the failure.

At minimum, cover these categories:

- invalid arguments and selected paths;
- a missing root document;
- malformed YAML and invalid definition field types or values;
- definition discovery, file-read, decoding, validation, and resolution
  failures;
- missing and duplicate variables or captures, including the affected name;
- invalid jq selectors and invalid expected or actual JSON;
- missing or failed `curl`, `jq`, and `git` commands;
- request construction and execution failures;
- temporary run storage and cookie-jar failures;
- reporting failures and cancellation; and
- genuinely unexpected internal failures.

For a malformed definition, prefer a diagnostic equivalent to:

```text
error: invalid definition in <file>: <yaml-path>: <meaningful cause>

please check user manual: <user-manual-ref>#invalid-yaml-definition
```

Do not describe a present but malformed root document as valid. During the
initial root-presence check it does not satisfy the root requirement, so an
invocation with no other qualifying root document receives the exact
root-missing diagnostic. After a valid root has established the suite, any
other malformed definition is reported as an invalid definition with its file
and YAML provenance.

## User-manual references

Use an absolute, stable user-manual reference suitable for output from an
installed binary. Resolve `<user-manual-ref>` to the canonical published URL
before implementation is complete. When versioned documentation is available,
the installed CLI should link to documentation matching its version rather
than silently linking to incompatible instructions.

Link to a particular troubleshooting subsection, not to the head of the
manual. Give each meaningful error category a stable descriptive anchor, for
example:

- `#root-definition-missing`;
- `#invalid-yaml-definition`;
- `#missing-or-duplicate-variable`;
- `#external-tool-failure`; and
- `#internal-errors`.

Group closely related low-level failures under one useful remediation section;
do not create a heading for every possible library error string. Keep anchors
stable when prose or visible headings change. Every linked section must explain
what the error means, how to identify the affected input, and concrete steps to
repair or further diagnose it.

Add automated documentation checks proving that every manual reference emitted
by production code targets exactly one existing anchor in
`docs/user-manual/apih.md`. A link to the document head or a nonexistent anchor
does not satisfy this requirement.

## Error reproduction registry

Maintain `agent/errors.md` as the reproducibility registry for user-visible
fatal errors. Each entry must identify:

- the error and its troubleshooting anchor;
- required fixtures and prerequisites;
- the exact working directory and command;
- expected stdout, stderr, and exit code;
- whether the entry captures behavior before or after this change; and
- cleanup or platform considerations.

The registry must include the repository-root reproduction that motivated this
change and both supported root-selection forms: `apih` from a selected suite
directory and `apih <directory>` from its parent. Keep commands copyable and do
not rely on network access.

## Required verification

Add unit and black-box integration coverage for every acceptance criterion.
Maintain greater than 95% unit-test coverage for all changed production code
and ensure each acceptance-criteria bullet has at least one meaningful unit
test, as required by `AGENTS.md`.

At minimum, verify:

- `apih` in a directory with no top-level YAML files returns the exact
  root-missing result;
- nested YAML files, including malformed files and nested root documents, do
  not satisfy or interfere with the selected directory's root check;
- `apih` started inside a valid suite and `apih <directory>` selecting that
  same suite both accept an arbitrarily named `.yaml` or `.yml` root document;
- wrong, missing, or non-string `app` and `kind` fields do not satisfy the root
  requirement;
- a qualifying root permits recursive discovery to proceed, after which an
  invalid nested definition reports its file and YAML provenance;
- missing-root stdout is empty, stderr is byte-for-byte exact, exit code is
  `102`, and no output follows the manual footer;
- every other fatal command path has one `error:` prefix, one blank-line-
  separated manual footer, the correct exit code, and a resolvable category
  anchor;
- help, success, and nonfatal validation exit `101` do not receive a fatal
  footer; and
- error wrapping continues to preserve identity and exit-code lookup.

Run `go test ./...`, `go test -race ./...`, `make check`, all black-box
integration tests, and `git diff --check` before completion.

## Acceptance criteria

1. `apih` with no positional argument selects the current directory, and
   `apih <directory>` selects the resolved directory argument; both require a
   qualifying root document directly in that selected directory.
2. A qualifying root is any regular top-level `.yaml` or `.yml` file, regardless
   of filename, whose envelope contains string values `app: apihydra` and
   `kind: root`.
3. When no qualifying root exists, `apih` stops before recursive discovery or
   other output, writes no stdout, returns `102`, and writes exactly the
   specified lowercase root-missing diagnostic and anchored manual footer to
   stderr.
4. Once the root requirement passes, malformed or invalid definitions and all
   other fatal failures report an actionable category, the most specific
   available safe provenance, and the original useful cause without exposing
   implementation-only Go details as the primary message.
5. Every fatal CLI error is presented once with one `error:` prefix, one blank
   line, and one stable category-specific manual sublink; no output follows the
   footer, while help, success, and nonfatal validation output receive no fatal
   footer.
6. Every emitted manual sublink resolves to exactly one remediation section in
   the canonical user manual. Links to the manual head are prohibited when a
   fatal diagnostic is emitted.
7. Existing product exit codes, error identities and chains, package ownership,
   output ordering, and supported CLI argument behavior are preserved except
   where this change explicitly revises root validation and presentation.
8. `agent/errors.md` contains copyable, network-independent reproductions of
   the motivating pre-change failure and the required post-change behavior for
   both suite-selection forms.
9. The protected skeleton, PRD, relevant package specifications, user manual,
   production code, unit tests, integration tests, and documentation tests are
   aligned under explicit protected-path authorization before implementation
   is considered complete.
10. Required unit coverage remains above 95%, every acceptance criterion has
    meaningful test coverage, all repository and integration checks pass, and
    `git diff --check` reports no errors.
