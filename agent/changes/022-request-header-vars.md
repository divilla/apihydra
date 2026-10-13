# `internal/execution` request header, query, and path variables

## Status and ownership

- Binding reference: [`skeleton/internal/execution/binder.go`](../../skeleton/internal/execution/binder.go)
- Reference tests: [`skeleton/internal/execution/binder_test.go`](../../skeleton/internal/execution/binder_test.go)
- Shared product contract: [`prd.md`](../prd.md)
- Shared domain types: [`000-domain-types.md`](../specs/000-domain-types.md)
- Variable phases: [`007-binder-service.md`](../specs/007-binder-service.md)
- Runtime orchestration: [`010-executor-service.md`](../specs/010-executor-service.md)
- Application verification: [`012-integration-tests.md`](../specs/012-integration-tests.md)
- Status: skeleton-aligned implementation guide. The Binder reference and
  reference tests were revised with explicit user authorization; production,
  documentation, and global skill alignment are required deliverables below.

## Path interpolation follow-up (2026-10-13)

The user explicitly approved revising the Binder skeleton and reference tests
to extend the query-string interpolation logic to `request.path`, including
`path: "api/v1/${path}"`. Both placeholder forms now resolve literal or
captured values, decode JSON strings, and URL-encode only inserted text.
Path encoding uses `url.QueryEscape` with `+` changed to `%20`, preserving
literal plus signs as `%2B` and variable slashes as `%2F`. Literal slashes
and existing percent escapes stay unchanged. Missing path variables preserve
the entire request and prevent both normal and Debug Curl execution.

The user manual and global `apih-build` skill annotate this extension and
include a path example. Older binaries may support query interpolation
without supporting path interpolation; verify the installed version.

## Reference contract

Binder's existing request interpolation phase resolves `$var` and `${var}`
in the runtime request body, effective request header values, query
string, and path using its injected `KeyValueStore`. Executor already calls this phase after
loading step variables and before building or executing Curl. The public
method retains its existing name and signature; this change introduces no new
shared API or domain fields.

The reference defines replacement according to the destination. Request and
expected-response bodies retain their existing JSON-value and JSON-string
content rules. In header values, query strings, and paths,
both placeholder forms insert text: stored JSON strings, including captured strings, are decoded;
other stored values are used verbatim. Only replacement text inserted into
queries and paths is URL-encoded, using the reference's `url.QueryEscape`
behavior, with `%20` instead of `+` for path spaces.
Literal query delimiters, path slashes, and encoded literal text remain
unchanged.

Replacement is single-pass. Variable loading, write-once storage, captures,
and run-wide store lifetime retain their existing ownership and behavior.
Missing variables retain `ErrVariable`, the underlying `ErrNotFound`, and
the affected variable name. A failed request interpolation leaves its body,
headers, query, and path unchanged.

## Boundaries

Binder owns interpolation of `Step.Request.Defaults.Headers` values,
`Step.Request.Query`, and `Step.Request.Path` during the existing runtime
request phase. Resolver continues to merge defaults without interpolating them; Executor's `Prepare`
continues to deep-copy resolved steps into runtime steps. Header values inherited
from root, directory, or steps-file defaults therefore resolve against the
store when their consuming step runs, without mutating resolved definitions
or another runtime step's headers.

Header names, methods, base URLs, base paths, capture selectors,
and other defaults fields are outside this interpolation contract. Runner
receives the resulting headers, query, and path through its existing API. Normal
execution and Debug use the same interpolated request values.

### Required examples

These header fragments belong inside an existing defaults mapping; they do
not introduce a direct `request.headers` field:

```yaml
headers:
  Authorization: $bearer
```

```yaml
headers:
  Authorization: "Bearer: ${token}"
```

For stored `bearer` equal to `Bearer: 123-some-token` and stored `token` equal
to either literal `123-some-token` or captured JSON `"123-some-token"`, the
corresponding Authorization value is `Bearer: 123-some-token`, with no JSON
quotes around the replacement.

A complete step fragment uses the existing request defaults nesting:

```yaml
request:
  defaults:
    headers:
      Authorization: "Bearer ${token}"
  query: "auth=${token}"
```

For literal or captured token text `a b+c&d=e/?#%`, the Authorization value
is `Bearer a b+c&d=e/?#%` and the query is
`auth=a+b%2Bc%26d%3De%2F%3F%23%25`. A query using `auth=$token` has the same
result. In `fixed=%2F&auth=${token}`, the literal `fixed=%2F&auth=` remains
unchanged; a variable whose text is `%2F` contributes `%252F`.

## Required implementation and tests

- Production output: align `internal/execution/binder.go` with the revised
  reference while retaining the existing public constructor, method
  signatures, and phase order. Preserve existing body replacement behavior.
- Unit-test output: extend `internal/execution/binder_test.go` with the binding
  header/query/path cases. Cover both forms, literal and captured strings, decoded
  JSON escapes, empty values, Unicode, query delimiters, repeated placeholders,
  percent signs, single-pass expansion, non-string stored JSON, missing-variable
  identities, atomic request mutation, and nil or empty headers, query, and path.
