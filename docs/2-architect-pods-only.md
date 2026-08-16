# Kubernetes MCP Server — Revised Architecture (Phase 1: Pods Only)

## Go-based Local Daemon with stdio + SSE, Scoped to Pod Workloads

> **Scope note — Stage 1.** This revision intentionally narrows the system to **Pod workloads only**. All Helm, Jobs/CronJobs, ConfigMaps, generic dynamic resources, metrics (pods_top), and non-pod workload types are **removed from scope for now**. They can be re-added in later stages. The doc below reflects only what Stage 1 implements.

---

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
│  │  CLI Layer (subcommands, pflag — minimal deps)      │    │
│  │                                                      │    │
│  │  mimiops-mcp mcp        → stdio transport            │    │
│  │  mimiops-mcp server     → HTTP SSE transport         │    │
│  │                                                      │    │
│  │  shared (global persistent) flags:                    │    │
│  │    --kubeconfig (default: ~/.kube/config)            │    │
│  │    --context    (default: current context)           │    │
│  │    --namespace  (default: "" = all namespaces)       │    │
│  │    --allow-destructive (default: false)              │    │
│  │    --log-level  (default: "info")                    │    │
│  │  server-only (local) flag:                            │    │
│  │    --port (default: 8080)                            │    │
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
│  │  ┌────────────────────────────────────────────────┐  │    │
│  │  │ Tool Registry (Stage-1: Pods only)             │  │    │
│  │  │                                                │  │    │
│  │  │  Read-only:                                    │  │    │
│  │  │   pods_list     pods_get    pods_describe      │  │    │
│  │  │   pods_log                                     │  │    │
│  │  │                                                │  │    │
│  │  │  Destructive (gated by --allow-destructive):   │  │    │
│  │  │   pods_delete                                  │  │    │
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
│  │   3. Execute (K8s typed client)                    │    │
│  │   4. Format response (text or JSON per format param)│    │
│  │   5. Return result or error                        │    │
│  └───────────────────────┬────────────────────────────┘    │
│                          │                                  │
│  ┌───────────────────────▼────────────────────────────┐    │
│  │  Backend Client                                    │    │
│  │                                                    │    │
│  │  client-go (typed client, pods + core resources)   │    │
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

---

## 2. Project Structure (Stage 1)

```
mimiops/
├── go.mod
├── go.sum
├── Makefile
├── README.md
├── cmd/
│   └── mimiops-mcp/
|       ├── flags.go
│       └── main.go
├── internal/
│   ├── config/
│   │   └── config.go            # Config struct, flag parsing
│   ├── server/
│   │   └── server.go            # MCPServer wrapper, tool registration
│   ├── k8s/
│   │   ├── client.go            # Factory: creates typed client-go from kubeconfig
│   │   └── types.go             # Shared response helpers
│   ├── tools/
│   │   ├── register.go          # Central MCP registration: wires every tool into the server
│   │   ├── cluster.go           # cluster_name (reports resolved cluster from kubeconfig)
│   │   ├── pods.go              # pods_list, pods_get, pods_describe
│   │   ├── pods_log.go          # pods_log (log fetch + container resolution)
│   │   └── pods_delete.go       # pods_delete (destructive, confirmation flow)
│   └── formatter/
│       ├── formatter.go         # Interface: Format(result, formatType) string
│       ├── text.go              # Markdown / human-readable formatting
│       └── json.go              # Raw JSON formatting
├── tests/
│   ├── mcp_test.go
│   └── k8s_test.go
└── deploy/
    └── rbac.yaml                # Minimal pods-only ClusterRole
```

---

## 3. Core Dependencies

```go

module github.com/sergelogvinov/mimiops-mcp

go 1.26

    github.com/spf13/pflag v1.0.10        // Flags
    github.com/spf13/cobra v1.10.2        // CLI command dispatch (mcp / server / version)
)

// Removed from Stage 1:
//   helm.sh/helm/v3        (no Helm in pods-only scope)
//   k8s.io/metrics          (pods_top deferred)
```

CLI dispatch uses **`github.com/spf13/cobra`** for the root command and its `mcp` / `server` / `version` subcommands; `pflag` backs the flags (cobra uses pflag natively).

---

## 4. CLI Interface

The binary exposes cobra subcommands that select the transport: `server` runs HTTP/SSE, `mcp` runs stdio, plus a `version` helper. This replaces the `--transport` flag with explicit, self-documenting commands. Flags are split into **global (persistent on the root command)** and **per-command**:

- **Global flags** — defined once on the root command with `AddPersistentFlags()` and inherited by every subcommand: `--kubeconfig`, `--context`, `--namespace`, `--impersonate`, `--allow-destructive`, `--log-level`.
- **Per-command flags** — `--port` is defined only on the `server` command, since it is meaningless for `mcp` (stdio has no listener).

```bash
# Help / version
./mimiops-mcp --help
./mimiops-mcp version

# SSE server for web/remote MCP clients (--port is server-only)
./mimiops-mcp server --port 8080

# stdio for desktop MCP clients (Claude Desktop, Cursor, VS Code)
./mimiops-mcp mcp

# Global flags are accepted on both subcommands
./mimiops-mcp mcp --kubeconfig /path/to/kubeconfig --context prod-east
./mimiops-mcp server --port 8080 --kubeconfig ~/.kube/dev.yaml

# Scope to a namespace + enable destructive actions
./mimiops-mcp mcp --namespace default --allow-destructive
./mimiops-mcp server --namespace default --allow-destructive --log-level debug

# Impersonate a user (overrides kubeconfig act-as)
./mimiops-mcp mcp --impersonate system:serviceaccount:default:app
./mimiops-mcp server --impersonate alice --port 8080
```

### Flag wiring (cobra)

- `rootCmd.PersistentFlags()` — global: kubeconfig, context, namespace, impersonate, allow-destructive, log-level.
- `serverCmd.Flags()` (local) — `--port`; **not** registered on `mcp`.
- `version` subcommand — no flags; prints the built-in version.

### Command tree (cobra)

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

The **`k8s.io/client-go/tools/clientcmd`** module is the canonical way to build a Kubernetes REST config from the user's kubeconfig settings. It handles the precedence rules (flag > `$KUBECONFIG` env > `~/.kube/config`), context selection, and merging of multiple files automatically, so we do **not** hand-roll the resolution logic.

```go
// internal/k8s/client.go

import (
    "k8s.io/client-go/tools/clientcmd"
)

func NewClient(cfg *config.Config) (kubernetes.Interface, error) {
    // Order of precedence, matching kubectl/clientcmd:
    //   --kubeconfig flag > $KUBECONFIG env > ~/.kube/config
    loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
    loadingRules.ExplicitPath = cfg.Kubeconfig   // empty → falls back to env/default

    // --context selects the active cluster when the config has multiple contexts;
    // if empty, clientcmd honors the file's current-context.
    // --impersonate overrides the user to act-as for all requests.
    overrides := &clientcmd.ConfigOverrides{
        CurrentContext: cfg.Context,
        AuthInfo: clientcmdapi.AuthInfo{
            Impersonate: cfg.Impersonate,
        },
    }

    kubeConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, overrides)
    restConfig, err := kubeConfig.ClientConfig()   // resolves + returns a valid rest.Config
    if err != nil {
        return nil, err
    }
    kubeConfig.Namespace()                          // → active namespace (from file or override)

    return kubernetes.NewForConfig(restConfig)
}
```

**Precedence summary (delegated to clientcmd):**

1. `--kubeconfig <path>` → `loadingRules.ExplicitPath` (explicit file wins).
2. `$KUBECONFIG` env → default loading rules merge those files.
3. `~/.kube/config` → fallback when neither is set.

With multiple cluster configs, `--context` sets the active context via `ConfigOverrides.CurrentContext`; if empty, `clientcmd` uses the file's `current-context`. Namespace resolution also flows through `clientcmd` (`ClientConfig().Namespace()`), honoring any `--namespace` override.

**Impersonation:** `--impersonate <user>` overrides the kubeconfig `AuthInfo.Impersonate` (act-as) for all requests via `ConfigOverrides.AuthInfo.Impersonate`. If unset, the kubeconfig's act-as value is used. The effective impersonation is surfaced in the server startup output.

---

### Config struct

`Transport` is no longer a field — the subcommand itself determines the mode. `Port` is only used by `server`.

```go
// internal/config/config.go

type Config struct {
    Kubeconfig       string   // flag > $KUBECONFIG env > ~/.kube/config
    Context          string   // selects active cluster in multi-cluster kubeconfig
    Namespace        string   // "" means all namespaces
    Impersonate      string   // overrides kubeconfig act-as user
    Port             int      // server (SSE) only; ignored by mcp
    AllowDestructive bool
    LogLevel         string   // "debug", "info", "warn", "error"
}
```

