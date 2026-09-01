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

APIHydra is an ultra-fast, agent-first API integration tester. Its `apih` CLI
discovers YAML definitions in a directory tree, decodes and validates those
definitions, resolves inherited request defaults, prepares request steps,
executes requests, and validates responses.

When no directory, steps file, or individual step supplies them, request
resolution uses a 10-second timeout and 3 retries. Nonzero values at each
narrower scope override the inherited value.

Automatic cookies are enabled when `disable_cookies` is absent. That default
is presence-sensitive at directory, steps-file, and individual-step scope:
`nil` inherits, `true` disables automatic cookie handling, and `false`
explicitly enables or re-enables it. This setting controls curl's automatic
cookie engine only and never removes an explicit user-supplied `Cookie` header.

The current reference CLI composes the complete flow: definition loading,
decoding, validation, and resolution; directory-tree validation; runtime-step
preparation and stage planning; and staged execution with response validation
and reporting.

A step with `debug: true` is a terminal breakpoint. Immediately before the
step finishes, or before processing returns a terminal error, Reporter prints
the latest runtime state in the exact Debug layout. The dump contains the
step's stage, directory path, file path, complete unredacted Curl statement,
and prettified jq-palette ANSI-colored JSON encoding. It never hides, masks,
filters, or omits a member or value from those representations, including
authorization headers and other security-sensitive values. For display only,
valid JSON in request, expected-response, and actual-response body strings is
embedded as structured JSON before jq formatting; empty or invalid JSON bodies
remain strings. A successful debug step completes the application
successfully; a terminal error retains its error and exit code after the dump.
No later step or stage is executed. No later execution output follows a
successful Debug dump; after a terminal error dump, fatal stderr diagnostics
remain owned by `cmd/apih`.

## Debug Curl presentation

Runner keeps request execution arguments separate from their Debug
presentation. `CurlBuild` applies these request-body rules:

- an empty body adds no `--data-binary` argument;
- a non-empty body of at most 1,024 Unicode characters is the final
  `--data-binary` argument value; and
- a longer body uses `@-` as that final value.

`CurlExecute` receives the executable and argument list returned by
`CurlBuild` unchanged and receives the original request body on standard input
regardless of which body argument is selected.

On the Debug path, `CurlRaw` derives the displayed statement from that same
executable and argument list without mutating them. It preserves their order,
separates every member with one ASCII space, and adds no trailing newline. If
the final argument is the value following `--data-binary` and is not `@-`, it
attempts `jq --compact-output .` using that value as input. Successful jq output
replaces only the displayed value; invalid JSON or any jq failure retains the
original value. It then wraps every value following `--header` or
`--data-binary` in POSIX single quotes and encodes every embedded single quote
as `'\''`. Every other argument remains untransformed. These presentation
rules never redact header or body data and do not change the request executed
by `CurlExecute`.

The cookie jar selected by Executor is an input to both `Curl` and `CurlBuild`.
A non-empty jar path adds `--cookie <path>` and `--cookie-jar <path>` using the
same run-local path. An empty path adds neither option. Consequently, Debug
shows the exact complete cookie arguments used by the executed request without
changing them.

## Package ownership

The repository has these reference packages and no parallel service or model
hierarchy:

| Package | Owned responsibility |
| --- | --- |
| `cmd/apih` | Working-directory selection, service composition, fatal-diagnostic logging, and process exit. |
| `internal/domain` | Shared suite, directory, file, definition, defaults, and step values. |
| `internal/definition` | Directory/file loading, document classification, definition decoding and validation, and resolution. |
| `internal/execution` | The key-value store, variable phases, response validation, step preparation, staged execution, and run-local cookie-jar ownership and inheritance. |
| `internal/reporting` | Human-readable execution output through an injected standard-output writer. |
| `pkg/errs` | Contextual error construction and exit-code metadata. |
| `pkg/runner` | External-command construction, cookie-aware Curl arguments, unredacted Curl rendering, and execution operations. |

