# `internal/definition` Resolver

## Status and ownership

- Binding reference: [`skeleton/internal/definition/resolver.go`](../../skeleton/internal/definition/resolver.go)
- Shared domain and pipeline: [`prd.md`](../prd.md)
- Shared domain types: [`000-domain-types.md`](000-domain-types.md)
- Contextual definition errors: [`002-errs-pkg.md`](002-errs-pkg.md)
- Status: skeleton-aligned implementation guide

## Reference contract

The binding skeleton defines Resolver's stateless API, mutations, and validation
scope. This guide does not reproduce those declarations or method contracts.
The PRD owns Resolver's position in the CLI pipeline.

For every steps file, its directory's resolved defaults flow into
`StepsDefinition.Spec.Defaults`; values declared by the file override inherited
directory values. The resulting effective file defaults are then the base for
each step, whose request values provide the narrowest overrides.

The skeleton does not define field-presence tracking, scalar-zero overlay
rules, map-copy behavior, or header collision semantics. The product fallback
values below define the behavior when timeout and retries remain unset.

It also does not define resolved-step ordering, copy/alias policy, other
implicit request values, or URL normalization. Cookie mode and keys follow the
binding root-to-step inheritance contract; mode defaults to `included`.

## Boundaries

`ValidateStepsDefinitions` is part of the skeleton API, although the current
CLI pipeline does not invoke it.

Its validation rules are not specified by the skeleton and must not duplicate
or contradict Decoder rules. Contextual error construction, when needed, is
defined by `002-errs-pkg.md`; the skeleton declares no Resolver-specific static
errors.

## Required implementation and tests

- Production output: `internal/definition/resolver.go` replaces all placeholders
  with defaults inheritance, resolved-step construction, and the binding
  validation traversal.
- Test output: `internal/definition/resolver_test.go` covers root and nested
  directories, directory-to-file and file-to-step defaults propagation,
  inherited and local values, multiple definitions/steps, provenance
  preservation, independent mutable values, context/error paths, and the
  chosen merge policy.
- Each acceptance criterion is traced to at least one meaningful unit test, and
  Resolver unit-test statement coverage remains greater than 95%.

## Acceptance criteria

1. Names, signatures, and the stateless constructor match the reference.
2. Each method traverses from `suite.Root` and populates or validates only the
   fields named by its reference contract.
3. Resolver does not decode YAML, execute steps, or populate `RuntimeSteps`.
4. Merge, ordering, and validation rules absent from the skeleton remain
   implementation choices rather than requirements.
5. Resolution starts with a 10-second timeout and 3 retries. Root, nested, and
   step request values override those fallbacks when nonzero, so every resolved
   request receives the inherited effective timeout and retry count.
6. Each steps file receives its directory's resolved defaults, overlays values
   from `StepsDefinition.Spec.Defaults`, and uses the resulting effective file
   defaults as the base for every step request. Step request values remain the
   narrowest overrides.
7. Cookie mode and keys follow the same defaults propagation as other request
   defaults, with mode defaulting to `included`.
8. No TODO or zero-value placeholder remains in Resolver production methods;
   its unit tests and `git diff --check` pass.
