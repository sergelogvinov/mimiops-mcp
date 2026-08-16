# Kubernetes MCP Server — Stage 3: Cluster Observability (Read-Only)

## Go-based Local Daemon, Scoped to Cluster-Level Introspection

This document extends the architecture in `1-architect.md` / `3-architecture-cronjobs.md` (Stages 1–2) with a **read-only cluster observability** tool set. It reuses the exact same CLI, MCP registration, formatting, and confirmation conventions established in earlier stages — only the tool catalog, backend calls, and RBAC are new.

**Scope for this stage:** eleven read-only tools that help observe the cluster and its namespaced policy objects:

| Resource | Tools |
|----------|-------|
| Nodes | `nodes_list` (refined), `nodes_get` (NEW) |
| Namespaces | `namespaces_list` (refined), `namespaces_get` (NEW) |
| ResourceQuotas | `resourcequotas_list`, `resourcequotas_get` (NEW) |
| LimitRanges | `limitranges_list`, `limitranges_get` (NEW) |
| StorageClasses | `storageclasses_list` (NEW) |
| PriorityClasses | `priorityclasses_list` (NEW) |
| Events | `events_get` (NEW) |

**All tools are read-only.** There are **no** mutating/destructive tools in this stage, so nothing is gated by `--allow-destructive`, and the `input_required` confirmation flow does not apply. Every tool is registered unconditionally.

---

## 1. Architectural Overview

The architecture is unchanged from Stages 1–2. The only delta is the tool registry content and the backend calls it makes against `core/v1`, `storage.k8s.io/v1`, and `scheduling.k8s.io/v1`.

```
┌──────────────────────────────────────────────────────────────┐
│                      MCP Client                              │
│   (Claude Desktop, Cursor, VS Code, Goose, custom script)    │
└───────────────────────────┬──────────────────────────────────┘
                            │  JSON-RPC 2.0 (stdio or SSE)
                            ▼
┌──────────────────────────────────────────────────────────────┐
│                   mimiops-mcp (single Go binary)             │
│                                                              │
│  ┌──────────────────────────────────────────────────────┐    │
│  │  CLI (cobra) — unchanged from Stages 1–2            │    │
│  │    mcp | server | version                            │    │
│  │    --kubeconfig --context --namespace --impersonate  │    │
│  │    --allow-destructive --log-level  (+ --port)       │    │
│  └──────────────────────────┬───────────────────────────┘    │
│                             │                                 │
│  ┌──────────────────────────▼───────────────────────────┐    │
│  │  MCP Server (mark3labs/mcp-go)                       │    │
│  │  Tool Registry — Stages 1–2 + Stage 3 additions:     │    │
│    nodes_list  namespaces_list  namespaces_get       │
│  │    resourcequotas_list  resourcequotas_get           │    │
│  │    limitranges_list  limitranges_get                 │    │
│  │    storageclasses_list  priorityclasses_list         │    │
│  │    events_get                                        │    │
│  │    nodes_get                                         │    │
│  │    (all read-only; registered unconditionally)       │    │
│  └───────────────────────┬────────────────────────────┘    │
│                          │                                  │
│  ┌───────────────────────▼────────────────────────────┐    │
│  │  Tool Handlers (internal/tools)                     │    │
│  │    one file per tool (see §2)                       │    │
│  └───────────────────────┬────────────────────────────┘    │
│                          │                                  │
│  ┌───────────────────────▼────────────────────────────┐    │
│  │  Backend: k8s typed client (client-go)             │    │
│  │    CoreV1().Nodes() / Namespaces()                 │    │
│  │    CoreV1().ResourceQuotas(ns) / LimitRanges(ns)   │    │
│  │    StorageV1().StorageClasses()                    │    │
│  │    SchedulingV1().PriorityClasses()                │    │
│  └───────────────────────┬────────────────────────────┘    │
└──────────────────────────┼─────────────────────────────────┘
                           ▼
                  ┌─────────────────────────┐
                  │   Kubernetes API Server  │
                  │   (authenticated via     │
                  │    kubeconfig)           │
                  └─────────────────────────┘
```

No new transports, no new CLI flags, no new external dependencies. This stage is purely additive to the tool catalog and is entirely read-only.

