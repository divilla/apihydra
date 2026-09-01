# Agent-first user manual

## Outcome

Create a complete, self-contained user manual at
`docs/user-manual/apih.md`.

The primary audience is coding agents such as Codex, Claude, and Cursor. A
human user must still be able to read the manual naturally from beginning to
end. Optimize the document for both audiences by using stable descriptive
headings, literal terminology, compact reference tables, explicit rules, and
copyable examples. Do not require a reader to infer behavior from source code
or follow another document to complete a normal task.

The manual is the only user-facing documentation deliverable introduced by
this change. Keep all essential instructions and examples in that one Markdown
file rather than splitting the manual into pages, generated includes, or
companion reference documents. Documentation-focused tests may be added as
required by the repository's test policy.

This is a documentation change. It does not authorize changes to production
APIs, behavior, `AGENTS.md`, or any file below `skeleton/`.

## Authority and product version

Document the product after changes 013 through 016. Use these sources in this
order:

1. `skeleton/` is the binding architecture, API, and behavior reference.
2. `agent/prd.md` owns the shared product contract.
3. `agent/specs/` explains package boundaries and supported behavior without
   overriding the skeleton or PRD.
4. Changes 013 through 016 define the defaults, Debug, parallelism, temporary
   storage, and cookie behavior that the manual must include.
5. Production code and tests may be used to find concrete, verified examples,
   but they do not override the sources above.

Do not turn a deliberately unspecified implementation choice into a user
promise. Do not document internal Go types, services, or phase methods unless
that detail is necessary to explain a user-visible rule. If an authoritative
source conflicts with another artifact, stop and report the mismatch instead
of choosing wording that conceals it.

## Writing and format requirements

Use one Markdown document with one H1 and a linked table of contents. Arrange
the material from task-oriented guidance to exhaustive reference so an agent
can retrieve a rule by heading and a human can learn the product progressively.

- Start with a short product description, prerequisites, a minimal runnable
  suite, and the exact command needed to run it.
- Use the exact product terms `suite`, `directory`, `root document`, `defaults
  document`, `steps document`, `steps file`, and `step` consistently.
- State defaults, precedence, scope, exceptional values, and failure effects
  explicitly. Avoid relying on words such as "normally", "appropriately", or
  "as expected" where a precise rule exists.
- Use tables for field references. Every table row must give the YAML key,
  accepted value shape, scope, default or inheritance rule, and effect.
- Put each command, YAML fragment, directory tree, and output sample in a
  fenced block with the correct language identifier where one applies.
- Examples must be internally consistent, copyable after substituting clearly
  marked environment-specific values, and explained immediately before or
  after the block.
- Keep security-sensitive behavior conspicuous. In particular, place an
  explicit warning next to Debug instructions that Debug output is complete
  and unredacted.
- Prefer one authoritative explanation of a rule with links from later
  sections over repeating slightly different versions of it.
- Compact the finished prose without removing constraints, field details,
  examples, warnings, or troubleshooting information. Concision must not make
  the manual less exhaustive.

## Required contents

The heading names may be adjusted for readability, but the document must make
every topic below directly discoverable from its table of contents.

### Installation, prerequisites, and CLI quick reference

Document:

- how to build or run the `apih` command from this repository;
- the runtime roles of `curl`, `jq`, and `git`, including which features need
  them;
- the command synopsis `apih [flags] [directory]`;
- `-h`/`--help`, `-p`/`--parallelism`, the default parallelism value, and the
  valid values `0`, `1`, and `2`;
- the optional positional suite directory and how it is resolved from the
  current working directory;
- supported pflag forms that matter to users, including attached/equals,
  repeated, interspersed, and `--` termination behavior; and
- success, validation, configuration, and internal exit codes `0`, `101`,
  `102`, and `103`, including the distinction between nonfatal validation
  output and a terminal error.

Do not invent installation packages, released binaries, flags, environment
variables, filters, or commands that are absent from the current contract.

### Suite discovery and execution model

Explain the filesystem model before the YAML reference:

- `apih` recursively discovers regular `.yaml` and `.yml` definition files
  below the selected directory;
- directory depth determines the stage, with the suite root at stage `0`;
- stages execute sequentially with a full barrier between them;
- only steps documents execute requests; root and defaults documents provide
  inherited configuration;
- the three parallelism modes, what can overlap in each mode, and the invariant
  that steps within one steps file remain serial; and
- all modes use one shared, concurrent, write-once variable store. Recommend
  mode `0` when a cross-file or cross-directory producer/consumer dependency
  must be deterministic.

Include a small directory-tree example that shows a root document, a directory
defaults document, multiple steps files, and at least one nested directory.
Annotate the stage and inherited-default relationship without implying file
placement, cardinality, hidden-directory, symlink, or ordering rules that the
binding contract leaves unspecified.

### Complete YAML reference

Provide one compact schema example and a field-reference table for each
document kind. Clearly distinguish declarative input from runtime-only values.

Document the common envelope:

