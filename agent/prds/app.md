# APIHydra Product Requirements Document

## Document status

- Product: APIHydra
- CLI command: `apih`
- Status: Initial draft
- Primary interface: YAML files and terminal output

## Product summary

APIHydra is an API integration-testing CLI designed primarily for use by AI
agents. Its test suites and terminal output must also remain easy for humans to
navigate, inspect, understand, and troubleshoot.

An APIHydra suite is a directory tree containing YAML configuration and task
files. APIHydra resolves inherited configuration, expands run-wide variables,
turns declarative tasks into concrete curl commands, executes those commands,
and validates their JSON responses.

## Goals

- Let AI agents define and run API integration tests through a concise,
  declarative YAML format.
- Keep test suites readable and directly inspectable by human users.
- Support reusable configuration across directories without duplicating common
  request values.
- Support stateful integration flows by passing values from earlier tasks to
  later tasks.
- Run independent work concurrently while preserving explicit dependency order.
- Produce clear configuration, runtime, request, and validation failures.

## Non-goals for the initial product

- A graphical interface.
- Mutable or reassignable variables.
- Full-response equality when only selected values are declared under
  `response.expected`.
- Parallel execution of tasks declared in the same task file.
- Arbitrary raw curl arguments. Curl capabilities must be exposed through
  explicit typed YAML fields.

## Core concepts

### Suite root

The suite root is the directory from which APIHydra discovers YAML files and
builds the execution hierarchy.

The user selects it in one of two ways:

```text
apih
apih <relative-path-to-apih-yaml-root>
```

With no argument, the current directory is the suite root. With an argument,
the given path is resolved relative to the current directory and becomes the
suite root.

### YAML document kinds

APIHydra recognizes two kinds of YAML documents:

- `config`: shared values used to resolve tasks.
- `tasks`: one or more declarative curl tasks.

Documents identify themselves as APIHydra documents and declare their kind.
The current schema uses `app: apihydra` and `kind: config|tasks`.

An APIHydra document may optionally declare metadata:

```yaml
metadata:
  name: create-tasks
  labels: [create, tasks]
```

Metadata is used only when the user requests a filtered test run. It is not
required for, and does not alter, a normal full-suite run.

### RuntimeConfiguration

Every directory in the discovered suite tree has one effective
`RuntimeConfiguration`. It is the resolved set of configuration values
available to task files in that directory.

Common configuration values initially include:

- `baseUrl`
- `basePath`
- `headers`
- `timeout`
- `retries`

### Task

A `Task` is a declarative set of curl execution parameters. A task may define
request values itself and may omit values that are available from its
directory's `RuntimeConfiguration`.

### RuntimeTask

A `RuntimeTask` is a task after all available missing values have been populated
from its directory's `RuntimeConfiguration`.

```text
Task + directory RuntimeConfiguration -> RuntimeTask
```

Task-defined values take precedence over configuration values. A value absent
from both sources remains undefined and causes validation failure when that
value is required to execute or validate the task.

## Functional requirements

### 1. Suite discovery and startup

1. APIHydra must accept either no positional path or one relative suite-root
   path.
2. APIHydra must recursively inspect regular files ending in `.yaml` or `.yml`
   beneath the selected suite root, at any directory depth.
3. A YAML file whose `app` field is missing or is not exactly `apihydra` must be
   treated as unrelated and ignored.
4. A YAML file declaring `app: apihydra` must declare `kind: config` or
   `kind: tasks`. A missing or unsupported kind must produce a configuration
   error.
5. No APIHydra document kinds other than `config` and `tasks` are supported.
6. `metadata`, `metadata.name`, and `metadata.labels` must all be optional.
7. When supplied, `metadata.name` must be unique within the suite so name-based
   filtering is unambiguous.
8. When supplied, `metadata.labels` must be an array of strings.
9. Metadata must affect task selection only when the user requests a filtered
   run. It must have no effect on an unfiltered full-suite run.
10. The suite root must contain exactly one `config` document.
11. If the suite root has no config, APIHydra must report a configuration error
   and must not execute tasks.
12. Any directory may contain at most one config document.
13. If any directory contains multiple config documents, APIHydra must report a
   configuration error and must not execute tasks.
14. A child directory may contain no config document or one config document.
15. A non-root config's parent must be discovered from the filesystem hierarchy.
   Starting with the config file's parent directory, APIHydra must search
   ancestor directories upward until it finds the nearest config file.
