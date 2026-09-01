# APIHydra error reproductions

## Purpose

This file is the reproducibility registry for user-visible fatal `apih` errors.
It gives coding agents and maintainers copyable ways to observe an error,
confirm its exit code and output streams, and verify the corresponding repaired
behavior.

Run reproductions with an `apih` binary built from the revision under test.
Commands must not require network access or mutate repository fixtures. When a
reproduction needs temporary inputs, create them below a fresh temporary
directory and remove that directory afterward.

Each new fatal error category must record:

1. the diagnostic name and user-manual troubleshooting anchor;
2. whether the entry describes pre-change or required behavior;
3. prerequisites and fixture state;
4. the exact working directory and command;
5. expected stdout, stderr, and process exit code; and
6. cleanup and platform-specific considerations.

## Root definition missing

Manual anchor: `#root-definition-missing`

### Motivating behavior before change 019

The APIHydra repository root has no top-level `.yaml` or `.yml` definition, but
it contains YAML fixtures in descendants. One fixture is deliberately invalid:

```text
int-tests/input/scenarios/invalid-base/definition.yaml
```

Its contents begin with:

```yaml
app: []
kind: root
spec: {}
```

From the APIHydra repository root, run:

```bash
apih
```

Before change 019, the command recursively reaches that nested fixture and
returns exit code `102`. Its combined terminal output is:

```text
Working Directory: /home/vito/go/src/apihydra

[1:6] cannot unmarshal []interface {} into Go struct field BaseDefinition.App of type string
>  1 | app: []
            ^
   2 | kind: root
   3 | spec: {}
```

The absolute working-directory line varies by checkout. The YAML diagnostic
does not name the nested fixture, and the nested fixture is irrelevant because
the selected repository root has no qualifying root document. This is the
misleading behavior change 019 fixes.

### Required behavior with no directory argument

Create an empty temporary suite directory and run `apih` from inside it:

```bash
suite_dir=$(mktemp -d)
cd "$suite_dir"
apih >stdout.txt 2>stderr.txt
status=$?
```

Expected exit code:

```text
102
```

`stdout.txt` must be empty. `stderr.txt` must contain exactly the following
diagnostic using the canonical published manual URL:

```text
error: root definition missing

please check user manual: https://github.com/divilla/apihydra/blob/master/docs/user-manual/apih.md#root-definition-missing
```

The final line ends with one newline. Remove the temporary directory after
inspection.

The same expected result applies when the selected directory has YAML only in
descendants. In particular, after change 019, running `apih` from the APIHydra
repository root must produce this root-missing diagnostic without decoding
`int-tests/input/scenarios/invalid-base/definition.yaml`.

### Required behavior with a directory argument

From a fresh temporary parent directory, create an empty selected directory and
pass it as the sole positional argument:

```bash
parent_dir=$(mktemp -d)
mkdir "$parent_dir/suite"
cd "$parent_dir"
apih suite >stdout.txt 2>stderr.txt
status=$?
```

The expected exit code and byte-for-byte stdout and stderr are identical to the
no-argument reproduction: exit `102`, empty stdout, and the exact anchored
root-missing diagnostic on stderr.

### Required success boundary

The root filename is arbitrary. For either invocation form, add a file directly
inside the selected suite directory, for example `suite.yml`:

```yaml
app: apihydra
kind: root
spec: {}
```

This file satisfies the root-presence check. `apih` must continue into ordinary
recursive discovery rather than return `root definition missing`.

The following do not satisfy the check when no other qualifying root exists:

```yaml
app: other
kind: root
```

```yaml
app: apihydra
kind: defaults
```

```yaml
app: []
kind: root
```

A qualifying root document in a nested directory also does not satisfy the
selected directory's root requirement.

## Adding another reproduction

Add one section per stable troubleshooting category rather than one section per
possible Go or dependency error string. Prefer deterministic local fixtures.
Record stream contents separately whenever stdout/stderr ordering matters, and
state which values are expected to vary, such as temporary paths or operating-
system wording.

Every emitted fatal manual link must target the section recorded by the entry,
and automated tests must prove that the corresponding anchor exists in
`docs/user-manual/apih.md`.
