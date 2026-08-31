# User-local temporary files and cookie support

## Status and dependency

Implement this change after change 015 defines application parallelism. Change
015 owns the `--parallelism` CLI contract and execution semantics. This change
only consumes the resulting effective parallelism value when managing cookie
jars.

The binding defaults field is `domain.Defaults.DisableCookies *bool` with the
YAML and JSON name `disable_cookies`. The protected skeleton contracts carry
the run-local temporary directory and cookie-jar state through the existing
resolution, execution, and runner flow. Bring the PRD, specifications,
examples, tests, and implementation into alignment with that skeleton before
this change is complete.

### User approval record

On 2026-08-31, the user explicitly authorized the protected skeleton changes
required by this change, including the Resolver overlay contract, cookie-aware
Runner signatures, Executor cookie-jar lifecycle, and their reference tests.

## Cookie configuration and inheritance

Cookies are enabled when no scope supplies `disable_cookies`.

`DisableCookies` is presence-sensitive:

- `nil` inherits the effective value from the broader scope;
- a pointer to `true` disables automatic cookie handling; and
- a pointer to `false` explicitly enables automatic cookie handling, including
  re-enabling it below a scope that disabled it.

Apply this overlay behavior through the existing defaults chain:

```text
root/directory defaults -> steps-file defaults -> individual-step defaults
```

Consequently, root, directory, steps-file, and individual-step configuration
can each enable or disable cookie support. The declarative YAML form is:

```yaml
disable_cookies: true   # disable at this scope
disable_cookies: false  # enable at this scope
```

The effective value controls only curl's automatic cookie engine. Disabling it
does not remove or alter an explicit user-supplied `Cookie` header.

## Cookie-jar lifecycle

1. Every run starts with fresh, empty cookie state. Create every cookie jar
   used by that run inside a cookie-specific namespace below
   `domain.Config.TempRunDir`; no cookie-jar path may escape the run directory.
2. A run must never discover, read, reuse, or copy a jar from another run.
   Cookie state is exchanged only through the within-run copies required by
   the selected parallelism mode. Jar lifetime and cleanup are exactly the
   `TempRunDir` lifetime and cleanup defined by change 015: jars have no
   separate persistence or cleanup lifecycle. If an abrupt termination leaves
   a run directory behind, later runs still never inspect or reuse its jars.
3. Use filesystem-safe names of the form `<unique>.cookie.jar`. UUIDv7 is
   acceptable, but a shorter collision-resistant identifier may be used. A jar
   name must be unique within its run. Create and initialize a required jar
   before passing its path to curl; do not rely on curl to create a missing jar.
   A fresh jar may be a pre-created zero-length file, which curl accepts and
   populates when it receives cookies.
4. An enabled request passes the same jar selected for it by the ownership
   rules below to both curl options:

   ```shell
   curl \
     --cookie <unique>.cookie.jar \
     --cookie-jar <unique>.cookie.jar \
     https://example.com/account
   ```

5. A disabled request omits both `--cookie` and `--cookie-jar`. It leaves its
   owning jar unchanged so a later request that re-enables cookie handling
   resumes that jar's cookie state.
