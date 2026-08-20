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

The reference functions are stubs except for their signatures and comments.
It therefore does not fix:

- executable names or argument vectors;
- use of stdin, temporary files, or environment variables;
- sorting beyond the `JQProject` and `JQPretty` result descriptions, including
  `JQFilter` output formatting;
- treatment of empty inputs, jq streams, or semantic non-zero statuses;
- stdout/stderr inclusion in errors;
- startup, cancellation, and operational exit-code normalization;
- header/query/body encoding or curl behavior.

Those details may be chosen during implementation but cannot be asserted as
product requirements until represented by the skeleton.

## Acceptance criteria

1. Exported names, signatures, and static error text match the reference.
2. Each function stays within the operation boundary defined by the reference.
3. `JQFilter` evaluates its filter against its input, `JQProject` and
   `JQPretty` return comparable normalized JSON, and `GitDiff` preserves the
   color behavior required by the reference contract.
4. No external command is invoked by another production package.
5. No `BatDiff`, `bat` dependency, shell policy, or command-line contract is
   invented here.