---

## 2. Project Structure (Stage 3 additions)

The existing tree is unchanged; new tool files are added under `internal/tools/`, **one file per tool** (each exposing a single `Register*` function). No changes to `cmd/`, `internal/config`, `internal/k8s`, or `internal/formatter`.

```
mimiops/
├── internal/
│   ├── tools/
│   │   ├── register.go          # central wiring: calls every Register* below
│   │   ├── nodes_list.go        # nodes_list                              (refined)
│   │   ├── nodes_get.go         # nodes_get                               (NEW)
│   │   ├── namespaces_list.go   # namespaces_list                         (refined)
│   │   ├── namespaces_get.go    # namespaces_get                          (NEW)
│   │   ├── resourcequotas_list.go  # resourcequotas_list                  (NEW)
│   │   ├── resourcequotas_get.go   # resourcequotas_get                   (NEW)
│   │   ├── limitranges_list.go     # limitranges_list                     (NEW)
│   │   ├── limitranges_get.go      # limitranges_get                      (NEW)
│   │   ├── storageclasses_list.go  # storageclasses_list                  (NEW)
│   │   ├── priorityclasses_list.go # priorityclasses_list                 (NEW)
│   │   └── events_get.go           # events_get                           (NEW)
│   └── ...
└── deploy/
    └── rbac.yaml                # + cluster observability read rules (see §8)
```

**File ownership rule (from Stage 1, enforced here):** each tool = one `Register*` function in its **own** file, with the `mcp.NewTool(...)` description/schema and the handler colocated. `internal/tools/register.go` is the only place that names every tool. No two tools share a file in this stage.

---

## 3. Core Dependencies

**No new dependencies.** `core/v1` (Nodes, Namespaces, ResourceQuotas, LimitRanges), `storage.k8s.io/v1` (StorageClasses), and `scheduling.k8s.io/v1` (PriorityClasses) are all part of the already-present `k8s.io/client-go` / `k8s.io/api` typed clientset.

```go
module github.com/sergelogvinov/mimiops-mcp

go 1.26

    github.com/spf13/pflag v1.0.10        // Flags
    github.com/spf13/cobra v1.10.2        // CLI command dispatch
    k8s.io/client-go                      // typed clientset (CoreV1, StorageV1, SchedulingV1)
    k8s.io/api                            // v1, storage/v1, scheduling/v1 types
```

Helm, metrics, and the dynamic client remain out of scope for this stage.

---

## 4. CLI Interface

**Unchanged from Stages 1–2.** No new flags or subcommands. The `--namespace` global flag scopes the namespaced tools (`resourcequotas_*`, `limitranges_*`); empty means all namespaces. Cluster-scoped tools (`nodes_list`, `nodes_get`, `namespaces_*`, `storageclasses_list`, `priorityclasses_list`) ignore `--namespace`. `--allow-destructive` is **not** relevant — this stage has no mutating tools.

---

## 5. Tool Specifications (Stage-3 Catalog)

All tools follow the established conventions: `format` param (`"text"` default / `"json"`), typed client-go, and the central `register.go` wiring. **All tools are read-only.**

### 5.1 Read Tools

<details>
<summary><b>nodes_list</b></summary>

| Field | Value |
|-------|-------|
| **name** | `nodes_list` |
| **description** | List cluster nodes and their status |
| **params** | `include_allocations` (bool, optional, default: `false` — include aggregate pod CPU/memory request & limit sums per node), `format` (string, optional, default: `"text"`) |
| **response** | Text: markdown table `name, status, roles, age, version, internal_ip` / JSON: `[]NodeSummary`. When `include_allocations=true`, adds `requests_cpu`, `requests_memory`, `limits_cpu`, `limits_memory` columns / fields. |
| **status derivation** | `Ready` if the `Ready` condition is `True`; `NotReady` if `False`; `Unknown` if `Unknown` or the condition is absent. If the node is cordoned (`spec.unschedulable=true`), append `SchedulingDisabled` (e.g. `Ready,SchedulingDisabled`). |
| **roles** | Derived from `node-role.kubernetes.io/<role>` labels (e.g. `control-plane`, `worker`); `none` if no role labels are present |
| **internal_ip** | From `status.addresses` where `type=InternalIP`; `-` if absent |
| **allocated-resources derivation** | Node `status` exposes only `capacity`/`allocatable`, never request/limit sums. Requests/limits are **computed** by listing all pods (all namespaces) in one `List`, grouping by `spec.nodeName`, and summing container `resources.requests` / `resources.limits` each against its node (CPU, memory). Values are shown as `request/allocatable` per resource, e.g. `1250m/8` or `4Gi/32Gi`; `-` when no pods allocate that resource. Mirrors `kubectl describe node`'s "Allocated resources". A warning is included that this is an approximation based on the last pod list (contended pods can skew it). |
| **RBAC** | `get`, `list` nodes; `get`, `list` pods (required only when `include_allocations=true`) |
</details>

