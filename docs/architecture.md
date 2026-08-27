# Kubernetes MCP Server — Architecture

## Go-based Local Daemon with stdio + SSE, Typed Structured Output for Every Tool

## 1. Architectural Overview

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
│  │  CLI Layer (cobra subcommands)                       │    │
│  │                                                      │    │
│  │  mimiops-mcp mcp        → stdio transport            │    │
│  │  mimiops-mcp server     → HTTP SSE transport         │    │
│  │  mimiops-mcp version    → print version              │    │
│  │                                                      │    │
│  │  --kubeconfig  --context  --namespace  --impersonate │    │
│  │  --allow-destructive  --log-level   (--port: server) │    │
│  └──────────────────────────┬───────────────────────────┘    │
│                             │                                 │
│  ┌──────────────────────────▼───────────────────────────┐    │
│  │  Transport Selector                                  │    │
│  │                                                      │    │
│  │  subcommand == "mcp":    ServeStdio(mcpServer)       │    │
│  │  subcommand == "server": ServeSSE(mcpServer, port)   │    │
│  └──────────────────────────┬───────────────────────────┘    │
│                             │                                 │
│  ┌──────────────────────────▼───────────────────────────┐    │
│  │  MCP Server (mark3labs/mcp-go)                       │    │
│  │                                                      │    │
│  │  Tool Registry (see §6 for the full catalog)         │    │
│  │  Every tool: WithOutputSchema[T]() +                 │    │
│  │              NewToolResultStructured(result, text)   │    │
│  │                                                      │    │
│  │  Read-only (unconditional):  (*_list/_get/_describe  │    │
│  │    /_log, helm_*, events_get, clusters …)            │    │
│  │  Destructive (gated by --allow-destructive, single-  │    │
│  │    phase, no confirmation):  *_delete, *_scale,      │    │
│  │    cronjobs_suspend/resume, jobs_create,             │    │
│  │    rollout_restart, helm_rollback                    │    │
│  └───────────────────────┬──────────────────────────────┘    │
│                          │                                   │
│  ┌───────────────────────▼────────────────────────────┐    │
│  │  Tool Handlers                                     │    │
│  │                                                    │    │
│  │  Each handler:                                     │    │
│  │   1. Validate params                               │    │
│  │   2. Execute (K8s typed client / Helm SDK)         │    │
│  │   3. Return NewToolResultStructured(result, text)  │    │
│  │      or NewToolResultError                         │    │
│  └───────────────────────┬────────────────────────────┘    │
│                          │                                 │
│  ┌───────────────────────▼────────────────────────────┐    │
│  │  Backend Clients                                   │    │
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

Every tool returns a typed structured result.

---

## 2. Project Structure

