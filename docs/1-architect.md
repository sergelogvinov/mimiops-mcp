

Perfect. Now I have everything I need. Let me produce the complete, final architecture document.

---

# Kubernetes MCP Server — Final Architecture Document

## Go-based Local Daemon with stdio + SSE, Helm SDK, Confirmation Flows

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
│                   mimiops-mcp (single Go binary)                  │
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
│  │  │   pods_list     pods_get    pods_describe      │  │    │
│  │  │   pods_log      pods_top                       │  │    │
│  │  │   workloads_list  workloads_get                │  │    │
│  │  │   workloads_describe                           │  │    │
│  │  │   helm_list    helm_status   helm_history      │  │    │
│  │  │   events_list  namespaces_list  nodes_list     │  │    │
│  │  │   resources_list  resources_describe           │  │    │
│  │  │   api_versions                                 │  │    │
│  │  │                                                │  │    │
│  │  │  Destructive (gated by --allow-destructive):   │  │    │
│  │  │   pods_delete   rollout_restart                │  │    │
│  │  └────────────────────────────────────────────────┘  │    │
│  └───────────────────────┬────────────────────────────┘    │
│                          │                                  │
│  ┌───────────────────────▼────────────────────────────┐    │
│  │  Tool Handlers                                     │    │
│  │                                                    │    │
│  │  Each handler:                                     │    │
│  │   1. Validate params                               │    │
│  │   2. If destructive + allowDestructive:             │    │
│  │      → Return input_required prompt                │    │
│  │   3. Execute (K8s API / Helm SDK)                  │    │
│  │   4. Format response (text or JSON per format param)│    │
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

---

## 2. Project Structure

```
mimiops-mcp/
├── go.mod
├── go.sum
├── Makefile
├── README.md
├── cmd/
│   └── mimiops-mcp/
│       └── main.go
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
│   │   ├── cronjobs.go          # cronjobs_list, cronjobs_get, cronjobs_describe,
│   │   │                        # cronjobs_suspend, cronjobs_resume           
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
├── tests/
│   ├── mcp_test.go
│   └── k8s_test.go
└── deploy/
    └── rbac.yaml                # Example ClusterRole for the service account
```

---

## 3. Core Dependencies

```go

module github.com/sergelogvinov/mimiops-mcp

go 1.26

require (
    github.com/mark3labs/mcp-go v0.58.0  // MCP SDK
    
    k8s.io/api v0.35.0                     // K8s API types
    k8s.io/apimachinery v0.35.0            // API machinery
    k8s.io/client-go v0.35.0              // K8s client
    k8s.io/metrics v0.35.0                // Metrics (pods_top, nodes_top)
    
    helm.sh/helm/v3 v3.21.3               // Helm SDK
    
    k8s.io/klog/v2 v2.140.0                // Structured logging

    github.com/spf13/pflag v1.0.10         // Flags monipulation
)
```

No external CLI frameworks — use `flag` stdlib for simplicity.

---

## 4. CLI Interface

```bash
# Default mode (stdio, reads ~/.kube/config)
./mimiops-mcp

# Explicit kubeconfig + context
./mimiops-mcp --kubeconfig /path/to/kubeconfig --context prod-east

# SSE mode for web/remote MCP clients
./mimiops-mcp --transport sse --port 8080 --namespace default

# Enable destructive actions
./mimiops-mcp --allow-destructive

# Combined
./mimiops-mcp --kubeconfig ~/.kube/dev.yaml --transport sse --port 8080 --allow-destructive --log-level debug

# Kubeconfig via environment variable (KUBECONFIG)
KUBECONFIG=~/path/to/kubeconfig ./mimiops-mcp --context prod-east
```

### Kubeconfig resolution

The kubeconfig path can be provided in two ways, with the CLI flag taking precedence:

1. **CLI flag** `--kubeconfig /path/to/kubeconfig` — explicit path.
2. **Environment variable** `KUBECONFIG=~/path/to/kubeconfig` — used when the flag is not set.
3. **Default** — falls back to the standard `~/.kube/config`.

When the resolved kubeconfig contains **multiple cluster configurations**, the active cluster is selected via the `--context` parameter. If no context is provided, the current-context defined inside the kubeconfig file is used.

### Config struct

