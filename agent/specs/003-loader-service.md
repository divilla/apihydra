# `internal/definition` Loader

## Status and ownership

- Binding reference: [`skeleton/internal/definition/loader.go`](../../skeleton/internal/definition/loader.go)
- Shared domain and pipeline: [`prd.md`](../prd.md)
- Shared domain types: [`000-domain-types.md`](000-domain-types.md)
- Status: skeleton-aligned implementation guide

## Reference contract

The binding skeleton defines Loader's API and the fields mutated by its selection and loading
phases. This guide does not reproduce those declarations or method contracts.
Keeping directory discovery, file discovery, and base classification separate
lets later services consume progressively richer state. Shared model fields
and the CLI phase order remain in the PRD.

`Select` resolves invocation targets to their common nearest root before cache
creation or reporting. Its binding selection contracts and reference tests live
in `skeleton/internal/definition/selection.go` and `selection_test.go`.
`LoadDirectoryStructure` builds the selected tree and necessary ancestor chains;
`LoadDirectoryFiles` includes selected files and applicable inherited defaults.
Full decoding stays in Decoder. Unselected steps and unrelated branches do not
expand the validation scope.

`ErrRootDefinitionMissing` uses `kind: root - file missing`; root qualification
ignores unrelated invalid kinds and malformed files. `ErrInvalidSelection`
classifies invalid paths, selectors, and inconsistent roots with code `102`.
Discovery failures preserve `ErrDefinitionDiscovery` and filesystem provenance.
`DecodeBaseDefinitions` retains atomic classification and app-scoped kind
validation for the files included in the selected scope. Kind violations keep
their exact existing diagnostic; malformed definitions keep the source cause.

## Deliberately unspecified

The skeleton does not define traversal ordering, recursive symlink or hidden-directory
policy, or behavior when multiple qualifying root definitions exist. These
choices must not be promoted to requirements in this guide.

## Required implementation and tests

- Production output: `internal/definition/loader.go` replaces every placeholder
  with directory discovery, YAML-file loading, and base classification against
  the binding `domain.Suite` tree.
- Test output: `internal/definition/loader_test.go` uses temporary directory
  trees and files to cover selection and all three loading phases, context/error paths, supported file
  extensions, nearest-root discovery, scoped traversal, and rejection cases, source links,
  discovery provenance, and malformed-definition provenance without turning
  deliberately unspecified choices into public contracts.
- Each acceptance criterion is traced to at least one meaningful unit test, and
  Loader unit-test statement coverage remains greater than 95%.

## Acceptance criteria

1. Names, signatures, and the stateless constructor match the reference.
2. Each phase starts from the reference field and mutates only its documented
   output fields.
3. Each target discovers its nearest qualifying root, all targets share one
   root, and the resulting tree includes selected subtrees plus ancestor chains.
   Symlink targets resolve before parent components, root discovery, and matching.
   Filesystem spelling is canonicalized before root and scope comparisons,
   including differently cased directory, file, and range selections where parent
   listing is permitted. When listing is denied, accessible component spelling
   is retained without requiring additional ancestor listing permission.
   Step suffixes are parsed only in the final path component, preserving colons
   in Unix parent directory names.
   Paths and stages remain relative to the discovered `Suite.WorkDir`.
4. No complete decoding, validation, resolution, execution, or output behavior
   is assigned to Loader.
5. No TODO or zero-value placeholder remains in Loader production methods;
   its unit tests and `git diff --check` pass.
6. Root, discovery, and invalid-definition failures preserve their binding
   static identity, exit-code meaning, affected path, and underlying cause.
7. Kind validation is scoped to string `app: apihydra`, rejects every value
   except string `root`, `defaults`, or `steps`, and preserves the exact
   `ErrInvalidKind` message and code `102`. Scoped kind validation follows root qualification;
   failed base classification does not change files or directories.

8. File selections include only selected files and applicable defaults; unrelated
   invalid files do not block execution. Required malformed defaults still fail,
   including uniformly indented top-level envelopes, flow mappings, and multiline
   envelope scalars.