```
mimiops/
├── go.mod
├── go.sum
├── Makefile
├── README.md
├── cmd/
│   └── mimiops-mcp/
│       ├── flags.go
│       └── main.go
├── internal/
│   ├── config/
│   │   └── config.go            # Config struct, flag parsing
│   ├── k8s/
│   │   ├── client.go            # Factory: creates typed client-go from kubeconfig
│   │   ├── owner.go             # Owned-resource lookup (GetControllerOf + UID)
│   │   └── types.go             # Shared response helpers
│   ├── helm/
│   │   ├── client.go            # Helm SDK wrapper (RESTClientGetter, namespace)
│   │   └── types.go             # ReleaseSummary, ReleaseStatus, HistoryEntry
│   ├── tools/
│   │   ├── register.go          # Central tool registration (names every tool)
│   │   ├── cluster.go           # cluster_name
│   │   ├── pods.go              # pods_list, pods_get, pods_describe
│   │   ├── pods_log.go          # pods_log
│   │   ├── pods_delete.go       # pods_delete (destructive)
│   │   ├── workloads_list.go    # workloads_list
│   │   ├── workloads_get.go     # workloads_get
│   │   ├── workloads_describe.go# workloads_describe
│   │   ├── workloads_resolve.go # shared kind-resolution helper (not a tool)
│   │   ├── workloads_scale.go   # workloads_scale (destructive)
│   │   ├── jobs_list.go         # jobs_list
│   │   ├── jobs_get.go          # jobs_get
│   │   ├── jobs_describe.go     # jobs_describe
│   │   ├── jobs_log.go          # jobs_log
│   │   ├── jobs_create.go       # jobs_create (destructive)
│   │   ├── jobs_delete.go       # jobs_delete (destructive)
│   │   ├── cronjobs_list.go     # cronjobs_list
│   │   ├── cronjobs_get.go      # cronjobs_get
│   │   ├── cronjobs_describe.go # cronjobs_describe
│   │   ├── cronjobs_suspend.go  # cronjobs_suspend (destructive)
│   │   ├── cronjobs_resume.go   # cronjobs_resume (destructive)
│   │   ├── rollout.go           # rollout_restart (destructive)
│   │   ├── nodes_list.go        # nodes_list
│   │   ├── nodes_get.go         # nodes_get
│   │   ├── nodes_describe.go    # nodes_describe
│   │   ├── namespaces_list.go   # namespaces_list
│   │   ├── namespaces_get.go    # namespaces_get
│   │   ├── resourcequotas_list.go # resourcequotas_list
│   │   ├── resourcequotas_get.go  # resourcequotas_get
│   │   ├── resourcequotas_describe.go # resourcequotas_describe
│   │   ├── limitranges_list.go    # limitranges_list
│   │   ├── limitranges_describe.go # limitranges_describe
│   │   ├── storageclasses_list.go # storageclasses_list
│   │   ├── priorityclasses_list.go # priorityclasses_list
│   │   ├── events.go            # events_list, events_get
│   │   ├── helm_list.go         # helm_list
│   │   ├── helm_status.go       # helm_status
│   │   └── helm_rollback.go     # helm_rollback (destructive)
│   └── formatter/
│       └── formatter.go         # transforms typed result structs → human-readable markdown (fallbackText)
│
├── tests/
│   ├── mcp_test.go
│   └── k8s_test.go
└── deploy/
    └── rbac.yaml                # reader + destructive ClusterRoles
```

**File ownership rule:** each tool = one `Register*` function in its **own** file, with `mcp.NewTool(...)` (description + schema) and the handler colocated. `internal/tools/register.go` is the only place that names every tool. Shared non-tool helpers (`resolveWorkloadKind`, owner lookup) live in separate files.

---

## 3. Core Dependencies

```go
module github.com/sergelogvinov/mimiops-mcp

go 1.26

require (
    github.com/mark3labs/mcp-go v0.58.0      // MCP SDK
    k8s.io/api v0.35.0                       // K8s API types
    k8s.io/apimachinery v0.35.0              // API machinery
    k8s.io/client-go v0.35.0                 // typed clientset
    k8s.io/metrics v0.35.0                   // metrics (pods_top / nodes_top, optional)
    helm.sh/helm/v3 v3.21.3                  // Helm SDK (Go library, no binary)
    github.com/spf13/pflag v1.0.10           // Flags
    github.com/spf13/cobra v1.10.2           // CLI command dispatch
)
```

- Kubeconfig resolution delegated to `k8s.io/client-go/tools/clientcmd`.
- Helm uses the same cluster identity via a `RESTClientGetter` built from `internal/k8s`.

---

## 4. CLI Interface

The binary exposes cobra subcommands that select the transport: `mcp` runs stdio, `server` runs SSE, `version` prints the version.

```bash
# Help / version
./mimiops-mcp --help
./mimiops-mcp version

# SSE server for web/remote MCP clients (--port is server-only)
./mimiops-mcp server --port 8080

# stdio for desktop MCP clients
./mimiops-mcp mcp

# Global flags are accepted on both subcommands
./mimiops-mcp mcp --kubeconfig /path/to/kubeconfig --context prod-east
./mimiops-mcp server --port 8080 --kubeconfig ~/.kube/dev.yaml

# Scope to a namespace + enable destructive actions
./mimiops-mcp mcp --namespace default --allow-destructive
./mimiops-mcp server --namespace default --allow-destructive --log-level debug

# Impersonate a user
./mimiops-mcp mcp --impersonate system:serviceaccount:default:app
./mimiops-mcp server --impersonate alice --port 8080
```