```go
// internal/config/config.go

type Config struct {
    Kubeconfig       string   // path; falls back to $KUBECONFIG env, then ~/.kube/config
    Context          string   // selects active cluster when kubeconfig has multiple configs
    Namespace        string   // "" means all namespaces
    Transport        string   // "stdio" or "sse"
    Port             int      // for SSE mode
    AllowDestructive bool
    LogLevel         string   // "debug", "info", "warn", "error"
}
```

---

## 5. Tool Specifications (Complete Catalog)

### 5.1 Read Tools

<details>
<summary><b>pods_list</b></summary>

| Field | Value |
|-------|-------|
| **name** | `pods_list` |
| **description** | List pods in a namespace (or all namespaces) |
| **params** | `namespace` (string, optional), `label_selector` (string, optional), `field_selector` (string, optional), `format` (string, optional: `"text"` or `"json"`, default: `"text"`) |
| **response** | Text: markdown table (name, namespace, status, age, node) OR JSON: `[]PodSummary`. Each pod includes `ownerReferences` with `apiVersion`, `kind`, and `name` of the owning workload (e.g. ReplicaSet/Deployment). |
| **RBAC** | `get`, `list` pods |
</details>

<details>
<summary><b>pods_get</b></summary>

| Field | Value |
|-------|-------|
| **name** | `pods_get` |
| **description** | Get full pod spec and status |
| **params** | `name` (required), `namespace` (required), `format` (optional, default: `"text"`) |
| **response** | Text: key-value describe format / JSON: full `v1.Pod` object |
</details>

<details>
<summary><b>pods_describe</b></summary>

| Field | Value |
|-------|-------|
| **name** | `pods_describe` |
| **description** | Human-readable pod summary (conditions, container statuses, events) |
| **params** | `name` (required), `namespace` (required), `format` (optional, default: `"text"`) |
| **response** | Rich formatted description with status, conditions, container states, recent events (Text only for human readability, JSON available for machine parsing) |
</details>

<details>
<summary><b>pods_log</b></summary>

| Field | Value |
|-------|-------|
| **name** | `pods_log` |
| **description** | Fetch pod logs |
| **params** | `name` (required), `namespace` (required), `container` (string, optional), `tail` (int, optional, default: `20`, max: `5000`), `previous` (bool, optional, default: `false` — reads logs from crashed container), `format` (optional, default: `"text"`) |
| **response** | Text: log lines as plain text / JSON: `[]LogLine` |
| **notes** | `previous=true` maps to `--previous` / `-p` in kubectl. `tail` maps to `--tail`. If `container` is omitted and pod has multiple containers, return an error listing available containers. |
</details>

<details>
<summary><b>pods_top</b></summary>

| Field | Value |
|-------|-------|
| **name** | `pods_top` |
| **description** | Show pod resource usage (requires metrics-server) |
| **params** | `namespace` (optional), `name` (optional), `format` (optional, default: `"text"`) |
| **response** | Table: pod, cpu, memory |
</details>

<details>
<summary><b>jobs_list</b></summary>

| Field | Value |
|-------|-------|
| **name** | `jobs_list` |
| **description** | List Jobs in a namespace (or all) |
| **params** | `namespace` (optional), `label_selector` (optional), `format` (optional, default: `"text"`) |
| **response** | Table: name, namespace, completions, duration, age, status (Complete/Failed/Running) |
</details>

<details>
<summary><b>jobs_get</b></summary>

| Field | Value |
|-------|-------|
| **name** | `jobs_get` |
| **description** | Get a single Job's full spec and status |
| **params** | `name` (required), `namespace` (required), `format` (optional, default: `"text"`) |
| **response** | Text: key-value describe / JSON: full `v1.Job` object |
</details>

<details>
<summary><b>jobs_describe</b></summary>

| Field | Value |
|-------|-------|
| **name** | `jobs_describe` |
| **description** | Human-readable Job summary (conditions, parallelism, backoff, pod selector) |
| **params** | `name` (required), `namespace` (required), `format` (optional, default: `"text"`) |
| **response** | Rich formatted description |
</details>

<details>
<summary><b>jobs_log</b></summary>

| Field | Value |
|-------|-------|
| **name** | `jobs_log` |
| **description** | Fetch logs from a Job's most recent pod (or all pods) |
| **params** | `name` (required), `namespace` (required), `tail` (int, optional, default: `20`), `previous` (bool, optional, default: `false`), `all_pods` (bool, optional, default: `false` — fetch logs from all pods owned by the job, not just the latest), `format` (optional, default: `"text"`) |
| **response** | Log lines, prefixed with pod name if `all_pods=true` |
| **implementation** | Finds pods owned by the Job via `metav1.GetControllerOf` with `batch/v1.Job` owner reference. If `all_pods=false`, picks the most recently created pod. Then calls the same log fetch as `pods_log`. |
</details>

