# Mimiops MCP Server

A Kubernetes **MCP (Model Context Protocol)** server written in Go. It exposes Kubernetes pod operations to MCP clients (Claude Desktop, Cursor, VS Code, etc.) over two transports:

- `mimiops-mcp mcp` — **stdio**, for desktop MCP clients.
- `mimiops-mcp server` — **HTTP/SSE**, for web/remote MCP clients.

> **Status: Stage 1 (Pods only).** The tool catalog is intentionally scoped to pod workloads (`pods_list`, `pods_get`, `pods_describe`, `pods_log`, `pods_delete`). Helm, metrics, and other workload types are out of scope for now — see `docs/2-architect-pods-only.md`.

---

## Requirements

- Go 1.26+
- `make`
- `golangci-lint` (for `make lint`)
- Access to a Kubernetes cluster (kubeconfig)

---

## Build

Build the binary for the current OS/arch:

```sh
make build
```

The artifact is written to `bin/mimiops-mcp-<arch>` (e.g. `bin/mimiops-mcp-arm64`).

Build for all supported architectures at once:

```sh
make build-all-archs
```

Build a specific OS/arch:

```sh
make build OS=linux ARCH=amd64
```

Cross-build the full matrix in one shot (Go cross-compiles, no CGO — binary is static):

```sh
GOOS=linux GOARCH=amd64 make build
```

---

## Running

### Quick dev run (stdio, debug logging)

```sh
make run-mcp
```

### Run the MCP binary (stdio)

```sh
make run-mcp
```

### Run as an HTTP/SSE server

```sh
make run
```

### Common flags

Flags are **global (persistent)** and accepted on both subcommands:

| Flag | Default | Description |
|------|---------|-------------|
| `--kubeconfig <path>` | auto | Path to kubeconfig (defaults to `$KUBECONFIG` then `~/.kube/config`) |
| `--context <name>` | current-context | Kubernetes context to use |
| `--namespace <name>` | all namespaces | Scope operations to a namespace |
| `--allow-destructive` | `false` | Enable destructive tools (e.g. `pods_delete`) |
| `--log-level <level>` | `info` | `debug`, `info`, `warn`, `error` |

Per-command flags:

| Command | Flag | Default | Description |
|---------|------|---------|-------------|
| `server` | `--port` | `8080` | HTTP/SSE listen port |

### Examples

```sh
# stdio against a specific context
./bin/mimiops-mcp mcp --kubeconfig ~/.kube/dev.yaml --context prod-east

# SSE server scoped to a namespace, with destructive tools enabled
./bin/mimiops-mcp server --namespace default --allow-destructive --port 8080
```

### Kubeconfig resolution

Handled by `k8s.io/client-go/tools/clientcmd` (canonical kubectl-compatible behavior):

1. `--kubeconfig` flag (explicit file wins)
2. `$KUBECONFIG` environment variable (may point to multiple files, which are merged)
3. `~/.kube/config` fallback

When the config contains multiple contexts, `--context` selects the active one; otherwise the file's `current-context` is used.

---

## Verification

```sh
make build && ./bin/mimiops-mcp version
```

The `version` subcommand prints the built-in version and commit.

---

## Development

```sh
make lint       # golangci-lint
make unit       # unit tests (build tag `unit`)
make test       # lint + unit
make licenses   # license audit (forbidden/restricted/unknown)
make docs       # (Docker image docs targets are also available; see `make help`)
```

Use `make help` for a full target list.

---

## Architecture

See:

- `docs/1-architect.md` — the original end-to-end architecture (superseded scope).
- `docs/2-architect-pods-only.md` — the current Stage-1 pods-only design: CLI (`mcp`/`server`/`version`), tool catalog, confirmation flow, output formats, and RBAC.

---

## License

[Apache-2.0](LICENSE)
