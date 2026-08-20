# `internal/definition` Loader

## Status and ownership

- Binding reference: [`skeleton/internal/definition/loader.go`](../../skeleton/internal/definition/loader.go)
- Shared domain and pipeline: [`prd.md`](../prd.md)
- Status: skeleton-aligned implementation guide

## Reference contract

The binding skeleton defines Loader's API and the fields mutated by its three
phases. This guide does not reproduce those declarations or method contracts.
Keeping directory discovery, file discovery, and base classification separate
lets later services consume progressively richer state. Shared model fields
and the CLI phase order remain in the PRD.

## Deliberately unspecified

The skeleton does not define traversal ordering, symlink or hidden-directory
policy, document placement/cardinality validation, or Loader-specific static
errors. These choices must not be promoted to requirements in this guide.

## Acceptance criteria

1. Names, signatures, and the stateless constructor match the reference.
2. Each phase starts from the reference field and mutates only its documented
   output fields.
3. Root and relative-path conventions match the shared domain contract.
4. No complete decoding, validation, resolution, execution, or output behavior
   is assigned to Loader.