<details>
<summary><b>cronjobs_list</b></summary>

| Field | Value |
|-------|-------|
| **name** | `cronjobs_list` |
| **description** | List CronJobs in a namespace (or all) |
| **params** | `namespace` (optional), `format` (optional, default: `"text"`) |
| **response** | Table: name, namespace, schedule, suspend, last_schedule, age |
</details>

<details>
<summary><b>cronjobs_get</b></summary>

| Field | Value |
|-------|-------|
| **name** | `cronjobs_get` |
| **description** | Get a single CronJob's full spec and status |
| **params** | `name` (required), `namespace` (required), `format` (optional, default: `"text"`) |
| **response** | Full describe |
</details>

<details>
<summary><b>cronjobs_describe</b></summary>

| Field | Value |
|-------|-------|
| **name** | `cronjobs_describe` |
| **description** | Human-readable CronJob summary (schedule, suspend, active jobs, last schedule, concurrency policy) |
| **params** | `name` (required), `namespace` (required), `format` (optional, default: `"text"`) |
| **response** | Rich formatted description |
</details>

<details>
<summary><b>workloads_list</b></summary>

| Field | Value |
|-------|-------|
| **name** | `workloads_list` |
| **description** | List Deployments, StatefulSets, or DaemonSets |
| **params** | `namespace` (optional), `kind` (required, enum: `"deployment"`, `"statefulset"`, `"daemonset"`), `format` (optional, default: `"text"`) |
| **response** | Table: name, namespace, replicas (ready/desired), age |
</details>

<details>
<summary><b>workloads_get</b></summary>

| Field | Value |
|-------|-------|
| **name** | `workloads_get` |
| **description** | Get a single workload's full spec and status |
| **params** | `name` (required), `namespace` (required), `kind` (required), `format` (optional, default: `"text"`) |
| **response** | Full describe output |
</details>

<details>
<summary><b>workloads_describe</b></summary>

| Field | Value |
|-------|-------|
| **name** | `workloads_describe` |
| **description** | Rich describe: replicas, conditions, selector, strategy, history |
| **params** | `name` (required), `namespace` (required), `kind` (required), `format` (optional, default: `"text"`) |
| **response** | Rich formatted description |
</details>

<details>
<summary><b>helm_list</b></summary>

| Field | Value |
|-------|-------|
| **name** | `helm_list` |
| **description** | List Helm releases |
| **params** | `namespace` (optional), `format` (optional, default: `"text"`) |
| **response** | Table: name, namespace, revision, updated, status, chart, app_version |
</details>

<details>
<summary><b>helm_status</b></summary>

| Field | Value |
|-------|-------|
| **name** | `helm_status` |
| **description** | Get detailed status of a Helm release |
| **params** | `name` (required), `namespace` (required), `format` (optional, default: `"text"`) |
| **response** | Status, revision, values (JSON), notes, description |
</details>

<details>
<summary><b>helm_history</b></summary>

| Field | Value |
|-------|-------|
| **name** | `helm_history` |
| **description** | Show revision history of a release |
| **params** | `name` (required), `namespace` (required), `format` (optional, default: `"text"`) |
| **response** | Table: revision, updated, status, chart, app_version, description |
</details>

<details>
<summary><b>events_list</b></summary>

| Field | Value |
|-------|-------|
| **name** | `events_list` |
| **description** | List Kubernetes events, sorted by time (warnings first) |
| **params** | `namespace` (optional), `field_selector` (optional), `format` (optional, default: `"text"`) |
| **response** | Table: last_seen, type (Warning/Normal), reason, object, message |
</details>

<details>
<summary><b>namespaces_list</b></summary>

| Field | Value |
|-------|-------|
| **name** | `namespaces_list` |
| **description** | List all namespaces |
| **params** | `format` (optional, default: `"text"`) |
| **response** | Table: name, status (Active/Terminating), age |
</details>

<details>
<summary><b>nodes_list</b></summary>

| Field | Value |
|-------|-------|
| **name** | `nodes_list` |
| **description** | List cluster nodes and their status |
| **params** | `format` (optional, default: `"text"`) |
| **response** | Table: name, status, roles, age, version |
</details>

<details>
<summary><b>resources_list</b></summary>

