# `cmd/cli` main application and integration QA

## Status and ownership

- Binding implementation reference: `skeleton/cmd/cli/main.go`
- Binding reference tests: `skeleton/cmd/cli/main_test.go`
- Shared CLI and exit-code contract: [`prd.md`](../prd.md)
- Reporter contract: [`09-reporter.md`](09-reporter.md)
- Definition collaborators: [`03-loader-service.md`](03-loader-service.md),
  [`04-decoder-service.md`](04-decoder-service.md), and
  [`05-resolver-service.md`](05-resolver-service.md)
- Status: skeleton-aligned specification

This specification owns the production composition root in `cmd/cli`, its
package-local QA tests, the black-box integration test suite under
`int-tests/tests`, and the root `Makefile` wiring needed to run that suite. It
does not expand the public API or the pipeline represented by the skeleton.

## Deliverables and change boundary

Implementation of this specification creates or updates only:

```text
cmd/cli/main.go
cmd/cli/main_test.go
int-tests/tests/**
Makefile
```

`cmd/cli/main_test.go` may be split into additional `*_test.go` files in the
same directory. All integration-test source, helpers, golden data, and fixture
trees must be maintained statically below `int-tests/tests`.

The root `Makefile` is the sole allowed file outside `cmd/**` and
`int-tests/**`; it is allowed only because this specification requires the
integration suite to be part of the repository's standard test entry points.
Touching any other directory or root-level file is out of scope. In particular,
the implementation must not modify `internal/**`, `pkg/**`, `skeleton/**`,
`agent/**`, `scripts/**`, `go.mod`, or `go.sum`.

Adding this specification at `agent/specs/011-main-app-int-tests.md` is the
documentation change that defines the implementation; it does not enlarge the
implementation write boundary above.

## Production contract

`cmd/cli/main.go` must reproduce `skeleton/cmd/cli/main.go` with imports rooted
at the production module (`apih/...`) rather than `apih/skeleton/...`.

The CLI package declares these exact package-level errors:

```go
var ErrInvalidPath = errors.New("invalid path")
var ErrWorkingDirectory = errors.New("working directory error")
```

It retains the reference entry points without adding flags, exported helpers,
configuration types, or alternate application constructors:

```go
func main()

func run(
    ctx context.Context,
    args []string,
    report *reporter.Reporter,
) (int, error)
```

`run` remains package-private. The application name is supplied through
`args[0]`; the reference does not inspect or validate its value.

## Process entry point

`main` must:

1. construct one Reporter backed by `os.Stdout` and a distinct Reporter backed
   by `os.Stderr`;
2. invoke `run(context.Background(), os.Args, outputReport)` exactly once;
3. pass a returned non-nil error to the stderr Reporter's `Error` method; and
4. call `os.Exit` with the exact code returned by `run`.

As in the skeleton, `main` ignores an error returned by the diagnostic
Reporter. No other production package may own process exit.

## Working-directory selection

`run` begins with `os.Getwd()`.

- A working-directory lookup failure returns `errs.ExitInternal` and an error
  built with `ErrWorkingDirectory` that preserves the original failure.
- With no first positional argument, the current working directory is used.
- When `len(args) > 1`, only `args[1]` participates in selection. `run` joins it
  to the current working directory using `filepath.Join`, calls `os.Stat`, and
  requires the result to be a directory.
- A missing, inaccessible, or non-directory selected path returns
  `errs.ExitConfiguration` and an error built with `ErrInvalidPath`. Stat
  failures preserve the original error, and both invalid-path forms include the
  selected path as error context.
- Arguments after `args[1]` are not interpreted by the current reference.

No working-directory line is written before selection and validation succeed.
After successful selection, `run` calls `report.WorkingDirectory(workDir)`. A
reporting failure is returned with `errs.ExitInternal` before any definition
service is invoked.

## Definition pipeline

After reporting the selected directory, `run` derives a cancelable child
context, defers cancellation, and creates exactly one shared suite:

```go
suite := &domain.Suite{WorkDir: workDir}
```

It then constructs the reference stateless collaborators and invokes these
eight phases, in this exact order, on the same context and suite:

1. `Loader.LoadDirectoryStructure`
2. `Loader.LoadDirectoryFiles`
3. `Loader.DecodeBaseDefinitions`
4. `Decoder.DecodeFiles`
5. `Decoder.ValidateDefaultsDefinitions`
6. `Decoder.ValidateStepsDefinitions`
7. `Resolver.ResolveDefaults`
8. `Resolver.ResolveSteps`

The first phase error stops the pipeline and is returned unchanged with
`errs.ExitConfiguration`. Successful completion of all eight phases returns
`errs.ExitSuccess, nil`.