### Command tree

```
mimiops-mcp
├── (global persistent flags)
│     --kubeconfig, --context, --namespace, --impersonate, --allow-destructive, --log-level
├── mcp        # Serve the MCP protocol over stdio (inherits global flags)
├── server     # Serve the MCP protocol over HTTP SSE (global flags + --port)
│     --port  # local, server only
└── version    # Print version (no flags)
```

### Kubeconfig resolution (clientcmd)

Precedence, delegated to `clientcmd`:

1. `--kubeconfig <path>` → `loadingRules.ExplicitPath`.
2. `$KUBECONFIG` env → default loading rules merge those files.
3. `~/.kube/config` → fallback.

With multiple cluster configs, `--context` sets the active context via `ConfigOverrides.CurrentContext`; if empty, `clientcmd` uses the file's `current-context`. Namespace resolution flows through `clientcmd` (`ClientConfig().Namespace()`), honoring any `--namespace` override. `--as` sets `ConfigOverrides.AuthInfo.Impersonate` (act-as).

### Config struct

`Transport` is no longer a field — the subcommand determines the mode. `Port` is used only by `server`.

```go
type Config struct {
    Kubeconfig       string   // flag > $KUBECONFIG env > ~/.kube/config
    Context          string   // selects active cluster in multi-cluster kubeconfig
    Namespace        string   // "" = all namespaces
    Impersonate      string   // overrides kubeconfig act-as user
    Port             int      // server (SSE) only; ignored by mcp
    AllowDestructive bool
    LogLevel         string   // "debug", "info", "warn", "error"
}
```

---

## 5. Tool Registration & Structured Output

Every tool is a `mark3labs/mcp-go` tool. At construction the name, description, input schema, and the **output schema** are declared with `mcp.NewTool(...)`. The handler is registered via `s.AddTool`.

The single, cross-cutting rule of this architecture:

```go
// Every tool declares a typed output schema …
tool := mcp.NewTool("pods_list",
    mcp.WithDescription(...),
    mcp.WithString("namespace", …),
    …
    mcp.WithOutputSchema[PodListResult](),   // ← typed output schema
)
s.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
    // Catch Kubernetes API error properly: retun NewToolResultErrorf("no Resource found")
    // check apierrors.IsNotFound(err) error type
    cronJobs, err := client.BatchV1().CronJobs(namespace).List(ctx, metav1.ListOptions{})
    if err != nil {
        if apierrors.IsNotFound(err) {
            return mcp.NewToolResultErrorf("no CronJobs found"), nil
        }
        return mcp.NewToolResultErrorf("failed to list CronJobs: %v", err), nil
    }

    // build the typed result
    return mcp.NewToolResultStructured(structured, fallbackText), nil
})
```

The MCP client receives the structured object directly. `fallbackText` is a short, humann-readable text.
Missing/invalid parameters are validated and returned as `mcp.NewToolResultError`.

The schemas shown below are simplified for readability — the authoritative contract is the Go type declared via `mcp.WithOutputSchema[T]()`.

---

## 6. Tool Catalog (Complete & Final)

Legend: **R** read-only, **D** destructive (registered only when `--allow-destructive` is set). Every tool returns `WithOutputSchema[T]()`.

### 6.1 Pods & Batch

> The table is dense. Full individual tool specs (input params, field lists, examples) are preserved from the stacked stage documents and consolidated below. The overriding detail in every row is that each tool's output is the typed output-schema object delivered via `mcp.NewToolResultStructured` — there is no `text`/`json` branch and no `format` parameter.

### 6.2 Pods

#### `pods_list`

- **type** R
- **output schema** `PodListResult` → `{ "pods": []PodSummary }`
- **params** `namespace` (string, optional), `label_selector` (string, optional), `field_selector` (string, optional)
- **PodSummary** fields: `namespace`, `name`, `ready`, `status`, `restarts`, `age`, `node`, `ownerReferences[apiVersion, kind, name]`.