16. A root config has no parent config.
17. An unfiltered suite that resolves to zero tasks, whether because it has no
    tasks documents or only empty tasks documents, must produce an error, execute
    no curl commands, and return exit code `2`.
18. Each APIHydra YAML file must contain exactly one YAML document. An APIHydra
    file containing multiple `---`-separated documents must produce a fatal
    configuration error.
19. APIHydra documents must be decoded using strict schema validation. Any
    unknown field must produce a fatal configuration error identifying the file
    and YAML field path, and APIHydra must return exit code `2` without executing
    tasks.
20. Duplicate YAML mapping keys must produce a fatal configuration error even
    when their values are identical. The error must identify the file and
    duplicated key.

### 1.1 Filtered execution

1. APIHydra must support `--name <name>` and its short form `-n <name>`.
2. A name filter must select a `tasks` document whose `metadata.name` exactly
   matches the supplied name.
3. APIHydra must support repeatable `--label <label>` and its short form
   `-l <label>`.
4. Repeated label filters must use AND semantics: a selected tasks document must
   contain every requested label.
5. When name and label filters are combined, a tasks document must satisfy both
   the exact name filter and every label filter.
6. Filters must select whole `tasks` documents. Every task in a selected
   document must execute; individual tasks are not filtered.
7. Config documents must not be excluded by task filters. APIHydra must load all
   configs needed to resolve selected task documents.
8. An invocation without name or label filters must execute the entire suite.
9. If the supplied filters select no tasks documents or resolve to zero tasks,
   APIHydra must report that no tasks matched, execute no curl commands, and
   return exit code `2`.

Examples:

```text
apih --name create-tasks
apih -n create-tasks
apih --label create
apih -l create -l smoke
apih tests --name create-tasks --label smoke
```

### 1.2 External-tool preflight

1. After filtering and runtime resolution but before executing any request,
   APIHydra must determine which external tools the selected tasks require.
2. `curl` must always be available.
3. `jq` must be available when any selected task has a request body or uses
   response capture, expected-value comparison, or type validation.
4. `git` must be available when any selected task uses `response.expected`.
5. If a required tool is unavailable, APIHydra must execute no requests, report
   the missing dependency, and return exit code `3`.
6. The initial product does not require the external `yq` command. This may be
   revisited if a later workflow requires it.

### 2. Configuration inheritance

1. The root config establishes the root directory's
   `RuntimeConfiguration`.
2. A child directory without its own config inherits the effective
   `RuntimeConfiguration` available from its nearest ancestor config.
3. A child directory with its own config inherits values from the nearest config
   found by searching upward from its parent directory and overrides inherited
   values that it defines locally.
4. Values not overridden by the child remain inherited from the parent
   configuration chain.
5. Header maps must merge by header name. Headers absent from the child config
   remain inherited, while a child header replaces the inherited header with the
   same name.
6. Header-name comparison must be case-insensitive. APIHydra must canonicalize
   emitted header names using standard HTTP header casing. Thus a child
   `Content-Type` overrides a parent `content-type` and is emitted as
   `Content-Type`.
7. The resolved result is the child directory's `RuntimeConfiguration`.
8. All task files in a directory receive that directory's
   `RuntimeConfiguration`.

### 3. Runtime task resolution

1. APIHydra must resolve every declared task against the
   `RuntimeConfiguration` of the directory containing its task file.
2. To locate a task file's config, APIHydra must search first in the task file's
   own directory and then upward through ancestor directories until it finds the
   nearest config file.
3. A value explicitly defined by the task must override the corresponding
   runtime-configuration value.
4. For each undefined task value, APIHydra must use the corresponding
   runtime-configuration value when one exists.
5. Task-level headers must merge with runtime-configuration headers by header
   name. Runtime headers absent from the task remain present, while a task header
   replaces the runtime header with the same name.
6. Task-level header merging must use the same case-insensitive comparison and
   canonical output names as configuration inheritance.
7. The result of resolution must be represented as a `RuntimeTask` suitable for
   curl execution and response validation.
8. APIHydra must validate a runtime task before executing it and report missing
   required values as errors.

### 4. Request execution

1. Each `RuntimeTask` must resolve to a concrete curl command.
2. Request parameters may include method, base URL, base path, path, headers,
   timeout, retries, query parameters, and body.
3. An explicitly declared `request.method` must always take precedence.
4. APIHydra must preserve an explicit method exactly as written and pass it to
   curl without restricting it to known or RFC-defined method names. A defined
   empty method must produce a fatal configuration error.
5. When `request.method` is omitted and the request has no body, the runtime
   method must default to `GET`.