<details>
<summary><b>nodes_get</b></summary>

| Field | Value |
|-------|-------|
| **name** | `nodes_get` |
| **description** | Get detailed information about a single node |
| **params** | `name` (required), `include_pods` (bool, optional, default: `false` — include a summary of pods running on the node), `format` (optional, default: `"text"`) |
| **response** | Text: rich describe-style key-value block / JSON: full `core/v1.Node` object |
| **fields (text)** | status (Ready/NotReady + SchedulingDisabled), roles, taints, schedulable, unschedulable, capacity (cpu/memory/pods), allocatable (cpu/memory/pods), **allocated resources (request/limit sums + percent across CPU/memory/pods)**, conditions (all `status.conditions`), addresses (InternalIP/InternalDNS/Hostname/ExternalIP), nodeInfo (kubeletVersion, osImage, containerRuntimeVersion, kernelVersion), pods (`core/v1.Pod` name + status, capped at 5 + `... and N more`), age |
| **allocated-resources (always shown)** | CPU/memory request & limit totals computed from running pods on the node, plus pod count and percentage of `allocatable` used. Presents the same derivation as `nodes_list`: sum container `resources.requests`/`resources.limits` (CPU, memory) across pods with `spec.nodeName == this node`, reported as request and limit sums (e.g. CPU `1250m` requests / `2000m` limits) with an `allocatable` percentage. This mirrors `kubectl describe node`'s "Allocated resources". Approximation warning as in `nodes_list`. |
| **implementation** | 1. `Get` the node. 2. List pods (all namespaces) with `spec.nodeName == name`; aggregate requests/limits and, if `include_pods=true`, list the matching pods (capped at 15). |
| **notes** | Useful for troubleshooting capacity, taints (which block scheduling), and node readiness before acting on workloads. `node-role.kubernetes.io/*` labels are surfaced to show roles. |
| **RBAC** | `get` nodes; `get`, `list` pods (needed in all cases to compute allocated-resource totals, and for the pod summary when `include_pods=true`) |
</details>

<details>
<summary><b>namespaces_list</b></summary>

| Field | Value |
|-------|-------|
| **name** | `namespaces_list` |
| **description** | List all namespaces |
| **params** | `format` (string, optional, default: `"text"`) |
| **response** | Text: markdown table `name, status, age` / JSON: `[]NamespaceSummary` |
| **status** | `status.phase` (`Active` / `Terminating`) |
| **age** | From `creationTimestamp` |
| **RBAC** | `get`, `list` namespaces |
</details>

<details>
<summary><b>namespaces_get</b></summary>

| Field | Value |
|-------|-------|
| **name** | `namespaces_get` |
| **description** | Get a single namespace's full spec and status |
| **params** | `name` (required), `format` (optional, default: `"text"`) |
| **response** | Text: key-value describe format (labels, annotations, status phase, finalizers, conditions) / JSON: full `core/v1.Namespace` object |
| **notes** | Useful for inspecting a namespace before acting on it (labels for selectors, finalizers that may block deletion, terminating state). |
| **RBAC** | `get` namespaces |
</details>

<details>
<summary><b>resourcequotas_list</b></summary>

