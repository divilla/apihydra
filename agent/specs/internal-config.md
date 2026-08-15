# internal.config

## Instructions

- Preserve all existing exported types and function signatures in the target
  package.
- Implement every currently unimplemented member described by this spec.
- Keep requirements and acceptance criteria grouped under the type they govern.

## Loader type

- Path: `internal/config/Loader`

### Behavior

- `NewLoader` creates a `Loader` configured with the supplied `WorkDir`.
- `Files` searches `WorkDir` and all nested subdirectories recursively.
- `Files` returns regular files with a `.yaml` or `.yml` extension.
- Files with other extensions and directories are not returned.
- Discovery does not read, parse, or validate file contents.
- A missing or non-directory `WorkDir` produces no results and does not panic.

### Acceptance criteria

#### AC-Loader-1: Find YAML files in the working directory

Given a `WorkDir` containing regular `.yaml` and `.yml` files, when `Files` is
called, then it returns the path of each matching file exactly once.

#### AC-Loader-2: Find YAML files recursively

Given matching files in one or more nested subdirectories of `WorkDir`, when
`Files` is called, then it returns every matching nested file regardless of its
depth.

#### AC-Loader-3: Exclude non-YAML entries

Given files with extensions other than `.yaml` or `.yml` and directories within
`WorkDir`, when `Files` is called, then none of those entries are returned.

#### AC-Loader-4: Handle an empty result

Given an existing `WorkDir` with no matching files, when `Files` is called,
then it returns no file paths and does not panic.

#### AC-Loader-5: Handle an invalid working directory

Given a `WorkDir` that does not exist or is not a directory, when `Files` is
called, then it returns no file paths and does not panic.

#### AC-Loader-6: Preserve the public API

When the loader is implemented, then the existing exported types and function
signatures in `internal/config` remain unchanged.

### Loader non-goals

- `Loader` does not parse or validate YAML content.
- `Loader` does not merge files or define configuration precedence.
- `Loader` does not watch the filesystem for changes.
- `Loader` does not search outside `WorkDir`.

## Parser type

- Path: `internal/config/Parser`

### Behavior

- `Parse` parses all YAML files to config.Config
- `Parse` returns only `Tasks` Configs

### Rules
- There can only be one `config` YAML file per folder
- Each Y