The architecture test makes four ownership rules enforceable:

- production command execution is confined to `pkg/runner`;
- production contextual error composition with `fmt.Errorf` or `errors.Join`
  is confined to `pkg/errs`;
- production execution-output writes are confined to `internal/reporting`;
- fatal standard-error diagnostics are confined to `cmd/apih`.

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

One invocation also uses one `domain.Config`. `Parallelism` is the parsed
execution mode, `Directory` is the optional positional suite directory, and
`TempRunDir` is the private per-run directory created below
`os.UserCacheDir()/apih`. CLI, Validator, Executor, and other runtime consumers
receive this value by dependency injection; package globals and process-wide
temporary-directory overrides are not used.

### Defaults and steps

`Defaults` has exactly `BaseURL`, `BasePath`, `Headers`, `DisableCookies`,
`Timeout`, and `Retries` with the YAML names defined in
`skeleton/internal/domain/suite.go`. `DisableCookies` is a `*bool` with YAML
and JSON name `disable_cookies`; nil is distinct from explicit false.
`DefaultsDefinition.Spec`, `Directory.ResolvedDefaults`,
`StepsDefinition.Spec.Defaults`, and `Step.Request.Defaults` all use this same
`domain.Defaults` type and structure. The steps-file and step forms do not
duplicate the default-related fields directly in their surrounding structs.

Resolution propagates defaults through this precedence chain:

```text
directory defaults -> steps-file defaults -> individual-step defaults
```

Each narrower level overlays the effective defaults inherited from the level
before it. Defaults remain values throughout resolution; the shared domain and
Resolver contracts do not use `*domain.Defaults` pointers.

String and numeric defaults retain their existing overlay rules.
`DisableCookies` alone is pointer-presence-sensitive: nil retains the inherited
pointer value, true replaces it with disabled, and false replaces it with
enabled. When every scope remains nil, automatic cookies are enabled.

`Step` has exactly the reference fields under `Vars`, `Request`, `Response`,
and `Debug`, plus `Index`, runtime-only `RawCurl`, and source `Definition`.
`RawCurl` retains the complete unredacted statement returned by
`runner.CurlRaw` for the latest runtime request. Fields typed as `YAMLString`
remain `YAMLString`; specs must not replace them with parallel presence or
arbitrary-value wrappers.

`Step.Response` carries expected and actual forms of both response values:

- `ExpectedStatus` and `ActualStatus` are `int` values under the YAML and JSON
  names `expected_status` and `actual_status`;
- `ExpectedBody` and `ActualBody` are `YAMLString` values under `expected_body`
  and `actual_body`, respectively;
- `ExpectedTypes` is a `map[string][]string` under `expected_types` and declares
  the expected types selected from `ActualBody`.

`ExpectedStatus` is one deterministic HTTP status. Its zero value is the
`<any>` substitute: when `ExpectedStatus == 0`, every `ActualStatus` is valid.
For every non-zero `ExpectedStatus`, `ActualStatus` must equal it.
`runner.Curl`, or `runner.CurlExecute` on the Debug path, supplies the two
actual response values at runtime.
Validator treats response types, status, and body as separate validation
phases. Type validation builds a jq filter from `ExpectedTypes`, evaluates it
against `ActualBody` through `runner.JQFilter`, and represents mismatches with a
non-empty failed string. Body inequality is likewise represented by a non-empty
diff string rather than as the fatal error result of `ValidateBody`.

The JSON names on declarative and runtime step fields match their YAML names.
Resolved request defaults are nested under `request.defaults` in both YAML and
JSON, using the field names defined by `domain.Defaults`.
`Definition` and `RawCurl` are omitted from YAML and JSON, while `Index` is
omitted from YAML and encoded as `index` in JSON. Reporter includes `index`
when it serializes a step for Debug output and emits `RawCurl` separately under
`curl-command:`. Debug presentation preserves the shared `YAMLString` fields
and projects only valid JSON values in `Request.Body`,
`Response.ExpectedBody`, and `Response.ActualBody` as structured JSON; invalid
or empty values remain strings.
`DirectoryStage`, `DirectoryPath`, and `FilePath` derive provenance through
`Step.Definition.File.Directory` exactly as implemented by the skeleton.