| Field | Value |
|-------|-------|
| **name** | `resourcequotas_list` |
| **description** | List ResourceQuotas in a namespace (or all namespaces) |
| **params** | `namespace` (string, optional), `format` (optional, default: `"text"`) |
| **response** | Text: markdown table `namespace, name, requests (cpu/memory), limits (cpu/memory), age` / JSON: `[]ResourceQuotaSummary` |
| **summary derivation** | For each quota, summarize the `status.used` and `spec.hard` values for the standard `requests.cpu`, `requests.memory`, `limits.cpu`, `limits.memory` keys. Columns show `used/hard` (e.g. `1/4` for cpu, `2Gi/8Gi` for memory); `-` when a key is absent. |
| **notes** | The full per-resource breakdown (pods, configmaps, pvc, etc.) is available via `resourcequotas_get`. |
| **RBAC** | `get`, `list` resourcequotas |
</details>

<details>
<summary><b>resourcequotas_get</b></summary>

| Field | Value |
|-------|-------|
| **name** | `resourcequotas_get` |
| **description** | Get a single ResourceQuota's full spec and status |
| **params** | `name` (required), `namespace` (required), `format` (optional, default: `"text"`) |
| **response** | Text: key-value describe format listing every `spec.hard` limit and its corresponding `status.used` value, plus `status.conditions` (e.g. `Exceeded`) / JSON: full `core/v1.ResourceQuota` object |
| **notes** | The authoritative view of how much of each resource a namespace has consumed against its quota. |
| **RBAC** | `get` resourcequotas |
</details>

<details>
<summary><b>limitranges_list</b></summary>

| Field | Value |
|-------|-------|
| **name** | `limitranges_list` |
| **description** | List LimitRanges in a namespace (or all namespaces) |
| **params** | `namespace` (string, optional), `format` (optional, default: `"text"`) |
| **response** | Text: markdown table `namespace, name, types, age` / JSON: `[]LimitRangeSummary` |
| **types** | Comma-separated list of the resource types the LimitRange constrains (e.g. `Container, Pod, PersistentVolumeClaim`), derived from `spec.limits[].type` |
| **notes** | The detailed per-type min/max/default values are available via `limitranges_get`. |
| **RBAC** | `get`, `list` limitranges |
</details>

<details>
<summary><b>limitranges_get</b></summary>

| Field | Value |
|-------|-------|
| **name** | `limitranges_get` |
| **description** | Get a single LimitRange's full spec |
| **params** | `name` (required), `namespace` (required), `format` (optional, default: `"text"`) |
| **response** | Text: key-value describe format showing each `spec.limits[]` entry (type, min, max, default, defaultRequest, maxLimitRequestRatio) / JSON: full `core/v1.LimitRange` object |
| **notes** | Useful for understanding the resource constraints that will be applied (or defaulted) to pods created in the namespace. |
| **RBAC** | `get` limitranges |
</details>

<details>
<summary><b>storageclasses_list</b></summary>

| Field | Value |
|-------|-------|
| **name** | `storageclasses_list` |
| **description** | List StorageClasses in the cluster |
| **params** | `format` (string, optional, default: `"text"`) |
| **response** | Text: markdown table `name, provisioner, reclaim_policy, volume_binding_mode, allow_volume_expansion, age` / JSON: `[]StorageClassSummary` |
| **notes** | Cluster-scoped. `provisioner` from `spec.provisioner`; `reclaim_policy` (`Delete`/`Retain`); `volume_binding_mode` (`Immediate`/`WaitForFirstConsumer`); `allow_volume_expansion` shown as `True`/`False`. |
| **RBAC** | `get`, `list` storageclasses |
</details>

<details>
<summary><b>priorityclasses_list</b></summary>

| Field | Value |
|-------|-------|
| **name** | `priorityclasses_list` |
| **description** | List PriorityClasses in the cluster |
| **params** | `format` (string, optional, default: `"text"`) |
| **response** | Text: markdown table `name, value, global_default, description, age` / JSON: `[]PriorityClassSummary` |
| **notes** | Cluster-scoped. `value` from `spec.value` (integer priority); `global_default` shown as `True`/`False` when `spec.globalDefault=true` (the `system-cluster-critical` / `system-node-critical` system classes are included). |
| **RBAC** | `get`, `list` priorityclasses |
</details>

<details>
<summary><b>events_get</b></summary>

