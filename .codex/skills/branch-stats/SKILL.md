---
name: branch-stats
description: Calculate a Git branch's changed-line statistics against its merge base, split into production code, unit tests, integration tests, tooling, protected skeleton references, documentation, dependency metadata, and comment versus non-comment lines. Use for branch-size and code/test/comment distribution requests in this repository; do not use for cumulative per-commit churn.
---

# Branch Stats

Run the bundled script from anywhere inside the repository:

```bash
.codex/skills/branch-stats/scripts/branch-stats.sh [--branch REF] [--base REF]
```

`--branch` defaults to `HEAD`. `--base` defaults to the remote default branch, then falls back to `origin/master`, `origin/main`, `master`, or `main`. The script uses only local refs; do not fetch unless the user explicitly asks for refreshed remote state.

Present the script's complete Markdown table and its snapshot metadata. State that working-tree changes are excluded when the reported working tree is dirty.

The calculation is the aggregate tree diff from the branch/base merge point to the branch tip, not the sum of every commit's churn. A modified line therefore contributes one addition and one deletion.

Classification rules are repository-specific:

- Non-test Go files under `cmd/`, `internal/`, and `pkg/` are production application code.
- Files under `int-tests/`, including YAML scenarios, are integration tests and fixtures.
- Other `*_test.go` files and test scripts under `scripts/` are unit tests.
- `skeleton/` production-shaped and test-shaped files remain separate because that directory is protected reference material.
- `scripts/codex-review-loop.pl` is developer tooling; `go.mod` and `go.sum` are dependency metadata; remaining files are documentation, specifications, and logs.

Comment counts come from `cloc`'s language-aware Git diff. Markdown is documentation content rather than comments. Blank and unrecognized lines count as non-comment so the table reconciles exactly with Git's line totals. If `git`, `cloc`, `jq`, or `awk` is unavailable, report the missing dependency instead of estimating.
