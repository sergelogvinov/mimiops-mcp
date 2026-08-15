# mimiops-mcp

Kubernetes MCP (Model Context Protocol) Server

A Go-based MCP server that provides read-only and destructive tools for interacting with Kubernetes clusters.

## Overview

**mimiops-mcp** is a Kubernetes operations tool that exposes a Model Context Protocol (MCP) interface for AI assistants and automation tools. It provides comprehensive Kubernetes resource exploration capabilities through a standardized protocol, enabling AI agents to interact with Kubernetes clusters safely and efficiently.

### Key Features

- **17+ read-only tools** for exploring Kubernetes resources (pods, workloads, Helm releases, events, nodes, namespaces)
- **2 destructive tools** (gated by `--allow-destructive` flag with two-phase confirmation)
- **Helm SDK integration** for managing Helm releases without requiring the Helm CLI
- **Dynamic client** for working with any Kubernetes resource type by Group-Version-Kind (GVK)
- **Multiple output formats** (text/markdown and JSON) for different use cases
- **Two-phase destructive confirmation** for safety - destructive actions require explicit confirmation
- **Two transport modes**: stdio (local) and SSE (HTTP for remote/web clients)
- **Structured logging** with klog for debugging and monitoring

### Architecture

```
┌──────────────────────────────────────────────────────────────┐
│                      MCP Client                              │
│   (Claude Desktop, Cursor, VS Code, Goose, custom script)    │
│                                                              │
│   Connects via:  stdio (pipe)  or  HTTP SSE (ws://...)       │
└───────────────────────────┬──────────────────────────────────┘
                            │  JSON-RPC 2.0
                            ▼
┌──────────────────────────────────────────────────────────────┐
│                   mimiops-mcp (single Go binary)             │
│                                                              │
│  ┌──────────────────────────────────────────────────────┐    │
│  │  CLI Layer (flag stdlib — no external dep)           │    │
│  │                                                      │    │
│  │  --kubeconfig   (default: ~/.kube/config)            │    │
│  │  --context      (default: current context)           │    │
│  │  --namespace    (default: "" = all namespaces)       │    │
│  │  --transport    (default: "stdio")                   │    │
│  │  --port         (default: 8080, for SSE)             │    │
│  │  --allow-destructive (default: false)                │    │
│  │  --log-level    (default: "info")                    │    │
│  └──────────────────────────┬───────────────────────────┘    │
│                             │                                 │
│  ┌──────────────────────────▼───────────────────────────┐    │
│  │  Transport Selector                                  │    │
│  │                                                      │    │
│  │  if transport == "stdio":  ServeStdio(mcpServer)     │    │
│  │  if transport == "sse":    ServeSSE(mcpServer, port) │    │
│  └──────────────────────────┬───────────────────────────┘    │
│                             │                                 │
│  ┌──────────────────────────▼───────────────────────────┐    │
│  │  MCP Server (mark3labs/mcp-go)                       │    │
│  │                                                      │    │
│  │  ┌────────────────────────────────────────────────┐  │    │
│  │  │ Tool Registry (+18 tools)                      │  │    │
│  │  │                                                │  │    │
│  │  │  Read-only:                                    │  │    │
│  │  │   pods_list, pods_get, pods_describe, pods_log │  │    │
│  │  │   pods_top, jobs_list, cronjobs_*              │  │    │
│  │  │   workloads_list, workloads_get, workloads_describe │  │  │
│  │  │   helm_list, helm_status, helm_history         │  │    │
│  │  │   events_list, namespaces_list, nodes_list     │  │    │
│  │  │   resources_list, resources_describe           │  │    │
│  │  │   api_versions                                 │  │    │
│  │  │                                                │  │    │
│  │  │  Destructive (gated):                          │  │    │
│  │  │   pods_delete, jobs_delete, rollout_restart    │  │    │
│  │  └────────────────────────────────────────────────┘  │    │
│  └───────────────────────┬────────────────────────────┘    │
│                          │                                  │
│  ┌───────────────────────▼────────────────────────────┐    │
│  │  Tool Handlers                                     │    │
│  │                                                    │    │
│  │  Each handler:                                     │    │
│  │   1. Validate params                               │    │
│  │   2. If destructive + allowDestructive:            │    │
│  │      → Return input_required prompt                │    │
│  │   3. Execute (K8s API / Helm SDK)                  │    │
│  │   4. Format response (text or JSON)                │    │
│  │   5. Return result or error                        │    │
│  └───────────────────────┬────────────────────────────┘    │
│                          │                                  │
│  ┌───────────────────────▼────────────────────────────┐    │
│  │  Backend Clients                                    │    │
│  │                                                    │    │
│  │  ┌─────────────────┐  ┌─────────────────────────┐  │    │
│  │  │ client-go        │  │ helm.sh/helm/v3 (SDK)   │  │    │
│  │  │ (typed + dynamic)│  │ (embedded, no helm bin) │  │    │
│  │  └─────────────────┘  └─────────────────────────┘  │    │
│  └───────────────────────┬────────────────────────────┘    │
└──────────────────────────┼─────────────────────────────────┘
                           │
                           ▼
                  ┌─────────────────────────┐
                  │   Kubernetes API Server  │
                  │   (authenticated via     │
                  │    kubeconfig)           │
                  └─────────────────────────┘
```