| Field | Value |
|-------|-------|
| **name** | `events_get` |
| **description** | Get Kubernetes events from a specific namespace (or all namespaces), sorted by time (warnings first) |
| **params** | `namespace` (string, optional — empty means all namespaces), `field_selector` (string, optional), `limit` (int, optional, default: `50`, max: `500`), `format` (optional, default: `"text"`) |
| **response** | Text: markdown table `last_seen, type (Warning/Normal), reason, object, message` / JSON: `[]EventSummary` |
| **sorting** | Warnings first, then by `lastTimestamp` descending (most recent first) |
| **notes** | This is a read-only, one-shot fetch (no streaming). `field_selector` supports e.g. `involvedObject.name=foo` or `reason=Failed`. The `limit` caps the number of events returned; `-` for `last_seen` when the event has no timestamp. |
| **RBAC** | `get`, `list` events |
</details>

> **Note on `nodes_get` / `storageclasses_get` / `priorityclasses_get`:** the user requested only **list** for nodes, storage classes, and priority classes and **get** for events. The `get` variants for nodes/storageclasses/priorityclasses are intentionally **not** added in this stage (see §10), while `events_get` is provided to fetch events scoped to a namespace or all namespaces. Any missing `get` variants can be added later if required.

---

## 6. Destructive Confirmation Flow

**Not applicable.** This stage contains **no** mutating or destructive tools. Every tool is read-only and registered unconditionally; none require `--allow-destructive` or the `input_required` confirmation flow. The `--allow-destructive` flag and confirmation machinery from Stages 1–2 remain untouched and continue to gate only the destructive tools registered in those stages.

---

## 7. Output Formatting

Same `format` param convention as Stages 1–2.

### Text

- Lists → markdown tables.
- Gets → key-value blocks.

Example (`nodes_list` text):

```
NAME       STATUS                  ROLES           AGE   VERSION   INTERNAL-IP
node-a     Ready                   control-plane   30d   v1.31.0   10.0.0.10
node-b     Ready                   worker          30d   v1.31.0   10.0.0.11
node-c     Ready,SchedulingDisabled worker         30d   v1.31.0   10.0.0.12
```

Example (`nodes_list` text, `include_allocations=true`):

```
NAME     STATUS   ROLES   AGE   VERSION   INTERNAL-IP   REQUESTS CPU   REQUESTS MEMORY   LIMITS CPU   LIMITS MEMORY
node-b   Ready    worker  30d   v1.31.0   10.0.0.11      1250m/8     4Gi/32Gi            2000m/8      8Gi/32Gi
```

Example (`resourcequotas_list` text):

```
NAMESPACE   NAME          REQUESTS CPU   REQUESTS MEMORY   LIMITS CPU   LIMITS MEMORY   AGE
default     compute-quota  1/4            2Gi/8Gi           2/8           4Gi/16Gi        30d
```

Example (`storageclasses_list` text):

```
NAME            PROVISIONER              RECLAIM POLICY   VOLUME BINDING MODE   ALLOW EXPANSION   AGE
standard        kubernetes.io/aws-ebs    Delete           WaitForFirstConsumer  True              30d
fast            kubernetes.io/aws-ebs    Retain           Immediate             False             30d
```

### JSON

```json
{
  "nodes": [
    {
      "name": "node-a",
      "status": "Ready",
      "roles": ["control-plane"],
      "age": "30d",
      "version": "v1.31.0",
      "internal_ip": "10.0.0.10",
      "requests_cpu": "1250m/8",
      "requests_memory": "4Gi/32Gi",
      "limits_cpu": "2000m/8",
      "limits_memory": "8Gi/32Gi"
    }
  ]
}
```

```json
{
  "resourcequotas": [
    {
      "namespace": "default",
      "name": "compute-quota",
      "requests_cpu": "1/4",
      "requests_memory": "2Gi/8Gi",
      "limits_cpu": "2/8",
      "limits_memory": "4Gi/16Gi",
      "age": "30d"
    }
  ]
}
```

```json
{
  "storageclasses": [
    {
      "name": "standard",
      "provisioner": "kubernetes.io/aws-ebs",
      "reclaim_policy": "Delete",
      "volume_binding_mode": "WaitForFirstConsumer",
      "allow_volume_expansion": true,
      "age": "30d"
    }
  ]
}
```

