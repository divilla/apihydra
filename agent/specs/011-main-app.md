# `cmd/cli` main application

## Status and ownership

- Binding implementation reference: [`skeleton/cmd/cli/main.go`](../../skeleton/cmd/cli/main.go)
- Binding reference tests: [`skeleton/cmd/cli/main_test.go`](../../skeleton/cmd/cli/main_test.go)
- Shared CLI and exit-code contract: [`prd.md`](../prd.md)
- Shared domain types: [`000-domain-types.md`](000-domain-types.md)
- Reporter guide: [`009-reporter-service.md`](009-reporter-service.md)
- Definition guides: [`003-loader-service.md`](003-loader-service.md),
  [`004-decoder-service.md`](004-decoder-service.md), and
  [`005-resolver-service.md`](005-resolver-service.md)
- Execution guide: [`010-executor-service.md`](010-executor-service.md)
- Status: skeleton-aligned implementation guide

## Reference contract

The binding skeleton implementation and tests define the composition root,
working-directory selection, pipeline order, collaborator construction,
diagnostic handling, and process exit. Production `cmd/cli/main.go` reproduces
that reference except for removing the `apih/skeleton/` import prefix. This
guide does not reproduce the reference code or control flow.

Keeping composition in `cmd/cli` prevents definition, execution, and reporting
packages from acquiring process-level responsibilities. CLI uses native
`pflag` behavior to populate the binding `domain.Config`, validates the
parallelism range and single optional directory, creates the per-run cache
directory, detects terminal stdout for Reporter, and injects Config into
Validator and Executor. It adds no filters, debug-selection policy, exported
helpers, configuration carriers, or alternate application constructors absent
from the reference.

Every valid run owns one private `run-*` directory below
`os.UserCacheDir()/apih`. `run` defers best-effort removal of the entire
directory and suppresses every cleanup failure. Abrupt termination may leave a
run directory. Help and invalid invocations do not create one. Executor places
all cookie jars in a namespaced child of this injected `Config.TempRunDir`;
CLI adds no cookie-specific persistence or cleanup path. Removing the run
directory removes its jars together with every other run-local artifact, and a
later run never reuses a directory left by abrupt termination.

## Application package tests

Tests remain in `cmd/cli/*_test.go` and follow
`skeleton/cmd/cli/main_test.go`, using production import paths. They verify
native pflag forms, help, invalid arguments and paths, cache creation and silent
best-effort cleanup, successful main-flow completion, terminal detection,
output failure handling, and final diagnostic ordering without adding
production test seams absent from the skeleton.

Black-box subprocess fixtures and application coverage belong to
[`012-integration-tests.md`](012-integration-tests.md), not this package guide.

## Required implementation and tests

- Production output: `cmd/cli/main.go` reproduces the binding composition root
  with production import paths and no placeholders or alternate API.
- Test output: `cmd/cli/main_test.go` reproduces and extends the binding package
  tests to cover path selection, pipeline failures reachable without a new
  public seam, reporter failures, main logging, and exact exit propagation.
- Each acceptance criterion is traced to at least one meaningful unit or helper
  subprocess test, and CLI package unit-test statement coverage remains greater
  than 95%.

## Acceptance criteria

1. `cmd/cli/main.go` is identical to `skeleton/cmd/cli/main.go` after replacing
   `apih/skeleton/` import prefixes with `apih/`.
2. Native pflag attached, equals, repeated, interspersed, and `--` behavior is
   preserved. `-p`/`--parallelism` defaults to `1`, the last occurrence wins,
   only `0..2` is valid, at most one directory is accepted, help exits `0` on
   stdout without a run, and other invocation errors exit `102` on stderr.
3. Invalid selected paths return `errs.ExitConfiguration` and no working
   directory output; a successful main flow returns `0` after reporting the
   working directory.
4. Every valid run uses a unique private directory below
   `os.UserCacheDir()/apih`, injects it as `Config.TempRunDir`, and attempts to
   remove it on every controlled return. Cleanup failures are silent and do not
   alter results or produce output. Cookie jars remain beneath that directory
   and rely exclusively on this same lifetime and cleanup contract.
5. Reporter output failures return `errs.ExitInternal` and preserve the writer
   failure.
6. Fatal errors are logged to stderr after the final ordered stdout stage
   render, retain the exact product exit code returned by `run`, include the
   available directory/file/step provenance, and are followed by no output.
7. After `Resolver.ResolveSteps`, the application constructs the execution
   collaborators, validates the directory tree, prepares runtime steps, plans
   stages, and executes the plan in the exact order represented by the
   skeleton, without adding behavior absent from that reference.
8. `go test ./cmd/cli`, `go test ./...`, `go test -race ./...`, and
   `git diff --check` pass.
9. With guides `000` through `010` implemented, completing this guide produces
   a runnable `apih` application and enables the `012` black-box suite.