#### `pods_get`

- **output schema** `PodGetResult` — full `v1.Pod` object.
- **params** `name` (required), `namespace` (required). This is the machine-readable source of truth.

#### `pods_describe`

- **output schema** `PodDescribeResult` — rich summary: status, phase, conditions, container states, node, recent events (fetched via `events_list` core filtered by pod UID).
- **params** `name` (required), `namespace` (required).

#### `pods_log`

- **output schema** `PodLogResult` embeds `PodSummary` + `Streams []LogStream`.
- **params** `name` (req), `namespace` (req), `container` (opt), `tail` (int, opt, default 20, max 5000), `previous` (bool, opt, default false), `since_seconds` (int, opt).
- `LogStream{Pod, Container, Logs}` — `logs` is the **raw** output verbatim (no line splitting, no timestamp parsing).
- If `container` omitted on a multi-container pod: return error listing available containers.

#### `pods_top`

- **output schema** `PodTopResult` — per-pod CPU/memory. Requires metrics-server.

### 6.3 Workloads (Deployment · StatefulSet · DaemonSet)

All four workload tools accept `kind` as **optional**. When omitted, `resolveWorkloadKind` probes deployment → statefulset → daemonset. The resolved `kind` is always included in the output. Ambiguity (multiple kinds share the name) returns an error listing the matches; the caller retries with explicit `kind`.

- `workloads_list` (R) — `WorkloadListResult` → `[]WorkloadSummary{kind, namespace, name, ready, desired, age}`. `kind` omitted lists all three merged into one table.
- `workloads_get` (R) — `WorkloadResult` → full `apps/v1` object (`Deployment`/`StatefulSet`/`DaemonSet`).
- `workloads_describe` (R) — rich summary: replicas, conditions, selector, strategy, update history.
- `workloads_scale` (D) — `Scale` object (`kind`, `namespace`, `name`, `replicas`). Params: `name` (req), `namespace` (req), `replicas` (int, req, min 0), `kind` (opt, deployment|statefulset). DaemonSets cannot be scaled (no `spec.replicas`) → error. Uses the `scale` subresource (`UpdateScale`). **Single-phase, no `confirm`.**

### 6.4 Jobs & CronJobs

Read:

- `jobs_list` (R) — `JobListResult` → `[]JobSummary{namespace, name, completions, duration, age, status}`. Status: Complete/Failed/Running/Pending.
- `jobs_get` (R) — full `batch/v1.Job`.
- `jobs_describe` (R) — parallelism, completions, backoff, conditions, selector, owned pods (capped at 5).
- `jobs_log` (R) — `JobLogResult` embeds `JobSummary` + `Streams []LogStream`. Params incl. `all_pods` (default false → most recent pod only).
- `cronjobs_list` (R) — `CronJobListResult` → `[]CronJobSummary{namespace, name, schedule, suspend, status, last_schedule, age}`.
- `cronjobs_get` (R) — full `batch/v1.CronJob`.
- `cronjobs_describe` (R) — schedule, suspend, concurrency policy, active jobs (capped at 5), last schedule, job template summary.

Mutating (D, single-phase — gated only by `--allow-destructive`, **no `confirm`/`input_required`**):

- `cronjobs_suspend` (D) — patches `spec.suspend=true`.
- `cronjobs_resume` (D) — patches `spec.suspend=false`.
- `jobs_create` (D) — builds a Job from `spec.jobTemplate.spec`; name `<cronjob>-manual-<random4>` with collision retry (up to 10); clears `suspend` and resets selector/labels.
- `jobs_delete` (D) — deletes a Job with `propagation_policy` (Background default | Foreground).

> **Note** — `cronjobs_log` is intentionally **not** added: CronJobs produce no logs directly; route through `cronjobs_get`/`cronjobs_describe` → `jobs_log`.

### 6.5 Cluster Observability

All read-only, registered unconditionally. The `--namespace` flag scopes the namespaced tools; cluster-scoped tools ignore it.

