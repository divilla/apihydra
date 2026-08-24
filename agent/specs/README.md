# Implementation guides

Files in this directory have stable, permanent paths. Keep an implementation
guide here after its acceptance criteria are satisfied so the PRD,
architecture, automation, and implementation history can continue to reference
the same location.

Do not encode implementation status in this directory structure or duplicate
it in this README. The production code and tests are the current source of
truth, while version control records when each guide was implemented.

## Status-line format

When a change, issue, or review needs a status snapshot, use one line per guide
in this format:

```markdown
| [`<number>` — <title>](<filename>) | <status> | <short evidence or blocker> |
```

For example:

```markdown
| [`NNN` — Example guide](NNN-example-guide.md) | In progress | Acceptance criterion 3 remains. |
```

Use `Pending`, `In progress`, `Implemented`, or `Blocked` as the status.