## Current reference CLI contract

The reference CLI uses `pflag` with its native attached, equals, repeated,
interspersed, and `--` parsing behavior. `-p` and `--parallelism` populate
`domain.Config.Parallelism`; the last repeated value wins, the default is `1`,
and values outside `0`, `1`, and `2` are rejected. At most one positional
directory is accepted. Help prints pflag usage to stdout and exits successfully
without starting a run. Other argument failures return configuration code
`102`, write no application stdout, and end with the CLI-owned stderr
diagnostic.

`skeleton/cmd/apih.run` starts with `os.Getwd()`. If `Config.Directory` is
non-empty, it joins that value to the current directory and requires the result
to be a directory. Invalid input returns configuration code `102` and an error
matching CLI-owned `ErrInvalidPath`.

For every valid run, CLI creates a private `run-*` directory below
`os.UserCacheDir()/apih`, assigns it to `Config.TempRunDir`, and defers
best-effort removal of the complete run directory. Cleanup failures are always
suppressed. Operations that need temporary artifacts create namespaced children
below that run directory and never fall back to the shared OS temporary
directory. Abrupt process or machine termination may prevent cleanup.
Cookie jars use a cookie-specific namespace below `Config.TempRunDir`, have no
independent persistence or cleanup lifecycle, and are never discovered, read,
reused, or copied by another invocation, including when abrupt termination
leaves an older run directory behind.

The reference CLI creates one Reporter for `os.Stdout`, explicitly identifying
whether stdout is a terminal. `run` reports the selected working directory,
creates
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
Config-injected `Validator`, then creates a Config-injected `Executor` with
those collaborators and the same Reporter used for working-directory output.
It then:

1. calls `Executor.ValidateDirectories(suite)` and returns its exit code and
   error if validation fails;
2. calls `Executor.Prepare(suite)`;
3. obtains the execution plan with `Executor.PlanStages(suite)`; and
4. returns `Executor.Execute(ctx, stagesPlan)` unchanged.

When `run` returns an error, `main` writes it to `os.Stderr` with the standard
logger and then calls `os.Exit` with the exact code returned by `run`. Reporter
does not own fatal diagnostics or process exit. Executor first finalizes the
active ordered stage render; the provenance-bearing stderr diagnostic is the
last application output, and no later reporting or cleanup diagnostic follows.

## Parallel execution and ordered output

Stages are always executed sequentially in plan order, with a complete barrier
between stages. Mode `0` executes directories, steps files, and their steps
serially. Mode `1` executes all directories in one stage concurrently while
executing each directory's steps files and each file's steps serially. Mode `2`
also executes a directory's steps files concurrently; steps within one file
remain serial. Directory and file task sets are unbounded. All modes retain one
shared, concurrent, write-once KeyValueStore, so mode `0` is the deterministic
choice for cross-file or cross-directory producer/consumer dependencies.

Canonical output order is always stage, directory, steps file, then step,
using the existing plan and slice order. Reporter buffers each steps file
independently. On a terminal, every reporting event clears and redraws only the
active-stage region in canonical order; the working-directory heading and
completed stages remain fixed. On non-terminal output, Reporter emits the
complete stage once at its barrier. On a fatal error, all active work is
canceled and joined, accumulated file output receives one final ordered render,
and the CLI writes the provenance-bearing fatal stderr diagnostic last. Debug
is likewise terminal: accumulated file blocks retain canonical order and the
Debug dump is rendered as the final stdout block.

### Cookie execution state

Every mode-owned jar is created before its owning work executes, even when all
currently assigned steps have cookies disabled, because the unchanged jar may
carry inherited state. Every jar exists below the current `Config.TempRunDir`;
a fresh jar may be a pre-created zero-length file. Enabled requests pass their
owning jar to Runner, while disabled requests pass an empty jar argument and
leave the owning jar unchanged for later re-enabling.

