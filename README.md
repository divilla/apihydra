# APIHydra (`apih`)

APIHydra is an ultra-fast, agent-first API integration tester. Its `apih` command discovers YAML suites, executes HTTP requests, and validates responses.

Request header values, query strings, and paths support `$name` and `${name}` from literal variables or earlier response captures. Header replacements insert text; query and path replacements encode only inserted text (path spaces use `%20`). See [variables and captures](docs/user-manual/apih.md#variables-and-captures) for authentication examples and body JSON rules.

See the [APIHydra user manual](docs/user-manual/apih.md) for the complete CLI, suite, execution, and troubleshooting reference.

Explore the [basic examples](docs/examples/basic/) or the [full examples with every option configured](docs/examples/full/). See the [example guide](docs/examples/README.md) for setup and run instructions.

## Installation

APIHydra requires the Go version declared in `go.mod` (currently Go 1.25.12). Install the command with:

```bash
go install github.com/divilla/apihydra/cmd/apih@latest
```

Go installs the command into `GOBIN` when it is set, or otherwise into the Go workspace's `bin` directory. Ensure that directory is on `PATH`.

From an existing source checkout, `make install` runs `go install ./cmd/apih`.

The command also uses these runtime dependencies:

- `curl` executes HTTP requests.
- `jq` performs response and Debug JSON processing.
- `git` renders a body diff when response-body validation fails.

Verify the installation with:

```bash
apih --help
```