| Field | Value |
|-------|-------|
| **name** | `resources_list` |
| **description** | List any resource type by GVK |
| **params** | `api_version` (required, e.g. `"batch/v1"`), `kind` (required, e.g. `"CronJob"`), `namespace` (optional), `label_selector` (optional), `format` (optional, default: `"text"`) |
| **response** | Table of resources |
| **notes** | Uses `client-go/dynamic` client to handle any GVK at runtime |
</details>

<details>
<summary><b>resources_describe</b></summary>

| Field | Value |
|-------|-------|
| **name** | `resources_describe` |
| **description** | Describe any resource by GVK + name |
| **params** | `api_version` (required), `kind` (required), `name` (required), `namespace` (optional), `format` (optional, default: `"text"`) |
| **response** | Rich describe of the resource |
</details>

<details>
<summary><b>api_versions</b></summary>

| Field | Value |
|-------|-------|
| **name** | `api_versions` |
| **description** | List available API versions and resource kinds |
| **params** | None |
| **response** | Structured list of API groups → versions → kinds |
</details>

### 5.2 Destructive Tools

<details>
<summary><b>pods_delete</b></summary>

| Field | Value |
|-------|-------|
| **name** | `pods_delete` |
| **description** | Delete a pod |
| **params** | `name` (required), `namespace` (required), `grace_period_seconds` (int, optional, default: `30`) |
| **flow** | 1. Only registered if `--allow-destructive` is set. 2. On call, handler returns `input_required` prompt: "Are you sure you want to delete pod 'foo' in namespace 'bar'? Type 'yes' to confirm." 3. Client sends confirmation. 4. If confirmed, execute delete and return result. 5. If not confirmed, return cancellation. |
| **RBAC** | `delete` pods |
</details>

<details>
<summary><b>cronjobs_suspend</b></summary>

| Field | Value |
|-------|-------|
| **name** | `cronjobs_suspend` |
| **description** | Suspend a CronJob (stops future runs) |
| **params** | `name` (required), `namespace` (required) |
| **destructive** | Yes — gated by `--allow-destructive` + `input_required` confirmation. Rationale: Pausing scheduled jobs can have business impact. |
| **action** | Patches the CronJob's `spec.suspend` to `true` |
| **RBAC** | `patch` cronjobs |
</details>

<details>
<summary><b>cronjobs_resume</b></summary>

| Field | Value |
|-------|-------|
| **name** | `cronjobs_resume` |
| **description** | Resume a suspended CronJob |
| **params** | `name` (required), `namespace` (required) |
| **destructive** | Yes — gated by `--allow-destructive` + `input_required` confirmation. Rationale: Resuming a CronJob that was intentionally suspended can have production impact. |
| **action** | Patches the CronJob's `spec.suspend` to `false` |
| **RBAC** | `patch` cronjobs |
</details>

<details>
<summary><b>jobs_delete</b></summary>

| Field | Value |
|-------|-------|
| **name** | `jobs_delete` |
| **description** | Delete a Job (cascading — also deletes owned pods) |
| **params** | `name` (required), `namespace` (required), `propagation_policy` (string, optional, enum: `"Background"`, `"Foreground"`, `"Orphan"`, default: `"Background"`) |
| **destructive** | Yes — gated by `--allow-destructive` + `input_required` confirmation |
| **RBAC** | `delete` jobs |
</details>

<details>
<summary><b>rollout_restart</b></summary>

| Field | Value |
|-------|-------|
| **name** | `rollout_restart` |
| **description** | Rollout restart a Deployment, StatefulSet, or DaemonSet |
| **params** | `name` (required), `namespace` (required), `kind` (required, enum: `"deployment"`, `"statefulset"`, `"daemonset"`) |
| **flow** | Same as `pods_delete`: registration gated by `--allow-destructive`, then `input_required` confirmation prompt. |
| **action** | Patches the workload's spec to trigger a rollout restart (injects a `kubectl.kubernetes.io/restartedAt` annotation, or touches the pod template) |
| **RBAC** | `patch` on the workload type |
</details>

---

## 6. Destructive Confirmation Flow (input_required)

This follows the MCP spec's `input_required` mechanism (MRTR — Mandatory Required Tool Response).