## Installation

### Prerequisites

- Go 1.22 or higher
- Git
- Make

### Build from Source

```bash
# Clone the repository
git clone https://github.com/sergelogvinov/mimiops-mcp.git
cd mimiops-mcp

# Build the binary
make build

# The binary will be available at bin/mimiops-mcp
```

### Build for Different Architectures

```bash
# Build for all supported architectures
make build-all-archs

# Build for specific architecture
make build ARCH=arm64
make build ARCH=amd64
```

### Docker Images

```bash
# Build Docker image
make images

# Build and push to registry
PUSH=true make images
```

### Helm Chart

```bash
# Package and push to OCI registry
make helm-release
```

## Build and Run

### Basic Usage (stdio)

```bash
# Default mode (stdio, reads ~/.kube/config)
./bin/mimiops-mcp

# With explicit kubeconfig
./bin/mimiops-mcp --kubeconfig /path/to/kubeconfig --context prod-east

# With specific namespace
./bin/mimiops-mcp --namespace production
```

### SSE Mode (HTTP Server)

```bash
# SSE mode for web/remote MCP clients
./bin/mimiops-mcp --transport sse --port 8080 --namespace default

# Combined flags
./bin/mimiops-mcp --kubeconfig ~/.kube/dev.yaml --transport sse --port 8080 --allow-destructive --log-level debug
```

### Development Mode

```bash
# Run directly from source (with race detector)
make run

# Lint the code
make lint

# Run tests
make test
```

## CLI Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--kubeconfig` | `~/.kube/config` | Path to kubeconfig file |
| `--context` | current context | Kubernetes context to use |
| `--namespace` | all namespaces | Default namespace for operations (empty = all) |
| `--transport` | `stdio` | Transport mode: `stdio` or `sse` |
| `--port` | `8080` | Port for SSE transport |
| `--allow-destructive` | `false` | Enable destructive tools |
| `--log-level` | `info` | Log level: `debug`, `info`, `warn`, `error` |

## Tools

### Read Tools

| Tool | Description |
|------|-------------|
| `pods_list` | List pods in a namespace |
| `pods_get` | Get full pod spec and status |
| `pods_describe` | Human-readable pod summary |
| `pods_log` | Fetch pod logs |
| `pods_top` | Show pod resource usage |
| `jobs_list` | List Jobs in a namespace |
| `jobs_get` | Get full Job spec and status |
| `jobs_describe` | Human-readable Job summary |
| `jobs_log` | Fetch Job logs |
| `cronjobs_list` | List CronJobs in a namespace |
| `cronjobs_get` | Get full CronJob spec |
| `cronjobs_describe` | Human-readable CronJob summary |
| `cronjobs_suspend` | Suspend a CronJob |
| `cronjobs_resume` | Resume a suspended CronJob |
| `workloads_list` | List Deployments, StatefulSets, or DaemonSets |
| `workloads_get` | Get a single workload's full spec and status |
| `workloads_describe` | Rich describe of a workload |
| `helm_list` | List Helm releases |
| `helm_status` | Get detailed status of a Helm release |
| `helm_history` | Show revision history of a release |
| `events_list` | List Kubernetes events |
| `namespaces_list` | List all namespaces |
| `nodes_list` | List cluster nodes |
| `resources_list` | List any resource type by GVK |
| `resources_describe` | Describe any resource by GVK + name |
| `api_versions` | List available API versions |

### Destructive Tools (gated by `--allow-destructive`)

| Tool | Description |
|------|-------------|
| `pods_delete` | Delete a pod (requires confirmation) |
| `jobs_delete` | Delete a Job (requires confirmation) |
| `rollout_restart` | Rollout restart a Deployment, StatefulSet, or DaemonSet |

