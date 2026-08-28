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

Keep your AI on the leash.

### Kubernetes core tools

Core tools are always registered. The names below are the MCP tool names exposed by the server:

- Clusters: `clusters_list` (multi-cluster configurations only), `clusters_describe`.
- Pods: `pods_list`, `pods_get`, `pods_describe`, `pods_log`.
- Jobs: `jobs_list`, `jobs_get`, `jobs_describe`, `jobs_log`, `jobs_create`.
- CronJobs: `cronjobs_list`, `cronjobs_get`, `cronjobs_describe`.
- Nodes: `nodes_list`, `nodes_get`, `nodes_describe`.
- Namespaces: `namespaces_list`, `namespaces_describe`.
- Resource configuration: `resourcequotas_list`, `resourcequotas_describe`, `limitranges_list`, `limitranges_describe`, `storageclasses_list`, `persistentvolumeclaims_list`, `persistentvolumeclaims_describe`, `priorityclasses_list`.
- Events: `events_get`.
- Workloads (Deployments, StatefulSets, and DaemonSets): `workloads_list`, `workloads_get`, `workloads_describe`.
- Autoscaling: `hpa_list`, `hpa_describe`.
- Services: `services_list`, `services_describe`.

The following additional recovery tools are registered only when `--allow-destructive` is enabled:

- `pods_delete` — delete a pod so its controller can recreate it.
- `jobs_delete` — delete a Job.
- `cronjobs_suspend`, `cronjobs_resume` — suspend or resume scheduled runs.
- `workloads_scale` — scale a Deployment, StatefulSet, or DaemonSet.

### Helm extension

Helm tools are enabled by default with `--extensions helm`:

- `helm_list` — list Helm releases.
- `helm_status` — inspect release status, revision, and related information.
- `helm_rollback` — roll back a release to its previous revision; available only with `--allow-destructive`.

### FluxCD extension

Enable FluxCD tools with `--extensions fluxcd` (or `--extensions all`). Read-only inspection tools include:

- GitRepository: `flux_gitrepositories_list`, `flux_gitrepositories_describe`.
- OCIRepository: `flux_ocirepositories_list`, `flux_ocirepositories_describe`.
- HelmRelease: `flux_helmreleases_list`, `flux_helmreleases_describe`.
- Kustomization: `flux_kustomizations_list`, `flux_kustomizations_describe`.

With `--allow-destructive`, Flux also exposes:

- `flux_reconcile` — trigger an immediate reconciliation of a GitRepository, HelmRelease, or Kustomization.
- `flux_reconciliation` — suspend or resume a Flux HelmRelease or Kustomization.

### Karpenter extension

Enable Karpenter tools with `--extensions karpenter` (or `--extensions all`). Read-only inspection tools include:

- NodePool: `karpenter_nodepools_list` — list NodePools with node class, node count, readiness, weight, and CPU/memory provisioned vs limits.

## Installation

```sh
brew install sergelogvinov/tap/mimiops-mcp
```

### MCPB-compatible clients

MimiOPS includes an MCPB manifest for clients that support MCP bundles, such as Claude Desktop.
Download the `.mcpb` release bundle and open/import it in the client. The bundle:

- starts `server/mimiops-mcp mcp`;
- prompts for a kubeconfig file; and
- passes that file as `KUBECONFIG` to the server.

### Manual editor configuration

For clients that use a JSON MCP configuration, add a server entry similar to the following:

```json
{
  "mcpServers": {
    "mimiops": {
      "command": "mimiops-mcp",
      "args": ["mcp"]
    }
  }
}
```

Zed uses a command object under `context_servers`:

```json
{
  "context_servers": {
    "mimiops": {
      "command": {
        "path": "mimiops-mcp",
        "args": ["mcp"],
        "env": {
          "KUBECONFIG": "/absolute/path/to/kubeconfig"
        }
      }
    }
  }
}
```

Restart or reload the client after saving its configuration.
The server will then appear as `mimiops` and expose the tools listed below.

## Running

### Common flags

Kubernetes and application flags are **global (persistent)** and inherited by `mcp`, `server`, and `tools`:

| Flag | Default | Environment variable | Description |
|------|---------|----------------------|-------------|
| `--kubeconfig <path>` | auto | `KUBECONFIG` | Path to kubeconfig; uses the default Kubernetes loading rules when unset |
| `--context <name>` | current context | `CONTEXT` | Kubernetes context to use |
| `--namespace <name>` | all namespaces | `NAMESPACE` | Default namespace scope for operations |
| `--as <user>` | unset | `AS` | Kubernetes impersonation user |
| `--extensions <list>` | `helm` | `EXTENSIONS` | Comma-separated extensions to enable, or `all` |
| `--allow-destructive` | `false` | `ALLOW_DESTRUCTIVE` | Enable destructive or mutating tools |
| `--log-level <level>` | `info` | `LOG_LEVEL` | `debug`, `info`, `warn`, or `error` |
| `--log-format <format>` | `text` | `LOG_FORMAT` | `text` or `json` |

The Kubernetes client also exposes the standard `genericclioptions` connection and authentication flags. The `tools` command adds this command-specific flag:

| Command | Flag | Default | Description |
|---------|------|---------|-------------|
| `tools` | `--output`, `-o` | `text` | Render tool results as `text`, `json`, or `yaml` |
| `server` | `--port` | `8080` | HTTP/SSE listen port; environment variable: `PORT` |

Available commands are `mcp` (stdio), `server` (HTTP/SSE), `tools` (direct CLI invocation), and `version`.

### Examples

```sh
# stdio with kubeconfig and impersonation
./bin/mimiops-mcp mcp --kubeconfig ~/.kube/dev.yaml --as developer

# SSE server scoped to a namespace, with destructive tools enabled
./bin/mimiops-mcp server --namespace default --allow-destructive --port 8080

# List enabled tools, or invoke one directly from the CLI
./bin/mimiops-mcp tools --extensions all
./bin/mimiops-mcp tools pods_list namespace=default -o json
```

## License

[Apache-2.0](LICENSE)
