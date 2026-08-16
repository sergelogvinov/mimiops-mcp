# Kubernetes MCP Server — Stage 4: Workloads (Deployment / StatefulSet / DaemonSet)

## Go-based Local Daemon, Scoped to Workload Controllers

This document extends the architecture in `1-architect.md` / `2-architect-pods-only.md` (Stage 1) with a refined **workload controller** tool set for Deployments, StatefulSets, and DaemonSets. It reuses the exact same CLI, MCP registration, formatting, and confirmation conventions established in earlier stages — only the tool catalog, backend calls, and RBAC are new. The original tool specs for these tools live in `1-architect.md` (§5.1 `workloads_*`); this document refines them to match the Stage-1 implementation patterns and adds **kind auto-detection**.

**Scope for this stage:** three read-only tools plus one mutating tool that operate on the three workload controller kinds:

| Resource | Tools |
|----------|-------|
| Deployments | `workloads_list`, `workloads_get`, `workloads_describe`, `workloads_scale` |
| StatefulSets | `workloads_list`, `workloads_get`, `workloads_describe`, `workloads_scale` |
| DaemonSets | `workloads_list`, `workloads_get`, `workloads_describe` |

**Three tools are read-only** (`workloads_list`, `workloads_get`, `workloads_describe`) and are registered unconditionally. **One tool is mutating** (`workloads_scale` — Deployments and StatefulSets only) and is gated by `--allow-destructive` + the `input_required` confirmation flow, exactly like the destructive tools from earlier stages. DaemonSets cannot be scaled (they have no `spec.replicas`), so `workloads_scale` does not apply to them.

**Key change vs. Stage 1:** the `kind` param is now **optional**. When omitted, the tool probes all three kinds (deployment, statefulset, daemonset) to locate the workload by name and reports which kind it found. This lets a caller reference a workload by name alone without knowing (or caring) which controller owns it.

---

## 1. Architectural Overview

The architecture is unchanged from earlier stages. The only delta is the tool registry content and the backend calls it makes against `apps/v1`.

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
│  │  CLI (cobra) — unchanged from earlier stages         │    │
│  │    mcp | server | version                            │    │
│  │    --kubeconfig --context --namespace --impersonate  │    │
│  │    --allow-destructive --log-level  (+ --port)       │    │
│  └──────────────────────────┬───────────────────────────┘    │
│                             │                                 │
│  ┌──────────────────────────▼───────────────────────────┐    │
│  │  MCP Server (mark3labs/mcp-go)                       │    │
│  │  Tool Registry — earlier stages + Stage 4 additions: │    │
│  │    workloads_list   (refined: kind optional)         │    │
│  │    workloads_get    (refined: kind optional)         │    │
│  │    workloads_describe (refined: kind optional)       │    │
│  │    workloads_scale  (NEW: mutating, gated)           │    │
│  │    (read-only tools registered unconditionally;      │    │
│  │     workloads_scale gated by --allow-destructive)    │    │
│  └───────────────────────┬────────────────────────────┘    │
│                          │                                  │
│  ┌───────────────────────▼────────────────────────────┐    │
│  │  Tool Handlers (internal/tools)                     │    │
│  │    one file per tool (see §2)                       │    │
│  └───────────────────────┬────────────────────────────┘    │
│                          │                                  │
│  ┌───────────────────────▼────────────────────────────┐    │
│  │  Backend: k8s typed client (client-go)             │    │
│  │    AppsV1().Deployments(ns)                        │    │
│  │    AppsV1().StatefulSets(ns)                       │    │
│  │    AppsV1().DaemonSets(ns)                         │    │
│  └───────────────────────┬────────────────────────────┘    │
└──────────────────────────┼─────────────────────────────────┘
                           ▼
                  ┌─────────────────────────┐
                  │   Kubernetes API Server  │
                  │   (authenticated via     │
                  │    kubeconfig)           │
                  └─────────────────────────┘
