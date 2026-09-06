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

`LoadDirectoryStructure` validates kinds in all parseable top-level documents
with string `app: apihydra`, then qualifies a root directly in `Suite.WorkDir`
according to the binding validator, returning
`ErrRootDefinitionMissing` (message `root defaults file missing`) before
recursive traversal if none qualifies.
Filesystem traversal failures use `ErrDefinitionDiscovery`; malformed base
definitions found after qualification use `ErrInvalidDefinition` with the
affected file and underlying YAML cause.

`ErrInvalidKind` owns the exact message `kind: must be one of: <root|defaults|steps>`
and configuration code `102`. Only the three exact string kinds are allowed
for `app: apihydra`; missing, unspecified, empty, null, and non-string values
also fail. This top-level error precedes both root outcomes, even when a valid
root sorts before the invalid file. Other app values retain existing behavior.
`DecodeBaseDefinitions` repeats kind validation before typed decoding for all
discovered files, including descendants. It commits no partial classification
on failure. Malformed YAML follows the existing parser-error behavior.

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
7. Kind validation is scoped to string `app: apihydra`, rejects every value
   except string `root`, `defaults`, or `steps`, and preserves the exact
   `ErrInvalidKind` message and code `102`. Top-level validation precedes root
   acceptance or rejection; nested validation follows root qualification;
   failed base classification does not change files or directories.
