# `internal/definition` Resolver

## Status and ownership

- Binding reference: [`skeleton/internal/definition/resolver.go`](../../skeleton/internal/definition/resolver.go)
- Shared domain and pipeline: [`prd.md`](../prd.md)
- Contextual definition errors: [`002-errs-pkg.md`](002-errs-pkg.md)
- Status: skeleton-aligned implementation guide

## Reference contract

The binding skeleton defines Resolver's stateless API, mutations, and validation
scope. This guide does not reproduce those declarations or method contracts.
The PRD owns Resolver's position in the CLI pipeline.

The skeleton does not define field-presence tracking, scalar-zero overlay
rules, map-copy behavior, or header collision semantics. Those details are not
requirements of this guide.

It also does not define resolved-step ordering, copy/alias policy, implicit
request values, or URL normalization.

## Integration boundary

`ValidateStepsDefinitions` is part of the skeleton API, although the current
CLI pipeline does not invoke it.

Its validation rules are not specified by the skeleton and must not duplicate
or contradict Decoder rules. Contextual error construction, when needed, is
defined by `002-errs-pkg.md`; the skeleton declares no Resolver-specific static
errors.

## Acceptance criteria

1. Names, signatures, and the stateless constructor match the reference.
2. Each method traverses from `suite.Root` and populates or validates only the
   fields named by its reference contract.
3. Resolver does not decode YAML, execute steps, or populate `RuntimeSteps`.
4. Merge, ordering, and validation rules absent from the skeleton remain
   implementation choices rather than requirements.