```
Client calls pods_delete(name="nginx-abc123", namespace="default")
                     │
                     ▼
Server receives request
  → Checks allowDestructive flag (fail if false)
  → Validates params
  → Returns CallToolResult with isError=false, but:
      - content: "This action will delete pod 'nginx-abc123' in namespace 'default'.
                  To confirm, call this tool again with confirm=true"
      - inputRequired set to true
      - OR returns a specific response structure that the client recognizes as a confirmation prompt

                     │
                     ▼
Client presents confirmation to user
  User clicks "Yes" or types confirmation
                     │
                     ▼
Client calls pods_delete again with same params + confirm=true
                     │
                     ▼
Server receives request
  → Sees confirm=true
  → Executes the delete
  → Returns result
```

**Implementation approach**: Use two-phase call.

- **Phase 1** (no `confirm` param or `confirm=false`): Return a description of what will happen and ask for confirmation. The response content is: `"This will delete pod 'nginx-abc123' in namespace 'default'. Call with confirm=true to proceed."`
- **Phase 2** (`confirm=true`): Execute and return result. If `confirm=true` is missing or `false`, return the prompt again.

This maps cleanly to how MCP clients handle tool responses that contain prompts for the user.

---

## 7. Output Formatting

```
format param: "text" (default) or "json"
```

### Text format

Designed for human readability. Uses:
- Tables for lists (pods_list, workloads_list, helm_list, etc.)
- Key-value blocks for describes (pods_describe, workloads_describe)
- Plain text for logs (pods_log)

Example (pods_list text):

```
NAMESPACE     NAME                    READY   STATUS    RESTARTS   AGE
default       nginx-abc123            1/1     Running   0          5d
default       redis-def456            0/1     CrashLoopBackOff 3   2h
kube-system   coredns-xyz789          1/1     Running   0          30d
```

### JSON format

Returns a JSON object/array that can be parsed programmatically by the MCP client.

Example (pods_list JSON):

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

---

## 8. Helm SDK Integration

Use `helm.sh/helm/v3` as a Go library (not subprocess calls).

```go
// internal/helm/client.go

type HelmClient struct {
    client *helm.Client
    // Namespace stored per request, not at client level
}

func NewHelmClient(kubeConfig *rest.Config) (*HelmClient, error) {
    client, err := helm.NewClient(&helm.Options{
        RESTClientGetter: &k8sConfig{
            restConfig: kubeConfig,
        },
    })
    return &HelmClient{client: client}, nil
}

// helm_list: calls client.ListReleases(namespace)
// helm_status: calls client.GetRelease(name, namespace)
// helm_history: calls client.GetReleaseHistory(name, namespace)
```

Key detail: Helm stores release metadata in Kubernetes Secrets (labeled `owner: helm`). The SDK reads these. The kubeconfig user needs `list` + `get` on Secrets in the relevant namespace, or a broad enough RBAC.

---

## 9. K8s Dynamic Client for Generic Resources

```go
// internal/k8s/dynamic.go

// resources_list and resources_describe use the dynamic client
// to handle any GVK at runtime without compile-time type registration.

func ListResources(dynamicClient dynamic.Interface, apiVersion, kind, namespace, labelSelector string) ([]unstructured.Unstructured, error) {
    gv, _ := schema.ParseGroupVersion(apiVersion)
    gvr := schema.GroupVersionResource{
        Group:    gv.Group,
        Version:  gv.Version,
        Resource: pluralize(kind), // e.g., "CronJob" → "cronjobs"
    }
    
    // Use discovery client to map Kind → Resource name
    // Then list via dynamicClient.Resource(gvr).Namespace(namespace).List(...)
}
```

---

## 10. Startup Sequence

```
1. Parse CLI flags → populate Config struct
   a. Resolve kubeconfig: --kubeconfig flag > $KUBECONFIG env > ~/.kube/config
   b. If kubeconfig has multiple cluster configs, select active one via --context
2. Initialize logger (klog/v2) with configured log level
3. Create root context in main.go:
   ctx, cancel := context.WithCancel(context.Background())
   defer cancel()
   - This single ctx is passed to all tools and utils
   - Do NOT create additional contexts inside functions
4. Create k8s client:
   a. Load kubeconfig from resolved path
   b. Create typed client (kubernetes.Interface)
   c. Create dynamic client (dynamic.Interface)
   d. Create metrics client (for pods_top / nodes_top)
5. Create Helm client (if needed, lazy init on first helm tool call)
6. Create MCPServer instance
7. Register tools:
   a. Register all read tools unconditionally
   b. Register destructive tools only if allowDestructive=true
8. Start transport:
   a. If stdio: call server.ServeStdio(mcpServer)
   b. If sse: call server.ServeSSE(mcpServer, port)
9. Block until signal (SIGINT/SIGTERM), then call cancel() for graceful shutdown
```