6. When `request.method` is omitted and the request has a body, the runtime
   method must default to `POST`.
7. An explicitly declared `POST` request is valid without a body.
8. The final request URL must be composed as:

   ```text
   baseUrl + [basePath] + path + [?query]
   ```

9. `baseUrl` and `path` must be present after runtime-task resolution.
10. `basePath` is optional. When undefined, it contributes an empty string to
   the final URL. When defined, it must not be an empty string and must be
   included between `baseUrl` and `path`.
11. `query` is optional. When undefined, no query delimiter or query text is
    appended. When defined, it must not be an empty string and APIHydra must
    append `?` followed by its value.
12. A defined-but-empty `basePath` or `query` must produce a runtime-task
    validation error.
13. APIHydra must use Go's URL-aware `net/url` behavior to parse the base URL,
    normalize and join URL path components, and assign the optional raw query.
    It must not use filesystem-path joining.
14. Joining must normalize component boundaries so redundant or missing `/`
    separators do not produce a malformed path while preserving the URL scheme
    and authority.
15. URL validation must remain lightweight and rely on Go's standard URL
    handling. APIHydra must not implement a custom exhaustive RFC validator;
    operational URL problems may be reported by curl as execution failures.
16. `timeout` must be expressed in seconds. If it remains undefined after task
    and runtime-configuration resolution, it must default to `10`.
17. `retries` must control curl retry behavior. If it remains undefined after
    task and runtime-configuration resolution, it must default to `3`.
18. The resolved timeout must be a positive number and must map directly to
    curl's `--max-time` option.
19. The resolved retry count must be a non-negative integer and must map directly
    to curl's `--retry` option. Thus `retries: 3` permits the initial attempt plus
    up to three retries.
20. A task-defined `timeout` or `retries` value must override the corresponding
    runtime-configuration value like any other scalar task setting.
21. APIHydra must pass the resolved timeout and retry values to curl.
22. The request `body` must be handled as a literal JSON string.
23. After variable substitution and before curl execution, APIHydra must use
    `jq` to validate the final request body as JSON.
24. Body validation must not reformat or otherwise change the literal body text
    passed to curl.
25. If jq rejects the request body, APIHydra must stop immediately and forward
    jq's exact exit code. The invalid request must not execute.

### 5. Global variable store

1. A run must have one active, in-memory key-value store shared by all task
   runners.
2. Both keys and values in the store must be strings.
3. Stored values must preserve their JSON literal representation. For example:

   ```text
   change_id -> 1
   date      -> "2026-01-01T00:00:00"
   ```

4. A task may declare literal variables in a task-level `vars` map using YAML
   key-value pairs. These variables are written before the request is resolved.
5. Values under `task.vars` may be any JSON-compatible YAML scalar, array,
   object, or `null`. APIHydra must serialize each value as compact JSON before
   writing it to the string-to-string store. For example, YAML `1`, `true`,
   `Test`, `[1, 2]`, and `null` are stored as `1`, `true`, `"Test"`, `[1,2]`,
   and `null` respectively.
6. A task variable value that cannot be represented as JSON must produce a
   configuration error.
7. A task may capture variables from its curl response using `jq` expressions
   declared in `response.capture`.
8. APIHydra must execute response-capture expressions after curl returns and
   before substituting or validating `response.expected`, allowing a task to use
   a value captured from its own response in its expected-value assertions.
9. Response capture must use the terminal `jq` application.
10. A response-derived variable must store the literal text emitted for the
   extracted JSON value.
11. Variables are global to the run. Every task runner may access variables set
   by previously completed tasks, regardless of whether they came from a
   task-level declaration or response capture.
12. Variable keys are write-once. Any second attempt to set an existing key must
   produce a fatal runtime configuration error at the exact assignment point.
   The original value must remain unchanged and APIHydra must stop the suite
   immediately.
13. Variables may be referenced only from a request `body` or response
   `expected` value in the initial product.
14. Referencing a variable key that does not exist in the global store at
    substitution time must produce a fatal runtime configuration error. APIHydra
    must identify the missing key, stop immediately, and return exit code `2`.
    Keys storing `null`, `false`, `0`, or an empty JSON string are present and
    must not be treated as missing.
15. Variable keys must match `[A-Za-z_][A-Za-z0-9_:]*`, allowing Redis-style
    grouped keys such as `change:id`.

### 6. Variable substitution

1. Request `body` and response `expected` values must be processed as literal
   JSON strings.
2. A whole-value variable reference uses `$<key>`, for example:

   ```yaml
   body: '{"id": $change_id}'
   ```