- `app`, using the supported value `apihydra`;
- `kind`, with `root`, `defaults`, and `steps`;
- `metadata.name` and `metadata.labels` where represented by the document
  contract; and
- `spec`, whose shape depends on `kind`.

Document root and defaults `spec` fields:

- `base_url`
- `base_path`
- `headers`
- `disable_cookies`
- `timeout`
- `retries`

Document steps `spec` fields:

- `defaults`, using the same complete defaults shape;
- `steps`; and
- every individual step field below.

Document individual-step fields:

- `vars`
- `request.path`
- `request.method`
- `request.query`
- `request.body`
- `request.defaults`, using the same complete defaults shape
- `response.expected_status`
- `response.expected_body`
- `response.expected_types`
- `response.capture`
- `debug`

Explain that response `actual_status` and `actual_body`, step `index`, the raw
Curl statement, and source-definition provenance are runtime-owned values and
are not configuration that a suite author should set. Do not introduce aliases
or flattened defaults fields that are absent from the shared domain contract.

### Defaults and request construction

Explain the complete overlay chain:

```text
parent directory -> current root/defaults document -> steps-file defaults -> step request defaults
```

Make the following rules explicit:

- `base_url`, `base_path`, `timeout`, and `retries` use a nonzero/nonempty
  narrower value as an override;
- headers merge by key, with a narrower value replacing the same inherited
  key;
- timeout defaults to 10 seconds and retries default to 3 when no scope
  supplies a nonzero value;
- `disable_cookies` is presence-sensitive: absence inherits, `true` disables,
  and `false` explicitly enables or re-enables automatic cookies; and
- when every scope omits `disable_cookies`, automatic cookies are enabled.

Show the effective request URL as the literal concatenation of resolved
`base_url`, resolved `base_path`, and `request.path`, followed by application
of `request.query`. Explain empty method behavior and the special `HEAD`
behavior only to the extent fixed by the supported contract. Explain that a
nonempty request body is sent as the request body and that headers come from
the resolved defaults map.

Include one worked inheritance example that shows the resolved values at each
scope, including a header override and `disable_cookies: false` re-enabling
cookies below a disabled scope.

### Variables and captures

Describe the single-run variable lifecycle in execution order:

1. `vars` loads literal values into the shared write-once store.
2. `$name` and `${name}` placeholders are interpolated in the request body and
   expected response body.
3. The request executes and produces an actual response body.
4. Each `response.capture` entry evaluates its jq selector against the actual
   body and stores the compact jq result under the capture name.
5. Later eligible steps may interpolate the captured value.

Explain that the shared store is write-once and therefore rejects a duplicate
variable or capture name. Give safe guidance for missing variables, invalid
selectors, and parallel producer/consumer execution, but do not claim an exact
error policy, variable grammar, or precedence where the binding product
contract deliberately leaves it unspecified.

Include a serial two-step example that captures a response value in the first
step and uses it in both a request body and expected body in the second step.

### Response validation

Explain the three independent validation phases and how multiple mismatches
from one step are reported:

- `expected_types` maps jq selectors to one or more allowed declarations;
  document all currently supported declarations, including jq JSON type names
  and the special `int` and `zero` declarations, with union examples;
- `expected_status: 0` or an omitted status accepts any actual status, while a
  nonzero value requires exact equality; and
- an empty `expected_body` skips body validation. A nonempty expected JSON body
  is recursively key-sorted and compared with the corresponding projection of
  the actual JSON body; explain object-field and array-shape behavior fixed by
  the current contract.

Differentiate validation mismatch behavior (reported, remaining eligible work
continues, final exit `101`) from command, parsing, jq, Git, reporting, or other
terminal failures. Include focused examples for successful validation, each of
the three mismatch types, optional extra response fields, type unions, and the
`<any>` status sentinel.

### Cookies and parallelism

Explain that automatic cookies are enabled by default, that
`disable_cookies` controls only curl's automatic cookie engine, and that it
does not remove an explicit `Cookie` header.

Document the run-local cookie behavior users can rely on:

- each invocation starts with fresh cookie state below its private run
  directory;
- disabling cookies for one request leaves its owning jar unchanged, so a
  later request can re-enable and resume that state;
- mode `0` uses one run jar;
- mode `1` uses one jar per directory and copies the parent's final state to
  distinct direct-child jars after a stage barrier;
- mode `2` uses one jar per steps file and child files inherit distinct copies
  of the parent directory jar whose step completed last;
- empty directories preserve incoming state; and
- siblings never merge cookie state and separate invocations never exchange
  it.

Call out that runtime completion order intentionally makes mode-2 inheritance
nondeterministic when multiple parent files complete in parallel. Include
examples for default-enabled cookies, disabling, explicit re-enabling, and an
explicit `Cookie` header.

### Debug, output, and temporary data

Describe `debug: true` as a terminal breakpoint. Explain that it executes the
selected request, records the latest runtime state, prints the stage,
directory path, file path, exact Curl statement, and prettified colored step
JSON, and then prevents later steps and stages from executing. Explain how a
terminal error at a debug step is retained after the latest state is printed.

