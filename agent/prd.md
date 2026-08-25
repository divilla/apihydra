# APIHydra Product Requirements Document

## Authority and status

- Product: APIHydra
- CLI command: `apih`
- Status: skeleton-aligned draft
- Binding reference: `skeleton/`

The checked-in skeleton is the authority for architecture, names, APIs, data,
and behavior. Its declarations, implementations, tests, and contract comments
define package-local behavior once, next to the referenced code. This PRD owns
only the shared product contract identified below. Files in `agent/specs/`
link to the skeleton and add rationale, boundaries, deliberately unspecified
behavior, and acceptance criteria without reproducing the local contract. A
specification may narrow an implementation choice only when the skeleton
already fixes that choice; it may not create behavior missing from the
skeleton.

## Product scope

APIHydra is a Go CLI for discovering YAML definitions in a directory tree,
decoding and validating those definitions, resolving inherited request
defaults, preparing request steps, executing requests, and validating
responses.

When no root, nested, or step value supplies them, request resolution uses a
10-second timeout and 3 retries. Nonzero values at each narrower scope override
the inherited value.

The current reference CLI composes the complete flow: definition loading,
decoding, validation, and resolution; directory-tree validation; runtime-step
preparation and stage planning; and staged execution with response validation
and reporting.

A step with `debug: true` is a terminal breakpoint. After the step finishes,
its runtime state is normalized through jq, printed as jq-palette ANSI-colored
JSON, and the application completes successfully. No later step or stage is
executed, and no success, validation, diagnostic, or other output follows the
debug JSON.

## Package ownership

The repository has these reference packages and no parallel service or model
hierarchy:

| Package | Owned responsibility |
| --- | --- |
| `cmd/cli` | Working-directory selection, service composition, fatal-diagnostic logging, and process exit. |
| `internal/domain` | Shared suite, directory, file, definition, defaults, and step values. |
| `internal/definition` | Directory/file loading, document classification, definition decoding and validation, and resolution. |
| `internal/execution` | The key-value store, variable phases, response validation, step preparation, and staged execution. |
| `internal/reporting` | Human-readable execution output through an injected standard-output writer. |
| `pkg/errs` | Contextual error construction and exit-code metadata. |
| `pkg/runner` | External-command operations. |

The architecture test makes four ownership rules enforceable:

- production command execution is confined to `pkg/runner`;
- production contextual error composition with `fmt.Errorf` or `errors.Join`
  is confined to `pkg/errs`;
- production execution-output writes are confined to `internal/reporting`;
- fatal standard-error diagnostics are confined to `cmd/cli`.

`bat` and a `BatDiff` API are expressly absent.

## Shared domain contract

This section owns the shared data vocabulary used by every package spec. Specs
reference it rather than restating fields.

### Documents

`domain.DocumentKind` has exactly these values:

```go
const (
    KindRoot     DocumentKind = "root"
    KindDefaults DocumentKind = "defaults"
    KindSteps    DocumentKind = "steps"
)
```

`BaseDefinition` contains `App`, `Kind`, and raw `Spec`.
`DefaultsDefinition` and `StepsDefinition` contain `App`, `Kind`, `Metadata`,
`Spec`, and source `File`. `Metadata` contains `Name` and `Labels`.

### Suite tree

One run uses one `domain.Suite` containing `WorkDir` and `Root`. A `Directory`
contains:

- `Stage`, `Path`, `Parent`, and `Children`;
- `Files`, `DefaultsFile`, and `StepsFiles`;
- `DefaultsDefinition` and `StepsDefinitions`;
- `ResolvedDefaults`, `ResolvedSteps`, and `RuntimeSteps`.

A `File` contains `Stage`, `Path`, `Kind`, `Bytes`, and its owning `Directory`.
Directory paths are relative to `Suite.WorkDir`; the root path is `/`.

### Defaults and steps