3. An embedded variable reference uses `${<key>}`, for example:

   ```yaml
   body: '{"path": "/changes/${change_id}"}'
   ```

4. For `$<key>`, APIHydra must insert the stored string unchanged, preserving
   the complete JSON literal.
5. For `${<key>}`, APIHydra must remove one leading and one trailing JSON double
   quote from the stored string when both are present, then insert the remaining
   string. If the stored string does not have both outer quotes, it must be
   inserted unchanged.
6. `$<key>` and `${<key>}` must recognize keys matching
   `[A-Za-z_][A-Za-z0-9_:]*`, including references such as `$change:id` and
   `${change:id}`.
7. `$$` must produce one literal `$` and must not start a variable reference.
8. APIHydra must use `jq` to validate the resulting body or expected text as JSON
   after all substitutions.
9. If jq rejects the substituted JSON, APIHydra must stop immediately and
   forward jq's exact exit code. An invalid request must not execute.

### 7. Response validation

1. A task's `response` section must be optional. When omitted, the task passes
   if curl completes successfully; APIHydra must not validate HTTP status or
   response body content.
2. APIHydra must require a valid JSON response body when the task declares any
   of `response.capture`, `response.expected`, or `response.types`. It must use
   `jq` to validate the actual response JSON.
3. When none of those JSON-based response features is present, the body may be
   empty or non-JSON and APIHydra may validate status alone.
4. If jq rejects an actual response that requires JSON processing, APIHydra must
   stop immediately and forward jq's exact exit code.
5. A task may declare accepted HTTP status codes as an array under
   `response.status`, for example `status: [200, 201]`.
6. When present, `response.status` must be a non-empty array of unique integers
   from `100` through `599`. An empty array, duplicate value, non-integer, or
   out-of-range value must produce a fatal configuration error with exit code
   `2`.
7. When `response.status` is present, the actual HTTP response status must equal
   one of the declared values. Otherwise, status validation must fail the task.
8. When `response.status` is omitted, APIHydra must accept any HTTP response
   status.
9. A status mismatch must not stop the remaining suite from executing.
10. A status mismatch must not short-circuit other response processing. When the
   response body is valid JSON, APIHydra must still run `response.capture`,
   `response.expected`, and `response.types` and collect all failures for the
   task.
11. Variables captured from a valid JSON response become available according to
   the normal variable rules even when status validation failed.
12. A task may declare value assertions under `response.expected`.
13. `response.expected` must be handled as a literal JSON string and expanded
    using the run-wide variable store before validation. APIHydra must use `jq`
    to validate the substituted expected JSON.
14. `response.expected` may contain any valid JSON value: object, array, string,
    number, boolean, or `null`.
15. When expected is an object, it is a partial assertion and may contain only
    the response members relevant to the test.
16. When expected is a top-level array, scalar, or `null`, APIHydra must compare
    it against the entire actual response rather than performing object-member
    projection.
17. In the same jq operation, APIHydra must recursively order expected object
    members alphabetically and render the result as pretty-formatted JSON.
    Compact JSON output must not be used for comparison. If jq rejects expected,
    APIHydra must stop immediately and forward jq's exact exit code.
18. For an object expectation, APIHydra must use `jq` to project the actual
    response down to the members declared by the expected JSON, recursively
    order the projected object members alphabetically, and render the result as
    pretty-formatted JSON.
19. Members present in the actual response but omitted from an object `expected`
    must be removed by the projection and must not cause a failure.
20. Every member declared in an object `expected` must exist in the actual
    response. A missing member and a member explicitly containing `null` are
    different and must produce different projected JSON.
21. APIHydra must compare the two pretty-formatted JSON documents using Git's
   diff command.
22. If Git reports a difference, exact-value validation must fail and APIHydra
   must report the Git diff to the user.
23. If Git reports no difference, exact-value validation must pass.
24. A task may declare type assertions under `response.types`.
25. A type assertion validates only that the selected response value has the
   declared target type; it does not validate the value itself.
26. Each entry in `response.types` must map a full `jq` expression selecting a
   response value to an array containing its required base type and any
   modifiers, for example:

   ```yaml
   response:
     types:
       ".change_id": [integer]
       ".project.owner_id": [uuid, optional]
       ".attempt_count": [integer, zero]
       ".previous_count": [integer, zero, optional]
   ```

27. The first array item must be exactly one base type. The initial supported
    base types must include `string`, `number`, `integer`,
    `boolean`, `object`, `array`, `null`, `datetime`, and `uuid`.
