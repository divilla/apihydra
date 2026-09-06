# Repository README and installation

## Outcome

Create a compact repository-root `README.md` that introduces APIHydra and gives
a new user a verified path to an installed `apih` command. The README is a
landing page, not a second user manual.

This change covers documentation, Makefile workflows, and installation
verification. It does not authorize production behavior or API changes, a
second user-manual document, or changes to `AGENTS.md` or any file below
`skeleton/`.

## Authority and scope

Use the following sources in order:

1. `skeleton/` remains the binding architecture, API, and behavior reference.
2. `agent/prd.md` owns the shared product and CLI contract.
3. `docs/user-manual/apih.md` is the single, self-contained user manual.
4. The module path and required Go version come from `go.mod`; the installable
   command package is `./cmd/apih`.

The README must link to the existing manual at `docs/user-manual/apih.md`.
Do not create a root `user-manual.md`, rename the canonical manual, or duplicate
its detailed CLI, YAML, execution, or troubleshooting reference.

## README contents

Keep the finished README short and directly scannable. It must contain:

- one H1 naming APIHydra and its `apih` command;
- one opening paragraph that begins, “APIHydra is an ultra-fast, agent-first API
  integration tester.” and says that it discovers YAML suites, executes HTTP
  requests, and validates responses;
- descriptive repository-relative links to `docs/user-manual/apih.md` and the
  basic/full example index at `docs/examples/README.md`; and
- an `Installation` section containing the requirements and commands below.

Do not add badges, roadmap material, contributor instructions, an exhaustive
feature list, copied manual sections, or release and platform claims that the
repository does not verify.

## Installation contract

The README must present `go install` as the installation mechanism and include
this copyable command:

```bash
go install github.com/divilla/apihydra/cmd/apih@latest
```

State that the user needs the Go version required by `go.mod`. State that the
installed directory must be on `PATH`: Go uses `GOBIN` when it is set and
otherwise installs to the Go workspace's `bin` directory. Do not promise a
package manager, downloadable release archive, container image, or installer
that this repository does not provide.

List the runtime command dependencies compactly and accurately:

- `curl` executes HTTP requests;
- `jq` performs response and Debug JSON processing; and
- `git` renders a body diff when response-body validation fails.

Finish the installation instructions with this verification command:

```bash
apih --help
```

The command must exit successfully and display `apih` usage. The README also
documents `make install` for an existing source checkout. This phony target
executes exactly `go install ./cmd/apih`, using Go's normal destination rules
above. It has no lint or test prerequisites. Cloning the repository is not a
prerequisite for the module-qualified installation.

## Repository check targets

Keep application checks and repository script checks independently runnable:

- `make check` remains the default and runs lint, vet, race, and black-box
  integration tests, without script tests.
- `make check-scripts` runs all repository script test commands previously
  owned by `tooling-test`.
- `make test` runs only short application tests.
- `make tooling-test` remains a compatibility alias for `make check-scripts`.

Document these workflows in the user manual and `scripts/README.md`. Verify
their separation through the existing Makefile wiring tests. They do not add
application flags or change runtime behavior.

## Required verification

Add documentation-focused tests at the repository root. Give every acceptance
criterion below meaningful coverage without snapshotting the complete README.
At minimum, automated checks must prove that:

- `README.md` exists, is compact, starts with one H1, and contains the required
  product-and-audience paragraph;
- its manual and example-index links are repository-relative and resolve to
  the canonical manual and `docs/examples/README.md`;
- its module-qualified install command agrees with the module path and
  `cmd/apih` command package in the current checkout;
- it names the Go, `PATH`, `curl`, `jq`, and `git` requirements and the
  `apih --help` verification command; and
- an isolated `GOBIN` installation from the current checkout produces an
  executable `apih` binary whose `--help` command exits successfully and shows
  usage; verify `make install` uses the same command package and destination.

The isolated source install is the deterministic acceptance check for the
unmerged checkout. Do not make ordinary tests depend on network access or on
the branch already being published as `@latest`.

Run `go test ./...`, `go test -race ./...`, `make check`, `make check-scripts`,
and `git diff --check` before completion.

## Acceptance criteria

1. The repository root contains one compact `README.md` with one H1 and one
   opening paragraph that accurately identifies the product, command,
   audience, and core purpose.
2. The README links directly to `docs/user-manual/apih.md`, the link resolves,
   and no duplicate user-manual document or detailed parallel reference is
   introduced. It also links to the basic/full examples at
   `docs/examples/README.md`.
3. The README provides the exact module-qualified `go install` command, the Go
   and `PATH` requirements, accurate roles for `curl`, `jq`, and `git`, and the
   `apih --help` verification command without claiming an unsupported delivery
   channel.
4. Installing `./cmd/apih` into an isolated `GOBIN` creates an executable named
   `apih`; running that executable with `--help` exits `0` and displays usage.
   `make install` performs that installation with `go install ./cmd/apih` and
   no lint or test prerequisites.
5. Documentation-focused tests trace every criterion above and do not require
   network access or a published version of the current branch.
6. Repository tests, race tests, both independent check targets, and
   `git diff --check` all pass. `make test` runs short application tests only;
   `make tooling-test` aliases `make check-scripts`. There are no production,
   public API, `AGENTS.md`, or `skeleton/` changes.
