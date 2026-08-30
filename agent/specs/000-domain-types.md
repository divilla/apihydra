# `internal/domain` and repository boundaries

## Status and ownership

- Binding domain reference: [`skeleton/internal/domain/suite.go`](../../skeleton/internal/domain/suite.go)
- Reference domain tests: [`skeleton/internal/domain/suite_test.go`](../../skeleton/internal/domain/suite_test.go)
- Binding package-boundary test: [`skeleton/architecture_test.go`](../../skeleton/architecture_test.go)
- Shared product contract: [`prd.md`](../prd.md)
- Status: skeleton-aligned implementation guide

## Reference contract

The binding skeleton and the PRD define the one shared domain vocabulary used
by the definition, execution, reporting, error, and CLI packages. Production
code uses those exact declarations and provenance helpers rather than adapters
or parallel models. The root architecture test makes the repository ownership
boundaries in the PRD executable.

The type declarations in the skeleton are implementation, not illustrative
schema. Their field types, tags, constants, and method behavior are retained
exactly after removing only the `apih/skeleton/` package location.

`DefaultsDefinition.Spec`, `Directory.ResolvedDefaults`,
`StepsDefinition.Spec.Defaults`, and `Step.Request.Defaults` are all
value-typed `domain.Defaults`. Steps do not redeclare `BaseURL`, `BasePath`,
`Headers`, `Timeout`, or `Retries` directly, and none of these defaults
positions uses `*domain.Defaults`.

`Step.Response.ExpectedBody` and `Step.Response.ActualBody` both use
`domain.YAMLString`, retaining its marshaling and unmarshaling behavior for the
expected and runtime body values. `Step.Index` is an `int`, is omitted from
YAML, and is encoded in JSON as `index`. `Step.RawCurl` is runtime-only state:
it retains the complete unredacted statement returned by `runner.CurlRaw` and
is omitted from both YAML and JSON because Reporter emits it separately from
the Debug Step encoding.

## Boundaries

`internal/domain` owns data and provenance access only. It does not discover or
decode files, merge defaults, interpolate variables, execute requests, validate
responses, report output, or compose contextual errors. Those operations remain
with the packages named in the PRD.

## Required implementation and tests

- Production output: `internal/domain/suite.go` implements the complete binding
  domain reference without a competing model hierarchy.
- Repository bootstrap: the existing module and Makefile keep protected
  `skeleton/` packages out of production targets, tolerate an empty production
  package list before this guide is implemented, and begin checking production
  packages as soon as they exist.
- Test outputs: `internal/domain/suite_test.go` covers YAML and JSON schema,
  including steps-file and request `defaults` nesting, zero values, rejected
  incompatible shapes, the absence of the former direct default-related
  request fields, value rather than pointer defaults fields, `YAMLString`
  expected/actual body round trips, the step index schema, and every provenance
  helper, plus `RawCurl` exclusion from YAML and JSON;
  root `architecture_test.go` enforces the package boundaries against
  production paths rather than the protected skeleton tree and preserves the
  canonical skeleton TODO convention.
- Each numbered acceptance criterion is traced to at least one meaningful unit
  or architecture test.
- Executable domain code has greater than 95% statement coverage and its tests
  contribute to repository-wide coverage.

## Deliberately unspecified

The domain package does not add nil-tolerant provenance access, validation,
normalization, copying, or serialization policy beyond the binding reference.
Such behavior belongs to an owning service or requires a prior skeleton change.

## Acceptance criteria

1. Production constants, types, fields, tags, and method signatures match the
   domain reference exactly.
2. YAML decoding and JSON encoding preserve the response expectation/runtime
   schema, keep both response bodies as `YAMLString`, encode `Step.Index` under
   `index` while omitting it from YAML, omit runtime-only `Step.RawCurl` from
   both formats, and reject a list where the scalar `ExpectedStatus` is
   required.
3. `ExpectedStatus` defaults to the zero-value `<any>` sentinel, and provenance
   helpers return the values reached through the binding definition/file/tree
   relationships.
4. Shared workflow state is carried only by `internal/domain` values.
   Directory, steps-file, and step request defaults all use the same
   `domain.Defaults` value type; no parallel fields or `*domain.Defaults`
   carriers are introduced.
5. The root architecture test enforces command, contextual-error,
   execution-output, fatal-diagnostic, and no-`bat` ownership rules across
   production code.
6. Every unfinished skeleton production body uses the exact
   `// TODO: implement` marker, has no placeholder panic, and returns only the
   zero values required to keep the reference compilable (or has no return for
   a void method).
7. `go test ./internal/domain`, the root architecture test, repository coverage,
   `make check`, and `git diff --check` pass.