Parallelism selects jar ownership and stage-transition inheritance:

- Mode `0` creates exactly one jar for the run. All requests use it serially,
  and stage transitions create no copies.
- Mode `1` creates exactly one jar per directory. The root starts empty. After
  a stage fully joins and before the next stage starts, each direct child
  receives its own byte-for-byte copy of its parent's final jar. Files within
  one directory use that directory jar serially.
- Mode `2` creates exactly one jar per steps file. Root file jars start empty,
  and steps within a file use its jar serially. After every step finishes,
  including a cookie-disabled step, Executor records that file's jar as the
  directory's latest completed jar. After the stage joins, every steps file in
  each direct child receives its own copy of the parent jar whose step
  completion was observed last. Go scheduling and actual runtime completion
  order intentionally select that source; jar modification timestamps are not
  used.

Directories with no executed steps preserve their incoming state through any
number of later stage transitions. Empty steps-file jars are unchanged copies
of that state and may be used as copy sources. A root with no steps-file jars
creates one additional empty inheritance jar. Copies flow only from a parent
to its direct children; concurrent work never shares a writable jar, sibling
state is never exchanged, and multiple jars are never merged. Run-directory or
required jar create, initialize, and copy failures are internal failures and
never silently downgrade an enabled request to cookie-less execution.

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
- presence-sensitive default merging beyond the defined `DisableCookies`
  overlay and the timeout/retry fallbacks,
  implicit HTTP methods, URL
  normalization, or header canonicalization;
- variable-name grammar within the documented `$var` and `${var}` forms,
  escaping, serialization, replacement precedence, or scope beyond the
  injected Binder store;
- response-type tokens/modifiers, type-filter or projection-selector
  construction, status rules beyond the documented `ExpectedStatus`
  comparison, or body-validation rules beyond the documented normalized
  expected-response comparison;
- curl, jq, or Git argument-vector choices and command-result normalization
  beyond the cookie arguments, request-body placement, and Debug Curl
  presentation rules above;
- success or validation-failure layouts not implemented or tested in
  `skeleton/`;
- selection precedence when concurrent directories or files reach debug steps;
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
   Directory, steps-file, and individual-step defaults use `domain.Defaults`
   values and resolve in that precedence order without `*domain.Defaults`.
4. Command execution, contextual error composition, execution output, and
   fatal diagnostic logging remain within their owner packages.
5. Every package-local behavior is defined once in the skeleton and referenced
   by the PRD, architecture, and specification guides.
6. No behavior listed as unspecified is asserted by a package guide.
7. `go test ./...`, `go test -race ./...`, and `git diff --check` pass.
8. Debug emits the exact provenance/Curl/Step layout, contains complete
   unredacted runtime values, presents the Curl body according to the
   1,024-character/final-argument, jq-compaction, fallback, and POSIX-quoting
   rules above, presents valid request/expected/actual JSON bodies as structured
   jq-prettified values while retaining other bodies as strings, uses the
   `CurlBuild`/`CurlRaw`/`CurlExecute` sequence for debug steps, and preserves
   terminal errors after reporting the latest available state.
9. Native pflag parsing, `domain.Config` injection, private per-run cache
   storage, the three parallelism modes, stage barriers, ordered terminal
   redraws, ordered non-terminal stage commits, terminal error ordering, and
   silent best-effort cleanup match the revised skeleton contract.
10. `disable_cookies` retains nil/true/false presence through decoding and
    resolution. Enabled requests use the same selected run-local path for
    `--cookie` and `--cookie-jar`; disabled requests use neither and leave their
    eagerly created owning jar unchanged.
11. Modes `0`, `1`, and `2` respectively use one run jar, one jar per directory,
    and one jar per steps file. Stage transitions copy only the state selected
    by the mode, mode-2 selection follows the last observed step completion,
    empty directories preserve incoming state, no writable jar is shared or
    merged, and separate invocations never exchange jars.
