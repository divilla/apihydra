# `internal/definition` Loader

## Status and ownership

- Binding reference: [`skeleton/internal/definition/loader.go`](../../skeleton/internal/definition/loader.go)
- Shared domain and pipeline: [`prd.md`](../prd.md)
- Shared domain types: [`000-domain-types.md`](000-domain-types.md)
- Status: skeleton-aligned implementation guide

## Reference contract

The binding skeleton defines Loader's API and the fields mutated by its three
phases. This guide does not reproduce those declarations or method contracts.
Keeping directory discovery, file discovery, and base classification separate
lets later services consume progressively richer state. Shared model fields
and the CLI phase order remain in the PRD.

`LoadDirectoryStructure` first qualifies a root definition directly in
`Suite.WorkDir` according to the binding validator, returning
`ErrRootDefinitionMissing` before recursive traversal if none qualifies.
Filesystem traversal failures use `ErrDefinitionDiscovery`; malformed base
definitions found after qualification use `ErrInvalidDefinition` with the
affected file and underlying YAML cause.

## Deliberately unspecified

The skeleton does not define traversal ordering, symlink or hidden-directory
policy, or behavior when multiple qualifying root definitions exist. These
choices must not be promoted to requirements in this guide.

## Required implementation and tests

- Production output: `internal/definition/loader.go` replaces every placeholder
  with directory discovery, YAML-file loading, and base classification against
  the binding `domain.Suite` tree.
- Test output: `internal/definition/loader_test.go` uses temporary directory
  trees and files to cover all three phases, context/error paths, supported file
  extensions, direct root qualification and rejection cases, source links,
  discovery provenance, and malformed-definition provenance without turning
  deliberately unspecified choices into public contracts.
- Each acceptance criterion is traced to at least one meaningful unit test, and
  Loader unit-test statement coverage remains greater than 95%.

## Acceptance criteria

1. Names, signatures, and the stateless constructor match the reference.
2. Each phase starts from the reference field and mutates only its documented
   output fields.
3. Root and relative-path conventions match the shared domain contract; only a
   qualifying regular YAML file directly in `Suite.WorkDir` permits recursive
   discovery.
4. No complete decoding, validation, resolution, execution, or output behavior
   is assigned to Loader.
5. No TODO or zero-value placeholder remains in Loader production methods;
   its unit tests and `git diff --check` pass.
6. Root, discovery, and invalid-definition failures preserve their binding
   static identity, exit-code meaning, affected path, and underlying cause.