`Defaults` has exactly `BaseURL`, `BasePath`, `Headers`, `Timeout`, and
`Retries` with the YAML names defined in `skeleton/internal/domain/suite.go`.

`Step` has exactly the reference fields under `Vars`, `Request`, `Response`,
and `Debug`, plus source `Definition` and `Index`. Fields typed as
`YAMLString` remain `YAMLString`; specs must not replace them with parallel
presence or arbitrary-value wrappers.

`Step.Response` carries expected and actual forms of both response values:

- `ExpectedStatus` and `ActualStatus` are `int` values under the YAML and JSON
  names `expected_status` and `actual_status`;
- `ExpectedBody` is a `YAMLString` under `expected_body`, while `ActualBody` is
  a `string` under `actual_body`;
- `ExpectedTypes` is a `map[string][]string` under `expected_types` and declares
  the expected types selected from `ActualBody`.

`ExpectedStatus` is one deterministic HTTP status. Its zero value is the
`<any>` substitute: when `ExpectedStatus == 0`, every `ActualStatus` is valid.
For every non-zero `ExpectedStatus`, `ActualStatus` must equal it.
`runner.Curl` supplies the two actual response values at runtime.
Validator treats response types, status, and body as separate validation
phases. Type validation builds a jq filter from `ExpectedTypes`, evaluates it
against `ActualBody` through `runner.JQFilter`, and represents mismatches with a
non-empty failed string. Body inequality is likewise represented by a non-empty
diff string rather than as the fatal error result of `ValidateBody`.

The JSON names on declarative and runtime step fields match their YAML names.
`Definition` is omitted from JSON and `Index` is encoded as `index`.
`DirectoryStage`, `DirectoryPath`, and `FilePath` derive provenance through
`Step.Definition.File.Directory` exactly as implemented by the skeleton.

## Current reference CLI contract

`skeleton/cmd/cli.run` starts with `os.Getwd()`. If a first positional argument
exists, it joins that argument to the current directory and requires the result
to be a directory. Invalid input returns configuration code `102` and an error
matching CLI-owned `ErrInvalidPath`.

The reference CLI creates one Reporter for `os.Stdout`. `run` reports the
selected working directory, creates
`domain.Suite{WorkDir: workDir}`, and invokes:

1. `Loader.LoadDirectoryStructure`
2. `Loader.LoadDirectoryFiles`
3. `Loader.DecodeBaseDefinitions`
4. `Decoder.DecodeFiles`
5. `Decoder.ValidateDefaultsDefinitions`
6. `Decoder.ValidateStepsDefinitions`
7. `Resolver.ResolveDefaults`
8. `Resolver.ResolveSteps`

After definition resolution, `run` creates one `KeyValueStore`, `Binder`, and
`Validator`, then creates an `Executor` with those
collaborators and the same Reporter used for working-directory output. It then:

1. calls `Executor.ValidateDirectories(suite)` and returns its exit code and
   error if validation fails;
2. calls `Executor.Prepare(suite)`;
3. obtains the execution plan with `Executor.PlanStages(suite)`; and
4. returns `Executor.Execute(ctx, stagesPlan)` unchanged.

When `run` returns an error, `main` writes it to `os.Stderr` with the standard
logger and then calls `os.Exit` with the exact code returned by `run`. Reporter
does not own fatal diagnostics or process exit.

## Exit-code contract

The product reserves:

| Outcome or constant | Code | Meaning |
| --- | ---: | --- |
| Success (no constant) | `0` | Success. |
| `errs.ExitValidation` | `101` | Execution completed with one or more validation failures reported to stdout; the result is not a fatal error. |
| `errs.ExitConfiguration` | `102` | Invocation or configuration failure. |
| `errs.ExitInternal` | `103` | Internal failure. |

The construction and lookup semantics for coded errors are owned by
[`002-errs-pkg.md`](specs/002-errs-pkg.md). Package guides must reference that
contract instead of redefining contextual error formatting.

