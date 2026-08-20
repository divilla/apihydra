# APIHydra Architecture

## Authority

The binding architecture and API live in `skeleton/`. The shared product
contract is [`prd.md`](prd.md), and package-local requirements live
in `specs/`. This document describes package relationships only; it does not
duplicate service behavior.

## Package layout

```text
cmd/cli/                    process composition and exit
internal/domain/            shared Suite/Directory/File/definition/step models
internal/definition/        Loader, Decoder, Resolver
internal/execution/         KeyValueStore, VariableProcessor, Validator, StepRunner
internal/reporter/          human-readable terminal output
pkg/errs/                   contextual errors and exit-code metadata
pkg/runner/                 external-command operations
```

Parallel production model, orchestration, runtime, variable, output, error, or
command-wrapper packages are not part of the reference architecture.

## Dependency boundaries

Shared workflow values belong to `internal/domain`. The current skeleton keeps
package dependencies acyclic and enforces three production boundaries:

- external command execution belongs to `pkg/runner`;
- contextual error composition belongs to `pkg/errs`;
- terminal writes belong to `internal/reporter`.

`cmd/cli` is the composition root and the only reference package that calls
`os.Exit`. `bat` and `BatDiff` are absent.

## Domain lifecycle

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
is owned by the applicable package spec.

## Definition services

`internal/definition` contains three stateless services:

- [`Loader`](specs/03-loader-service.md)
- [`Decoder`](specs/04-decoder-service.md)
- [`Resolver`](specs/05-resolver-service.md)

The current CLI composition order is owned by the PRD.

## Execution services

`internal/execution` contains:

- [`KeyValueStore`](specs/06-key-value-store-service.md)
- [`VariableProcessor`](specs/07-variable-processor.md)
- [`Validator`](specs/08-validator.md)
- [`StepRunner`](specs/10-step-runner.md)

StepRunner owns preparation scope, execution phase order, tree validation, and
stage scheduling. The other execution specs define only their own APIs and do
not duplicate orchestration rules.

## Reporter and commands

[`Reporter`](specs/09-reporter.md) owns the terminal-output API and the exact
working-directory and fatal-diagnostic behavior implemented by the skeleton.
Its other reporting methods remain stubbed and are specified only to the extent
of their reference comments.

[`pkg/runner`](specs/01-runner-pkg.md) owns Curl, JQProject, JQExtract,
JQPretty, and GitDiff. Their signatures and reference comments are binding;
command-line construction and result-normalization details not described by
those comments are not yet architectural requirements.

## Errors and exits

Static classifications originate in the package that declares them.
[`pkg/errs`](specs/02-errs-pkg.md) alone owns contextual construction and attached
codes. The PRD owns the shared meanings of codes `0`, `101`, `102`, and `103`.

## Architecture constraints

1. `skeleton/` remains the binding architecture and API.
2. Shared carriers remain in `internal/domain`.
3. Contextual errors, external commands, and terminal writes remain in their
   owner packages.
4. Package specs do not create parallel APIs or restate another spec's
   requirements.
5. Behavior absent from the skeleton remains an implementation choice, not a
   product or architecture commitment.