The current reference ends after definition resolution. This specification
must not compose `internal/execution`, execute HTTP requests, add filtering or
debug behavior, or introduce any additional pipeline phase.

## Package QA tests

Tests maintained with the CLI in `cmd/cli/*_test.go` must cover the production
contract without modifying a collaborator API or introducing production test
seams absent from the skeleton. Each required QA case has a stable identifier;
the integration suite must reproduce every identifier as specified in the
integration traceability section. At minimum the package QA suite must prove:

- **QA-01, static classifications:** the two static errors have the reference
  text;
- **QA-02, invalid selection:** a nonexistent selected path and a selected
  regular file both return
  `errs.ExitConfiguration`, match `ErrInvalidPath`, preserve the required
  original error when one exists, and write no stdout;
- **QA-03, successful selection:** a successful run with the current directory
  and a successful run with an
  explicit directory return `errs.ExitSuccess` and report the selected working
  directory;
- **QA-04, output failure:** a Reporter write failure returns
  `errs.ExitInternal`, preserves the writer failure, and prevents pipeline
  work;
- **QA-05, definition failure:** a malformed static definition fixture causes
  the first applicable definition phase to return its error with
  `errs.ExitConfiguration`;
- **QA-06, working-directory failure:** working-directory lookup failure
  returns `errs.ExitInternal` and matches
  `ErrWorkingDirectory`; this case must be isolated in a subprocess so a
  process-wide directory change cannot leak into other tests;
- **QA-07, process routing:** the process entry point routes fatal diagnostics
  to stderr and exits with the code returned by `run`, using a subprocess test
  because `main` calls `os.Exit`; and
- **QA-08, pipeline structure:** a source-level QA test built with Go's parser
  verifies the exact eight-phase call order and absence of
  `internal/execution` composition. This test is required because the order and
  non-composition constraints are not fully observable through the reference
  collaborators' external output.

Tests must not call `t.Parallel` when they affect process-global state. Every
production behavior in this specification's acceptance criteria must have a
corresponding unit or QA assertion, and CLI production-code statement coverage
must remain above the repository-required 95 percent when package tests are
combined.

## Static integration test suite

The black-box suite must be a Go test package rooted at:

```text
int-tests/tests/
```

All integration-test Go sources must be `*_test.go` files. Helpers remain
unexported and test-only so `int-tests` does not become a production API.
Fixtures are checked-in files and directories under
`int-tests/tests/testdata`; the suite must contain multiple independent static
fixture directories, including at least:

```text
int-tests/tests/testdata/success-current/
int-tests/tests/testdata/success-tree/
int-tests/tests/testdata/invalid-yaml/
int-tests/tests/testdata/non-directory/
```

- `success-current` contains a checked-in minimal valid definition tree and
  proves no-argument selection of the process working directory.
- `success-tree` contains checked-in valid defaults and steps YAML across more
  than one directory, proving explicit path selection and full definition
  pipeline traversal.
- `invalid-yaml` contains checked-in malformed YAML and proves configuration
  failure after the working directory has been reported.
- `non-directory` contains a checked-in regular-file path used to prove
  invocation rejection.

Fixture contents must align with the existing domain and definition contracts;
they must not assert validation rules absent from the skeleton. Tests may read
these trees directly or copy them to `t.TempDir` when isolation is needed, but
must never create, rewrite, or delete the maintained fixture source. Generated
fixture programs, runtime-downloaded fixtures, and setup scripts that synthesize
the checked-in YAML are prohibited.

The integration suite builds `./cmd/cli` as an instrumented `apih` binary and
executes it as an external process. The binary and all coverage fragments are
created below a Go test temporary directory, never in a maintained fixture
directory. Test-only subprocess modes may be implemented in checked-in
`*_test.go` files under `int-tests/tests` to establish otherwise unreachable
operating-system conditions. They must still launch the built `apih` binary as
an external process and must not replace it with a copy of `run`.

The behavioral integration scenarios must verify, at minimum:

1. no-argument success when the process working directory is
   `success-current`;
2. explicit-directory success for `success-tree`, including the exact working
   directory report on stdout, empty stderr, and exit code `0`;
3. nonexistent-path failure with empty stdout, a red-wrapped fatal diagnostic
   matching `invalid path` on stderr, and exit code `102`;
4. regular-file path failure with the same output channels and exit-code
   contract;
5. malformed-YAML failure with the working-directory report on stdout, a
   red-wrapped fatal diagnostic on stderr, and exit code `102`; and
6. extra positional arguments do not alter the result selected by `args[1]`.

The suite must additionally reproduce the package-only edge cases:

- A test-only subprocess starts the real CLI from a temporary working directory
  that has been removed after it became the subprocess's current directory. It
  verifies the `working directory error` diagnostic and exit code `103` without
  changing the parent test process's working directory.
