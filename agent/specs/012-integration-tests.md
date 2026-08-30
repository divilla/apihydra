# Application integration tests

## Status and ownership

- Production composition guide: [`011-main-app.md`](011-main-app.md)
- Shared product contract: [`prd.md`](../prd.md)
- Static inputs: [`int-tests/input`](../../int-tests/input)
- Test harness: [`int-tests/integration_test.go`](../../int-tests/integration_test.go)
- Status: skeleton-aligned black-box verification guide

## Reference contract

This guide owns no production API or behavior. It verifies the completed
application from guides `000` through `011` by building and running the real
`cmd/cli` process against checked-in YAML directory trees. Assertions are
limited to behavior already owned by the skeleton and PRD: working-directory
selection, the complete pipeline, HTTP execution, response validation,
reporting boundaries, and product exit codes.

## Boundaries

The harness does not import an alternate application constructor, add a test
seam to production, replace Runner commands, or duplicate package unit tests.
It uses a local HTTP server and the real instrumented binary. Fixture setup may
replace only the explicit local-server marker in a temporary copy; checked-in
YAML remains static and unchanged.

## Required implementation and tests

- Test output: `int-tests/integration_test.go` is a build-tagged Go black-box
  suite runnable with `go test -tags=integration ./int-tests -count=1`.
- Fixture output: `int-tests/input/test1/` contains a successful multi-step
  nested suite exercising variables, both placeholder forms,
  request/expected-body interpolation, inheritance, HTTP, response validation,
  and capture; `int-tests/input/test2/` contains nonfatal status and body
  validation mismatches plus a valid sibling definition in the same directory;
  and `int-tests/input/scenarios/` contains auxiliary
  failure and edge-case suites, including a debug step that exposes the
  resolved `request.defaults` value with its 10-second timeout and 3 retries,
  a complete authorization header, the raw Curl statement, and latest runtime
  response values, plus a terminal-error debug scenario; these scenarios must
  not affect either top-level fixture.
- Build output is temporary. The harness builds `./cmd/cli` with Go coverage
  instrumentation over `apih/...`, runs the scenarios with `GOCOVERDIR`, and
  removes artifacts through the test temporary directory lifecycle.
- Coverage means aggregate statement coverage of production packages linked
  into the real CLI binary and executed only by these black-box scenarios.
  Skeleton and integration-harness statements are not part of the denominator.
  The test fails below 90% on Linux, 89% on other Unix systems, or 86% on
  non-Unix systems; the lower platform baselines account only for production
  error branches requiring Unix shell/filesystem facilities or Linux PTY and
  inotify facilities.
- Until `cmd/cli` is created by guide `011`, the tagged suite skips with an
  explicit prerequisite message. Once that path exists, build, scenario, and
  coverage failures are hard failures rather than skips.

## Deliberately unspecified

Except for the exact Debug layout and the actual Curl statement exposed by the
Debug contract, the suite does not make Reporter layout, same-stage ordering,
particular external-command argument choices, or other PRD-unspecified choices
into assertions. Other scenarios check only stable output fragments needed to
identify the selected work directory or a fatal diagnostic.

## Acceptance criteria

1. The integration command builds and executes the production `apih` CLI
   without modifying production code or protected fixtures.
2. `test1` completes with exit code `0`, no fatal stderr diagnostic, expected
   local HTTP traffic, and working-directory output after exercising the full
   runtime phase chain.
3. `test2` reports its status mismatch, completes remaining validation work,
   exits with code `101`, and emits no fatal stderr diagnostic.
4. An invalid selected directory exits with configuration code `102`, emits an
   invalid-path diagnostic on stderr, and does not emit working-directory
   output; selecting a YAML file exercises the same contract.
5. Coverage data from all CLI subprocesses is merged and total linked
   production statement coverage meets the platform baseline specified above;
   missing or malformed coverage data fails the suite.
6. The suite uses only the static trees under `int-tests/input`, temporary
   copies, and a loopback HTTP server; it needs no remote service.
7. An unavailable loopback server produces a fatal nonzero process result and
   stderr diagnostic, with its coverage merged into the same profile.
8. Fixtures exercise directory-to-steps-file-to-individual-step defaults
   propagation using the same `domain.Defaults` structure at all three levels.
   A step with `debug: true` and no configured timeout or retries prints its
   resolved `request.defaults` with timeout `10` and retries `3` as jq-palette
   colored JSON, exits successfully, and leaves its later sibling step
   unexecuted. The Debug dump uses the exact stage/directory/file/Curl/JSON
   layout; exposes complete POSIX-quoted authorization and cookie headers;
   prints a multiline request body as the compact, single-quoted final
   `--data-binary` value; contains no legacy direct request-default fields;
   contains `index` in recursively sorted order; omits runtime-only `raw_curl`;
   and serializes valid request, expected, and actual bodies from their shared
   `domain.YAMLString` fields as structured, jq-prettified JSON while retaining
   invalid or empty bodies as strings. The dump is the final successful process
   output.
9. A debug step whose processing ends in a terminal Curl error prints the
   complete layout with its latest state and raw attempted Curl statement,
   retains the terminal exit code and stderr diagnostic, and executes no later
   step or stage.
10. `make integration-test`, `go test ./...`, `go test -race ./...`, and
   `git diff --check` pass after guides `000` through `011` are implemented.