---

## 5. Tool Specifications (Stage-1 Catalog)

### 5.1 Read Tools

<details>
<summary><b>cluster_name</b></summary>

| Field | Value |
|-------|-------|
| **name** | `cluster_name` |
| **description** | Report the name of the connected cluster |
| **params** | none |
| **response** | Text: the cluster name resolved from the active kubeconfig context (e.g. `prod`, `kind-dev`). Empty string if the active context has no cluster set. |
| **RBAC** | none (resolved once at startup from kubeconfig, no cluster read) |
| **notes** | Implemented in `internal/tools/cluster.go`. Mirrors the `client.ClusterName` value that startup logging already surfaces. |
</details>

<details>
<summary><b>pods_list</b></summary>

| Field | Value |
|-------|-------|
| **name** | `pods_list` |
| **description** | List pods in a namespace (or all namespaces) |
| **params** | `namespace` (string, optional), `label_selector` (string, optional), `field_selector` (string, optional), `format` (string, optional: `"text"` or `"json"`, default: `"text"`) |
| **response** | Text: markdown table (name, namespace, status, age, node) OR JSON: `[]PodSummary`. Each pod includes optional `ownerReferences` (apiVersion, kind, name) of the owning workload so the agent can correlate Pods to their controller. |
| **RBAC** | `get`, `list` pods |
</details>

<details>
<summary><b>pods_get</b></summary>

| Field | Value |
|-------|-------|
| **name** | `pods_get` |
| **description** | Get full pod spec and status |
| **params** | `name` (required), `namespace` (required), `format` (optional, default: `"text"`) |
| **response** | Text: key-value describe / JSON: full `v1.Pod` object |
| **notes** | This is the machine-readable source of truth for a pod. |
</details>

<details>
<summary><b>pods_describe</b></summary>

| Field | Value |
|-------|-------|
| **name** | `pods_describe` |
| **description** | Human-readable pod summary (conditions, container statuses, node, tolerations) |
| **params** | `name` (required), `namespace` (required), `format` (optional, default: `"text"`) |
| **response** | Rich formatted description: status, phase, conditions, container states (which container is waiting/CrashLoopBackOff and why), node assignment, and recent events for the pod. Events are fetched via the `events_list` core and filtered to the pod UID. |
| **RBAC** | `get` pods, `list` events (scoped to pod namespace) |
</details>

<details>
<summary><b>pods_log</b></summary>

| Field | Value |
|-------|-------|
| **name** | `pods_log` |
| **description** | Fetch pod logs |
| **params** | `name` (required), `namespace` (required), `container` (string, optional), `tail` (int, optional, default: `20`, max: `5000`), `previous` (bool, optional, default: `false` — reads logs from the last terminated container), `since_seconds` (int, optional — only return logs newer than N seconds), `format` (optional, default: `"text"`) |
| **response** | Text: log lines as plain text / JSON: `[]LogLine { timestamp, line }` |
| **notes** | Maps to `kubectl logs`: `tail`, `-p/--previous`, `--since`. If `container` is omitted and pod has multiple containers, return an error listing available container names. |
</details>

### 5.2 Destructive Tools

<details>
<summary><b>pods_delete</b></summary>

| Field | Value |
|-------|-------|
| **name** | `pods_delete` |
| **description** | Delete a pod |
| **params** | `name` (required), `namespace` (required), `grace_period_seconds` (int, optional, default: `30`), `confirm` (bool, optional, default: `false`) |
| **flow** | See Section 6. Registered only when `--allow-destructive` is set. |
| **RBAC** | `delete` pods |
</details>

### 5.3 MCP Tool Registration & Descriptions (`internal/tools`)

Every tool is a `mark3labs/mcp-go` tool. Its **name** and **description** (plus the JSON schema for its input params) are defined at construction time with `mcp.NewTool(...)` and its option helpers; its **implementation** is the handler registered via `server.AddTool(tool, handler)`. Each tool lives in its own file (`cluster.go`, `pods.go`, `pods_log.go`, `pods_delete.go`) and exposes a `Register*` function. The central `internal/tools/register.go` is the *only* place that names every tool — it calls each `Register*` so the server layer never hard-codes tool names.

