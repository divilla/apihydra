# `internal/definition` Resolver

## Status and ownership

- Binding reference: `skeleton/internal/definition/resolver.go`
- Shared domain and pipeline: [`prd.md`](../prd.md)
- Contextual definition errors: [`02-errs-pkg.md`](002-errs-pkg.md)
- Status: skeleton-aligned specification

This specification owns the three Resolver operations. The PRD owns their
position in the CLI pipeline.

## Public API

```go
type Resolver struct{}

func NewResolver() *Resolver
func (r *Resolver) ResolveDefaults(ctx context.Context, suite *domain.Suite) error
func (r *Resolver) ResolveSteps(ctx context.Context, suite *domain.Suite) error
func (r *Resolver) ValidateStepsDefinitions(ctx context.Context, suite *domain.Suite) error
```

`NewResolver` returns an empty Resolver. The reference has no constructor
dependencies or retained Suite.

## `ResolveDefaults`

The method traverses from `suite.Root` and populates each directory's
`ResolvedDefaults` with values merged from that directory's
`DefaultsDefinition` and its parent directory's defaults definition.

The skeleton does not define field-presence tracking, scalar-zero overlay
rules, map-copy behavior, or header collision semantics. Those details are not
requirements of this spec.

## `ResolveSteps`

The method traverses from `suite.Root` and populates each directory's
`ResolvedSteps` from its steps definitions and defaults definition.

The output uses the shared two-dimensional `[][]domain.Step` field. The
skeleton does not define ordering, copy/alias policy, implicit request values,
or URL normalization.

## `ValidateStepsDefinitions`

This method traverses from `suite.Root` and validates each directory's decoded
steps definitions. It is an exported Resolver operation, although the current
CLI pipeline does not invoke it.

Its validation rules are not specified by the skeleton and must not duplicate
or contradict Decoder rules. Contextual error construction, when needed, is
owned by `02-errs-pkg.md`; the skeleton declares no Resolver-specific static
errors.

## Acceptance criteria

1. Names, signatures, and the stateless constructor match the reference.
2. Each method traverses from `suite.Root` and populates or validates only the
   fields named by its reference comment.
3. Resolver does not decode YAML, execute steps, or populate `RuntimeSteps`.
4. Merge, ordering, and validation rules absent from the skeleton remain
   implementation choices rather than requirements.
