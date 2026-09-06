# Example suites

Choose one suite directory:

- [basic](basic/root.yaml): a GET request with status validation and inherited
  directory defaults. Optional settings use their application defaults.
- [full](full/root.yaml): every user-configurable YAML field is explicitly set,
  including metadata, all six defaults fields at root, directory, file, and
  request scope, variables, request fields, response validations, captures, and
  Debug.

Each group contains a `root.yaml` and a child directory with `defaults.yaml`
and `steps.yaml`. See the [user manual](../user-manual/apih.md) for field
semantics and inheritance rules.

## Run

With `apih`, `curl`, `jq`, and `git` on `PATH`, run from the repository root:

```sh
apih docs/examples/basic
apih --parallelism=1 docs/examples/full
```

Alternatively, change into either group and run `apih` without a directory
argument. Select `basic` or `full` directly: `docs/examples` itself is not a
suite root.

Both examples expect a local HTTP service on `127.0.0.1:18080`:

| Suite | Request | Required response |
| --- | --- | --- |
| basic | `GET /api/health` | Status `200`. |
| full | `POST /api/items?source=example&limit=20` with `{"title":"Example item"}` | Status `201`, JSON body `{"id":7,"title":"Example item","ok":true}`. Extra object fields are allowed. |

Adapt the URLs, paths, payloads, and expectations to your API. The full example
sets `base_url` at every defaults scope, so update all four occurrences when
changing the service address.

## Full configuration notes

`disable_cookies: false` on the request re-enables cookies after the directory
and file defaults disable them. Headers merge across scopes. The response
captures `.id` as `item_id`, which subsequent steps can reference as `$item_id`.

`debug: false` allows normal validation. Set it to `true` to print the complete
Debug report and stop at that step.

The CLI's `--parallelism` option is explicitly set in the full invocation. Its
supported values are `0`, `1`, and `2`; `--help` displays usage without running
the suite. Temporary run paths are managed by `apih`.

Runtime fields such as `index`, `response.actual_status`, and
`response.actual_body` are populated by `apih`, so they are not configured in
the examples.