| Tool                       | Output schema                        | Notable behavior                                                                                                                       |
| -------------------------- | ------------------------------------ | -------------------------------------------------------------------------------------------------------------------------------------- |
| `nodes_list` (R)           | `NodeListResult` → `[]NodeSummary`   | `include_allocations` (bool, opt, default false) computes per-node CPU/mem request & limit sums from pods; shown `request/allocatable` |
| `nodes_get` (R)            | `NodeResult`                         | Always computes allocated-resource totals; `include_pods` (bool, opt, default false) adds pod summary (capped at 15)                   |
| `nodes_describe` (R)       | `NodeDescribeResult`                 | conditions, addresses, taints, allocated resources (`used (percent%)`), pods (capped at 20), events                                   |
| `namespaces_list` (R)      | `NamespaceListResult`                | `status`, `age`                                                                                                                        |
| `namespaces_get` (R)       | `NamespaceResult`                    | full `v1.Namespace`                                                                                                                    |
| `resourcequotas_list` (R)  | `ResourceQuotaListResult`            | `used/hard` per `requests.cpu/memory`, `limits.cpu/memory`, `age`                                                                      |
| `resourcequotas_get` (R)   | `ResourceQuotaResult`                | every `spec.hard` + `status.used`, conditions                                                                                          |
| `resourcequotas_describe` (R) | `ResourceQuotaDescribeResult`     | summary + kubectl-style `Resource/Used/Hard` table (`rows` + pre-rendered `table`)                                                     |
| `limitranges_list` (R)     | `LimitRangeListResult`               | `namespace, name, types, age`                                                                                                          |
| `limitranges_describe` (R) | `LimitRangeDescribeResult`           | typed `spec.limits[]` with `min/max/default/defaultRequest/maxLimitRequestRatio` per type                                                |
| `storageclasses_list` (R)  | `StorageClassListResult`             | `provisioner, reclaim_policy, volume_binding_mode, allow_volume_expansion, age`                                                        |
| `priorityclasses_list` (R) | `PriorityClassListResult`            | `value, global_default, description, age`                                                                                              |
| `events_get` (R)           | `EventListResult` → `[]EventSummary` | warnings first then `lastTimestamp` desc; `namespace`, `field_selector`, `limit` (default 50, max 500)                                 |

> Note: `storageclasses_get` and `priorityclasses_get` are intentionally **not** added (list-only requirement). `events_list` is superseded by `events_get`.

### 6.6 Helm

Two read-only + one destructive tool.

- `helm_list` (R) — `ReleaseListResult` → `[]ReleaseSummary{name, namespace, revision, updated, status, chart, app_version}`. `namespace` is **required** (error if missing). Supports `label_selector` (opt) and `status_filter` (opt, enum `failed`/`deployed`).
- `helm_status` (R) — `ReleaseStatusResult` = status metadata (name, namespace, revision, status, last_deployed, description) **plus** `history` = last **3** revisions. No rendered `values`/`notes`.
- `helm_rollback` (D) — `RollbackResult{name, namespace, previous_revision, new_revision, status}`. **Always rolls back exactly one revision** (`current − 1`); no arbitrary `revision` param. Error at revision 1. **Non-blocking.** Gated by `--allow-destructive` only — **no `confirm`/`input_required`**.

`helm_history` is intentionally **not** implemented (`helm_status` embeds the last 3 revisions).

### 6.7 rollouts + misc destructive

`rollout_restart` (D) — restarts a Deployment/StatefulSet/DaemonSet by patching the pod template (injects `kubectl.kubernetes.io/restartedAt` annotation). Params: `name`, `namespace`, `kind` (req, enum). Single-phase, gated by `--allow-destructive`, no `confirm`.

---

## 7. Destructive Gating (single-phase)

A destructive tool:

- is **registered only** when `--allow-destructive` is set (so it is absent from `tools/list` otherwise);
- executes **immediately** on the single call;
- returns only an error if `--allow-destructive` is unset.