6. Jar ownership and stage-transition inheritance are selected strictly by the
   effective parallelism supplied by change 015. Create every mode-owned jar
   regardless of whether cookies are currently disabled for all of its steps;
   an unchanged jar still carries inheritance state:

   - **Parallelism 0 (run jar):** Create exactly one jar for the run. Every
     steps file in every directory uses that jar. Because mode 0 executes all
     work serially, later requests observe all cookies written by earlier
     enabled requests in execution order. Stage transitions create no copies.
   - **Parallelism 1 (directory jars):** Create exactly one jar for each
     directory. The root directory starts with a fresh, empty jar. After a
     parent stage has fully joined and before the next stage starts, create a
     distinct jar for every direct child directory by copying its parent's
     final jar byte for byte. All steps files in a directory use that
     directory's jar serially. Sibling directories never share a writable jar.
   - **Parallelism 2 (steps-file jars):** Create exactly one jar for each steps
     file. Every steps file in the root directory starts with its own fresh,
     empty jar. After a parent stage has fully joined and before the next stage
     starts, use the jar owned by the steps file whose step completion was
     observed last in each parent directory. Create the initial jar for every
     steps file in each direct child directory as a distinct byte-for-byte copy
     of that selected parent jar. Record the owning file's jar after every step
     finishes, including a step with effective cookies disabled; do not infer
     completion order from jar modification timestamps. Go scheduling and
     actual runtime completion order intentionally determine which jar wins.
     Steps within a file use its jar serially; concurrently executing files and
     directories never share a writable jar.

7. In parallelism 2, a directory in which no step executes preserves its
   incoming cookie state unchanged for its children, whether it contains no
   steps files or only empty steps files. Every jar for an empty steps file is
   an unchanged copy of that same incoming state and may serve as the copy
   source. If the root has no steps-file jars at all, create a fresh, empty
   inheritance jar below `TempRunDir`. This rule applies through any chain of
   directories without executed steps and ensures that a later non-empty
   descendant can create its file jars without an undefined source.
8. Apply the selected mode's inheritance at every stage transition. Copies
   always flow from a parent to its direct children after the parent has
   finished. Never copy state between siblings or merge cookie state from
   multiple directory or file jars.
9. A missing or unusable run directory, or failure to create, initialize, or
   copy a required jar, is an internal failure. Do not execute an affected
   request without the cookie behavior selected by its resolved defaults.

## Curl and Debug behavior

Cookie options are part of the argument list built for the real curl request.
`CurlRaw` therefore reports the same complete `--cookie` and `--cookie-jar`
arguments, including their user-local jar path, on the Debug path. Debug must
not mutate the arguments or use a different jar from the request that
`CurlExecute` runs.

## Acceptance criteria

1. `disable_cookies` is absent-by-default and presence-sensitive at every
   defaults scope: `nil` inherits, `true` disables, and `false` re-enables.
2. Enabled requests use one jar path for both `--cookie` and `--cookie-jar`;
   disabled requests use neither option and preserve explicit `Cookie`
   headers. Mode-owned jars are created even when every assigned step has
   cookies disabled so their unchanged state remains available for inheritance.
3. Every jar is created below the current run's `Config.TempRunDir`, exists
   before its path is passed to curl, is removed with that complete run
   directory, and is never discovered, read, reused, or copied by another
   invocation, including when an earlier run directory remains after abrupt
   termination.
4. Parallelism 0 uses exactly one run jar without stage-transition copies, and
   every enabled request observes cookies written earlier in serial execution
   order.
5. Parallelism 1 uses exactly one jar per directory. Each direct child starts
   with an independent copy of its parent's final jar at every stage
   transition, all files in that directory use its jar serially, and sibling
   directories never share or merge jars.
6. Parallelism 2 uses exactly one jar per steps file. Each root file starts
   empty; after every step completion the file's jar becomes its directory's
   latest completed jar; and each child file starts with an independent copy of
   the parent jar whose step completion was observed last. Files and
   directories never share or merge writable jars.
7. Parallelism-2 parent source selection follows actual runtime completion
   order, including cookie-disabled steps, without using jar modification
   timestamps. A directory with no executed steps preserves its incoming state;
   only a root with no steps-file jars creates an additional empty inheritance
   jar.
8. Debug Curl output contains the exact cookie arguments and jar selected for
   the executed request.
9. Run-directory and jar creation, initialization, or copy failures are
   reported as internal failures and do not silently downgrade an enabled
   request to cookie-less execution.
10. Every acceptance criterion has unit coverage, all production additions
   keep unit-test coverage above 95%, and the repository's required checks
   pass.