```

No new transports, no new CLI flags, no new external dependencies. This stage is purely additive to the tool catalog; three tools are read-only and one (`workloads_scale`) is mutating.

---

## 2. Project Structure (Stage 4 additions)

The existing tree is unchanged; the three read-only workload tool files are **refined** and one mutating tool file is **added** under `internal/tools/`, one file per tool (each exposing a single `Register*` function). No changes to `cmd/`, `internal/config`, `internal/k8s`, or `internal/formatter`.

```
mimiops/
├── internal/
│   ├── tools/
│   │   ├── register.go          # central wiring: calls every Register* below
│   │   ├── workloads_list.go    # workloads_list       (refined: kind optional)
│   │   ├── workloads_get.go     # workloads_get        (refined: kind optional)
│   │   ├── workloads_describe.go# workloads_describe   (refined: kind optional)
│   │   ├── workloads_scale.go   # workloads_scale      (NEW: mutating, gated)
│   │   └── ...
│   └── ...
└── deploy/
    └── rbac.yaml                # + workload read rules + scale patch rule (see §10)
```

**File ownership rule (from Stage 1, enforced here):** each tool = one `Register*` function in its **own** file, with the `mcp.NewTool(...)` description/schema and the handler colocated. `internal/tools/register.go` is the only place that names every tool. No two tools share a file in this stage.

**Shared kind-resolution helper:** because all four tools need the same "resolve kind by name" logic, a small shared helper (e.g. `internal/tools/workloads_resolve.go`) is introduced. It is **not** a tool file — it exposes a `resolveWorkloadKind(ctx, client, namespace, name, kind)` function used by the four handlers. This keeps the auto-detection logic in one place and avoids duplication.

---

## 3. Core Dependencies

**No new dependencies.** `apps/v1` (Deployments, StatefulSets, DaemonSets) is part of the already-present `k8s.io/client-go` / `k8s.io/api` typed clientset.

```go
module github.com/sergelogvinov/mimiops-mcp

go 1.26

    github.com/spf13/pflag v1.0.10        // Flags
    github.com/spf13/cobra v1.10.2        // CLI command dispatch
    k8s.io/client-go                      // typed clientset (AppsV1)
    k8s.io/api                            // apps/v1 types
```

Helm, metrics, and the dynamic client remain out of scope for this stage.

---

## 4. CLI Interface

**Unchanged from earlier stages.** No new flags or subcommands. The `--namespace` global flag scopes the workload tools; empty means all namespaces. `--allow-destructive` is **not** relevant — this stage has no mutating tools.

---

## 5. Kind Resolution (the core new behavior)

The defining feature of this stage is that `kind` is **optional** on all three tools. When omitted, the handler must locate the workload by name across the three controller kinds.

### 5.1 Resolution order

The probe order is fixed and deterministic: **deployment → statefulset → daemonset**.

```
resolveWorkloadKind(ctx, client, namespace, name, kind):
  if kind is set:
      validate kind ∈ {deployment, statefulset, daemonset}
      return (kind, nil)                       # caller uses the typed client directly

  # kind omitted → probe each kind in order
  matches = []
  for k in [deployment, statefulset, daemonset]:
      if Get(name) succeeds:
          matches.append(k)

  if len(matches) == 0:
      return ("", NotFoundError)              # not found in any kind
  if len(matches) > 1:
      return ("", AmbiguousError(matches))    # multiple kinds share the name
  return (matches[0], nil)
```

### 5.2 Design decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| **Probe order** | deployment → statefulset → daemonset | Deterministic and matches the most common controller |
| **Probe method** | Typed `Get` per kind (three sequential `Get` calls) | Type-safe; no dynamic client needed. The cost of up to three `Get`s is negligible for a single-name lookup |
| **Ambiguity** | Return an error listing every matching kind and ask the caller to pick one | A name collision across kinds is rare but possible; silently choosing one could act on the wrong workload. The caller must disambiguate by passing `kind` explicitly |
| **`kind` validation** | Invalid kind string → error | Prevents typos from silently probing the wrong resource |
| **Not-found error** | Reports the name and that no matching kind was found | Clear signal to the caller that the workload does not exist under any of the three kinds |

### 5.3 Ambiguity error (multiple kinds share a name)

If more than one kind has a workload with the given `name` in the given `namespace`, the resolver returns an **ambiguity error** instead of guessing. The error lists every matching kind so the caller can re-invoke the tool with an explicit `kind`.

```
Ambiguous workload 'postgres' in namespace 'default':
  - deployment/postgres
  - statefulset/postgres
