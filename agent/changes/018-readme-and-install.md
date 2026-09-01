# Repository README and installation

## Outcome

Create a compact repository-root `README.md` that introduces APIHydra and gives
a new user a verified path to an installed `apih` command. The README is a
landing page, not a second user manual.

This is a documentation and installation-verification change. It does not
authorize production behavior or API changes, a second user-manual document,
or changes to `AGENTS.md` or any file below `skeleton/`.

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
- a descriptive repository-relative link to `docs/user-manual/apih.md`; and
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

The command must exit successfully and display `apih` usage. The README may
also mention `go install ./cmd/apih` for installation from an existing source
checkout, but it must not make cloning the repository a prerequisite for the
module-qualified installation.

## Required verification

Add documentation-focused tests at the repository root. Give every acceptance
criterion below meaningful coverage without snapshotting the complete README.
At minimum, automated checks must prove that:

- `README.md` exists, is compact, starts with one H1, and contains the required
  product-and-audience paragraph;
- its manual link is repository-relative and resolves to the one canonical
  manual;
- its module-qualified install command agrees with the module path and
  `cmd/apih` command package in the current checkout;
- it names the Go, `PATH`, `curl`, `jq`, and `git` requirements and the
  `apih --help` verification command; and
- an isolated `GOBIN` installation from the current checkout produces an
  executable `apih` binary whose `--help` command exits successfully and shows
  usage.

The isolated source install is the deterministic acceptance check for the
unmerged checkout. Do not make ordinary tests depend on network access or on
the branch already being published as `@latest`.

Run `go test ./...`, `go test -race ./...`, `make check`, and
`git diff --check` before completion.

## Acceptance criteria

1. The repository root contains one compact `README.md` with one H1 and one
   opening paragraph that accurately identifies the product, command,
   audience, and core purpose.
2. The README links directly to `docs/user-manual/apih.md`, the link resolves,
   and no duplicate user-manual document or detailed parallel reference is
   introduced.
3. The README provides the exact module-qualified `go install` command, the Go
   and `PATH` requirements, accurate roles for `curl`, `jq`, and `git`, and the
   `apih --help` verification command without claiming an unsupported delivery
   channel.
4. Installing `./cmd/apih` into an isolated `GOBIN` creates an executable named
   `apih`; running that executable with `--help` exits `0` and displays usage.
5. Documentation-focused tests trace every criterion above and do not require
   network access or a published version of the current branch.
6. Repository tests, race tests, required checks, and `git diff --check` all
   pass, with no production, public-contract, `AGENTS.md`, or `skeleton/`
   changes.