```go
// internal/tools/register.go — central, only place that lists every tool
package tools

import (
    "context"

    "github.com/mark3labs/mcp-go/server"
    "github.com/sergelogvinov/mimiops-mcp/internal/k8s"
)

// RegisterTools wires every tool into the MCP server. Read tools are always
// registered; destructive tools are registered only when allowDestructive is set.
func RegisterTools(srv *server.MCPServer, client *k8s.Client, allowDestructive bool) {
    RegisterClusterName(srv, client)
    RegisterPodsList(srv, client)
    RegisterPodsGet(srv, client)
    RegisterPodsDescribe(srv, client)
    RegisterPodsLog(srv, client)
    if allowDestructive {
        RegisterPodsDelete(srv, client)
    }
}
```

Each tool file owns its description and its implementation together:

```go
// internal/tools/pods.go — one tool per file
package tools

import (
    "context"

    "github.com/mark3labs/mcp-go/mcp"
    "github.com/mark3labs/mcp-go/server"
    "github.com/sergeloginovi/mimiops-mcp/internal/k8s"
)

// RegisterPodsList adds the pods_list tool: description + schema at construction,
// implementation in the handler.
func RegisterPodsList(s *server.MCPServer, client *k8s.Client) {
    listTool := mcp.NewTool("pods_list",
        mcp.WithDescription("List pods in a namespace (or all namespaces)."),
        mcp.WithString("namespace", mcp.Description("namespace; empty = all"), mcp.Required()),
        mcp.WithString("label_selector", mcp.Description("label selector filter")),
        mcp.WithString("field_selector", mcp.Description("field selector filter")),
        mcp.WithString("format", mcp.Description(`"text" or "json"`), mcp.DefaultString("text")),
    )
    s.AddTool(listTool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
        // implementation: list pods scoped by namespace, format, return
        return nil, nil
    })
}
```

Key consequences of this layout:

- **Descriptions are discoverable**: `mcp.WithDescription` is what a model sees for each tool; keeping it next to the handler in the tool's own file keeps docs in sync with code.
- **Registration gating**: the MCP server only advertises tools that are actually registered, so `pods_delete` is *invisible* in `tools/list` unless `--allow-destructive` is set — not merely rejected at call time.
- **Adding a tool** = add one file + one `Register*` call in `register.go`.

#### `cluster_name` implementation

The resolved cluster name is produced once by `internal/k8s` during `NewClient` (from the active kubeconfig context) and stored on `Client.ClusterName`. `cluster_name` just reports it — no params, no cluster RBAC:

```go
// internal/tools/cluster.go
func RegisterClusterName(s *server.MCPServer, client *k8s.Client) {
    tool := mcp.NewTool("cluster_name",
        mcp.WithDescription("Return the name of the connected cluster (resolved from the active kubeconfig context)."),
    )
    s.AddTool(tool, func(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
        return mcp.NewToolResult(mcp.NewTextContent(client.ClusterName)), nil
    })
}
```

---

## 6. Destructive Confirmation Flow (input_required)

Uses the MCP `input_required` mechanism (two-phase call) to avoid accidental deletion.

```
Client calls pods_delete(name="nginx-abc123", namespace="default")
                     │
                     ▼
Server: checks --allow-destructive (error if false)
  → Validates params
  → confirm is absent / false
  → Returns CallToolResult (isError=false) with:
      — content: "This will delete pod 'nginx-abc123' in namespace 'default'.
                  Call again with confirm=true to proceed."
      — inputRequired: true
                     │
                     ▼
Client presents confirmation to user
                     │
                     ▼
Client calls pods_delete again with confirm=true
                     │
                     ▼
Server: sees confirm=true → executes delete → returns result
```

If `confirm=true` is missing, the confirmation prompt is returned again. All destructive tools are **also** gated by the `--allow-destructive` flag; if unset, the handler returns an error.

---

## 7. Output Formatting

```
format param: "text" (default) or "json"
```

### Text

- Lists → markdown tables (pods_list).
- Describes → key-value blocks (pods_describe, pods_get).
- Logs → plain text (pods_log).

Example (pods_list text):

```
NAMESPACE     NAME                    READY   STATUS    RESTARTS   AGE
default       nginx-abc123            1/1     Running   0          5d
default       redis-def456            0/1     CrashLoopBackOff 3   2h
kube-system   coredns-xyz789          1/1     Running   0          30d
```

### JSON

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
        { "apiVersion": "apps/v1", "kind": "ReplicaSet", "name": "nginx-abc123" }
      ]
    }
  ]
}
```

---

## 8. Backend: K8s Typed Client (only what Pods need)

```go
// internal/k8s/client.go