28. With no modifiers, the selected response member must exist, must not be
    `null`, must have the declared base type, and must not contain that type's
    zero value. Thus `[integer]` rejects numeric `0`.
29. The optional `zero` modifier must allow the selected member's zero value
    while retaining base-type validation. Thus `[integer, zero]` accepts `0`.
30. The optional `optional` modifier must allow the selected member to be absent
    or explicitly `null`. If the member exists with a non-null value, normal
    base-type and zero-value validation still applies.
31. The modifiers may be combined. `[integer, zero, optional]` accepts an absent
    member, `null`, `0`, or a non-zero integer, but rejects a value of another
    type.
32. A type selector may emit multiple values. APIHydra must validate every value
    emitted by the `jq` expression against the same declaration.
33. A selector emitting zero values must pass only when `optional` is present.
34. A selector emitting `null` must pass only when `optional` is present.
35. When a selector emits one or more non-null values, every value must satisfy
    the declared base type and zero-value rule; any invalid value must fail the
    assertion.
36. Zero values must be defined as follows:

    | Base type | Zero value |
    | --- | --- |
    | `string` | `""` |
    | `number` | `0` |
    | `integer` | `0` |
    | `boolean` | `false` |
    | `object` | `{}` |
    | `array` | `[]` |
    | `datetime` | Any accepted representation of the instant `0001-01-01T00:00:00Z` |
    | `uuid` | `00000000-0000-0000-0000-000000000000` |

37. `datetime` must accept values matching one of these forms:

    ```text
    YYYY-MM-DD
    YYYY-MM-DDZ
    YYYY-MM-DD±HH:MM
    YYYY-MM-DDTHH:MM:SSZ
    YYYY-MM-DDTHH:MM:SS±HH:MM
    YYYY-MM-DDTHH:MM:SS.sssZ
    YYYY-MM-DDTHH:MM:SS.sss±HH:MM
    YYYY-MM-DDTHH:MM:SS.ssssssZ
    YYYY-MM-DDTHH:MM:SS.ssssss±HH:MM
    ```

38. A datetime timezone offset may use any valid `±HH:MM` value, not only
    `+00:00`. A value containing a time component must include either `Z` or an
    offset. Fractional seconds, when present, must contain exactly three or six
    digits. A date-only value may omit its timezone.
39. `uuid` must accept valid canonical, hyphenated UUID strings of any version.
40. `null` is a special case: `[null]` requires the selector to produce a
    present `null` value. It does not require a `zero` modifier.
41. Type declaration arrays must contain exactly one supported base type in the
    first position. Only `zero` and `optional` may follow, each at most once and
    in either order.
42. An empty declaration array, unknown token, duplicate modifier, or base type
    outside the first position must produce a fatal configuration error with
    exit code `2`.
43. A task may use `expected`, `types`, or both.
44. A mismatch in status, exact-value, or type validation must fail the task and
    must identify the failed assertion clearly.

### 8. Response variable extraction

1. A task may map variable keys to `jq` expressions under `response.capture`.
2. APIHydra must apply each expression to the curl response JSON using the
   terminal `jq` application.
3. Each successful expression result must be written to the global variable
   store using the configured key.
4. Duplicate keys must be rejected according to the store's write-once rule.
5. Captured variables become visible immediately after successful extraction,
   including to `response.expected` in the producing task.
6. APIHydra must accept a non-zero `jq` exit status when it is caused by the
   filter producing the JSON value `null` or `false`; the corresponding literal
   value must still be captured.
7. Any other non-zero `jq` exit status must be treated as an operational
   external-tool failure: APIHydra must report the capture configuration context,
   stop immediately, and forward jq's exact exit code.
8. APIHydra must not reject a successful `jq` invocation because it emits an
   object, an array, or multiple JSON values. Its emitted text must be stored as
   the captured string.
9. Capture does not independently validate whether the stored string can be
   embedded into another JSON document. If later substitution makes a request
   `body` or `response.expected` invalid, its required jq validation must stop
   immediately and forward jq's exit code.

### 9. Execution order and concurrency

APIHydra executes the suite using directory barriers and parallel independent
branches:

1. Tasks within one task YAML file must execute sequentially in declaration
   order.
2. Different task YAML files in the same directory must execute in parallel.
3. All task files in a directory must finish before any immediate child
   directory begins.
4. After the parent directory finishes, all of its immediate child directories
   must execute in parallel.
5. Each child directory recursively follows the same rules: its files run in
   parallel, tasks within each file run sequentially, and its children wait for
   it to finish.