## Specification guides

Each package guide points to its binding skeleton contract:

| Contract | Implementation guide |
| --- | --- |
| Shared domain and repository boundaries | [`000-domain-types.md`](specs/000-domain-types.md) |
| External-command functions | [`001-runner-pkg.md`](specs/001-runner-pkg.md) |
| Contextual errors | [`002-errs-pkg.md`](specs/002-errs-pkg.md) |
| Loader | [`003-loader-service.md`](specs/003-loader-service.md) |
| Decoder | [`004-decoder-service.md`](specs/004-decoder-service.md) |
| Resolver | [`005-resolver-service.md`](specs/005-resolver-service.md) |
| KeyValueStore | [`006-key-value-store-service.md`](specs/006-key-value-store-service.md) |
| Binder | [`007-binder-service.md`](specs/007-binder-service.md) |
| Validator | [`008-validator-service.md`](specs/008-validator-service.md) |
| Reporter methods and output fixed by the reference implementation | [`009-reporter-service.md`](specs/009-reporter-service.md) |
| Preparation, execution phase order, tree validation, and stage scheduling | [`010-executor-service.md`](specs/010-executor-service.md) |
| CLI composition and process behavior | [`011-main-app.md`](specs/011-main-app.md) |
| Black-box application verification | [`012-integration-tests.md`](specs/012-integration-tests.md) |

The guides do not reproduce public declarations, method contracts, or reference
implementations. A consumer guide references the applicable skeleton contract
and states only rationale, boundaries, unspecified behavior, and acceptance
criteria relevant to its own package.

Guides `000` through `011` form the complete implementation sequence. After
`011` is implemented, `apih` must be a working application rather than a set of
compiling placeholders. Skeleton placeholders consistently use
`// TODO: implement` with zero-value returns (or no return for void methods).
They are work markers, not binding zero-value behavior, and are replaced while
implementing the behavior fixed by the surrounding declarations, comments,
implemented code, and tests. Guide `012` then verifies that application as a
separate black-box integration suite.

## Not specified by the skeleton

The following are not product requirements:

- definition placement/cardinality rules beyond the reference fields and
  service comments;
- deterministic file ordering or symlink/hidden-directory policy;
- presence-sensitive default merging beyond the timeout/retry fallbacks,
  implicit HTTP methods, URL
  normalization, or header canonicalization;
- variable-name grammar within the documented `$var` and `${var}` forms,
  escaping, serialization, replacement precedence, or scope beyond the
  injected Binder store;
- response-type tokens/modifiers, type-filter or projection-selector
  construction, status rules beyond the documented `ExpectedStatus`
  comparison, or body-validation rules beyond the documented normalized
  expected-response comparison;
- exact curl, jq, or Git argument vectors and command-result normalization;
- success or validation-failure layouts not implemented or tested in
  `skeleton/`;
- selection precedence when concurrent directories reach debug steps;
- name/label filters, preflight APIs, events, summaries, or additional CLI
  flags;
- additional packages, services, models, fields, methods, static errors, or
  exit codes.

An implementation choice in one of these areas does not become a contract
until the protected skeleton is explicitly revised and the PRD/spec owner is
updated to match.

## Acceptance criteria

1. Production packages compile against the exact reference names, types, and
   method signatures without adapters that create a competing API.
2. The current CLI follows the eight definition phases in order, then validates
   the directory tree, prepares runtime steps, plans stages, and executes that
   plan in the order fixed by the skeleton.
3. Shared workflow state uses `internal/domain` rather than parallel carriers.
4. Command execution, contextual error composition, execution output, and
   fatal diagnostic logging remain within their owner packages.
5. Every package-local behavior is defined once in the skeleton and referenced
   by the PRD, architecture, and specification guides.
6. No behavior listed as unspecified is asserted by a package guide.
7. `go test ./...`, `go test -race ./...`, and `git diff --check` pass.
