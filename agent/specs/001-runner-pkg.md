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

`Curl` is the ordinary build-and-execute convenience operation. Debug
execution uses the same phases explicitly: `CurlBuild` produces a complete,
unredacted executable and deterministically ordered argument list;
`CurlRaw` renders that pair, using jq to compact its final data value when
possible and shell-quoting and escaping header and data values; and
`CurlExecute` executes the executable and arguments unchanged while supplying
the request body on standard input. This split exposes data, not `exec.Cmd`;
command construction and execution internals remain encapsulated by Runner.

`Curl` and `CurlBuild` receive the cookie jar selected by Executor. A non-empty
path adds `--cookie <path>` and `--cookie-jar <path>` with the same value; an
empty path adds neither option. Runner does not select, create, copy, persist,
or clean up jars and does not interpret `disable_cookies`.

`JQProject` and `JQExtract` are distinct operations: projection preserves a
selected object shape for document comparison, while extraction returns the
selected value for capture. `JQFilter` is the generic filtering boundary used
by type validation. `GitDiff` replaces any legacy `BatDiff`/`bat` boundary and
receives the CLI-created run directory explicitly.

## Deliberately unspecified

The reference functions use canonical TODO bodies beneath their signatures and
comments. Those bodies must be replaced by working command implementations;
their empty results are not production behavior. Except for the binding Curl
cookie, body-placement, and raw-rendering rules, the reference does not fix:

- the executable and remaining argument-list choices made for Curl, jq, and Git
  operations;
- stdin policy outside Curl's binding request-body contract;
- sorting beyond the `JQProject` and `JQPretty` result descriptions, including
  `JQFilter` output formatting;
- treatment of empty inputs, jq streams, or semantic non-zero statuses;
- stdout/stderr inclusion in errors;
- startup, cancellation, and operational exit-code normalization;
- header/query/body encoding or curl behavior beyond the binding
  construction, raw-rendering, and execution semantics.

Those details may be chosen during implementation but cannot be asserted as
product requirements until represented by the skeleton.

## Current implementation behavior

The current command normalization makes these choices within the reference
contract:

- `Curl` omits `--request` when the resolved method is empty. Curl therefore
  selects `GET` when no request body is supplied and `POST` when body data is
  supplied.
- `CurlBuild` places both automatic-cookie options in the complete argument
  list when its cookie-jar path is non-empty, using the exact same path for
  `--cookie` and `--cookie-jar`. It omits both when the path is empty and never
  removes an explicit user-supplied `Cookie` header.
- `Curl` delegates to `CurlBuild` and then `CurlExecute`. `CurlRaw` renders the
  executable and every argument in order, separated by one ASCII space with no
  trailing newline. Values following `--header` and `--data-binary` are wrapped
  in single quotes, with embedded single quotes shell-escaped; all other values
  remain untransformed. When the final argument is a `--data-binary` value,
  `CurlRaw` runs jq's compact-output filter over it and uses the compact result;
  invalid JSON, jq failures, and the `@-` standard-input marker remain
  unchanged. A non-empty request body of up to 1,024 Unicode characters is also
  used as Curl's data argument so the command shown by Debug contains the
  complete body; larger bodies use Curl's standard-input marker. The selected
  data value is the final Curl argument. In both cases, the original request
  body remains command standard input and is visible in Debug through
  `Step.Request.Body`.
- `GitDiff(tempRunDir, expected, actual)` creates private operation directories
  only below `tempRunDir/git-diff`, removes each operation directory before
  returning, never uses or falls back to the system temporary directory, keeps
  the named documents separate, and compares
  `actual` as the source with `expected` as the target. Red `-` values are
  actual values to remove or replace; green `+` values are expected values to
  add. Runner explicitly selects terminal palette color 210 (`#ff8787`) for
  coral red and palette color 10 for green and disables moved-line coloring,
  so user Git color configuration cannot change the validation palette. The
  returned output retains those Git ANSI colors but contains only changed value
  lines from every hunk: file headers, hunk headers, and unchanged surrounding
  values are omitted.
- Reporter debug serialization uses `JQPretty` for recursively key-sorted JSON
  before rendering that JSON with jq's terminal token palette.

## Required implementation and tests

- Production output: `pkg/runner/runner.go` implements all nine Runner
  operations and retains the binding exported API and static errors.
- Test output: `pkg/runner/runner_test.go` covers successful results, command
  startup/failure/cancellation, data passed to each operation, and result/error
  normalization chosen within the contract. GitDiff coverage proves every
  artifact is namespaced under the injected run directory, concurrent
  operations use distinct private children, operation cleanup is attempted on
  every return, and the shared OS temporary directory is unused. Curl coverage
  proves `Curl` is
  equivalent to `CurlBuild` plus `CurlExecute`, the executable and arguments
  reach execution unchanged, the request body reaches standard input, and
  `CurlBuild` covers empty/non-empty cookie-jar paths, uses one selected path
  for both cookie options, preserves explicit Cookie headers, and uses the
  inclusive 1,024-Unicode-character threshold and final data-value position.
  `CurlRaw` coverage proves valid final data compaction,
  invalid/jq-failure/`@-` fallback, header/data quoting, embedded-single-quote
  encoding, preservation of every other argument, and complete unredacted
  values. Tests also cover implicit curl methods and actual-to-expected
  changed-value-only diff output with fixed palette colors 210 and 10.
- Root `architecture_test.go` proves that no other production package executes
  external commands and that `bat`/`BatDiff` is absent.
- Each acceptance criterion is traced to a meaningful unit or architecture
  test, and package unit-test statement coverage remains greater than 95%.

## Acceptance criteria

1. Exported names, signatures, and static error text match the reference.
2. Each function stays within the operation boundary defined by the reference.
   `CurlBuild`, `CurlRaw`, and `CurlExecute` neither expose `exec.Cmd` nor move
   command execution outside Runner.
3. `JQFilter` evaluates its filter against its input, `JQProject` and
   `JQPretty` return comparable normalized JSON, and `GitDiff` uses only its
   injected run directory while preserving the color behavior required by the
   reference contract.
4. No external command is invoked by another production package.
5. No `BatDiff`, `bat` dependency, additional shell policy, or command-line
   contract beyond the reference-defined compaction and quoting is invented
   here.
6. `CurlBuild` omits data arguments for an empty body; otherwise it places
   `--data-binary` and its value last, using the complete body through 1,024
   Unicode characters inclusive and `@-` above that boundary.
7. A non-empty cookie-jar parameter produces `--cookie` and `--cookie-jar`
   arguments with that same path; an empty parameter produces neither. The
   cookie selection never alters an explicit `Cookie` header, and `Curl`
   remains equivalent to the corresponding `CurlBuild` plus `CurlExecute`.
8. `CurlRaw` preserves all argument members and values, including
   security-sensitive header values, in the exact format fixed by the
   skeleton. It compacts only a valid final data value, retains the original on
   invalid JSON or jq failure, leaves `@-` unchanged, POSIX-quotes only header
   and data values, encodes embedded single quotes as `'\''`, and performs no
   filtering or masking.
9. No reference TODO body remains in production, and the package's tests and
   repository ownership test pass.
