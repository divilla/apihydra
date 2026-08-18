# `internal/definition` Loader

## Status and ownership

- Binding reference: `skeleton/internal/definition/loader.go`
- Shared domain and pipeline: [`prd.md`](../prd.md)
- Status: skeleton-aligned specification

This specification owns only Loader's three mutations. Shared model fields and
the CLI phase order are defined by the PRD.

## Public API

```go
type Loader struct{}

func NewLoader() *Loader
func (l *Loader) LoadDirectoryStructure(ctx context.Context, suite *domain.Suite) error
func (l *Loader) LoadDirectoryFiles(ctx context.Context, suite *domain.Suite) error
func (l *Loader) DecodeBaseDefinitions(ctx context.Context, suite *domain.Suite) error
```

`NewLoader` returns an empty Loader. It has no configured working directory or
retained collaborators.

## `LoadDirectoryStructure`

The method traverses from `suite.WorkDir` and builds the `domain.Directory`
tree assigned to `suite.Root`. Directory paths are relative to
`suite.WorkDir`, and the root path is `/`.

This phase owns directory structure only. File discovery and definition
classification belong to the later Loader phases.

## `LoadDirectoryFiles`

The method traverses from `suite.Root` and populates only each directory's
`Files` slice with files whose names end in `.yaml` or `.yml`.

It does not populate `DefaultsFile`, `StepsFiles`, or decoded definitions.

## `DecodeBaseDefinitions`

The method traverses the directory tree, attempts to decode each loaded file
as `domain.BaseDefinition`, sets `File.Kind` on success, and populates that
directory's singular `DefaultsFile` and `StepsFiles` classification fields.

Complete definition decoding belongs to Decoder. Default and step merging
belongs to Resolver.

## Deliberately unspecified

The skeleton does not define traversal ordering, symlink or hidden-directory
policy, document placement/cardinality validation, or Loader-specific static
errors. These choices must not be promoted to requirements in this spec.

## Acceptance criteria

1. Names, signatures, and the stateless constructor match the reference.
2. Each phase starts from the reference field and mutates only its documented
   output fields.
3. Root and relative-path conventions match the shared domain contract.
4. No complete decoding, validation, resolution, execution, or output behavior
   is assigned to Loader.