---

## 8. Backend: K8s Typed Client (cluster observability additions)

The existing `k8s.Client` (typed clientset + resolved identity) is reused unchanged. Stage 3 only adds calls against `CoreV1()`, `StorageV1()`, and `SchedulingV1()`.

```go
// internal/k8s/client.go — no structural change; new call sites:

// nodes_list            → client.CoreV1().Nodes().List(ctx, opts)
//                         (+ CoreV1().Pods(metav1.NamespaceAll).List grouped by spec.nodeName
//                          to compute request/limit sums when include_allocations=true)
// nodes_get             → client.CoreV1().Nodes().Get(ctx, name, opts)
//                         (+ CoreV1().Pods(metav1.NamespaceAll).List filtered by spec.nodeName
//                          for allocated-resource totals, and for the pod summary when include_pods=true)
// namespaces_list       → client.CoreV1().Namespaces().List(ctx, opts)
// namespaces_get        → client.CoreV1().Namespaces().Get(ctx, name, opts)
// resourcequotas_list   → client.CoreV1().ResourceQuotas(ns).List(ctx, opts)
// resourcequotas_get    → client.CoreV1().ResourceQuotas(ns).Get(ctx, name, opts)
// limitranges_list      → client.CoreV1().LimitRanges(ns).List(ctx, opts)
// limitranges_get       → client.CoreV1().LimitRanges(ns).Get(ctx, name, opts)
// storageclasses_list   → client.StorageV1().StorageClasses().List(ctx, opts)
// priorityclasses_list  → client.SchedulingV1().PriorityClasses().List(ctx, opts)
// events_get            → client.CoreV1().Events(ns).List(ctx, opts)  (ns = "" for all)
//                         then sort warnings-first, by lastTimestamp desc, cap at `limit`
```

**Namespace scoping:** for the namespaced tools (`resourcequotas_*`, `limitranges_*`), the `--namespace` global flag (or the per-call `namespace` param) selects the namespace; empty means `List` across all namespaces. Cluster-scoped tools (`nodes_list`, `nodes_get`, `namespaces_*`, `storageclasses_list`, `priorityclasses_list`) always use the cluster-scoped client and ignore `namespace`.

---

## 9. RBAC Template (cluster observability additions)

The `deploy/rbac.yaml` **reader** ClusterRole gains read rules for the new resources. Because this stage is read-only, **no changes** are made to the destructive ClusterRole.

```yaml
# deploy/rbac.yaml  (reader ClusterRole — additions shown)
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: mimiops-mcp-reader
rules:
# Pods — Stage 1 (unchanged) — also used by nodes tools for allocated-resource sums
- apiGroups: [""]
  resources: [pods, pods/log, pods/status]
  verbs: [get, list, watch]
# Jobs + CronJobs — Stage 2 (unchanged)
- apiGroups: ["batch"]
  resources: [jobs, jobs/status, cronjobs, cronjobs/status]
  verbs: [get, list, watch]
# Nodes + Namespaces — Stage 3 (NEW)
- apiGroups: [""]
  resources: [nodes, nodes/status, namespaces, namespaces/status]
  verbs: [get, list, watch]
# ResourceQuotas + LimitRanges — Stage 3 (NEW, namespaced)
- apiGroups: [""]
  resources: [resourcequotas, resourcequotas/status, limitranges]
  verbs: [get, list, watch]
# StorageClasses — Stage 3 (NEW, storage.k8s.io/v1)
- apiGroups: ["storage.k8s.io"]
  resources: [storageclasses]
  verbs: [get, list, watch]
# PriorityClasses — Stage 3 (NEW, scheduling.k8s.io/v1)
- apiGroups: ["scheduling.k8s.io"]
  resources: [priorityclasses]
  verbs: [get, list, watch]
# Events — Stage 3 (NEW, core/v1)
- apiGroups: [""]
  resources: [events]
  verbs: [get, list, watch]
---
# Destructive ClusterRole — NO changes in this stage (all Stage-3 tools are read-only)
```

