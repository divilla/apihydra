# `pkg/runner`

## Status and ownership

- Binding reference: [`skeleton/pkg/runner/runner.go`](../../skeleton/pkg/runner/runner.go)
- Package-boundary test: [`skeleton/architecture_test.go`](../../skeleton/architecture_test.go)
- Shared product contract: [`prd.md`](../prd.md)
- Status: skeleton-aligned implementation guide

## Reference contract

The binding skeleton defines Runner's exported errors, function signatures,
and operation contracts. This guide does not reproduce those declarations or
comments. The PRD owns the repository-wide rule that external command
execution remains in this package.

`JQProject` and `JQExtract` are distinct operations: projection preserves a
selected object shape for document comparison, while extraction returns the
selected value for capture. `JQFilter` is the generic filtering boundary used
by type validation. `GitDiff` replaces any legacy `BatDiff`/`bat` boundary.

## Deliberately unspecified

The reference functions use canonical TODO bodies beneath their signatures and
comments. Those bodies must be replaced by working command implementations;
their empty results are not production behavior. The reference does not fix:

- the selected executable names or argument vectors, provided that the exact
  Curl command selected for a step is the command exposed by Debug;
- use of stdin, temporary files, or environment variables for non-Curl
  operations; Curl may use them only when the exact displayed command remains
  self-contained and executable as-is after Debug prints it;
- sorting beyond the `JQProject` and `JQPretty` result descriptions, including
  `JQFilter` output formatting;
- treatment of empty inputs, jq streams, or semantic non-zero statuses;
- stdout/stderr inclusion in errors;
- startup, cancellation, and operational exit-code normalization;
- header/query/body encoding or curl behavior.

Those details may be chosen during implementation but cannot be asserted as
product requirements until represented by the skeleton.

## Current implementation behavior

The current command normalization makes these choices within the reference
contract:

- `Curl` omits `--request` when the resolved method is empty. Curl therefore
  selects `GET` when no request body is supplied and `POST` when body data is
  supplied.
- `GitDiff(expected, actual)` keeps the named documents separate but compares
  `actual` as the source with `expected` as the target. Red `-` values are
  actual values to remove or replace; green `+` values are expected values to
  add. Runner explicitly selects terminal palette color 210 (`#ff8787`) for
  coral red and palette color 10 for green and disables moved-line coloring,
  so user Git color configuration cannot change the validation palette. The
  returned output retains those Git ANSI colors but contains only changed value
  lines from every hunk: file headers, hunk headers, and unchanged surrounding
  values are omitted.
- Curl retains one raw command representation for Debug. That representation
  is the exact command executed by `apih`: no argument, header, cookie-jar
  value, body, or other member is hidden, redacted, stringified,
  destringified, or otherwise transformed. It remains directly copy-pastable
  and executable as the same Curl command without depending on transient input
  or artifacts unavailable after Debug prints it.

## Required implementation and tests

- Production output: `pkg/runner/runner.go` implements all six command
  operations and retains the binding exported API and static errors.
- Test output: `pkg/runner/runner_test.go` covers successful results, command
  startup/failure/cancellation, data passed to each operation, and result/error
  normalization chosen within the contract, including implicit curl methods
  and actual-to-expected changed-value-only diff output with fixed palette
  colors 210 and 10. Curl tests also prove that the raw command retained for
  Debug is byte-for-byte the command executed, remains copy-pastable, and does
  not suppress Bearer authorization headers, cookie-jar contents, or any other
  values.
- Root `architecture_test.go` proves that no other production package executes
  external commands and that `bat`/`BatDiff` is absent.
- Each acceptance criterion is traced to a meaningful unit or architecture
  test, and package unit-test statement coverage remains greater than 95%.

## Acceptance criteria

1. Exported names, signatures, and static error text match the reference.
2. Each function stays within the operation boundary defined by the reference.
3. `JQFilter` evaluates its filter against its input, `JQProject` and
   `JQPretty` return comparable normalized JSON, and `GitDiff` preserves the
   color behavior required by the reference contract.
4. No external command is invoked by another production package.
5. Runner does not choose a redacted or separately reconstructed Debug form of
   a Curl command; Debug receives the complete command that Curl executes.
6. No `BatDiff`, `bat` dependency, or command-line choice beyond the enhanced
   Debug contract is invented here.
7. No reference TODO body remains in production, and the package's tests and
   repository ownership test pass.
