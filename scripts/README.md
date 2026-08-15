# Repository Helper Scripts

This directory contains maintained, user-owned helpers for working with the APIHydra repository. These files are intentional project tooling, not generated output or repository-hygiene candidates. Do not delete or relocate them without explicit user approval.

## `commit.pl`

Stages all repository changes, creates a commit, and pushes the current branch to `origin`. With no argument, the commit message defaults to `Commit by user`:

```shell
scripts/commit.pl
```

Pass one argument to provide a different commit message:

```shell
scripts/commit.pl "Initial commit"
```

The script resolves the repository root from its own location, so it can be invoked from any working directory. It refuses to run from a detached `HEAD`.
