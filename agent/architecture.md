# APIHydra Architecture

## Authority

The binding architecture, API, and package-local behavior live in `skeleton/`.
The shared product contract is [`prd.md`](prd.md), and package guides live in
`specs/`. The guides add rationale, boundaries, unspecified behavior, and
acceptance criteria without reproducing skeleton declarations or method
contracts. This document describes package relationships only.

## Package layout

```text
cmd/cli/                    process composition and exit
internal/domain/            shared Suite/Directory/File/definition/step models
internal/definition/        Loader, Decoder, Resolver
internal/execution/         KeyValueStore, VariableProcessor, Validator, StepRunner
internal/reporting/         human-readable terminal output
pkg/errs/                   contextual errors and exit-code metadata
pkg/runner/                 external-command operations
```

Parallel production model, orchestration, runtime, variable, output, error, or
command-wrapper packages are not part of the reference architecture.

## Dependency boundaries

Shared workflow values belong to `internal/domain`. The current skeleton keeps
package dependencies acyclic and enforces four production boundaries:

- external command execution belongs to `pkg/runner`;
- contextual error composition belongs to `pkg/errs`;
- execution-output writes belong to `internal/reporting`;
- fatal standard-error diagnostics belong to `cmd/cli`.

`cmd/cli` is the composition root and the only reference package that calls
`os.Exit`. `bat` and `BatDiff` are absent.

## Domain lifecycle

The shared declarations and executable repository boundaries are implemented by
[`000-domain-types.md`](specs/000-domain-types.md).

The same `domain.Suite` tree exposes fields for these phases:

```text
WorkDir
  -> Root / Children
  -> Files
  -> DefaultsFile / StepsFiles
  -> DefaultsDefinition / StepsDefinitions
  -> ResolvedDefaults / ResolvedSteps
  -> RuntimeSteps
```

The field schema and provenance helpers are owned by the PRD. Mutation behavior
is defined by the applicable skeleton code and comments.

## Definition services

`internal/definition` contains three stateless services:

- [`Loader`](specs/003-loader-service.md)
- [`Decoder`](specs/004-decoder-service.md)
- [`Resolver`](specs/005-resolver-service.md)

The current CLI composition order is owned by the PRD.

## Execution services

`internal/execution` contains:

- [`KeyValueStore`](specs/006-key-value-store-service.md)
- [`VariableProcessor`](specs/007-variable-processor.md)
- [`Validator`](specs/008-validator-service.md)
- [`StepRunner`](specs/010-step-runner-service.md)

The StepRunner skeleton contract defines preparation scope, execution phase
order, tree validation, and stage scheduling. The execution guides do not
duplicate those orchestration rules.

## Reporter and commands

The [`Reporter`](specs/009-reporter-service.md) skeleton contract defines the
execution-output API and exact working-directory behavior. Reporter never owns
fatal standard-error diagnostics; `cmd/cli` logs those before process exit.

The [`pkg/runner`](specs/001-runner-pkg.md) skeleton contract defines Curl,
JQProject, JQExtract, JQFilter, JQPretty, and GitDiff. Command-line construction
and result-normalization details absent from that contract are not
architectural requirements.

The [`cmd/cli` guide](specs/011-main-app-int-tests.md) completes the production
composition root. The separate [`integration-test guide`](specs/012-integration-tests.md)
owns only black-box verification and fixtures; it introduces no production API.

## Errors and exits

Static classifications originate in the package that declares them.
[`pkg/errs`](specs/002-errs-pkg.md) alone owns contextual construction and attached
codes. The PRD owns the shared meanings of codes `0`, `101`, `102`, and `103`.

## Architecture constraints

1. `skeleton/` remains the binding architecture and API.
2. Shared carriers remain in `internal/domain`.
3. Contextual errors, external commands, execution output, and fatal diagnostic
   logging remain in their owner packages.
4. Package guides do not create parallel APIs or restate skeleton contracts.
5. Behavior absent from the skeleton remains an implementation choice, not a
   product or architecture commitment.
6. Unfinished skeleton bodies use the single `// TODO: implement` plus
   zero-value-return convention; production guides require replacing those
   bodies with working implementations.