```

Client calls workloads_scale(name="nginx", namespace="default", replicas=5)
│
▼
Server:
→ if !allowDestructive: return NewToolResultError("Destructive action not enabled (start with --allow-destructive)")
→ validate params; resolve kind (if omitted)
→ execute the scale
→ return NewToolResultStructured(scale, "Scaled deployment/nginx in namespace 'default' to 5 replicas.")

```

This applies uniformly to `pods_delete`, `workloads_scale`, `cronjobs_suspend`, `cronjobs_resume`, `jobs_create`, `jobs_delete`, `rollout_restart`, and `helm_rollback`.

---

## 8. Output Formatting

The `format` param is gone; there is **no** text-or-JSON branching.

- **Primary output** — the typed result object, delivered via `mcp.NewToolResultStructured(result, fallbackText)`. MCP clients consume this as the machine-readable payload.
- **Fallback text** — a short, human-readable sentence (never a full render) for text-only clients. Examples:
    - `pods_list` → `"3 pods in namespace 'default'."`
    - `jobs_log` → `"Job 'pi' in namespace 'default' has 1 log stream(s)."`
    - `helm_rollback` → `"Rolled back release 'nginx' in namespace 'default' to revision 4."`

### Listed example (typed `PodListResult`)

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
            "node": "worker-1",
            "ownerReferences": [
                {
                    "apiVersion": "apps/v1",
                    "kind": "ReplicaSet",
                    "name": "nginx-abc123"
                }
            ]
        }
    ]
}
```

### `jobs_log` typed example

```json
{
    "namespace": "default",
    "name": "pi",
    "completions": "1/1",
    "duration": "1m30s",
    "age": "5d",
    "status": "Complete",
    "streams": [
        {
            "pod": "pi-abcde",
            "container": "pi",
            "logs": "3.14159265358979323846264338327950288419716939937510\nJob completed\n"
        }
    ]
}
```

No `format` param, no `formatPod*` helpers. Raw log bytes are preserved verbatim in `LogStream.Logs`.

---

## 9. Backend: K8s Typed Client

The typed `client-go` clientset covers all resources. The `internal/k8s` package resolves and holds the active identity.

```go
// internal/k8s/client.go
type Client struct {
    kubernetes.Interface

    ContextName string
    ClusterName string
    Namespace   string
    User        UserInfo
}
```

Key call sites and shared helpers:

- `internal/k8s/owner.go` — `OwnerPods(ctx, ns, ownerUID)` / `OwnerJobs(ctx, ns, ownerUID)` via `metav1.GetControllerOf` + UID filter. Used by `jobs_describe`, `jobs_log`, and `cronjobs_describe`.
- `workloads_resolve.go` — `resolveWorkloadKind(ctx, client, namespace, name, kind)` probes deployment → statefulset → daemonset.

Representative verb map: `CoreV1().Pods(ns)` (list/get/getLogs/delete), `AppsV1().Deployments/StatefulSets/DaemonSets` (list/get + `UpdateScale`), `BatchV1().Jobs/CronJobs` (list/get/create/delete/patch suspend), `CoreV1()` nodes/namespaces/resourcequotas/limitranges/events, `StorageV1().StorageClasses`, `SchedulingV1().PriorityClasses`, and the **Helm SDK** via a `RESTClientGetter` built from the same loading rules/overrides.

---

## 10. Startup Sequence

```
1. Parse CLI flags → Config (kubeconfig: flag > $KUBECONFIG > ~/.kube/config; context selects active cluster)
2. Initialize logger (log/klog) with configured level
3. Create a single root ctx in main.go (context.WithCancel) — passed to all tools; one cancel point
4. Create k8s client (typed clientset) + resolve identity
5. Create Helm client (RESTClientGetter reuse)
6. Create MCPServer instance
7. Register tools:
   a. read + structured-output tools unconditionally
   b. destructive tools only if AllowDestructive=true
