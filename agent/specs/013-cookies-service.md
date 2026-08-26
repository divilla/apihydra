# Cookie collection and propagation

## Status and ownership

- Binding domain reference: [`skeleton/internal/domain/suite.go`](../../skeleton/internal/domain/suite.go)
- Binding resolver reference: [`skeleton/internal/definition/resolver.go`](../../skeleton/internal/definition/resolver.go)
- Binding store reference: [`skeleton/internal/execution/ckvs.go`](../../skeleton/internal/execution/ckvs.go)
- Binding execution reference: [`skeleton/internal/execution/executor.go`](../../skeleton/internal/execution/executor.go)
- Binding command reference: [`skeleton/pkg/runner/runner.go`](../../skeleton/pkg/runner/runner.go)
- Binding composition reference: [`skeleton/cmd/cli/main.go`](../../skeleton/cmd/cli/main.go)
- Shared product contract: [`prd.md`](../prd.md)
- Shared domain types: [`000-domain-types.md`](000-domain-types.md)
- Related guides: [`001-runner-pkg.md`](001-runner-pkg.md),
  [`005-resolver-service.md`](005-resolver-service.md),
  [`010-executor-service.md`](010-executor-service.md), and
  [`011-main-app.md`](011-main-app.md)
- Status: skeleton-aligned implementation guide

## Reference contract

The binding skeleton defines the shared cookie store, inherited selection
configuration, request and response cookie-jar fields, runner file boundary,
execution flow, and composition. This guide does not reproduce those
declarations or algorithms.

One cookie store is shared by the executor. Before each request, the effective
mode and exact cookie keys select the Netscape jar content for that step. Curl
receives the selected content through a temporary request jar and collects a
response jar. Every completed response jar is retained on the runtime step and
given to the store independently of the request's selection mode.

## Boundaries

Resolver owns root-to-step inheritance of cookie mode and keys.
`CookieKeyValueStore` owns cookie records and exact-key selection. Executor
owns selection timing, runtime-step mutation, and store coordination. Runner
alone owns curl invocation and the cookie-jar file boundary. The CLI owns
construction and injection of the one shared store.

## Required implementation and tests

- Production outputs update the binding declarations and contracts in
  `internal/domain/suite.go`, `internal/definition/resolver.go`,
  `internal/execution/ckvs.go`, `internal/execution/executor.go`,
  `pkg/runner/runner.go`, and `cmd/cli/main.go` without parallel cookie models
  or command execution outside Runner.
- Unit tests cover domain serialization, effective cookie configuration, store
  selection and bulk loading, Curl's request and response jars, Executor's
  cookie mutations and ordering around Curl, and CLI collaborator composition.
- Each numbered acceptance criterion is traced to at least one meaningful unit
  test. Changed component unit-test statement coverage remains greater than
  95%, and concurrent store and execution tests run under the race detector.

## Deliberately unspecified

The skeleton does not define:

- ordering of cookie records returned by the map-backed store;
- handling of malformed cookie-jar lines;
- visibility ordering between concurrently executing steps in the same stage;
- selection behavior for a cookie mode other than `included` or `excluded`;
- temporary cookie-jar filenames or cleanup mechanics.

## Acceptance criteria

1. Names, signatures, fields, tags, constructor state, and store behavior match
   the binding references.
2. Resolved step requests receive their effective cookie mode and keys from
   Resolver, with mode defaulting to `included`.
3. Included mode assigns only exact `CookieKeys` records to
   `Step.Request.CookieJar`, with an empty list assigning no cookies. Excluded
   mode assigns every record except exact `CookieKeys`, with an empty list
   assigning all cookies.
4. Before Curl executes, Runner writes non-empty Netscape request jar content
   to a temporary file and passes its filename with `curl --cookie`. Runner
   always declares a response jar with `curl --cookie-jar` and returns its
   contents with the response body and status.
5. After every completed response, Executor assigns the returned jar to
   `Step.Response.CookieJar` and calls `CookieKeyValueStore.SetAll`, independent
   of cookie mode.
6. Bulk loading uses the binding store method and indexes complete cookie lines
   by `name:domain:path`; consumers do not reimplement its parsing or mutation
   behavior.
7. The CLI constructs one `CookieKeyValueStore` and injects that same instance
   into Executor alongside the existing collaborators.
8. Cookie record ordering, malformed input, and same-stage visibility are not
   introduced as requirements.
9. Component tests, `go test ./...`, `go test -race ./...`, and
   `git diff --check` pass.
