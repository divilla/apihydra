# `cmd/cli` main application

## Status and ownership

- Binding implementation reference: [`skeleton/cmd/cli/main.go`](../../skeleton/cmd/cli/main.go)
- Binding reference tests: [`skeleton/cmd/cli/main_test.go`](../../skeleton/cmd/cli/main_test.go)
- Shared CLI and exit-code contract: [`prd.md`](../prd.md)
- Reporter guide: [`009-reporter-service.md`](009-reporter-service.md)
- Definition guides: [`003-loader-service.md`](003-loader-service.md),
  [`004-decoder-service.md`](004-decoder-service.md), and
  [`005-resolver-service.md`](005-resolver-service.md)
- Execution guide: [`010-step-runner-service.md`](010-step-runner-service.md)
- Status: skeleton-aligned implementation guide

## Reference contract

The binding skeleton implementation and tests define the composition root,
working-directory selection, pipeline order, collaborator construction,
diagnostic handling, and process exit. Production `cmd/cli/main.go` reproduces
that reference except for removing the `apih/skeleton/` import prefix. This
guide does not reproduce the reference code or control flow.

Keeping composition in `cmd/cli` prevents definition, execution, and reporting
packages from acquiring process-level responsibilities. The CLI adds no flags,
filters, debug-selection policy, exported helpers, configuration types, or
alternate application constructors absent from the reference.

## Package tests

Tests remain in `cmd/cli/*_test.go` and follow
`skeleton/cmd/cli/main_test.go`, using production import paths. They verify
invalid-path rejection, successful main-flow completion, and output failure
handling without adding production test seams absent from the skeleton.

## Acceptance criteria

1. `cmd/cli/main.go` is identical to `skeleton/cmd/cli/main.go` after replacing
   `apih/skeleton/` import prefixes with `apih/`.
2. Invalid selected paths return `errs.ExitConfiguration` and no working
   directory output; a successful main flow returns `0` after
   reporting the working directory.
3. Reporter output failures return `errs.ExitInternal` and preserve the writer
   failure.
4. Fatal errors are logged to stderr and retain the exact product exit code
   returned by `run`.
5. After `Resolver.ResolveSteps`, the application constructs the execution
   collaborators, validates the directory tree, prepares runtime steps, plans
   stages, and executes the plan in the exact order represented by the
   skeleton, without adding behavior absent from that reference.
6. `go test ./cmd/cli`, `go test ./...`, `go test -race ./...`, and
   `git diff --check` pass.