// Client embeds the typed Kubernetes clientset plus the resolved identity of the
// active context/cluster/namespace, so callers (and the cluster_name tool) can
// report *what* they are talking to.
type Client struct {
    kubernetes.Interface

    ContextName string   // resolved active context
    ClusterName string   // cluster the active context points to (from kubeconfig)
    Namespace   string   // --namespace > context namespace > "default"
    User        UserInfo // resolved AuthInfo incl. impersonation
}

func NewClient(cfg *Config) (*Client, error) {
    // clientcmd loading rules + overrides → ClientConfig() → kubernetes.NewForConfig
    // resolveContext(loadingRules, cfg) fills ContextName / ClusterName / Namespace / User
}

// pods_list      → client.CoreV1().Pods(ns).List(ctx, opts)
// pods_get       → client.CoreV1().Pods(ns).Get(ctx, name, opts)
// pods_log       → client.CoreV1().Pods(ns).GetLogs(name, opts).Stream(ctx)
// pods_describe  → GET pod + list events for pod UID
// pods_delete    → client.CoreV1().Pods(ns).Delete(ctx, name, opts) + grace period
// cluster_name   → client.ClusterName (resolved at startup, no API call)
```

`k8s/dynamic.go` is **not** part of Stage 1. When generic resources are later desired, the dynamic client and `K8s Dynamic Client` section return in a future stage.

---

## 9. RBAC Template (pods-only)

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
# Pods — read focus of Stage 1
- apiGroups: [""]
  resources: [pods, pods/log, pods/status]
  verbs: [get, list, watch]
# Events — needed by pods_describe
- apiGroups: [""]
  resources: [events]
  verbs: [get, list, watch]
---
# Destructive permissions (NOT bound by default)
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: mimiops-mcp-destructive
rules:
- apiGroups: [""]
  resources: [pods]
  verbs: [delete]
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

No Helm secrets read, no metrics, no workloads, no jobs/cronjobs in Stage-1 RBAC.

---

## 10. Error Handling Strategy

| Scenario | Behavior |
|----------|----------|
| Invalid kubeconfig path | Log error and exit immediately |
| K8s API failure (network/auth/RBAC) | `Failed to list pods: <cause>` |
| Pod not found | `Pod 'foo' not found in namespace 'bar'` |
| Permission denied | `Permission denied: user cannot list pods in namespace 'bar'` |
| Invalid tool params | `Invalid parameter 'kind' ...` |
| Destructive tool without `--allow-destructive` | `Destructive tool disabled. Restart with --allow-destructive.` |
| `pods_log` container omitted + multi-container pod | Error listing available containers |
| Pod deletion with confirm=false | Return confirmation prompt (inputRequired=true) |

---

## 11. Summary of Key Design Decisions (Rev.)

| Decision | Choice | Rationale |
|----------|--------|-----------|
| **Language** | Go | Required by user |
| **K8s client** | client-go **typed** (no dynamic, no metrics) | Pods-only stage; typed is type-safe and minimal |
| **Helm** | Out of scope | No Helm in pods-only stage |
| **MCP SDK** | mark3labs/mcp-go | 2026 spec support |
| **Transports** | Two subcommands: `mcp` = stdio, `server` = SSE | Explicit, self-documenting mode selection |
| **Clusters** | Single at a time | Simple |
| **Kubeconfig source** | `--kubeconfig` > `$KUBECONFIG` > `~/.kube/config` | kubectl-compatible |
| **Context lifecycle** | Single root ctx in main, passed to all tools | One cancel for graceful shutdown |
| **Destructive guard** | `--allow-destructive` + `input_required` | Two layers of safety |
| **Output** | `format` param `text`/`json` | Both human & machine |
| **Logs** | Static fetch, `tail`, `previous`, `since_seconds` | Simple, no streaming complexity |
| **Config file** | None, CLI flags only | Keep it simple |
| **RBAC** | Separate read vs destructive roles | Least privilege |
| **Events** | Pulled in `pods_describe` per-pod | No separate events tool in Stage 1 |
| **Tool registration** | Central `internal/tools/register.go`; each tool = description + implementation in its own file (`cluster.go`, `pods.go`, `pods_log.go`, `pods_delete.go`) | Self-documenting, easy to add/extend |
| **Cluster identity** | `cluster_name` reads `Client.ClusterName` resolved from kubeconfig at startup | No API call or RBAC per call |

---

This is the Stage-1 pods-only architecture. When ready, implementation can proceed to code within this scope and register the 5 tools with dependencies only.