Place this warning immediately beside the Debug example:

> **Warning:** Debug output is complete and unredacted. It can expose
> authorization headers, cookies, request bodies, response bodies, captured
> values, and other secrets.

Explain the user-visible Curl rendering rules that affect copying a Debug
command: header and inline body quoting, the 1,024-Unicode-character inline
body limit, `@-` for longer bodies, and JSON compaction/fallback behavior. Make
clear that presentation does not mutate the executed request.

Describe canonical stage/directory/file/step output order, terminal live
redraw versus non-terminal stage commits, fatal diagnostics on stderr, and the
guarantee that a terminal diagnostic is the final application output.

Explain that every valid run uses a private
`os.UserCacheDir()/apih/run-*` directory, that temporary artifacts remain
namespaced below it, and that controlled returns attempt silent best-effort
cleanup. Do not promise cleanup after abrupt process or machine termination or
startup scavenging of abandoned directories.

### Task recipes and troubleshooting

Finish with compact, goal-oriented recipes that an agent can copy or adapt:

- run the current directory and a selected directory;
- force deterministic serial execution;
- create a minimal GET step;
- send headers, query parameters, and a JSON body;
- inherit and override defaults through a nested suite;
- load, interpolate, capture, and reuse variables;
- validate status, body, and types;
- use and disable cookies safely; and
- stop at a Debug breakpoint.

Include a troubleshooting table keyed by observable symptom. At minimum cover
missing `curl`, `jq`, or `git`; invalid suite directory; malformed YAML;
missing or duplicate variables; invalid jq selectors or JSON bodies; response
validation exit `101`; configuration exit `102`; internal exit `103`;
parallel dependency races; cookie-state surprises; and Debug secret exposure.
Where the contract does not define a recovery action, explain the boundary
instead of inventing one.

## Example completeness and accuracy

The examples collectively must exercise every declarative field listed in the
complete YAML reference and every user-visible feature described in this
change. Include at least one complete runnable suite as a directory tree with
the full contents of every file; it may depend on a clearly described HTTP
fixture. Focused examples may omit unrelated fields.

Audit every file below `agent/examples/` and ensure that every construct shown
there is explained by the manual. Examples may be corrected, consolidated, or
adapted into more useful scenarios rather than copied verbatim, and the manual
must not depend on those source files to be complete. Application-specific
fixtures below `work/` and `int-tests/` may inform examples but do not each
need to be reproduced.

Use loopback or reserved example domains and clearly identify any server
response assumed by an example. Do not embed credentials, machine-specific
absolute paths, live third-party endpoints, UUIDs that imply required formats,
or app-specific `work/` fixtures as universal product behavior.

Before completion, audit every YAML and shell block against the production
decoder, resolver, CLI, and execution behavior. YAML examples intended to be
complete must decode successfully. Output examples must be labeled as exact
only when the binding contract fixes their bytes; otherwise label them as
illustrative.

## Required verification

- Add documentation-focused unit coverage for every acceptance criterion, as
  required by `AGENTS.md`. Prefer resilient semantic checks over snapshots of
  the entire manual.
- At minimum, automated checks must prove that the manual exists at the exact
  path, is one self-contained Markdown document, contains the required
  reference sections and security warning, mentions every supported
  declarative key, and has no broken repository-relative links.
- Extract and decode every YAML block labeled as complete or runnable. Verify
  the complete field-coverage example reaches every declarative field.
- Run `go test ./...`, `go test -race ./...`, `make check`, and
  `git diff --check`.
- Perform a final source-to-manual audit for behavioral details that cannot be
  usefully established by a structural documentation test.

## Acceptance criteria

1. `docs/user-manual/apih.md` is the single self-contained user-facing manual,
   is optimized for coding-agent retrieval, remains naturally human-readable,
   and contains a working table of contents with stable descriptive headings.
2. The manual provides an executable quick start and accurately documents
   prerequisites, CLI syntax and parsing, selected-directory behavior,
   parallelism flags, and exit codes without inventing unsupported interfaces.
3. The suite-tree, document-kind, defaults, step, request, response, variable,
   capture, and runtime-only references cover every supported declarative key
   and distinguish configuration from observed runtime state.
4. Defaults inheritance, request construction, variable phases, validation,
   cookies, parallel scheduling, Debug behavior, output ordering, errors, and
   temporary-data lifecycle agree with the binding skeleton and shared product
   contract, including all security and nondeterminism warnings.
5. The manual contains explained, copyable examples for every declarative
   field and user-visible feature required by this change, plus one complete
   runnable suite whose files decode successfully and whose assumed server
   behavior is explicit.
6. Troubleshooting guidance covers the required observable failures and never
   presents deliberately unspecified behavior as a stable product guarantee.
7. The final manual has been compacted without losing relevant information;
   documentation-focused tests, the source-to-manual audit, repository tests,
   race tests, required checks, and `git diff --check` all pass.