## Output Formats

### Text Format (default)

```text
NAMESPACE     NAME                    READY   STATUS    RESTARTS   AGE
default       nginx-abc123            1/1     Running   0          5d
default       redis-def456            0/1     CrashLoopBackOff 3   2h
```

### JSON Format

```json
{
  "pods": [
    {
      "namespace": "default",
      "name": "nginx-abc123",
      "ready": "1/1",
      "status": "Running",
      "restarts": 0,
      "age": "5d",
      "node": "worker-1"
    }
  ]
}
```

## Destructive Confirmation Flow

Destructive tools use a two-phase confirmation flow for safety:

1. **Phase 1**: Call the tool without `confirm=true` to get a confirmation prompt
2. **Phase 2**: Call the tool with `confirm=true` to execute the action

Example:

```bash
# Phase 1: Get confirmation prompt
./bin/mimiops-mcp --allow-destructive
> pods_delete(name="nginx-abc123", namespace="default")

# Phase 2: Confirm and execute
> pods_delete(name="nginx-abc123", namespace="default", confirm=true)
```

## RBAC Configuration

See `deploy/rbac.yaml` for the recommended RBAC configuration.

To apply the RBAC configuration:

```bash
kubectl apply -f deploy/rbac.yaml
```

The RBAC configuration includes:

- **ClusterRole `mimiops-mcp-reader`**: Read-only permissions for all resources
- **ClusterRole `mimiops-mcp-destructive`**: Permissions for destructive actions (delete, rollout restart)
- **ClusterRoleBinding**: Binds the reader role to the service account

## Project Structure

```
mimiops-mcp/
├── cmd/
│   └── mimiops-mcp/
│       └── main.go              # Entry point, CLI flag parsing
├── internal/
│   ├── config/
│   │   └── config.go            # Config struct, flag parsing
│   ├── server/
│   │   └── server.go            # MCPServer wrapper, tool registration
│   ├── k8s/
│   │   ├── client.go            # Factory: creates client-go from kubeconfig
│   │   └── types.go             # Shared response helpers
│   ├── helm/
│   │   └── client.go            # Helm SDK wrapper (action config, namespace)
│   ├── tools/
│   │   ├── register.go          # Central tool registration
│   │   ├── pods.go              # pods_list, pods_get, pods_describe, pods_log
│   │   ├── workloads.go         # workloads_list, workloads_get, workloads_describe
│   │   ├── jobs.go              # jobs_list, jobs_get, jobs_describe, jobs_log
│   │   ├── cronjobs.go          # cronjobs_list, cronjobs_get, cronjobs_describe, cronjobs_suspend, cronjobs_resume
│   │   ├── configmaps.go        # configmaps_list
│   │   ├── helm_tools.go        # helm_list, helm_status, helm_history
│   │   ├── events.go            # events_list
│   │   ├── cluster.go           # namespaces_list, nodes_list, api_versions
│   │   ├── resources.go         # resources_list, resources_describe (dynamic client)
│   │   ├── pods_delete.go       # pods_delete (destructive)
│   │   ├── jobs_delete.go       # jobs_delete (destructive)
│   │   └── rollout.go           # rollout_restart (destructive)
│   └── formatter/
│       ├── formatter.go         # Interface: Format(result, formatType) string
│       ├── text.go              # Markdown / human-readable formatting
│       └── json.go              # Raw JSON formatting
├── deploy/
│   └── rbac.yaml                # Example ClusterRole for the service account
├── charts/
│   └── mimiops-mcp/             # Helm chart for deployment
├── Makefile
├── go.mod
└── README.md
```

## Development

### Prerequisites

- Go 1.22+
- Make
- golangci-lint
- Docker (for building images)
- Helm (for chart development)

### Common Commands

```bash
# Build the binary
make build

# Run with SSE on default port
make run

# Lint the code
make lint

# Run all tests (lint + unit)
make test

# Build for all architectures
make build-all-archs

# Build Docker images
make images

# Run Helm lint and template checks
make helm-unit
```

### Adding New Tools

1. Create a new file in `internal/tools/` (e.g., `mytool.go`)
2. Implement the tool handler function
3. Register the tool in `internal/tools/register.go`
4. Add documentation in this README

## License

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

[http://www.apache.org/licenses/LICENSE-2.0](http://www.apache.org/licenses/LICENSE-2.0)

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
