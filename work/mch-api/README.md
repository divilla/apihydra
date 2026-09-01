# MCH API integration suite

This `apih` tree mirrors the HTTP request/response coverage in
`../project-manager/backend/api-tests`. It targets the running API at
`http://127.0.0.1:8080` and keeps each resource group isolated with its own
created records and cleanup requests.

Run it from the APIHydra repository root:

```bash
go run ./cmd/apih work/mch-api
```

Database-unavailable error handling has a separate suite so the normal suite
remains valid against a healthy API. Start a second backend on port `19082`
with a deliberately unavailable PostgreSQL connection, then run:

```bash
go run ./cmd/apih work/mch-api-unavailable
```

The following source tests are intentionally outside this HTTP-only suite:

- `TestChangeDefinitionUpdatesPreserveVersionAndHistory`, because its defining
  assertion reads PostgreSQL directly rather than an HTTP response.
- `TestChangeAssignFlowConcurrentRequestsPreserveSingleRef` and
  `TestChangeStartRunConcurrentRequestsPreserveSingleClaim`, because they
  aggregate twelve concurrent client responses, which an `apih` definition
  cannot reduce into a winner count.