**Why `nodes/status`, `namespaces/status`, `resourcequotas/status`:** the `status` subresources are required to read the status fields used by the summaries (`Ready` condition, `status.phase`, `status.used`). `watch` is granted for consistency with the existing read rules and to support future watch-based tools.

**Extra pod rule for node allocations:** `nodes_list` (`include_allocations`) and `nodes_get` (always) compute request/limit sums from `CoreV1().Pods(metav1.NamespaceAll)`. The `pods` `list` verb (already granted in Stage 1) is therefore required and implicit; no new RBAC rule is needed beyond what is already defined.

---

## 10. Error Handling Strategy

Extends the earlier error table with cluster-observability cases.

| Scenario | Behavior |
|----------|----------|
| Namespace not found | `Namespace 'foo' not found` |
| Node not found | `Node 'foo' not found` |
| Node `include_pods=true` and node not found | `Node 'foo' not found` |
| `nodes_list`/`nodes_get` without `list` pods (RBAC) | `Permission denied: user cannot list pods` (required to compute allocated-resource sums) |
| ResourceQuota not found | `ResourceQuota 'foo' not found in namespace 'bar'` |
| LimitRange not found | `LimitRange 'foo' not found in namespace 'bar'` |
| K8s API failure (network/auth/RBAC) | `Failed to list nodes: <cause>` |
| Permission denied | `Permission denied: user cannot get storageclasses` |
| Invalid tool params | `Invalid parameter 'name' ...` |

---

## 11. Summary of Key Design Decisions (Stage 3)

| Decision | Choice | Rationale |
|----------|--------|-----------|
| **Scope** | Read-only cluster observability: `nodes_list`, `nodes_get`, `namespaces_list`, `namespaces_get`, `resourcequotas_list`, `resourcequotas_get`, `limitranges_list`, `limitranges_get`, `storageclasses_list`, `priorityclasses_list`, `events_get` | Observe the cluster and its policy objects; no mutation |
| **Mutating tools** | None | All tools read-only; `--allow-destructive` / `input_required` not applicable |
| **K8s client** | client-go **typed** `CoreV1()`, `StorageV1()`, `SchedulingV1()` (no dynamic client) | Type-safe, minimal; all three are stable built-in APIs |
| **New dependencies** | None | All types ship with existing client-go/api |
| **`get` variants** | Added for namespaces, resourcequotas, limitranges, and nodes; **not** for storageclasses, priorityclasses | Matches the requested list/get split; remaining `get` variants can be added later if needed |
| **Node allocations** | `nodes_list` accepts `include_allocations`; `nodes_get` always shows allocated resources. CPU/memory request & limit sums are **computed** from pods and reported against `allocatable` (e.g. `1250m/8`) — not read from node `status`, which only holds capacity/allocatable | Mirrors `kubectl describe node`; requires `list` pods |
| **Tool registration** | One `Register*` per file, wired in central `register.go` | Consistent with Stages 1–2 |
| **Formatting** | `format` param `text`/`json`, same conventions | Consistent with Stages 1–2 |
| **RBAC** | Read rules added to reader ClusterRole only; destructive ClusterRole unchanged | Least privilege; read-only stage |
| **CLI / config** | Unchanged | Additive stage, no new flags |

---

## 12. Open Questions

1. **`get` variants for cluster-scoped resources.** **Partially resolved:** `nodes_get` is now provided (rich describe-style output with optional `include_pods`). `storageclasses_get` and `priorityclasses_get` remain out of scope — the user requested list-only for storage classes and priority classes. If full-object detail is later needed, they can be added following the same pattern.
2. **Node live metrics (CPU/memory utilization).** **Deferred:** `nodes_list`/`nodes_get` now report capacity, allocatable, and aggregate **request/limit** allocations per node — a scheduling-view of reserved resources. They do not report **live** utilization. A future `nodes_top` (mirroring `pods_top`, requiring metrics-server) could be added if actual runtime utilization observability is desired.

---

This is the Stage-3 architecture for cluster observability. It is purely additive to Stages 1–2 — no CLI, config, or dependency changes, and no destructive tools — and can be implemented by adding eleven tool files plus the cluster-observability read rules to the reader ClusterRole.