Please retry with an explicit 'kind' parameter.
```

The error is returned as a normal tool error (not a confirmation prompt). The caller should retry the same call with `kind` set to one of the listed values. This is the only case where a caller must supply `kind` — it is otherwise optional.

### 5.4 Returned `kind`

Every tool that resolves a kind (or is given one) includes the resolved `kind` in its output — both in the text header and in the JSON object — so the caller always knows which controller was addressed.

---

## 6. Tool Specifications (Stage-4 Catalog)

All tools follow the established conventions: `format` param (`"text"` default / `"json"`), typed client-go, and the central `register.go` wiring. **Three tools are read-only** (`workloads_list`, `workloads_get`, `workloads_describe`); **one is mutating** (`workloads_scale`).

### 6.1 Read Tools

<details>
<summary><b>workloads_list</b></summary>

| Field | Value |
|-------|-------|
| **name** | `workloads_list` |
| **description** | List Deployments, StatefulSets, or DaemonSets in a namespace (or all) |
| **params** | `namespace` (string, optional), `kind` (string, optional, enum: `"deployment"`, `"statefulset"`, `"daemonset"`), `label_selector` (string, optional), `format` (string, optional, default: `"text"`) |
| **response** | Text: markdown table `kind, name, namespace, replicas (ready/desired), age` / JSON: `[]WorkloadSummary` (each entry includes `kind`). |
| **kind semantics** | `kind` is **optional**. If set, only that kind is listed. If omitted, **all three kinds** are listed in a single combined table (deployments, then statefulsets, then daemonsets), each row tagged with its `kind`. |
| **replicas** | Deployment/StatefulSet: `ready/desired` from `status.readyReplicas` / `status.replicas`. DaemonSet: `ready/desired` from `status.numberReady` / `status.desiredNumberScheduled`. |
| **age** | From `creationTimestamp` |
| **RBAC** | `get`, `list` deployments, statefulsets, daemonsets |
</details>

<details>
<summary><b>workloads_get</b></summary>

| Field | Value |
|-------|-------|
| **name** | `workloads_get` |
| **description** | Get a single workload's full spec and status (Deployment, StatefulSet, or DaemonSet) |
| **params** | `name` (required), `namespace` (required), `kind` (string, optional, enum: `"deployment"`, `"statefulset"`, `"daemonset"`), `format` (optional, default: `"text"`) |
| **response** | Text: key-value describe format / JSON: full `apps/v1` object (`Deployment`, `StatefulSet`, or `DaemonSet`) |
| **kind semantics** | `kind` is **optional**. If omitted, the tool resolves the kind via `resolveWorkloadKind` (probes deployment → statefulset → daemonset) and returns the matching object. The resolved `kind` is included in the output. |
| **notes** | Useful for inspecting a workload's full spec (containers, volumes, strategy, selector) before acting on it. |
| **RBAC** | `get` deployments, statefulsets, daemonsets |
</details>

<details>
<summary><b>workloads_describe</b></summary>

| Field | Value |
|-------|-------|
| **name** | `workloads_describe` |
| **description** | Rich human-readable summary of a workload: replicas, conditions, selector, strategy, update history |
| **params** | `name` (required), `namespace` (required), `kind` (string, optional, enum: `"deployment"`, `"statefulset"`, `"daemonset"`), `format` (optional, default: `"text"`) |
| **response** | Rich formatted description (Text) / structured summary (JSON) |
| **kind semantics** | `kind` is **optional**. If omitted, the tool resolves the kind via `resolveWorkloadKind` and describes the matching object. The resolved `kind` is included in the output. |
| **fields (text)** | kind, name, namespace, replicas (ready/desired/available), selector, strategy (type + rolling update params), conditions, update strategy / revision history limit, pod template summary (containers, images, restart policy), age |
| **RBAC** | `get` deployments, statefulsets, daemonsets |
</details>

### 6.2 Mutating Tools

<details>
<summary><b>workloads_scale</b></summary>

| Field | Value |
|-------|-------|
| **name** | `workloads_scale` |
| **description** | Scale a Deployment or StatefulSet to a target replica count |
| **params** | `name` (required), `namespace` (required), `replicas` (int, required, min: `0`), `kind` (string, optional, enum: `"deployment"`, `"statefulset"`), `format` (optional, default: `"text"`) |
| **destructive** | Yes — gated by `--allow-destructive` + `input_required` confirmation. Rationale: changing the replica count changes capacity and can affect production traffic. |
| **kind semantics** | `kind` is **optional** and limited to `deployment` or `statefulset`. If omitted, the tool resolves the kind via `resolveWorkloadKind` and scales the matching object. DaemonSets cannot be scaled — passing `kind=daemonset` or resolving to a DaemonSet returns an error (see §11). |
| **action** | Patches the workload's `spec.replicas` to `replicas` via the `scale` subresource (`UpdateScale`). Applies to Deployments and StatefulSets only. |
| **response** | Text: confirmation of the new replica count (e.g. `Scaled deployment/nginx to 5 replicas`) / JSON: the resulting `Scale` object (`spec.replicas`, `status.replicas`, `status.selector`) |
| **notes** | `replicas=0` is allowed (scale to zero). Uses the `scale` subresource so it works uniformly across Deployment and StatefulSet and records the change in the object's scale status. |
| **RBAC** | `get`, `update`/`patch` on the `scale` subresource of the workload type (`deployments/scale`, `statefulsets/scale`) |
</details>

---

## 7. Destructive Confirmation Flow

**Applies to `workloads_scale` only.** The three read-only tools (`workloads_list`, `workloads_get`, `workloads_describe`) are registered unconditionally and need no confirmation. `workloads_scale` is registered **only** when `--allow-destructive` is set and uses the same two-phase `input_required` flow as the destructive tools from earlier stages.

```
Client calls workloads_scale(name="nginx", namespace="default", replicas=5)
                     │
                     ▼