- A test-only subprocess connects the real CLI's stdout to an operating-system
  output descriptor that deterministically rejects writes. It uses a malformed
  definition fixture and verifies exit code `103`, proving working-directory
  reporting fails before the definition pipeline could return configuration
  code `102`. This is an external-process reproduction of QA-04, not a mock
  Reporter or a new production injection seam.
- An integration-side Go-parser test independently reads `cmd/cli/main.go` and
  verifies the same eight phase calls, order, and lack of
  `internal/execution` composition as QA-08.

Assertions against operating-system error suffixes must be semantic rather than
byte-exact so tests do not depend on platform-specific wording. Reporter-owned
prefixes, ANSI framing, channel selection, and exit codes are exact contracts.

## QA-to-integration traceability

Every package QA case must have an independently implemented integration case;
running package tests from the integration suite does not count as
reproduction. The required mapping is:

| QA case | Required integration reproduction |
| --- | --- |
| QA-01 | Observe `invalid path` and `working directory error` through fatal CLI diagnostics. |
| QA-02 | Execute the built CLI with nonexistent and regular-file selections. |
| QA-03 | Execute no-argument and explicit-directory success cases. |
| QA-04 | Execute the built CLI with a rejecting stdout descriptor and prove early internal failure. |
| QA-05 | Execute the built CLI against the checked-in malformed-YAML fixture. |
| QA-06 | Execute the built CLI from an isolated removed current directory. |
| QA-07 | Assert the built process's stdout, stderr, and exit status for every success and failure scenario. |
| QA-08 | Run the independent integration-side Go-parser structure assertion. |

The case identifiers must appear in test names or subtest names in both suites
so an automated repository-layout QA test under `int-tests/tests` can verify
that neither side of the mapping is missing. Adding a package QA case requires
adding its integration reproduction in the same change. A waived, skipped, or
platform-excluded integration case does not satisfy this requirement on the
repository's supported CI platform.

## Integration coverage gate

Integration coverage is aggregate statement coverage of the production package
`apih/cmd/cli` across all black-box scenarios. The suite must use Go's native
binary coverage flow: build the CLI with coverage instrumentation, give each
process a `GOCOVERDIR`, merge the resulting coverage data, and calculate the
package percentage with `go tool covdata`. Hand-maintained coverage numbers or
parsing ordinary unit-test coverage as a substitute are not allowed.

The integration suite fails when aggregate `apih/cmd/cli` statement coverage is
below **90.0%**. Coverage fragments and reports are temporary test artifacts and
must not be committed below `int-tests/tests`.

## Makefile integration

The root `Makefile` must expose a documented, phony `integration-test` target
that runs the static suite in `int-tests/tests` and enforces its 90 percent
coverage gate. The target must be included in both the normal `test` workflow
and the default `check` workflow without relying only on `go list` to discover
it indirectly.

The `coverage` workflow must also run or consume the integration coverage gate
so a successful `make coverage` cannot omit CLI integration coverage. Existing
lint, vet, race, tooling-test, benchmark, and version-test behavior must remain
intact. The Makefile change must not install a new tool or require a dependency
outside the Go toolchain already declared by the repository.

## Acceptance criteria

1. `cmd/cli/main.go` matches the names, signatures, errors, working-directory
   behavior, output routing, exit behavior, and eight-phase order in
   `skeleton/cmd/cli/main.go`, using production import paths.
2. `run` returns configuration code for any definition-phase failure, returns
   internal code for working-directory or working-directory-report failures,
   and returns success only after all eight phases complete.
3. The application stops after `Resolver.ResolveSteps`; it introduces no new
   public contract, CLI option, execution composition, or behavior absent from
   the skeleton.
4. Package QA tests exercise every CLI branch that is externally inducible and
   include isolated subprocess and Go-parser checks for process exit,
   working-directory failure, phase order, and composition boundaries.
5. All integration-test source and all immutable scenario fixtures are checked
   in under `int-tests/tests`, with multiple static fixture directories and no
   generated or downloaded fixture source.
6. Every package QA case QA-01 through QA-08 is independently reproduced under
   `int-tests/tests`; black-box cases build and execute the real CLI, while the
   structural phase-order check is independently reproduced with Go's parser.
7. The integration suite enforces at least 90.0 percent aggregate statement
   coverage of `apih/cmd/cli` from instrumented external CLI processes.
8. `integration-test` is an explicit Makefile target and participates in
   `make test`, `make check`, and `make coverage`.
9. `go test ./cmd/cli`, the Makefile integration target, `make check`, and
   `git diff --check` pass, with CLI package test coverage above 95 percent.
10. Implementation changes are limited to `cmd/**`, `int-tests/**`, and the
    root `Makefile`; no other directory or root-level file is touched.
