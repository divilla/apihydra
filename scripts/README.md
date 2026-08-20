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

## `codex-review-loop.pl`

Run Codex review-and-fix passes until the native review returns no comments:

```shell
scripts/codex-review-loop.pl agent/specs/010-step-runner-service.md
```

The required first positional argument is the specification file. The script
always reviews a branch range so every pass includes fixes committed by earlier
passes. It uses the default remote branch from `origin/HEAD` unless
`--base BRANCH` is supplied; arguments after the specification are forwarded
to Codex. Every review pass uses the native `codex exec review --base` target,
with the base resolved to a pinned commit before the loop begins. The
specification is supplied only to the subsequent `$change-code` fixer because
Codex treats a custom review prompt and `--base` as conflicting review targets.
Custom review instructions from the caller are therefore rejected, whether
supplied as a bare positional prompt, `-` for standard input, or after `--`.
For example:

```shell
scripts/codex-review-loop.pl agent/specs/010-step-runner-service.md --base develop
```

The `--uncommitted` and `--commit SHA` review targets are rejected because they
cannot include the loop's later fix commits.

Each invocation creates a private temporary directory outside the repository,
preferring `TMPDIR`, `TMP`, `TEMP`, and then `/tmp`, and prints its
`findings.md` path. A candidate that resolves inside the checkout is skipped.
Every findings pass redirects that file
through standard input to a fresh `codex exec` invocation and explicitly invokes
`$change-code` with the positional specification file. The prompt tells
the fixer to treat stdin as review feedback against that specification,
validate each finding, implement valid fixes with tests and verification,
preserve unrelated changes, and leave commits and pushes to the loop. The loop
captures the fixer's final response. If the fixer makes no repository changes,
the loop prints that response so protected-contract blockers and rejected
findings are visible, then stops without committing. Otherwise, it prints
changed files, then commits and pushes them as
`review fixes 01`, `review fixes 02`, and so on. Native Codex review rendering
adds an exact `Review comment:` or `Full review comments:` header whenever its
structured result contains findings. The loop checks those headers instead of
searching unconstrained review prose for verdict text, and rejects empty output
plus Codex's failed-response and interrupted-review fallbacks before a
headerless result can be accepted as clean. `codex exec review`
ignores `--output-schema`, so the script rejects that option. The commit
operation is built into the running review-loop process, so a fixer-modified
repository helper is never executed with the caller's permissions. The script
requires a clean working tree so automated commits cannot include unrelated
work. All generated findings, final-response, and progress-log artifacts
stay outside the repository, and their private directory is removed on every
exit path.
The default `/tmp` location works on Linux and macOS. The script requires Perl
and a POSIX environment because interruption is propagated to the active Codex
process group.

While Codex runs, its output is replaced by a progress line containing elapsed
minutes and seconds plus accumulated output dots, rate-limited to at most one
dot per second while output is received. In a terminal, Perl's timed I/O
multiplexing updates the activity marker every 250 milliseconds independently
of Codex output, without delivering asynchronous progress signals to the
process. It resolves to a green
`✅` on success or a red `❌` on failure or interruption. Pressing Ctrl+C
terminates the active Codex command and the loop. Startup output identifies the
repository, branch, base, review options, and findings file, with terminal
colors unless `NO_COLOR` is set.
Before each run, the complete copy/pasteable Codex command, including the
fixer's stdin redirection, is printed with readable single-quoted arguments
between terminal-width
separator lines. Codex runs in JSON mode so progress events remain streamable
while their raw content is suppressed on success. If Codex fails, its captured
output is printed to standard error for diagnosis.