8. Start transport: "mcp" → server.ServeStdio; "server" → server.ServeSSE(port)
9. Block on SIGINT/SIGTERM → cancel() for graceful shutdown
```

---

## 11. Error Handling Strategy

| Scenario                                           | Behavior                                                                                                       |
| -------------------------------------------------- | -------------------------------------------------------------------------------------------------------------- |
| Invalid kubeconfig path                            | Log error and exit immediately                                                                                 |
| K8s API failure (network/auth/RBAC)                | `Failed to list pods: <cause>` (as `NewToolResultError`)                                                       |
| Resource not found                                 | `Pod 'foo' not found in namespace 'bar'`                                                                       |
| Permission denied                                  | `Permission denied: user cannot list pods in namespace 'bar'`                                                  |
| Invalid tool params                                | `Invalid parameter 'kind': must be one of 'deployment','statefulset','daemonset'` …                            |
| Multiple workload kinds share a name               | `Ambiguous workload 'foo' in namespace 'bar': deployment/foo, statefulset/foo — retry with an explicit 'kind'` |
| `workloads_scale` DaemonSet                        | `Cannot scale daemonset 'foo': DaemonSets have no spec.replicas`                                               |
| `helm_list` missing namespace                      | `Invalid parameter 'namespace': required`                                                                      |
| `helm_rollback` at revision 1                      | `Release 'foo' in namespace 'bar' is at revision 1 — nothing to roll back to`                                  |
| `pods_log` container omitted + multi-container pod | Error listing available containers                                                                             |
| `jobs_log` with no pods                            | Structured result with empty `streams` (not an error)                                                          |
| Destructive tool without `--allow-destructive`     | `Destructive tool disabled. Restart with --allow-destructive.`                                                 |

---

## 12. RBAC Template

```yaml
# deploy/rbac.yaml
apiVersion: v1
kind: ServiceAccount
metadata:
    name: mimiops-mcp-reader
    namespace: default
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
    name: mimiops-mcp-reader
rules:
    # Pods (incl. logs)
    - apiGroups: [""]
      resources: [pods, pods/log, pods/status]
      verbs: [get, list, watch]
    # Events
    - apiGroups: [""]
      resources: [events]
      verbs: [get, list, watch]
    # Jobs + CronJobs
    - apiGroups: ["batch"]
      resources: [jobs, jobs/status, cronjobs, cronjobs/status]
      verbs: [get, list, watch]
    # Workloads (incl. scale read for workloads)
    - apiGroups: ["apps"]
      resources:
          [
              deployments,
              deployments/status,
              deployments/scale,
              statefulsets,
              statefulsets/status,
              statefulsets/scale,
              daemonsets,
              daemonsets/status,
          ]
      verbs: [get, list, watch]
    # Nodes, Namespaces
    - apiGroups: [""]
      resources: [nodes, nodes/status, namespaces, namespaces/status]
      verbs: [get, list, watch]
    # ResourceQuotas + LimitRanges
    - apiGroups: [""]
      resources: [resourcequotas, resourcequotas/status, limitranges]
      verbs: [get, list, watch]
    # StorageClasses
    - apiGroups: ["storage.k8s.io"]
      resources: [storageclasses]
      verbs: [get, list, watch]
    # PriorityClasses
    - apiGroups: ["scheduling.k8s.io"]
      resources: [priorityclasses]
      verbs: [get, list, watch]
    # Helm — read release metadata from Secrets
    - apiGroups: [""]
      resources: [secrets]
      verbs: [get, list]
      resourceNames: ["sh.helm.release.v1.*"]
---
# Destructive permissions (separate ClusterRole, NOT bound by default)
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
    name: mimiops-mcp-destructive
rules:
    # Pods
    - apiGroups: [""]
      resources: [pods]
      verbs: [delete]
    # Workloads — scale + rollout restart
    - apiGroups: ["apps"]
      resources: [deployments/scale, statefulsets/scale]
      verbs: [get, update, patch]
    - apiGroups: ["apps"]
      resources: [deployments, statefulsets, daemonsets]
      verbs: [patch] # rollout_restart
    # Jobs + CronJobs
    - apiGroups: ["batch"]
      resources: [jobs]
      verbs: [create, delete] # jobs_create, jobs_delete
    - apiGroups: ["batch"]
      resources: [cronjobs]
      verbs: [patch] # cronjobs_suspend / cronjobs_resume
    # Helm rollback (writes a new release revision to a Secret)
    - apiGroups: [""]
      resources: [secrets]
      verbs: [create, update]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
    name: mimiops-mcp-reader-binding
