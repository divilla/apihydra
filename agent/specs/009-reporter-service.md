# `internal/reporting` Reporter

## Status and ownership

- Binding reference: [`skeleton/internal/reporting/reporter.go`](../../skeleton/internal/reporting/reporter.go)
- Reference tests: [`skeleton/internal/reporting/reporter_test.go`](../../skeleton/internal/reporting/reporter_test.go)
- Execution-output boundary: [`prd.md`](../prd.md#package-ownership)
- Shared domain types: [`000-domain-types.md`](000-domain-types.md)
- Status: skeleton-aligned implementation guide

## Reference contract

The binding skeleton implementation, comments, and tests define Reporter's API
and output contract. This guide does not reproduce them. Reporter owns
human-readable execution output through its injected writer; fatal diagnostics
and process exit remain in `cmd/cli`. The validation reporting methods mirror
Validator's separate type, status, and body operations without taking ownership
of validation itself.

## Terminal output contract

Reporter uses terminal ANSI colors. In the examples below, `«color:text»`
documents colored text; the notation itself is not printed.

A successful definition file is printed once using its path relative to the
working directory with the YAML extension removed. The check uses terminal
palette color 10, matching added body-diff values. Success is tracked per
definition: when one file in a directory fails, every valid sibling file still
receives its own success line:

```text
Working Directory: /home/vito/go/src/apihydra/work/mch

[«green:✓»] /change/create
```

A definition file with validation failures is headed once by a cross in
terminal palette color 210 (`#ff8787`), matching failed type values and removed
body-diff values. Every failing step is also headed by one palette-210 cross,
followed without indentation by its resolved
`request.defaults.base_path + request.path`, effective method, and one-based
step number in cyan. An omitted method is effectively `POST` when the request
has a body and `GET` otherwise.

Failed type declarations use the original `expected_types` key and complete
value. The label has exactly four leading spaces; each declaration has exactly
eight. Keys use terminal palette color 15 and complete values use terminal
palette color 210 (`#ff8787`), matching removed body-diff values.

Status mismatches use the original response field names with exactly four
leading spaces. `actual_status` is rendered in terminal palette color 210 and
`expected_status` in palette color 10:

```text
    actual_status: «color-210:201»
    expected_status: «color-10:200»
```

Body diffs use the `expected_body` label with four leading spaces and show the
correction from actual to expected. Red `-` values are actual values to remove
or replace; green `+` values are expected values to add. Runner fixes coral red
to terminal palette color 210 (`#ff8787`) and green to palette color 10,
independent of user Git color configuration. Only changed values are printed:
file headers, hunk headers, and unchanged surrounding values are omitted.
Reporter prefixes every changed-value line with exactly eight spaces while
retaining the diff's existing colors.

```text
[«red:✗»] /change/create
[«red:✗»] /api/v1/change/create POST «cyan:step-3»
    expected_types:
        «color-15:.version:» «color-210:[number, null]»
        «color-15:.change_types:» «color-210:[array]»
    expected_body:
        «red:- "version": 0»
        «green:+ "aaa": null»
        «green:+ "version": 3»

```

The file header and each failing-step line are emitted once. When one step has
multiple validation failures, every failure is appended beneath that step.
Exactly one blank line follows the complete validation block for every failing
step. Later steps and files continue executing and reporting after nonfatal
validation failures, and valid sibling definitions are reported before the
directory returns validation exit status.

Debug writes exactly this layout:

```text
stage: <Step.DirectoryStage()>
dir-path: <Step.DirectoryPath()>
file-path: <Step.FilePath()>

curl-command:
<Step.RawCurl>

<step-json>
```

`Step.RawCurl` is written verbatim. Reporter does not hide, mask, filter,
project, quote, escape, stringify, destringify, or otherwise transform it.
Security-sensitive values, including complete authorization headers and
cookie-jar contents when present, remain visible. Runner has already applied
the binding final-data jq compaction/fallback and POSIX header/data quoting to
`Step.RawCurl`; Reporter neither repeats nor reverses those transformations.

`<step-json>` preserves every member and value from the latest runtime `Step`,
then projects exactly `Request.Body`, `Response.ExpectedBody`, and
`Response.ActualBody` for display. Valid JSON body strings are embedded as JSON
values; empty or invalid JSON remains encoded as a string. The result is
normalized into recursively key-sorted pretty JSON through `runner.JQPretty`
and rendered using jq's ANSI token palette: blue keys, green string values,
gray nulls, and jq's scalar and punctuation styles. The JSON contains `index`;
`RawCurl` and `Definition` remain absent according to their JSON tags. Reporter
omits no JSON member or value. Once Reporter successfully writes the complete
block, all later reporting calls are no-ops, including output racing from
another directory.

## Deliberately unspecified

The skeleton does not define precedence when concurrent directories reach
different debug steps. Reporter does not perform validation itself. Canonical
zero-value TODO bodies are not acceptable production implementations.

## Required implementation and tests

- Production output: `internal/reporting/reporter.go` retains the complete
  working-directory implementation and replaces all remaining TODO bodies with
  serialized writes to the injected writer.
- Test output: `internal/reporting/reporter_test.go` covers every method,
  byte-exact working-directory and terminal output, grouped multi-validation
  output, colored expected-type declarations, indented colored diffs,
  the exact Debug provenance/Curl/JSON layout, verbatim pre-rendered Curl
  content, complete unredacted sensitive values, structured valid JSON bodies
  with string fallback for invalid or empty bodies, omission of `RawCurl` and
  `Definition`, nil/failing writers, cancellation policy, and concurrent writes
  under the race detector.
- Root `architecture_test.go` proves that production execution output remains
  in this package and fatal diagnostics remain in `cmd/cli`.
- Each acceptance criterion is traced to a meaningful unit or architecture
  test, and Reporter unit-test statement coverage remains greater than 95%.

## Acceptance criteria

1. Public names, signatures, static errors, and writer injection match the
   reference.
2. Working-directory output is byte-exact and write failures preserve their
   causes.
3. Reporter has no stderr or process-termination responsibility.
4. Successful definition files and grouped validation failures follow the
   terminal output contract above, including path derivation, indentation,
   palette-10 checks, one palette-210 cross per file and failing step, effective
   methods, one-based cyan step numbers, four/eight-space indentation, one
   trailing blank line per failing step, per-definition success in mixed-result
   directories, file/step deduplication, and palette-210 `actual_status` plus
   palette-10 `expected_status` mismatch values.
5. `ValidationTypes` accepts the failed string returned by type validation and
   renders the corresponding original `ExpectedTypes` entries with keys in
   terminal palette color 15 and complete values in palette color 210.
6. `ValidationBody` prefixes every diff line with eight spaces and preserves its
   calculated actual-to-expected colors. Its rendered body block contains only
   changed red and green values, with no diff metadata or unchanged context.
7. `Debug` emits the byte-exact stage, directory-path, file-path,
   `curl-command`, and Step JSON layout. It writes `Step.RawCurl` verbatim and
   complete, preserves every latest Step member and value while projecting
   valid request/expected/actual JSON bodies as structured values and retaining
   invalid or empty bodies as strings, normalizes the result through
   `runner.JQPretty`, emits jq-palette colored JSON with `index` and without
   `RawCurl` or `Definition`, and atomically suppresses every later reporting
   call after the block is successfully written. Reporter does not perform
   validation, schedule debug steps, redact any value, or call `os.Exit`.
8. No TODO or zero-value placeholder remains in a reporting method; package
   tests, race tests, the ownership test, and `git diff --check` pass.