---

## 11. Error Handling Strategy

| Scenario | Behavior |
|----------|----------|
| Invalid kubeconfig path | Log error and exit immediately |
| K8s API call fails (network, auth, RBAC) | Return error with message: "Failed to list pods: ..." |
| Resource not found | Return error with message: "Pod 'foo' not found in namespace 'bar'" |
| Permission denied (RBAC) | Return error with message: "Permission denied: user cannot list pods in namespace 'bar'. Missing verb 'list' on resource 'pods'" |
| Invalid tool params | Return error with message: "Invalid parameter 'kind': must be one of 'deployment', 'statefulset', 'daemonset'" |
| Destructive tool without `--allow-destructive` | Return error with message: "Destructive tools are disabled. Restart with --allow-destructive to enable." |
| Helm call fails (no helm secrets found) | Return error describing the issue |
| Logs: pod not found or container not found | Return error with message and suggestion of available containers |

---

## 12. RBAC Template (for cluster setup)

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
# Pods
- apiGroups: [""]
  resources: [pods, pods/log, pods/status, pods/metrics]
  verbs: [get, list, watch]
# Jobs + CronJobs
- apiGroups: ["batch"]
  resources: [jobs, jobs/status, cronjobs, cronjobs/status]
  verbs: [get, list, watch]
# ConfigMaps — read metadata only, data is never returned by the tool
- apiGroups: [""]
  resources: [configmaps]
  verbs: [get, list]
# Workloads
- apiGroups: ["apps"]
  resources: [deployments, deployments/status, deployments/scale,
              statefulsets, statefulsets/status, statefulsets/scale,
              daemonsets, daemonsets/status]
  verbs: [get, list, watch]
# Core infrastructure
- apiGroups: [""]
  resources: [events, namespaces, nodes, nodes/status, nodes/metrics,
              persistentvolumeclaims, services, endpoints]
  verbs: [get, list, watch]
# Helm — needed by Helm SDK to read release metadata from Secrets
- apiGroups: [""]
  resources: [secrets]
  verbs: [get, list]
  resourceNames: ["sh.helm.release.v1.*"]
# Metrics
- apiGroups: ["metrics.k8s.io"]
  resources: [pods, nodes]
  verbs: [get, list]
---
# Destructive permissions (separate ClusterRole, not bound by default)
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: mimiops-mcp-destructive
rules:
- apiGroups: [""]
  resources: [pods]
  verbs: [delete]
- apiGroups: ["apps"]
  resources: [deployments, statefulsets, daemonsets]
  verbs: [patch]
- apiGroups: ["batch"]                    # NEW
  resources: [jobs, cronjobs]
  verbs: [delete, patch]
- apiGroups: [""]                         # Required by Helm SDK for rollback
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

## 14. Summary of Key Design Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| **Language** | Go | Requested |
| **K8s client** | client-go | Standard, typed + dynamic |
| **Helm** | helm.sh/helm/v3 SDK | No external binary dependency |
| **MCP SDK** | mark3labs/mcp-go | Actively maintained, supports 2026 spec |
| **Transports** | stdio + SSE (both) | stdio for desktop, SSE for flexibility |
| **Clusters** | Single at a time | Keep it simple; one kubeconfig per invocation |
| **Kubeconfig source** | `--kubeconfig` flag > `$KUBECONFIG` env > `~/.kube/config` | Standard kubectl-compatible resolution |
| **Multi-cluster kubeconfig** | `--context` selects active cluster | Switch between cluster configs in one file |
| **Context lifecycle** | Single root `ctx` created in `main.go` via `context.WithCancel`, passed to all tools/utils | No per-function contexts; one cancel point for graceful shutdown |
| **Destructive guard** | `--allow-destructive` flag + `input_required` | Two layers of safety |
| **Output** | `format` param: `text` or `json` | Both human and machine readable |
| **Logs** | Static fetch, `tail=N`, `previous=bool` | Simple, no streaming complexity |
| **Config file** | None | CLI flags only, keep it simple |
| **Error handling** | Descriptive error messages | Help the user fix the problem |
| **RBAC** | Separate read vs destructive ClusterRoles | Principle of least privilege |

---

This is the complete architecture. You can start implementing with confidence — the design covers all the decisions you specified.