Server receives request
  → Checks allowDestructive flag (fail if false)
  → Validates params; resolves kind (if omitted)
  → Returns CallToolResult with isError=false, but:
      - content: "This will scale deployment/nginx in namespace 'default' to 5 replicas.
                  To confirm, call this tool again with confirm=true"
      - inputRequired set to true
                     │
                     ▼
Client presents confirmation to user
  User confirms
                     │
                     ▼
Client calls workloads_scale again with same params + confirm=true
                     │
                     ▼
Server receives request
  → Sees confirm=true
  → Executes the scale (UpdateScale on the scale subresource)
  → Returns result
```

**Implementation approach:** two-phase call, identical to `pods_delete` / `rollout_restart` from Stage 1.

- **Phase 1** (no `confirm` param or `confirm=false`): Return a description of what will happen and ask for confirmation.
- **Phase 2** (`confirm=true`): Execute the scale and return the result. If `confirm=true` is missing or `false`, return the prompt again.

---

## 8. Output Formatting

Same `format` param convention as earlier stages.

### Text

- Lists → markdown tables.
- Gets → key-value blocks.

Example (`workloads_list` text, `kind` omitted — all three kinds):

```
KIND         NAMESPACE   NAME            READY   DESIRED   AGE
deployment   default     nginx           3/3     3         5d
deployment   default     api             2/3     3         2h
statefulset  default     postgres        1/1     1         30d
daemonset    kube-system fluentd         4/4     4         30d
```

Example (`workloads_list` text, `kind=deployment`):

```
KIND         NAMESPACE   NAME      READY   DESIRED   AGE
deployment   default     nginx     3/3     3         5d
deployment   default     api       2/3     3         2h
```

Example (`workloads_get` text, `kind` omitted — resolved to statefulset):

```
KIND:        statefulset
NAME:        postgres
NAMESPACE:   default
SERVICE:     postgres
REPLICAS:    1/1
...
```

Example (`workloads_scale` text, confirmed):

```
Scaled deployment/nginx in namespace 'default' to 5 replicas.
```

### JSON

```json
{
  "workloads": [
    {
      "kind": "deployment",
      "namespace": "default",
      "name": "nginx",
      "ready": "3/3",
      "desired": 3,
      "age": "5d"
    },
    {
      "kind": "statefulset",
      "namespace": "default",
      "name": "postgres",
      "ready": "1/1",
      "desired": 1,
      "age": "30d"
    }
  ]
}
```

```json
{
  "workload": {
    "kind": "statefulset",
    "namespace": "default",
    "name": "postgres",
    "replicas": {
      "ready": 1,
      "desired": 1
    },
    "selector": "app=postgres",
    "service": "postgres",
    "update_strategy": "RollingUpdate",
    "age": "30d"
  }
}
```

```json
{
  "scale": {
    "kind": "deployment",
    "namespace": "default",
    "name": "nginx",
    "replicas": 5
  }
}
```

---

## 9. Backend: K8s Typed Client (workload additions)

The existing `k8s.Client` (typed clientset + resolved identity) is reused unchanged. Stage 4 only adds calls against `AppsV1()`.

```go
// internal/k8s/client.go — no structural change; new call sites:

// workloads_list (kind=deployment)   → client.AppsV1().Deployments(ns).List(ctx, opts)
// workloads_list (kind=statefulset)  → client.AppsV1().StatefulSets(ns).List(ctx, opts)
// workloads_list (kind=daemonset)    → client.AppsV1().DaemonSets(ns).List(ctx, opts)
// workloads_list (kind omitted)      → List all three, merge into one table tagged by kind
// workloads_get / workloads_describe → resolve kind, then:
//   deployment   → client.AppsV1().Deployments(ns).Get(ctx, name, opts)
//   statefulset  → client.AppsV1().StatefulSets(ns).Get(ctx, name, opts)
//   daemonset    → client.AppsV1().DaemonSets(ns).Get(ctx, name, opts)
// workloads_scale → resolve kind, then (Deployment / StatefulSet only):
//   deployment   → client.AppsV1().Deployments(ns).UpdateScale(ctx, name, scale, opts)
//   statefulset  → client.AppsV1().StatefulSets(ns).UpdateScale(ctx, name, scale, opts)
//   daemonset    → error: DaemonSets have no spec.replicas and cannot be scaled
```

**Kind resolution helper** (`internal/tools/workloads_resolve.go`):

```go
// resolveWorkloadKind returns the kind of the workload named `name` in `namespace`.
// If `kind` is provided it is validated and returned directly.
// Otherwise it probes deployment → statefulset → daemonset via typed Get.
func resolveWorkloadKind(ctx context.Context, client k8s.Interface,
    namespace, name, kind string) (string, error)
```

**Namespace scoping:** the `--namespace` global flag (or the per-call `namespace` param) selects the namespace; empty means `List` across all namespaces for `workloads_list`. `workloads_get` / `workloads_describe` / `workloads_scale` require an explicit `namespace`.

---

## 10. RBAC Template (workload additions)

The `deploy/rbac.yaml` **reader** ClusterRole already grants `get, list, watch` on `deployments`, `statefulsets`, and `daemonsets` (from Stage 1), which covers the three read-only tools. `workloads_scale` needs write access to the `scale` subresource, which belongs in the **destructive** ClusterRole. The existing rules are reproduced here for clarity.

```yaml
# deploy/rbac.yaml  (reader ClusterRole — existing workload rule, unchanged)
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: mimiops-mcp-reader
rules:
# Workloads — Stage 1 (unchanged) — used by the read-only Stage-4 tools
- apiGroups: ["apps"]
  resources: [deployments, deployments/status, deployments/scale,
              statefulsets, statefulsets/status, statefulsets/scale,
              daemonsets, daemonsets/status]
  verbs: [get, list, watch]
---
# Destructive ClusterRole — Stage 4 addition: scale subresource for workloads_scale
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: mimiops-mcp-destructive
rules:
- apiGroups: ["apps"]
  resources: [deployments/scale, statefulsets/scale]
  verbs: [get, update, patch]
