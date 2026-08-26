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
  zero values, rejected incompatible shapes, and every provenance helper;
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
2. YAML decoding and JSON encoding preserve the request cookie configuration,
   request and response cookie jars, and response expectation/runtime schema,
   and reject a list where the scalar `ExpectedStatus` is required.
3. `ExpectedStatus` defaults to the zero-value `<any>` sentinel, and provenance
   helpers return the values reached through the binding definition/file/tree
   relationships.
4. Shared workflow state is carried only by `internal/domain` values.
5. The root architecture test enforces command, contextual-error,
   execution-output, fatal-diagnostic, and no-`bat` ownership rules across
   production code.
6. Every unfinished skeleton production body uses the exact
   `// TODO: implement` marker, has no placeholder panic, and returns only the
   zero values required to keep the reference compilable (or has no return for
   a void method).
7. `go test ./internal/domain`, the root architecture test, repository coverage,
   `make check`, and `git diff --check` pass.