6. A task may rely on variables created by an earlier task in the same file or
   by a completed ancestor directory.
7. A task must not rely on a variable created concurrently in another file or
   sibling directory because no completion order is guaranteed between parallel
   branches.
8. Concurrent attempts to create the same variable key must not overwrite one
   another; at most one write may succeed. Detection of any duplicate write must
   trigger the fatal duplicate-assignment policy.
9. A task validation failure must not stop suite execution.
10. After validation fails, APIHydra must continue later sequential tasks in the
    same file, other task files, and all child and sibling directories according
    to the normal scheduling rules.
11. APIHydra must collect validation failures throughout the run and return exit
    code `101` after the entire suite finishes when at least one validation
    failed.
12. Pre-execution configuration errors that prevent the suite from being
    resolved remain fatal and must prevent task execution.
13. An operational failure from curl, jq, Git, or another external tool must be
    fatal. APIHydra must stop execution immediately and forward that tool's exact
    non-zero exit code.
14. A non-zero tool status with defined semantic meaning must follow that
    meaning instead of the operational-failure rule. In particular, Git diff
    status `1` means a validation mismatch, and the accepted jq status for a
    `null` or `false` result is not an error.
15. Duplicate variable assignment is a fatal runtime configuration error, not a
    recoverable validation failure. It must stop execution when detected.
16. Missing variable references are fatal runtime configuration errors and must
    stop execution when detected.
17. When any fatal error occurs during parallel execution, APIHydra must stop
    scheduling work, cancel all in-flight external processes, wait for them to
    terminate, and return the original fatal error code. Cancellation outcomes
    must not replace that original code.

An example execution tree is:

```text
root task files (parallel; each file internally sequential)
  -> wait for root completion
  -> child directories (parallel)
       -> each child's task files (parallel; each file internally sequential)
       -> wait for that child
       -> that child's child directories (parallel)
```

### 10. Errors and observability

1. APIHydra must distinguish configuration errors, runtime-resolution errors,
   curl execution errors, and response-validation failures.
2. Errors must identify the relevant file and task whenever applicable.
3. Configuration errors must be detected before any task execution when they
   can be found during suite loading and resolution.
4. Terminal output must be structured and consistent enough for AI agents to
   interpret reliably.
5. Terminal output must remain concise and readable enough for a human to trace
   directory, file, task, request, and validation context.
6. The command must return a non-zero exit status when configuration,
   execution, or validation fails.
7. Task validation errors must be reported without preventing the rest of a
   successfully resolved suite from running.
8. Exit codes must follow this contract:

   - `0`: the suite completed with no validation failures.
   - `2`: invocation, discovery, YAML, configuration, or filter-selection error;
     fatal immediately. This includes duplicate variable assignment detected
     during task execution and missing variable references detected during
     substitution.
   - `3`: missing external dependency or internal APIHydra failure; fatal
     immediately.
   - `101`: one or more task validation failures; returned after the complete
     suite runs.
   - Any operational non-zero exit from an invoked external tool: forwarded
     exactly and fatal immediately.
9. Exact error-message formatting is deferred to a later product decision.

### 11. Terminal and machine-readable output

1. Default output must be concise, human-readable terminal text.
2. APIHydra must support `--json` for agent-oriented machine-readable output.
3. `--json` output must use newline-delimited JSON: each output line must be one
   complete, independently parseable JSON event object.
4. JSON events must stream as execution progresses rather than being emitted
   only after the whole suite completes.
5. The event stream must represent suite lifecycle, task results, errors, and a
   final suite summary.
6. Parallel events may appear in actual completion order. Each event must carry
   enough file and task identity to associate it with the correct task.
7. In JSON mode, standard output must not contain ANSI escape codes or
   human-formatted prose outside JSON event objects.
8. Failure details, including Git diffs from expected-value validation, must be
   represented as JSON string fields in the relevant event.
9. The final summary event must state whether the suite passed and provide
   enough counts to determine how many tasks passed and failed.

## Acceptance scenarios

### Root config is required

Given a selected suite root with task files but no config file, when `apih` is
run, then it reports a configuration error and executes no curl commands.

Given a valid root config but zero tasks anywhere in an unfiltered suite, when
`apih` is run, then it reports an error, executes no curl commands, and returns
exit code `2`.

### Unrelated YAML files are ignored

Given YAML files without `app: apihydra`, when the suite is discovered, then
APIHydra ignores them. Given a file with `app: apihydra` but no supported
`kind`, APIHydra reports a configuration error and executes no curl commands.