roleRef:
    apiGroup: rbac.authorization.k8s.io
    kind: ClusterRole
    name: mimiops-mcp-reader
subjects:
    - kind: ServiceAccount
      name: mimiops-mcp-reader
      namespace: default
```

---

## 13. Summary of Key Design Decisions

| Decision                  | Choice                                                                                                | Rationale                                                                       |
| ------------------------- | ----------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------- |
| **Language**              | Go                                                                                                    | Requested                                                                       |
| **K8s client**            | client-go typed (`kubernetes.Interface`)                                                              | Type-safe, minimal; no dynamic client except for future generic resources       |
| **Helm**                  | `helm.sh/helm/v3` SDK (Go)                                                                            | No external binary                                                              |
| **MCP SDK**               | `mark3labs/mcp-go`                                                                                    | 2025/2026 spec support, includes `WithOutputSchema` / `NewToolResultStructured` |
| **Transports**            | `mcp` (stdio) + `server` (SSE) subcommands                                                            | Explicit, self-documenting mode selection                                       |
| **Output**                | **Every tool → `WithOutputSchema[T]()` + `NewToolResultStructured`**                                  | Single, predictable structured contract for all clients                         |
| **`format` param**        | **Removed**                                                                                           | Replaced by the typed output schema on every tool                               |
| **Confirmation flow**     | **`confirm` / `input_required` removed**                                                              | Destructive tools gated by `--allow-destructive` only, single-phase execution   |
| **Workload kind**         | Optional; auto-detect deployment → statefulset → daemonset                                            | Reference a workload by name alone; ambiguity returns an error                  |
| **Scale**                 | `scale` subresource (`UpdateScale`); DaemonSets unsupported                                           | Uniform, records scale status; can't scale daemonsets                           |
| **`helm_rollback`**       | Always one revision back; no `revision` param                                                         | Simple, predictable rollback semantics                                          |
| **`helm_list` namespace** | Required                                                                                              | No all-namespaces fallback                                                      |
| **`helm_status` output**  | Status + description only; embeds last 3 revisions                                                    | Avoid leaking values/notes; rollback context in one call                        |
| **`helm_history`**        | Not implemented                                                                                       | `helm_status` covers the need                                                   |
| **`cronjobs_log`**        | Not added                                                                                             | CronJobs produce no logs; route via `jobs_log`                                  |
| **Log streams**           | `LogStream{pod, container, logs}` raw; `jobs_log` embeds `JobSummary`, `pods_log` embeds `PodSummary` | Shared shape, raw output, no timestamp synthesis                                |
| **Context lifecycle**     | Single root `ctx` from main                                                                           | One cancel point for graceful shutdown                                          |
| **Config file**           | None; CLI flags only                                                                                  | Keep it simple                                                                  |
| **RBAC**                  | Separate reader vs destructive ClusterRoles                                                           | Least privilege; destructive not bound by default                               |

---

## 14. Final Tool Inventory (unique, non-parameterized)

**Read-only (registered unconditionally):**
cluster_name, pods_list, pods_get, pods_describe, pods_log, pods_top, workloads_list, workloads_get, workloads_describe, jobs_list, jobs_get, jobs_describe, jobs_log, cronjobs_list, cronjobs_get, cronjobs_describe, nodes_list, nodes_get, nodes_describe, namespaces_list, namespaces_get, resourcequotas_list, resourcequotas_get, resourcequotas_describe, limitranges_list, limitranges_describe, storageclasses_list, priorityclasses_list, events_get, helm_list, helm_status

**Destructive (registered only with `--allow-destructive`, single-phase):**
pods_delete, workloads_scale, cronjobs_suspend, cronjobs_resume, jobs_create, jobs_delete, rollout_restart, helm_rollback

Every one of the above declares `mcp.WithOutputSchema[T]()` and returns `mcp.NewToolResultStructured(result, fallbackText)`.