- Runtime-test output: extend `internal/execution/executor_test.go` to verify
  effective inherited headers, isolation of resolved and sibling runtime
  steps, and delivery of interpolated headers, query, and path to normal and Debug
  Curl execution. Missing header, query, or path variables must fail before Curl runs.
- Integration output: extend the static CLI fixtures and local-server assertions
  under `int-tests/` to verify that a token captured by an earlier serial step
  reaches a later request's Authorization header, decoded query parameter, and
  encoded path. The `request-path-vars` fixture exercises captured and literal
  values, repeated placeholders, Unicode, and preserved literal percent escapes.
  Exercise both forms and reserved characters using synthetic credentials.
- Documentation output: align `agent/prd.md`, affected guides under
  `agent/specs/`, `README.md` where applicable, and other affected documentation
  with the expanded contract. Remove claims that interpolation is limited to
  bodies. Explain the distinction between body JSON rules and header/query/path
  text rules, URL encoding, runtime defaults interpolation, and missing
  variables without adding unsupported YAML fields or public APIs.
- User-manual output: update `docs/user-manual/apih.md`, including the variables
  and captures phase list, supported-field descriptions, headers/query/path examples,
  and authentication guidance. Show capture-to-Authorization and query reuse,
  both placeholder forms, valid `request.defaults.headers` nesting, and the
  difference between variable URL encoding and literal URL text. Annotate the
  path follow-up and explain `%20` path spaces versus `+` query spaces. Retain
  the existing warning that Debug exposes complete request values.
- Global skill output: update `/home/vito/.codex/skills/apih-build/SKILL.md`,
  specifically its interpolation and credentials guidance. For versions
  implementing this contract, teach literal and captured header/query/path reuse,
  both forms, URL encoding, and a serial authentication producer/consumer
  example. Remove the blanket statement that a captured bearer token cannot
  populate Authorization. Retain installed-version verification and explicitly
  handle older binaries lacking these features, including binaries that
  support query variables but not path variables. Preserve the run-scoped,
  write-once store rules, existing credential safeguards, and the distinction
  between capture reuse and a general secret-input facility. Do not invent
  credential-management commands or imply cookies are stored only in memory.
- Validate documentation and skill examples through corresponding unit cases
  and runnable local CLI fixtures. Each numbered acceptance criterion must
  have at least one meaningful unit test. Component and repository-wide
  unit-test statement coverage must remain greater than 95%; maintain the
  existing integration coverage thresholds.
- Required checks: focused Binder and Executor tests, revised skeleton reference
  tests, `make check`, and `git diff --check`. A global skill edit is a separate
  deliverable outside this repository and must be reported alongside repository
  changes when implementation is completed.

## Deliberately unspecified

This guide does not impose a header traversal order or choose which missing
variable is reported when different headers contain missing variables. It
does not add recursive expansion, a new variable scope, a secret-input API,
or header validation and canonicalization rules. Existing HTTP handling and
credential exposure behavior remain governed by their owning contracts.

## Acceptance criteria

1. Both `$var` and `${var}` resolve in effective request header values using
   literal text or decoded captured JSON strings, without JSON quoting or
   URL encoding; the required Authorization examples work.
2. Both forms resolve in query strings and URL-encode only inserted text.
   Spaces, plus signs, delimiters, percent signs, Unicode, and decoded escapes
   have the reference encoding; literal query text is preserved.
3. Header/query substitution handles empty values, repeated placeholders,
   and stored non-string values as defined by the reference. Replacement
   values are never recursively interpolated.
4. Request and expected-response body JSON substitution remains unchanged;
   header names and unrelated fields remain unchanged, and empty headers/query/path
   need no variables.
5. Missing variables in the body, headers, query, or path retain existing
   error identities and affected name, leave all four request fields unchanged,
   and prevent request execution.
6. Inherited headers resolve at runtime after variable loading or an earlier
   step's capture. Resolved definitions and sibling runtime steps remain
   isolated, and normal and Debug Curl execution receive the same resolved
   header/query/path values.
7. Product documentation and the user manual describe the expanded supported
   fields and destination-specific rules. Their header/query/path and captured-token
   examples use valid schema and agree with the unit and CLI fixture results.
8. The global `apih-build` skill teaches header/query authentication and path
   variable reuse and encoding, preserves version checks and credential safeguards,
   and uses examples verified by the corresponding unit and CLI fixtures.
9. Every criterion has meaningful unit-test coverage, coverage targets remain
   satisfied, revised reference tests and `make check` pass, and
   `git diff --check` reports no whitespace errors.
10. The approved path extension resolves both forms with the query text rules,
    `%20` spaces, encoded reserved characters and Unicode, unchanged literal
    text, empty values, repeated placeholders, and single-pass expansion.
    Binder contract cases and failure cases, Executor normal/Debug and isolation
    cases, and the documented example test cover this criterion; the
    `request-path-vars` CLI fixture verifies the encoded HTTP request.
