# Mimiops MCP Server

## Motivation

There are many ways to manage Kubernetes workloads, but there are also many ways to break them by accident.
We have already seen cases where AI agents did something that we did not expect. Giving an AI agent full access to Kubernetes can be risky, especially when it can create, update, or delete resources directly.

In my opinion, we need to provide safe and well-defined tools for AI agents. These tools should limit what an agent can do, validate changes before applying them, and help prevent dangerous operations.

We also need tools that can help investigate and fix workload issues.
Many workloads are already managed by well-known tools such as Argo CD, FluxCD, Helm, and other GitOps or deployment systems.
An AI agent should understand who manages a resource and avoid making direct changes that can conflict with these tools.

The goal is not to give AI full control of Kubernetes.
The goal is to give AI a safe interface that allows it to understand problems, suggest changes, and perform only controlled operations.

## Overview

Mimi OPS is not designed to give AI agents unrestricted Kubernetes access.
Instead, it provides a small set of well-known and controlled operations.

The agent can observe, investigate, and perform a limited number of safe recovery actions. More dangerous operations, such as editing arbitrary resources, applying YAML, changing secrets, or deleting persistent data are strongly restricted.

This makes Mimi OPS an opinionated interface between AI agents and Kubernetes: powerful enough for common troubleshooting, but limited enough to reduce the risk of unexpected changes.

### Kubernetes basic tools:

- Pods
  - list — list pods and their current status.
  - get — get detailed information about a pod.
  - describe — inspect pod status, conditions, events, and related information.
  - logs — read container logs for troubleshooting.
  - delete — safely restart a workload by deleting a pod and allowing its controller to recreate it.
- CronJobs and Jobs
  - list — list CronJobs and Jobs.
  - get — get information about a specific resource.
  - describe — inspect status, conditions, events, and failures.
- Deployments, StatefulSets, and DaemonSets
  - list — list workloads and their current status.
  - get — get information about a specific workload.
  - describe — inspect replicas, rollout status, conditions, and related events.

### Helm integration

Mimi OPS can inspect Helm-managed workloads and help recover them when a deployment fails:

- list — list Helm releases.
- describe — show release information, status, revision, and related resources.
- rollback — roll back a release to a previous working revision when the current release is in a broken state.

### FluxCD integration

Mimi OPS can work with FluxCD-managed resources without directly changing resources that are controlled by GitOps:

- Reconcile Flux source resources to fetch the latest configuration.
- Reconcile HelmRelease resources to retry or apply the expected Helm state.
- Inspect reconciliation status and errors to understand why a deployment is not ready.

## Installation

A Kubernetes **MCP (Model Context Protocol)** server written in Go.
It exposes Kubernetes operations to MCP clients (Claude Desktop, Cursor, VS Code, etc.) over two transports:

- `mimiops-mcp mcp` — **stdio**, for desktop MCP clients.
- `mimiops-mcp server` — **HTTP/SSE**, for web/remote MCP clients.

## Running

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

## Development

```sh
make lint       # golangci-lint
make unit       # unit tests (build tag `unit`)
make test       # lint + unit
make licenses   # license audit (forbidden/restricted/unknown)
make docs       # (Docker image docs targets are also available; see `make help`)
```

Use `make help` for a full target list.

## License

[Apache-2.0](LICENSE)
