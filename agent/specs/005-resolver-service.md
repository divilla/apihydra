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

The skeleton does not define field-presence tracking, scalar-zero overlay
rules, map-copy behavior, or header collision semantics. The product fallback
values below define the behavior when timeout and retries remain unset.

It also does not define resolved-step ordering, header-map copy/alias policy,
other implicit request values, or URL normalization. It does define the
defaults carrier at every level: directory resolution,
`StepsDefinition.Spec.Defaults`, and `Step.Request.Defaults` all use
`domain.Defaults` values rather than duplicated step fields or
`*domain.Defaults` pointers.

Resolver propagates effective defaults from directory defaults to each steps
file's defaults and then to each individual step's request defaults. At each
stage the narrower `domain.Defaults` value overlays the inherited value before
the result is propagated to the next stage.

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
  directories, directory-to-steps-file-to-step precedence, inherited and local
  values, multiple definitions/steps, provenance preservation, value-typed
  defaults propagation, context/error paths, and the chosen scalar and header
  merge policies.
- Each acceptance criterion is traced to at least one meaningful unit test, and
  Resolver unit-test statement coverage remains greater than 95%.

## Acceptance criteria

1. Names, signatures, and the stateless constructor match the reference.
2. Each method traverses from `suite.Root` and populates or validates only the
   fields named by its reference contract.
3. Resolver does not decode YAML, execute steps, or populate `RuntimeSteps`.
4. Merge, ordering, and validation rules absent from the skeleton remain
   implementation choices rather than requirements.
5. Resolution starts with a 10-second timeout and 3 retries. Nonzero `Timeout`
   and `Retries` in directory, steps-file, and individual-step
   `domain.Defaults` values override those fallbacks in that order, so every
   resolved `Step.Request.Defaults` receives the inherited effective timeout
   and retry count.
6. Defaults are passed and stored as `domain.Defaults` values throughout
   resolution. Resolver neither restores the former direct request fields nor
   introduces `*domain.Defaults` pointers.
7. No TODO or zero-value placeholder remains in Resolver production methods;
   its unit tests and `git diff --check` pass.
