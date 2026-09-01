# APIHydra (`apih`)

APIHydra is an ultra-fast, agent-first API integration tester. Its `apih` command discovers YAML suites, executes HTTP requests, and validates responses.

See the [APIHydra user manual](docs/user-manual/apih.md) for the complete CLI, suite, execution, and troubleshooting reference.

## Installation

APIHydra requires the Go version declared in `go.mod` (currently Go 1.25.12). Install the command with:

```bash
go install github.com/divilla/apihydra/cmd/apih@latest
```

Go installs the command into `GOBIN` when it is set, or otherwise into the Go workspace's `bin` directory. Ensure that directory is on `PATH`.

The command also uses these runtime dependencies:

- `curl` executes HTTP requests.
- `jq` performs response and Debug JSON processing.
- `git` renders a body diff when response-body validation fails.

Verify the installation with:

```bash
apih --help
```