Given an APIHydra file containing multiple YAML documents separated by `---`,
when the suite is loaded, then APIHydra reports a configuration error and
executes no curl commands.

Given an APIHydra document containing an unknown or misspelled field, when the
suite is loaded, then APIHydra reports the file and field path, returns exit code
`2`, and executes no curl commands.

Given an APIHydra document containing a duplicate mapping key, when the suite is
loaded, then APIHydra identifies the file and duplicated key, returns exit code
`2`, and executes no curl commands.

### Metadata is optional

Given APIHydra documents without metadata, when an unfiltered suite is run, then
all tasks execute normally. Given optional names or labels, they affect task
selection only during a filtered run.

### Filter by name and labels

Given tasks documents with optional metadata, when `apih -n create-tasks` is
run, then only the document named `create-tasks` is selected. When
`apih -l create -l smoke` is run, only documents containing both labels are
selected. All config documents required by the selected tasks remain available
for resolution.

Given filters that match no tasks documents, APIHydra reports an error, executes
no curl commands, and returns exit code `2`.

### External-tool preflight

Given selected tasks that require curl, jq, or Git, when any required executable
is unavailable, then APIHydra identifies it, executes no requests, and returns
exit code `3`. The absence of `yq` does not affect the initial product.

### One config per directory

Given any directory containing two config files, when `apih` loads the suite,
then it reports the conflicting files as a configuration error and executes no
curl commands.

### Configuration inheritance

Given a root config with `baseUrl`, `basePath`, and headers, and a child config
that overrides `basePath`, when the child config searches its ancestor
directories and resolves the root config as its nearest parent, then a task in
the child uses the root `baseUrl` and inherited headers together with the child
`basePath`.

Given a parent header named `content-type` and a child header named
`Content-Type`, when the child runtime configuration is resolved, then only the
child value remains and the header is emitted as `Content-Type`.

### Directory configuration applies to local tasks

Given multiple task files in one directory, when they are resolved, then every
task uses the same directory `RuntimeConfiguration` except where a task defines
its own value.

### Request method defaults

Given a task without an explicit method or body, when it is resolved, then its
runtime method is `GET`. Given a task without an explicit method but with a body,
its runtime method is `POST`. Given any explicit method, the explicit value is
used regardless of body presence.

### Timeout and retry defaults

Given no configured or task-defined timeout or retry count, when a runtime task
is resolved, then it uses a 10-second timeout and 3 retries. Given inherited
values, the task uses them; given task-level values, they override the inherited
values and are passed to curl.

### URL composition

Given `baseUrl: https://api.example.com`, `basePath: /api/v1`, `path: /users`,
and `query: page=1&limit=20`, when the runtime request is built, then its URL is
`https://api.example.com/api/v1/users?page=1&limit=20`. Given no `basePath` or
`query`, only `baseUrl + path` is used. Given a present but empty `basePath` or
`query`, runtime-task validation fails.

Given redundant boundary slashes in `baseUrl`, `basePath`, or `path`, when the
URL is joined, then APIHydra normalizes the URL path without corrupting its
scheme or authority. URL failures not rejected by standard URL handling are
reported through the normal curl execution failure.

### Request variable assignment and use

Given one task with `vars: {change_id: 1}`, when a later task uses
`$change_id` in its request body, then APIHydra replaces the reference with the
literal `1` and validates the resulting body as JSON before executing curl.

Given JSON-compatible scalar, array, object, or null values under `task.vars`,
when the task is loaded, then each value is serialized as compact JSON and
stored as a string. A value that cannot be represented as JSON causes a
configuration error.

### Embedded variable interpolation

Given a stored value of `date -> "2026-01-01"`, when a JSON string contains
`"Date: ${date}"`, then APIHydra removes the stored value's outer quotes and
produces the JSON string `"Date: 2026-01-01"`.

Given a stored `change:id` variable, `$change:id` and `${change:id}` resolve it.
Given `$$` in a body or expected string, substitution produces a literal `$`.

### Response extraction and later use

Given a task with `response.capture: {change_id: .id}`, when curl returns and
the capture succeeds, then the same task's `response.expected` and later
sequential tasks may reference the stored JSON literal through `$change_id`.

### Variables are write-once

Given a store that already contains `change_id`, when any task tries to set
`change_id` again, then APIHydra reports a fatal error at that assignment,
preserves the original value, stops the suite immediately, and returns exit code
`2`.