```

**Why the scale rule is destructive:** `workloads_scale` changes the replica count, which changes capacity and can affect production traffic. It therefore lives in the destructive ClusterRole (not bound by default) and is gated by `--allow-destructive`, consistent with the existing `patch` rule on `deployments, statefulsets, daemonsets` from Stage 1. DaemonSets are excluded because they have no `spec.replicas` and cannot be scaled.

---

## 11. Error Handling Strategy

Extends the earlier error table with workload cases.

| Scenario | Behavior |
|----------|----------|
| Workload not found, `kind` omitted | `Workload 'foo' not found in namespace 'bar' (checked deployment, statefulset, daemonset)` |
| Multiple kinds share the name, `kind` omitted | `Ambiguous workload 'foo' in namespace 'bar': deployment/foo, statefulset/foo — retry with an explicit 'kind'` |
| Workload not found, `kind` set | `Deployment 'foo' not found in namespace 'bar'` (kind-specific message) |
| Invalid `kind` value | `Invalid parameter 'kind': must be one of deployment, statefulset, daemonset` |
| `workloads_get`/`workloads_describe` without `namespace` | `Invalid parameter 'namespace': required` |
| `workloads_scale` scaling a DaemonSet | `Cannot scale daemonset 'foo': DaemonSets have no spec.replicas` |
| `workloads_scale` without `replicas` | `Invalid parameter 'replicas': required` |
| `workloads_scale` called without `--allow-destructive` | `Destructive action not allowed (start with --allow-destructive)` |
| K8s API failure (network/auth/RBAC) | `Failed to list deployments: <cause>` |
| Permission denied | `Permission denied: user cannot get statefulsets` |
| Invalid tool params | `Invalid parameter 'name' ...` |

---

## 12. Summary of Key Design Decisions (Stage 4)

| Decision | Choice | Rationale |
|----------|--------|-----------|
| **Scope** | Workload tools across Deployments, StatefulSets, DaemonSets: `workloads_list`, `workloads_get`, `workloads_describe` (read-only) + `workloads_scale` (mutating) | Observe and scale workload controllers |
| **Mutating tools** | One: `workloads_scale` (Deployment / StatefulSet only), gated by `--allow-destructive` + `input_required`; DaemonSets unsupported (no `spec.replicas`) | Scaling changes capacity and can affect production traffic; requires explicit opt-in and confirmation |
| **`kind` optional** | When omitted, auto-detect by probing deployment → statefulset → daemonset | Caller can reference a workload by name alone; resolved `kind` is always reported |
| **Ambiguity handling** | If multiple kinds share the name, return an error listing all matches and ask the caller to retry with an explicit `kind` | Never silently guess which workload to act on; the caller must disambiguate |
| **Probe method** | Typed `Get` per kind (up to three sequential calls) | Type-safe; no dynamic client; negligible cost for a single-name lookup |
| **Probe order** | deployment → statefulset → daemonset | Deterministic; matches most common controller |
| **`workloads_list` without kind** | Lists all three kinds merged into one table, each row tagged with `kind` | One call to see the whole namespace's workload surface |
| **Shared helper** | `resolveWorkloadKind` in `workloads_resolve.go` | Single source of truth for auto-detection across the four tools |
| **K8s client** | client-go **typed** `AppsV1()` (no dynamic client); scale via the `scale` subresource (`UpdateScale`) | Type-safe, minimal; all three are stable built-in APIs |
| **New dependencies** | None | All types ship with existing client-go/api |
| **RBAC** | Reader ClusterRole unchanged (`apps` read rules already granted in Stage 1); destructive ClusterRole gains `deployments/scale`, `statefulsets/scale` with `get, update, patch` | Least privilege; scaling is mutating and lives in the destructive role |
| **Tool registration** | One `Register*` per file, wired in central `register.go` | Consistent with earlier stages |
| **Formatting** | `format` param `text`/`json`, same conventions | Consistent with earlier stages |
| **CLI / config** | Unchanged | Additive stage, no new flags |

---

## 13. Open Questions

1. **`workloads_get`/`workloads_describe` with `kind` omitted and a missing workload.** The current design returns a single "not found (checked all kinds)" error. If richer diagnostics are desired, the error could list which kinds were probed and whether any existed under a different name. Deferred as a nicety.
2. **Scale for DaemonSets.** DaemonSets have no `spec.replicas` and cannot be scaled; `workloads_scale` returns an error for them. If daemon-set rollout control is ever needed, a separate `daemonsets_restart` / rollout-status tool (gated like this stage's other mutating tools) could be added.
3. **Further mutating workload tools.** `rollout_restart` already exists as a destructive tool from Stage 1. Future stages could add `workloads_rollout_status` or `workloads_restart` (gated by `--allow-destructive`) following the same kind-resolution pattern. Out of scope here.

---

This is the Stage-4 architecture for workload controllers. Three tools are read-only (`workloads_list`, `workloads_get`, `workloads_describe`) and one is mutating (`workloads_scale`, gated by `--allow-destructive`). `workloads_scale` applies to **Deployments and StatefulSets only** — DaemonSets have no `spec.replicas` and cannot be scaled. It is purely additive to earlier stages — no CLI, config, or dependency changes — and can be implemented by refining the three read tool files, adding the `workloads_scale` tool file, adding the shared `resolveWorkloadKind` helper, and adding the `scale` subresource rule to the destructive ClusterRole.