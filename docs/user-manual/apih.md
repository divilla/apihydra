# APIHydra (`apih`) user manual

APIHydra is an agent-first API integration tester. An `apih` suite is a
directory tree of YAML definitions: root and defaults documents configure
requests, while steps documents send HTTP requests and validate responses.

This manual is written for coding agents first and humans too. Rules are stated
literally, reference tables are exhaustive, and examples are designed to be
copied.

## Contents

- [Quick start](#quick-start)
- [CLI reference](#cli-reference)
- [Suites, stages, and parallelism](#suites-stages-and-parallelism)
- [YAML document reference](#yaml-document-reference)
- [Defaults and request construction](#defaults-and-request-construction)
- [Variables and captures](#variables-and-captures)
- [Response validation](#response-validation)
- [Cookies](#cookies)
- [Debug breakpoints](#debug-breakpoints)
- [Output, failures, and temporary data](#output-failures-and-temporary-data)
- [Complete runnable suite](#complete-runnable-suite)
- [Task recipes](#task-recipes)
- [Troubleshooting](#troubleshooting)
- [Agent checklist](#agent-checklist)

## Quick start

### Prerequisites

| Program | When `apih` needs it |
| --- | --- |
| Go 1.25 or newer | To build `apih` or run it from this repository. |
| `curl` | For every HTTP request. |
| `jq` | For every step's type-validation phase, response capture and body processing, and Debug JSON formatting. |
| `git` | When a body mismatch must be rendered as a colored diff. |

Build the command from the repository root:

```sh
mkdir -p bin
go build -o ./bin/apih ./cmd/cli
./bin/apih --help
```

For development, `go run ./cmd/cli` can replace `./bin/apih` in every command
below.

### Minimal suite

Assume an HTTP service at `http://127.0.0.1:8080` responds to `GET /health`
with status `200` and body `{"ok":true}`. Create this directory:

```text
quick-start/
├── root.yaml
└── health-steps.yaml
```

`quick-start/root.yaml`:

```yaml
app: apihydra
kind: root
spec:
  base_url: http://127.0.0.1:8080
```

`quick-start/health-steps.yaml`:

```yaml
app: apihydra
kind: steps
metadata:
  name: health
spec:
  steps:
    - request:
        method: GET
        path: /health
      response:
        expected_status: 200
        expected_body: '{"ok":true}'
```

Run it from the directory containing `quick-start`:

```sh
./bin/apih quick-start
```

Exit code `0` means the request ran and all configured validations passed.

## CLI reference

### Synopsis

```text
apih [flags] [directory]
```

With no positional `directory`, `apih` uses the current working directory. A
relative positional directory is joined to the current working directory and
must resolve to a directory. At most one positional directory is accepted.

| Flag | Default | Effect |
| --- | ---: | --- |
| `-h`, `--help` | — | Print pflag-generated usage to stdout, exit `0`, and do not start a run. |
| `-p`, `--parallelism` | `1` | Select execution mode `0`, `1`, or `2`. |

Argument parsing uses native pflag behavior:

- short attached and separate forms work: `-p0`, `-p 0`;
- long equals and separate forms work: `--parallelism=2`, `--parallelism 2`;
- flags may be interspersed with the positional directory;
- a repeated flag uses its final value; and
- `--` ends flag parsing, so `apih -- -suite` treats `-suite` as the directory.

Examples:

```sh
apih
apih suites/smoke
apih -p0 suites/stateful
apih suites/smoke --parallelism=2
apih -p1 --parallelism 0 suites/stateful  # final value, 0, wins
apih -- -suite
```

Unknown flags, malformed values, parallelism outside `0..2`, and extra
positional arguments are configuration failures. They produce no application
stdout; the CLI writes the fatal diagnostic to stderr.

### Exit codes

| Code | Meaning | Error value |
| ---: | --- | --- |
| `0` | Success, including a successful Debug breakpoint. | No error. |
| `101` | One or more response validations failed. Eligible work completed and validation details were reported to stdout. | No final fatal error. |
| `102` | Invocation, selected-directory, YAML, definition, or other configuration failure. | Fatal diagnostic on stderr. |
| `103` | Internal execution, command, storage, validation-tool, or reporting failure without a more specific nonzero code. | Fatal diagnostic on stderr. |

Some terminal external-command paths preserve that command's nonzero exit code
instead of converting it to `103`. Treat any other nonzero code as terminal and
use the final stderr diagnostic to identify the failed operation.

## Suites, stages, and parallelism

`apih` recursively builds a directory tree from the selected suite directory.
It reads regular files whose names end in lowercase `.yaml` or `.yml`.
Directory depth determines execution stage: the selected directory is stage
`0`, its direct children are stage `1`, and so on.

Only steps documents contain executable requests. Root and defaults documents
provide defaults inherited by steps in their directory and descendants.

```text
suite/                              stage 0
├── root.yaml                       root defaults
├── auth-steps.yaml                 executable steps file
├── health-steps.yml                executable steps file
└── billing/                        stage 1; inherits stage 0 defaults
    ├── defaults.yaml               directory override
    ├── invoice-steps.yaml          executable steps file
    └── refunds/                    stage 2; inherits billing defaults
        └── refund-steps.yaml       executable steps file
```

Use a root document at the suite root and a defaults document for a nested
directory. Both have the same defaults shape. Keep at most one root/defaults
document in a directory for predictable suites: placement and cardinality when
multiple default-bearing documents coexist are not a published contract.

### Stage and parallelism rules

Stages always execute sequentially, with a full join barrier before the next
stage starts.

| Mode | Directories in one stage | Steps files in one directory | Steps in one file | Cookie owner |
| ---: | --- | --- | --- | --- |
| `0` | Serial | Serial | Serial | One jar for the run. |
| `1` (default) | Concurrent | Serial | Serial | One jar per directory. |
| `2` | Concurrent | Concurrent | Serial | One jar per steps file. |

Directory and file task sets have no worker-count limit. All modes share one
concurrent, write-once variable store. Use mode `0` when a producer and
consumer in different files or directories must execute in deterministic
order. A parent-stage capture is available to a child stage after the barrier,
provided no competing write uses the same name.

The order in which concurrently eligible files reach Debug is unspecified.
Do not build a suite whose correct result depends on which concurrent Debug
step wins.

## YAML document reference

### Common envelope

Definitions use the following envelope. `spec` changes shape by document kind.

| YAML key | Value shape | Scope/default | Effect |
| --- | --- | --- | --- |
| `app` | String; use `apihydra` | Document; empty if omitted | Identifies the APIHydra definition vocabulary. |
| `kind` | `root`, `defaults`, or `steps` | Document; required for useful classification | Selects the `spec` shape and whether the document configures or executes. |
| `metadata.name` | String | Root, defaults, or steps document; empty if omitted | Descriptive definition name and reporting identity where applicable. |
| `metadata.labels` | List of strings | Root, defaults, or steps document; empty if omitted | Descriptive labels. No CLI name/label filter is currently defined. |
| `spec` | Mapping | Document | Contains defaults for `root`/`defaults`, or defaults plus steps for `steps`. |

### Root and defaults documents

A root document and a defaults document decode to the same defaults structure.
The conventional difference is location and intent:

```yaml
app: apihydra
kind: defaults
metadata:
  name: billing-defaults
  labels: [billing]
spec:
  base_url: https://api.example.test
  base_path: /v2
  headers:
    Accept: application/json
  disable_cookies: false
  timeout: 10
  retries: 3
```

| YAML key | Value shape | Scope/default or inheritance | Effect |
| --- | --- | --- | --- |
| `spec.base_url` | String | Every defaults scope; empty inherits | First part of the request URL. |
| `spec.base_path` | String | Every defaults scope; empty inherits | Middle part of the request URL. |
| `spec.headers` | Map of string to string | Every defaults scope; keys merge; narrower keys replace inherited keys | Supplies HTTP request headers. |
| `spec.disable_cookies` | Boolean | Every defaults scope; absent inherits; globally absent leaves automatic cookies enabled | `true` disables curl's automatic cookie engine; `false` explicitly enables or re-enables it. |
| `spec.timeout` | Integer seconds | Every defaults scope; zero inherits; initial fallback `10` | Passed to curl as its maximum request time. |
| `spec.retries` | Integer | Every defaults scope; zero inherits; initial fallback `3` | Passed to curl as its retry count. |

`timeout: 0` and `retries: 0` do not override an inherited nonzero value. They
therefore cannot be used at a narrower scope to disable an inherited timeout
or retry count.

### Steps documents

A steps document has optional file-level defaults and an ordered list of
steps:

```yaml
app: apihydra
kind: steps
metadata:
  name: item-flow
  labels: [items, smoke]
spec:
  defaults:
    base_path: /v2
    headers:
      Accept: application/json
  steps:
    - vars:
        item_name: "demo"
      request:
        method: POST
        path: /items
        query: source=manual
        body: '{"name":"$item_name"}'
        defaults:
          headers:
            Content-Type: application/json
      response:
        expected_status: 201
        expected_body: '{"id":7,"name":"demo"}'
        expected_types:
          .id: [int]
          .name: [string]
        capture:
          item_id: .id
      debug: false
```

| YAML key | Value shape | Scope/default or inheritance | Effect |
| --- | --- | --- | --- |
| `spec.defaults` | Defaults mapping | Steps file; inherits directory defaults | Overlays the complete defaults shape for all steps in this file. |
| `spec.steps` | Ordered list of step mappings | Steps file; empty if omitted | Requests execute serially in this order within the file. |
| `spec.steps[].vars` | Map of string to string | Step; empty if omitted; names enter the run-wide write-once store | Loads literal values before interpolation. Quote numeric and Boolean-looking values so YAML decodes them as strings. |
| `spec.steps[].request.path` | String | Step; empty if omitted | Final path portion of the request URL. |
| `spec.steps[].request.method` | String | Step; empty lets curl infer its method | Selects the HTTP method. `HEAD` uses curl's header-only mode. |
| `spec.steps[].request.query` | String such as `a=1&b=2` | Step; empty if omitted | Appended as the URL query. Supply a valid query fragment. |
| `spec.steps[].request.body` | String | Step; empty sends no data argument | Request body; `$name` and `${name}` placeholders are interpolated. Use a quoted scalar or block scalar for JSON. |
| `spec.steps[].request.defaults` | Defaults mapping | Step; inherits steps-file defaults | Final defaults overlay for this request. |
| `spec.steps[].response.expected_status` | Integer | Step; zero or omitted means any status | Requires exact equality when nonzero. |
| `spec.steps[].response.expected_body` | String containing JSON | Step; empty or omitted skips body validation | Declares the expected JSON shape and values after interpolation. |
| `spec.steps[].response.expected_types` | Map of jq selector to list of type declarations | Step; empty if omitted | Validates selected actual response values against allowed types. |
| `spec.steps[].response.capture` | Map of variable name to jq selector string | Step; empty if omitted | Extracts compact jq results from the actual body into the shared store after validation. |
| `spec.steps[].debug` | Boolean | Step; `false` if omitted | Makes this step a terminal, complete, unredacted Debug breakpoint. |

Every defaults mapping—directory `spec`, steps-file `spec.defaults`, and step
`request.defaults`—has exactly the same six fields: `base_url`, `base_path`,
`headers`, `disable_cookies`, `timeout`, and `retries`.

### Runtime-owned fields

Do not set these as suite input:

- `index` is the zero-based runtime step index and appears in Debug JSON;
- `response.actual_status` and `response.actual_body` are assigned from curl's
  response;
- the raw Curl statement is created only for Debug; and
- source-definition provenance is attached internally for reporting errors.

The raw Curl statement and source-definition object are omitted from the Debug
step JSON. The Curl statement is printed separately.

## Defaults and request construction

Defaults overlay from broadest to narrowest:

```text
parent directory
  -> current directory root/defaults document
  -> steps-file spec.defaults
  -> step request.defaults
```

For `base_url`, `base_path`, `timeout`, and `retries`, a nonempty/nonzero local
value replaces the inherited value. Headers merge by key. For
`disable_cookies`, presence matters: absent inherits, `true` disables, and
`false` explicitly re-enables.

### Worked inheritance example

Given these overlays:

```yaml
# Root document spec
base_url: https://api.example.test
base_path: /v1
headers:
  Accept: application/json
  X-Scope: root
disable_cookies: false
timeout: 20
retries: 3
```

```yaml
# Child defaults document spec
base_path: /v2
headers:
  X-Scope: child
  X-Child: "yes"
disable_cookies: true
```

```yaml
# Steps-file spec.defaults
headers:
  X-File: smoke
timeout: 5
```

```yaml
# Individual step request.defaults
headers:
  X-Step: create
disable_cookies: false
retries: 1
```

The step resolves to:

| Field | Effective value | Reason |
| --- | --- | --- |
| `base_url` | `https://api.example.test` | Inherited from root. |
| `base_path` | `/v2` | Child overrides root. |
| `headers.Accept` | `application/json` | Inherited root key. |
| `headers.X-Scope` | `child` | Child replaces the root key. |
| `headers.X-Child` | `yes` | Added by child. |
| `headers.X-File` | `smoke` | Added by steps file. |
| `headers.X-Step` | `create` | Added by step. |
| `disable_cookies` | `false` | Step explicitly re-enables below the child `true`. |
| `timeout` | `5` | Steps file overrides root. |
| `retries` | `1` | Step overrides root. |

### Request construction

The effective URL starts as literal concatenation:

```text
resolved base_url + resolved base_path + request.path
```

`request.query` is then added as the query portion. Supply slashes and query
encoding deliberately; `apih` does not define a URL-normalization contract.

An empty method lets curl infer the method: with no body curl selects `GET`,
and with body data it selects `POST`. An explicit `HEAD` uses curl's header-only
mode and returns an empty response body to validation. Other nonempty methods
are passed as the requested method.

Resolved headers become curl headers. An empty body adds no curl data argument.
A nonempty body is always supplied as request standard input; its Debug
presentation also follows the inline/`@-` rules described under
[Debug breakpoints](#debug-breakpoints).

## Variables and captures

One invocation has one concurrent, write-once string store. Variable phases
for each step are:

1. Load every `vars` entry.
2. Interpolate `$name` and `${name}` in `request.body`.
3. Interpolate the same forms in `response.expected_body`.
4. Execute the request and all response validations.
5. Evaluate each `response.capture` jq selector against
   `response.actual_body` and store its compact result.

Interpolation is textual and applies only to the request body and expected
body—not URLs, query strings, headers, selectors, or defaults. A captured JSON
number such as `7` is stored as `7`; a captured JSON string such as `"open"` is
stored with its JSON quotes because capture uses compact jq output.

The store rejects a second write to an existing name and preserves the first
value. Define a variable before use, use unique names across `vars` and
captures, and use parallelism `0` for deterministic cross-file dependencies.
The exact missing-variable diagnostic, variable-name grammar beyond the two
placeholder forms, escaping, and replacement precedence are not stable product
contracts.

### Capture and reuse in two serial steps

The following steps are in one steps file, so their order is serial:

```yaml
spec:
  steps:
    - request:
        method: POST
        path: /items
        body: '{"name":"demo"}'
      response:
        expected_status: 201
        capture:
          item_id: .id

    - request:
        method: POST
        path: /items/confirm
        body: '{"id":${item_id}}'
      response:
        expected_status: 200
        expected_body: '{"id":$item_id,"confirmed":true}'
```

If the first response body is `{"id":7}`, the second request body becomes
`{"id":7}` and its expected body becomes `{"id":7,"confirmed":true}`.

## Response validation

Every completed request runs type, status, and body validation as independent
phases. All mismatches found for a step are reported. A mismatch is nonfatal:
eligible work continues and the final exit code is `101`. A jq, Git, curl,
parsing, storage, or reporting error is terminal instead.

### Type validation

`response.expected_types` maps a jq selector evaluated against the actual JSON
body to alternative allowed declarations.

| Declaration | Accepted actual JSON value |
| --- | --- |
| `null` | JSON null. |
| `boolean` | `true` or `false`. |
| `number` | Any JSON number. |
| `string` | Any JSON string. |
| `array` | Any JSON array. |
| `object` | Any JSON object. |
| `int` | Any integer-valued JSON number, including zero. |
| `zero` | Numeric `0`. |

Multiple declarations are a union: any one may match.

```yaml
response:
  expected_types:
    .id: [int]
    .version: [int, zero]
    .owner: [string, null]
    .active: [boolean]
    .tags: [array]
    .metadata: [object]
```

Selectors and declaration behavior beyond the listed current forms are not a
published validation language. An invalid selector or invalid actual JSON
prevents validation from continuing and is terminal.

### Status validation

Omitted `expected_status` and explicit `expected_status: 0` mean `<any>`: every
actual HTTP status is accepted. Any nonzero value requires exact equality.

```yaml
# Accept any actual status.
response:
  expected_status: 0
```

```yaml
# Require exactly 204.
response:
  expected_status: 204
```

### Body validation

An omitted or whitespace-only `expected_body` skips body validation. A
nonempty body must contain valid JSON.

For object bodies, `apih` recursively projects the actual response to fields
declared by the expected response, recursively sorts keys, and compares the
normalized values. Extra actual object fields are ignored. Missing expected
fields are not synthesized, including fields expected to be `null`, so they
fail validation. Arrays are projected recursively when expected and actual
arrays have the same length; a different length remains a mismatch. Scalars
must match after JSON normalization.

For example, this expectation passes for actual body
`{"id":7,"ok":true,"server_time":"ignored"}`:

```yaml
response:
  expected_body: '{"id":7,"ok":true}'
```

These focused mismatch cases are illustrative:

```yaml
# Type mismatch when .id is "7" instead of numeric 7.
response:
  expected_types:
    .id: [int]
```

```yaml
# Status mismatch when the actual status is not 201.
response:
  expected_status: 201
```

```yaml
# Body mismatch when .ok is false or absent.
response:
  expected_body: '{"ok":true}'
```

Body mismatches are rendered as corrections from actual to expected: red lines
are actual values to remove or replace, and green lines are expected values to
add. Rendering this diff requires `git`.

## Cookies

Automatic cookies are enabled when every scope omits `disable_cookies`.
`disable_cookies` controls only curl's automatic cookie engine. It never
removes or changes an explicit `Cookie` header.

```yaml
# Disable automatic cookies for a steps file.
spec:
  defaults:
    disable_cookies: true
```

```yaml
# Explicitly re-enable for one request.
request:
  path: /account
  defaults:
    disable_cookies: false
```

```yaml
# An explicit Cookie header remains even while automatic cookies are disabled.
request:
  path: /account
  defaults:
    disable_cookies: true
    headers:
      Cookie: manual_session=example
```

Each invocation creates fresh cookie state below its private run directory.
Separate invocations never discover, read, copy, or reuse each other's jars,
including jars abandoned by abrupt termination. A request with cookies
disabled does not read or update its owning jar; the jar remains unchanged so
a later request can re-enable and resume it. Every mode-owned jar is created
even when all of its current steps disable cookies, because its unchanged state
may still be inherited.

### Cookie ownership by parallelism

| Mode | Ownership and inheritance |
| ---: | --- |
| `0` | Exactly one run jar. All steps use it serially. Stage transitions do not copy it. |
| `1` | Exactly one jar per directory. The root starts empty. After a parent stage joins, every direct child receives a distinct byte-for-byte copy of its parent's final jar. Siblings never share a writable jar. |
| `2` | Exactly one jar per steps file. Root files start empty. After every step completes—including a cookie-disabled step—the file's jar becomes that directory's latest-completed source. Every child file receives its own copy of the source selected after the stage joins. |

In mode `2`, actual runtime completion order selects the parent source. When
multiple parent files complete concurrently, the cookie state inherited by
children is intentionally nondeterministic. Modification timestamps are not
used and multiple jars are never merged.

A directory with no executed steps preserves its incoming state for children.
This remains true across chains of empty directories and empty steps files.
In mode `2`, a root with no steps files creates an empty inheritance jar so a
later descendant still starts from defined empty cookie state.

## Debug breakpoints

`debug: true` makes a step a terminal breakpoint. The request still executes,
its actual response is assigned, validations and captures run, and then the
latest step state is printed. After a successful Debug dump, no later step or
stage executes and the process exits `0`.

If the Debug step encounters a terminal error, `apih` prints the latest state
available and retains the error and its nonzero exit code. A concurrent Debug
winner is unspecified.

> **Warning:** Debug output is complete and unredacted. It can expose
> authorization headers, cookies, request bodies, response bodies, captured
> values, and other secrets.

```yaml
app: apihydra
kind: steps
metadata:
  name: inspect-account
spec:
  steps:
    - request:
        method: GET
        path: /account
        defaults:
          headers:
            Authorization: Bearer example-secret
      response:
        expected_status: 200
      debug: true
```

The Debug layout is exact in structure:

```text
stage: <directory-stage>
dir-path: <directory-path>
file-path: <steps-file-path>

curl-command:
<complete-unredacted-curl-statement>

<prettified-and-ANSI-colored-step-JSON>
```

The JSON includes the step index, variables, request, fully resolved request
defaults, expected and actual response values, captures, and Debug flag. Valid
JSON strings in the request, expected-response, and actual-response bodies are
displayed as structured JSON; empty or invalid strings remain strings.

### Copying the Debug Curl statement

The displayed executable and arguments are derived from the exact executable
and argument list used for the request; rendering never mutates execution.

- Values after `--header` and `--data-binary` are POSIX single-quoted.
- Embedded single quotes use the POSIX `'\''` sequence.
- A nonempty body of at most 1,024 Unicode characters appears as the final
  `--data-binary` value. Valid JSON is compacted with jq for display; invalid
  JSON or a jq failure retains the original value.
- A longer body uses final value `@-`. The original body was supplied on stdin,
  so copying that command alone does not recreate the body; pipe or redirect
  the body into curl.
- An empty body adds no `--data-binary` argument.
- Enabled automatic cookies expose the selected run-local path in both
  `--cookie` and `--cookie-jar`.

## Output, failures, and temporary data

Logical stdout order is always stage, directory, steps file, then step, using
the suite plan and slice order. Runtime completion order does not reorder the
final logical output.

On a terminal, Reporter clears and redraws only the active-stage region as
events arrive. The working-directory heading and completed stages stay fixed.
When stdout is not a terminal, `apih` buffers the stage and writes it once at
the stage barrier. Success and validation output stay grouped under the owning
steps file.

A terminal error cancels and joins active work, commits the accumulated stage
output in canonical order, and prevents later work. The CLI then writes the
provenance-bearing fatal diagnostic to stderr. That diagnostic is the final
application output; cleanup failures never add diagnostics.

### Private run directory

Every valid run creates a private directory matching
`os.UserCacheDir()/apih/run-*`. Curl cookie jars and Git body-diff operations
use separate namespaces below it. Runtime consumers do not use a shared OS
temporary directory or process-wide temporary-directory override.

Every controlled return attempts best-effort removal of the complete run
directory, and cleanup errors are silently suppressed. Abrupt process or
machine termination may leave it behind. Later runs do not scavenge the old
directory and never reuse its cookie state.

Help and argument-parsing failures do not start a valid run and therefore do
not create the run directory.

## Complete runnable suite

This example covers every declarative field. It is runnable once an HTTP
fixture is listening on `127.0.0.1:18080` with this contract:

| Request | Required response |
| --- | --- |
| `POST /api/session?source=manual` with `{"name":"manual"}` | Status `201`, `Set-Cookie: sid=example`, body `{"id":7,"session":"ready","extra":"allowed"}`. |
| `POST /api/confirm` with `{"id":7}` | Status `200`, body `{"id":7,"confirmed":true}`. |
| `GET /api/health` | Status `200`, body `{"ok":true}`. |
| `POST /v2/items?source=child` with `{"id":7}` and inherited cookie `sid=example` | Status `201`, body `{"id":7,"ok":true}`. |

Run it with the default mode `1`. Root-stage files are serial within the root
directory, the entire root stage joins, and then the child stage begins. The
captured `item_id` and parent cookie are therefore available to the child.

```text
manual-suite/
├── root.yaml
├── session-steps.yaml
├── health-steps.yml
└── items/
    ├── defaults.yaml
    └── item-steps.yaml
```

<!-- complete-yaml: manual-suite/root.yaml -->
`manual-suite/root.yaml`:

```yaml
app: apihydra
kind: root
metadata:
  name: manual-root
  labels: [manual, complete]
spec:
  base_url: http://127.0.0.1:18080
  base_path: /api
  headers:
    Accept: application/json
    X-Suite: manual
  disable_cookies: false
  timeout: 10
  retries: 3
```

<!-- complete-yaml: manual-suite/session-steps.yaml -->
`manual-suite/session-steps.yaml`:

```yaml
app: apihydra
kind: steps
metadata:
  name: session-flow
  labels: [manual, session]
spec:
  defaults:
    base_url: http://127.0.0.1:18080
    base_path: /api
    headers:
      Content-Type: application/json
      X-File: session
    disable_cookies: true
    timeout: 8
    retries: 2
  steps:
    - vars:
        session_name: "manual"
        expected_id: "7"
      request:
        path: /session
        method: POST
        query: source=manual
        body: |
          {"name":"$session_name"}
        defaults:
          base_url: http://127.0.0.1:18080
          base_path: /api
          headers:
            X-Step: create-session
          disable_cookies: false
          timeout: 5
          retries: 1
      response:
        expected_status: 201
        expected_body: |
          {"id":$expected_id,"session":"ready"}
        expected_types:
          .id: [int, zero]
          .session: [string]
        capture:
          item_id: .id
      debug: false

    - request:
        method: POST
        path: /confirm
        body: |
          {"id":${item_id}}
      response:
        expected_status: 200
        expected_body: |
          {"id":$item_id,"confirmed":true}
```

<!-- complete-yaml: manual-suite/health-steps.yml -->
`manual-suite/health-steps.yml`:

```yaml
app: apihydra
kind: steps
metadata:
  name: independent-health
  labels: [manual, health]
spec:
  steps:
    - request:
        method: GET
        path: /health
      response:
        expected_status: 200
        expected_body: '{"ok":true}'
        expected_types:
          .ok: [boolean]
```

<!-- complete-yaml: manual-suite/items/defaults.yaml -->
`manual-suite/items/defaults.yaml`:

```yaml
app: apihydra
kind: defaults
metadata:
  name: item-defaults
  labels: [manual, items]
spec:
  base_url: http://127.0.0.1:18080
  base_path: /v2
  headers:
    Accept: application/json
    X-Scope: child
  disable_cookies: true
  timeout: 7
  retries: 2
```

<!-- complete-yaml: manual-suite/items/item-steps.yaml -->
`manual-suite/items/item-steps.yaml`:

```yaml
app: apihydra
kind: steps
metadata:
  name: child-item
  labels: [manual, child]
spec:
  defaults:
    headers:
      Content-Type: application/json
  steps:
    - request:
        method: POST
        path: /items
        query: source=child
        body: |
          {"id":${item_id}}
        defaults:
          disable_cookies: false
      response:
        expected_status: 201
        expected_body: |
          {"id":$item_id,"ok":true}
        expected_types:
          .id: [int]
          .ok: [boolean]
        capture:
          child_item_id: .id
      debug: false
```

From the directory containing `manual-suite`, run:

```sh
apih --parallelism=1 manual-suite
```

## Task recipes

### Run a suite

```sh
apih                         # current directory
apih path/to/suite           # selected directory
apih -p0 path/to/suite       # deterministic serial mode
```

### Minimal GET

```yaml
app: apihydra
kind: steps
spec:
  steps:
    - request:
        method: GET
        path: /health
      response:
        expected_status: 200
```

### Headers, query, and JSON body

Headers belong under a defaults mapping:

```yaml
request:
  method: POST
  path: /items
  query: notify=true&source=agent
  body: '{"name":"demo"}'
  defaults:
    headers:
      Content-Type: application/json
      Authorization: Bearer replace-me
```

### Inherit and override defaults

Put shared `base_url`, headers, timeout, retries, and cookie policy in the root
document. Put a nested directory's `base_path` or header changes in its
defaults document. Put file-wide changes in `spec.defaults`, then only the
exception in `request.defaults`. See the [worked inheritance
example](#worked-inheritance-example).

### Variables, captures, and validation

Use `vars` for literal run values, `$name`/`${name}` only in bodies, and
`response.capture` for jq extraction. Keep a dependent flow in one steps file
or use mode `0`; see [Variables and captures](#variables-and-captures).

Set any combination of `expected_types`, `expected_status`, and
`expected_body`. Omit a phase to accept any status or skip body comparison as
described in [Response validation](#response-validation).

### Cookie safety

Omit `disable_cookies` to use automatic cookies. Set it to `true` at a broad
scope, then use `false` only where re-enabling is intended. An explicit
`Cookie` header is independent. Prefer mode `0` when a deterministic cookie
history across otherwise parallel files matters.

### Stop after one inspected step

Add `debug: true` to that step. Remove secrets first if output may be shared;
Debug cannot redact them. See [Debug breakpoints](#debug-breakpoints).

## Troubleshooting

| Symptom | Meaning and action |
| --- | --- |
| `curl` command error or executable not found | `curl` is required for requests. Install it and ensure it is on `PATH`. |
| `jq` selector/pretty error or executable not found | `jq` is required during step validation and JSON operations. Check `PATH`, selectors, and actual/expected JSON. |
| `git` diff error or executable not found after a body mismatch | `git` is required to render unequal expected and actual bodies. Install it and ensure the run cache is writable. |
| Invalid selected directory | Pass zero or one existing directory. Relative paths resolve from the current working directory. Exit is `102`. |
| Malformed YAML or a scalar type error | Correct the reported file/YAML location. Body, variable, capture, metadata, header, path, query, and URL values are strings; quote numeric-looking variable values. |
| Missing variable | Define it before interpolation. Keep dependent steps serial. Exact missing-key diagnostics are not a stable contract. |
| Duplicate variable or capture | The store is run-wide and write-once. Rename the later key; the first value is preserved. |
| Invalid jq selector | Test it against the actual response with `jq`. Capture and type selectors delegate to jq; selector failure is terminal. |
| Invalid expected or actual JSON during body validation | Supply a valid nonempty JSON expected body and ensure the service returns JSON. Omit `expected_body` only when body validation is intentionally skipped. |
| Exit `101` | One or more configured type, status, or body expectations mismatched. Review stdout; eligible work still completed. |
| Exit `102` | Arguments, selected path, YAML, or definitions were invalid. Review the final stderr diagnostic. |
| Exit `103` | An internal command, storage, validation-tool, cookie-jar, temporary-data, or reporting operation failed. Review stderr and prerequisite/cache access. |
| A consumer cannot find a cross-file value | Parallel scheduling may let it run before the producer. Put the steps in one file, separate them by directory stage, or use `-p0`. |
| A child sees an unexpected cookie in mode `2` | The last parent step completion selects inheritance, so concurrent parent files make the source nondeterministic. Use mode `0`/`1` or redesign ownership. |
| Cookies still arrive with `disable_cookies: true` | An explicit `Cookie` header is unaffected. Remove that header if it is not wanted. |
| A later request lost cookie updates | A disabled request leaves the owning jar unchanged; parallel modes also isolate writable jars. Check scope overlays and the selected mode. |
| Debug exposed a secret | Treat the output as compromised, rotate the secret, and remove it before sharing future Debug output. Debug has no redaction mode. |
| An old `run-*` cache directory remains | Abrupt termination can prevent cleanup. Later runs do not reuse it. The product defines no startup scavenger; manage abandoned user-cache data outside active runs. |

## Agent checklist

Before creating or editing a suite:

1. Identify the selected suite directory and expected directory stages.
2. Choose parallelism `0`, `1`, or `2` from dependency and cookie needs.
3. Put shared request defaults in a root/defaults document.
4. Keep every narrower override inside the same six-field defaults shape.
5. Quote values that must decode as strings, especially `vars` values.
6. Keep producer/consumer steps serial or separated by a completed stage.
7. Define each variable or capture name only once per invocation.
8. Set response type, status, and body expectations independently.
9. Decide whether automatic cookies should inherit, disable, or explicitly
   re-enable at each scope.
10. Treat `debug: true` as a terminal, unredacted breakpoint.
11. Confirm `curl`, `jq`, and—when body diffs are possible—`git` are available.
12. Run the suite and interpret `0`, `101`, `102`, and `103` distinctly.