Given a body or expected value referencing a key absent from the global store,
when substitution reaches that reference, then APIHydra identifies the missing
key, stops immediately, and returns exit code `2`. A present key whose value is
`null`, `false`, `0`, or `""` does not trigger this error.

### Expected response is a subset

Given an actual response of `{"id":1,"name":"A","active":true}` and an
expected value of `{"id":1}`, when the response is validated, then exact-value
validation projects actual to `{"id":1}`, pretty-formats both documents, finds
no Git diff, and passes.

Given a top-level array, scalar, or `null` under `response.expected`, when the
response is validated, then APIHydra pretty-normalizes and compares the entire
actual JSON response with the expected value.

### HTTP status validation

Given `response.status: [200, 201]`, when curl returns status `201`, then status
validation passes; when curl returns status `400`, the task fails, the failure
identifies the unexpected status, and the remaining suite continues.

Given a status-only task and a `204 No Content` response, when the task is
validated, then the empty body is accepted. Given the same empty body with
`capture`, `expected`, or `types` configured, jq rejects the response, APIHydra
stops immediately, and jq's exit code is forwarded.

Given a task without a `response` section, when curl exits successfully, then
the task passes without inspecting its HTTP status or body.

### Status failure does not skip body validation

Given a status mismatch and a valid JSON response body, when the response is
processed, then APIHydra records the status failure, still performs capture,
expected-value comparison, and type validation, and reports every failure found
for the task.

### Exact-value mismatch

Given an actual response of `{"id":2}` and an expected value of `{"id":1}`,
when the response is validated, then Git reports a diff and the task fails with
that diff.

Given invalid substituted JSON under `response.expected`, when jq validation
runs, then APIHydra stops immediately and forwards jq's exact exit code.

### Missing and null expected members differ

Given an expected value of `{"deleted_at":null}` and an actual response of
`{}`, when the response is projected and compared, then Git reports a diff and
validation fails. A type assertion using the `optional` modifier is the
mechanism for accepting either a missing member or an explicit `null`.

### Type-only validation

Given the response type assertion `".id": [number]`, when the actual response
contains a non-zero numeric `id`, then that assertion passes regardless of the
number's specific value.

Given `".items[].id": [integer]`, when the selector emits multiple values, then
every value is validated and any non-integer or zero value fails the assertion.
Given an empty result or `null`, the assertion passes only when it includes the
`optional` modifier.

### Zero and optional type modifiers

Given the response type assertion `".change_id": [integer, zero, optional]`,
when `change_id` is absent, `null`, `0`, or a non-zero integer, then validation
passes; when it is present with a value of another type, validation fails.

### Zero-value validation

Given `".items": [array]`, when the selector produces `[]`, then validation
fails; given `".items": [array, zero]`, the same value passes.

### Datetime validation

Given a `datetime` assertion, values such as `2026-01-01`, `2026-01-01Z`,
`2026-01-01+00:00`, `2026-01-01T00:00:00Z`,
`2026-01-01T00:00:00.000+00:00`, and
`2026-01-01T00:00:00.000000Z` pass format validation. A timestamp with a time
component but no timezone, or with a fractional-second precision other than
three or six digits, fails.

### File and directory scheduling

Given two task files in the root and two child directories, when the suite runs,
then the root files run concurrently, each file's tasks retain declaration
order, neither child starts until both root files finish, and both children may
then run concurrently.

### Validation failures do not stop the suite

Given a task that fails status, expected-value comparison, or type validation,
when other tasks remain anywhere in the resolved suite, then APIHydra records
the failure, executes every remaining task according to the normal schedule,
and returns exit code `101` after the suite completes.

### External-tool failures are forwarded

Given curl, jq, Git, or another invoked tool that terminates with an operational
non-zero exit code, when APIHydra receives that code, then it stops immediately
and returns the exact same code. Git diff status `1` representing a comparison
difference and the accepted jq status for `null` or `false` retain their defined
semantic handling instead.

Given a fatal error while other parallel external processes are running, when
fatal shutdown begins, then APIHydra schedules no new work, cancels the in-flight
processes, waits for them to terminate, and returns the code from the original
fatal error rather than a cancellation code.

### JSON event output

Given `apih --json`, when the suite runs, then every standard-output line is a
valid JSON event without ANSI formatting, task events identify their source file
and task, failures contain structured details, and the final event summarizes
passed and failed task counts.

## Open product decisions

The following details require an explicit product decision before their related
schema or behavior can be considered implementation-ready:

1. **CLI contract:** Exact human and JSON output fields remain to be defined.
   Exact error formatting is intentionally deferred until the user supplies it.
