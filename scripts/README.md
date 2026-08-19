# Repository Helper Scripts

This directory contains maintained, user-owned helpers for working with the APIHydra repository. These files are intentional project tooling, not generated output or repository-hygiene candidates. Do not delete or relocate them without explicit user approval.

## `commit-user.pl` and `commit-agent.pl`

Both scripts stage all repository changes, create a commit, and push the current branch to `origin`. Use `commit-user.pl` for user-authored changes; with no argument, its commit message defaults to `User commit`:

```shell
scripts/commit-user.pl
```

Use `commit-agent.pl` for agent-authored changes; with no argument, its commit message defaults to `Agent commit`:

```shell
scripts/commit-agent.pl
```

Pass either script one argument to provide a different commit message:

```shell
scripts/commit-user.pl "Initial commit"
scripts/commit-agent.pl "Implement runner"
```

Both scripts resolve the repository root from their own location, so they can be invoked from any working directory. They refuse to run from a detached `HEAD`.

## `codex-review-loop.sh`

Run Codex review-and-fix passes until the complete review output is exactly
`No findings.`:

```shell
scripts/codex-review-loop.sh
```

The script always reviews a branch range so every pass includes fixes committed
by earlier passes. It uses the default remote branch from `origin/HEAD` unless
`--base BRANCH` is supplied; other arguments are forwarded to Codex. For
example:

```shell
scripts/codex-review-loop.sh --base develop
```

The `--uncommitted` and `--commit SHA` review targets are rejected because they
cannot include the loop's later fix commits.

Each findings pass prints `comments.md`, runs `codex exec "fix all comments"`,
prints the changed files, and commits and pushes them as `review fixes 01`,
`review fixes 02`, and so on. The commit operation is built into the running
review-loop process, so a fixer-modified repository helper is never executed
with the caller's permissions. The script requires a clean working tree so
automated commits cannot include unrelated work. The generated `comments.md`
remains untracked, is overwritten on every review pass, and is removed when the
review completes with no findings.

While Codex runs, its output is replaced by a progress line containing elapsed
minutes and seconds plus accumulated output dots, rate-limited to at most one
dot per second while output is received. In a terminal, the activity marker
cycles through `·`, `•`, `●`, `⬤`, `●`, `•` within each second and is removed
when the command finishes. It resolves to a green `✅` on success or a red `❌`
on failure or interruption. Pressing Ctrl+C terminates the active Codex command
and the loop. Startup output identifies the repository, branch, base, review
options, and comments file, with terminal colors unless `NO_COLOR` is set.
Before each run, the complete shell-escaped Codex command and its parameters are
printed between separator lines. Codex runs in JSON mode so progress events
remain streamable while their raw content is suppressed.
