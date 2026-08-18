# Kubernetes MCP Server — Stage 7: Helm Tools

## Go-based Local Daemon, Scoped to Helm Releases

This document extends the architecture in `1-architect.md` / `2-architect-pods-only.md` (Stage 1) and follows the same conventions as the workload stage (`5-architecture-workloads.md`). It adds a **Helm** tool set for inspecting and rolling back Helm releases, using the `helm.sh/helm/v3` SDK as a Go library (not subprocess calls).

**Scope for this stage:** two read-only tools plus one destructive tool.

| Tool            | Type        | Description                                                                      |
| --------------- | ----------- | -------------------------------------------------------------------------------- |
| `helm_list`     | read-only   | List Helm releases in a namespace                                                |
| `helm_status`   | read-only   | Classic `helm status` output for a release, plus the last 3 revisions of history |
| `helm_rollback` | destructive | Roll a release back to the **previous** revision (one back)                      |

**Key decisions (per design review):**

- `helm_rollback` **always rolls back exactly one revision** — it does **not** accept an arbitrary `revision` parameter.
- `helm_status` returns the **classic `helm status` output** (status, revision, namespace, last deployed time, and a **description message only** — it does **not** show rendered values) **and** embeds the **last 3 revisions** of release history so the caller has rollback context in one call.
- `helm_history` is **not** implemented — it is intentionally skipped.
- `helm_list` requires a `namespace` — if it is missing, the tool returns an error.
- `helm_list` supports a `label_selector` filter and a `status_filter` (enum: `failed` or `deployed`).
- `helm_rollback` does **not** use the `input_required` confirmation flow — it executes immediately (still gated by `--allow-destructive`) and is **non-blocking** (returns after submission, does not wait for the release to reach a ready state).

---

## 1. Architectural Overview

The architecture is unchanged from earlier stages. The only delta is the tool registry content, the backend calls, and the new `internal/helm` package that wraps the Helm SDK.

```
┌──────────────────────────────────────────────────────────────┐
│                      MCP Client                              │
│   (Claude Desktop, Cursor, VS Code, Goose, custom script)    │
└───────────────────────────┬──────────────────────────────────┘
                            │  JSON-RPC 2.0 (stdio or SSE)
                            ▼
┌──────────────────────────────────────────────────────────────┐
│                      mimiops-mcp daemon                      │
│   ┌───────────────┐   ┌───────────────┐   ┌───────────────┐  │
│   │  helm_list    │   │  helm_status  │   │ helm_rollback │  │
│   │  (read-only)  │   │  (read-only)  │   │  (destructive)│  │
│   └───────┬───────┘   └───────┬───────┘   └───────┬───────┘  │
│           └───────────────────┼───────────────────┘          │
│                               ▼                              │
│                 ┌───────────────────────────┐                │
│                 │  internal/helm            │                │
│                 │  (helm.sh/helm/v3 SDK)    │                │
│                 └───────────────────────────┘                │
└───────────────────────────┬──────────────────────────────────┘
                            │  HTTPS
                            ▼
                 ┌──────────────────────┐
                 │  Kubernetes API      │
                 │  (release metadata   │
                 │   stored in Secrets) │
                 └──────────────────────┘
```

No new transports, no new CLI flags. This stage is purely additive to the tool catalog and introduces one new internal package (`internal/helm`) plus the `helm.sh/helm/v3` dependency.

---

## 2. Project Structure (Stage 7 additions)

```
mimiops/
├── internal/
│   ├── helm/
│   │   ├── client.go          # HelmClient wrapper around the Helm SDK
│   │   └── types.go           # ReleaseSummary, ReleaseStatus, HistoryEntry
│   ├── tools/
│   │   ├── register.go        # + RegisterHelmList / RegisterHelmStatus / RegisterHelmRollback
│   │   ├── helm_list.go       # helm_list       (read-only)
│   │   ├── helm_status.go     # helm_status     (read-only)
│   │   ├── helm_rollback.go   # helm_rollback   (destructive, gated)
│   │   └── ...
│   └── ...
└── deploy/
    └── rbac.yaml              # + Helm secret read rules + rollback write rules (see §9)
```

**File ownership rule (from Stage 1, enforced here):** each tool = one `Register*` function in its **own** file, with the `mcp.NewTool(...)` description/schema and the handler colocated. `internal/tools/register.go` is the only place that names every tool.

**New package:** `internal/helm` wraps the Helm SDK. It is **not** a tool package — it exposes a `HelmClient` with methods the three tool handlers call. This keeps all Helm SDK specifics out of the tool files.

---

## 3. Core Dependencies

Adds the Helm SDK as a direct dependency. Everything else is unchanged.

```go
module github.com/sergelogvinov/mimiops-mcp

go 1.26.5

    github.com/mark3labs/mcp-go v0.58.0   // MCP server (existing)
    k8s.io/client-go v0.36.3              // typed clientset (existing)
    helm.sh/helm/v4                       // NEW: Helm SDK (Go library, no binary)
```

The Helm SDK needs a `RESTClientGetter` to reach the cluster. We reuse the same kubeconfig resolution already performed by `internal/k8s` so Helm talks to the same context/cluster/namespace as the rest of the server.

---

## 4. CLI Interface

**Unchanged from earlier stages.** No new flags or subcommands. The `--namespace` global flag scopes the Helm tools; empty means all namespaces for `helm_list`. `--allow-destructive` gates `helm_rollback`.

---

## 5. `internal/helm` Package

### 5.1 `HelmClient`

```go
// internal/helm/client.go

type HelmClient struct {
    client *helm.Client
    // Namespace is passed per call, not stored at client level.
}

func NewHelmClient(restClientGetter genericclioptions.RESTClientGetter) (*HelmClient, error) {
    client, err := helm.NewClient(&helm.Options{
        RESTClientGetter: restClientGetter,
    })
    if err != nil {
        return nil, err
    }
    return &HelmClient{client: client}, nil
}
```

### 5.2 Wiring the REST client getter

The `internal/k8s.Client` already resolves the kubeconfig, context, and namespace. We add a small helper to build a `genericclioptions.RESTClientGetter` from the same `clientcmd` loading rules so Helm uses the identical cluster identity (including impersonation). This keeps a single source of truth for "which cluster am I talking to."

```go
// internal/k8s/client.go — new helper
func (c *Client) RESTClientGetter() genericclioptions.RESTClientGetter {
    // Reuses the same loading rules / overrides as NewClient.
}
```

### 5.3 Methods

```go
// helm_list   → ListReleases(namespace)
// helm_status → GetRelease(name, namespace) + GetReleaseHistory(name, namespace, max=3)
// helm_rollback → Rollback(name, namespace, previousRevision)
```

---

## 6. Tool Specifications (Stage-7 Catalog)

All tools follow the established conventions: `mcp.WithOutputSchema[T]()`, `mcp.NewToolResultStructured(result, fallbackText)`, and the central `register.go` wiring. **Two tools are read-only** (`helm_list`, `helm_status`); **one is destructive** (`helm_rollback`).

### 6.1 Read Tools

<details>
<summary><b>helm_list</b></summary>

| Field                   | Value                                                                                                                                                                                   |
| ----------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| **name**                | `helm_list`                                                                                                                                                                             |
| **description**         | List Helm releases in a namespace (or all namespaces)                                                                                                                                   |
| **params**              | `namespace` (string, **required**), `label_selector` (string, optional), `status_filter` (string, optional, enum: `failed`, `deployed`), `format` (string, optional, default: `"text"`) |
| **response**            | Text: markdown table `name, namespace, revision, updated, status, chart, app_version` / JSON: `[]ReleaseSummary`                                                                        |
| **namespace semantics** | `namespace` is **required**. If it is missing or empty, the tool returns an error. It does **not** fall back to all namespaces.                                                         |
| **label_selector**      | Optional label selector filter (e.g. `app=nginx,env=prod`). Only releases whose labels match are returned.                                                                              |
| **status_filter**       | Optional status filter, enum: `failed` or `deployed`. When set, only releases with that status are returned.                                                                            |
| **RBAC**                | `get`, `list` secrets (Helm stores release metadata in Secrets labeled `owner: helm`)                                                                                                   |

</details>

<details>
<summary><b>helm_status</b></summary>

| Field           | Value                                                                                                                                                                             |
| --------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| **name**        | `helm_status`                                                                                                                                                                     |
| **description** | Classic `helm status` output for a release, plus the last 3 revisions of history                                                                                                  |
| **params**      | `name` (required), `namespace` (required), `format` (string, optional, default: `"text"`)                                                                                         |
| **response**    | Text: classic `helm status` block (name, namespace, revision, status, last deployed, description) followed by a **last 3 revisions** history table / JSON: `ReleaseStatus` object |
| **history**     | Always includes the **last 3 revisions** (revision, updated, status, chart, app_version, description) so the caller has rollback context in the same call.                        |
| **notes**       | The rendered `notes` and `description message` returned.                                                                                                                          |
| **RBAC**        | `get`, `list` secrets                                                                                                                                                             |

</details>

### 6.2 Destructive Tools

<details>
<summary><b>helm_rollback</b></summary>

| Field                  | Value                                                                                                                                                                                                                                                                       |
| ---------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| **name**               | `helm_rollback`                                                                                                                                                                                                                                                             |
| **description**        | Roll a Helm release back to the **previous** revision (one back)                                                                                                                                                                                                            |
| **params**             | `name` (required), `namespace` (required)                                                                                                                                                                                                                                   |
| **destructive**        | Yes — gated by `--allow-destructive`. **No `input_required` confirmation flow** — the rollback executes immediately. Rationale: rolling back changes the live release state, so it still requires the `--allow-destructive` opt-in, but no per-call confirmation is needed. |
| **revision semantics** | **Always rolls back exactly one revision** — it targets `current_revision - 1`. There is **no** arbitrary `revision` parameter. If the release is already at revision 1 (nothing to roll back to), return an error.                                                         |
| **action**             | Calls the Helm SDK `Rollback` with the previous revision. **Non-blocking** — returns immediately after the rollback is submitted and does **not** wait for the release to reach a ready state.                                                                              |
| **response**           | Text: `Rolled back release 'foo' in namespace 'bar' to revision N` / JSON: the resulting `RollbackResult` (previous revision, new revision, status).                                                                                                                        |
| **RBAC**               | `create`, `update` secrets (Helm writes the new rollback revision to a Secret)                                                                                                                                                                                              |

</details>

---

## 7. Destructive Gating (no confirmation flow)

**Applies to `helm_rollback` only.** The two read-only tools (`helm_list`, `helm_status`) are registered unconditionally. `helm_rollback` is registered **only** when `--allow-destructive` is set.

Unlike the destructive tools from earlier stages (`pods_delete`, `workloads_scale`), `helm_rollback` does **not** use the two-phase `input_required` confirmation flow. It executes immediately once called. The `--allow-destructive` flag is the only gate.

```
Client calls helm_rollback(name="nginx", namespace="default")
                     │
                     ▼
Server receives request
  → Checks allowDestructive flag (fail if false)
  → Resolves the release; computes previous revision = current - 1
  → Executes the rollback to the previous revision
  → Returns result
```

**Implementation approach:** single-phase call. There is **no** `confirm` parameter and **no** `input_required` prompt. If `--allow-destructive` is not set, the tool returns an error.

---

## 8. Output Formatting

Same `format` param convention as earlier stages.

### Text

Example (`helm_list` text):

```
NAME      NAMESPACE   REVISION   UPDATED                  STATUS      CHART             APP VERSION
nginx     default     5          2026-08-18 10:00:00      deployed    nginx-4.1.0       1.25.0
postgres  default     2          2026-08-17 09:00:00      deployed    postgresql-15.5.0 15.5.0
```

Example (`helm_status` text — classic `helm status` block + last 3 revisions):

```
NAME: nginx
LAST DEPLOYED: 2026-08-18 10:00:00
NAMESPACE: default
STATUS: deployed
REVISION: 5
DESCRIPTION: Upgrade complete

HISTORY (last 3 revisions):
REVISION   UPDATED                  STATUS      CHART        APP VERSION   DESCRIPTION
5          2026-08-18 10:00:00      deployed    nginx-4.1.0  1.25.0        Upgrade complete
4          2026-08-18 09:00:00      superseded  nginx-4.1.0  1.25.0        Upgrade complete
3          2026-08-17 08:00:00      superseded  nginx-4.0.0  1.24.0        Install complete
```

Example (`helm_rollback` text):

```
Rolled back release 'nginx' in namespace 'default' to revision 4.
```

### JSON

```json
{
    "releases": [
        {
            "name": "nginx",
            "namespace": "default",
            "revision": 5,
            "updated": "2026-08-18T10:00:00Z",
            "status": "deployed",
            "chart": "nginx-4.1.0",
            "app_version": "1.25.0"
        }
    ]
}
```

```json
{
    "release": {
        "name": "nginx",
        "namespace": "default",
        "revision": 5,
        "status": "deployed",
        "last_deployed": "2026-08-18T10:00:00Z",
        "description": "Upgrade complete",
        "history": [
            {
                "revision": 5,
                "updated": "2026-08-18T10:00:00Z",
                "status": "deployed",
                "chart": "nginx-4.1.0",
                "app_version": "1.25.0",
                "description": "Upgrade complete"
            },
            {
                "revision": 4,
                "updated": "2026-08-18T09:00:00Z",
                "status": "superseded",
                "chart": "nginx-4.1.0",
                "app_version": "1.25.0",
                "description": "Upgrade complete"
            },
            {
                "revision": 3,
                "updated": "2026-08-17T08:00:00Z",
                "status": "superseded",
                "chart": "nginx-4.0.0",
                "app_version": "1.24.0",
                "description": "Install complete"
            }
        ]
    }
}
```

```json
{
    "rollback": {
        "name": "nginx",
        "namespace": "default",
        "previous_revision": 4,
        "new_revision": 6,
        "status": "deployed"
    }
}
```

---

## 9. RBAC Template (Helm additions)

Helm stores release metadata in Kubernetes **Secrets** labeled `owner: helm`. Reading releases requires `get`/`list` on those secrets; rolling back writes a new revision, which requires `create`/`update`.

The `deploy/rbac.yaml` **reader** ClusterRole already grants `get, list` on secrets scoped to `sh.helm.release.v1.*` resource names (from Stage 1). This covers `helm_list` and `helm_status`. `helm_rollback` needs write access to secrets, which belongs in the **destructive** ClusterRole.

```yaml
# deploy/rbac.yaml  (reader ClusterRole — existing Helm rule, unchanged)
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
    name: mimiops-mcp-reader
rules:
    # Helm — needed by Helm SDK to read release metadata from Secrets
    - apiGroups: [""]
      resources: [secrets]
      verbs: [get, list]
      resourceNames: ["sh.helm.release.v1.*"]
---
# Destructive ClusterRole — Stage 7 addition: secret write for helm_rollback
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
    name: mimiops-mcp-destructive
rules:
    - apiGroups: [""]
      resources: [secrets]
      verbs: [create, update]
```

**Why the secret-write rule is destructive:** `helm_rollback` writes a new release revision to a Secret, changing the live release state. It therefore lives in the destructive ClusterRole (not bound by default) and is gated by `--allow-destructive`, consistent with the existing destructive rules from earlier stages.

---

## 10. Error Handling Strategy

Extends the earlier error table with Helm cases.

| Scenario                                      | Behavior                                                                      |
| --------------------------------------------- | ----------------------------------------------------------------------------- |
| Release not found                             | `Release 'foo' not found in namespace 'bar'`                                  |
| No releases in namespace                      | `helm_list` returns an empty `releases` array (not an error)                  |
| `helm_rollback` at revision 1                 | `Release 'foo' in namespace 'bar' is at revision 1 — nothing to roll back to` |
| `helm_list` without `namespace`               | `Invalid parameter 'namespace': required`                                     |
| `helm_list` with invalid `status_filter`      | `Invalid parameter 'status_filter': must be one of failed, deployed`          |
| `helm_rollback` without `--allow-destructive` | `Destructive action not allowed (start with --allow-destructive)`             |
| Helm SDK call fails (no helm secrets found)   | `Failed to list releases in namespace 'bar': <cause>`                         |
| Permission denied (RBAC on secrets)           | `Permission denied: user cannot get secrets in namespace 'bar'`               |
| Invalid tool params                           | `Invalid parameter 'name' ...`                                                |

---

## 11. Summary of Key Design Decisions (Stage 7)

| Decision                         | Choice                                                                                                                | Rationale                                                                                                              |
| -------------------------------- | --------------------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------- |
| **Scope**                        | Three Helm tools: `helm_list`, `helm_status` (read-only) + `helm_rollback` (destructive)                              | Inspect and roll back Helm releases                                                                                    |
| **Helm SDK**                     | `helm.sh/helm/v3` as a Go library (no subprocess calls)                                                               | No external binary dependency; consistent with the original architecture                                               |
| **`helm_rollback` revision**     | **Always one back** (`current - 1`); no arbitrary `revision` param                                                    | Per design review — simple, predictable rollback semantics                                                             |
| **`helm_list` namespace**        | **Required** — missing/empty `namespace` returns an error                                                             | Per design review — no all-namespaces fallback                                                                         |
| **`helm_list` filters**          | `label_selector` (optional) + `status_filter` (optional, enum `failed`/`deployed`)                                    | Filter releases by labels and/or deployment status                                                                     |
| **`helm_status` output**         | Status metadata + **description message only**; `values`/`notes` are **not** returned                                 | Avoid leaking rendered values/secrets; keep output focused                                                             |
| **`helm_rollback` confirmation** | **No `input_required` flow** — executes immediately, gated only by `--allow-destructive`                              | Per design review — no per-call confirmation needed                                                                    |
| **`helm_rollback` blocking**     | **Non-blocking** — returns after submission, does not wait for ready state                                            | Matches `helm rollback` default behavior; fast response                                                                |
| **`helm_status` history**        | Embeds the **last 3 revisions** of history                                                                            | Gives the caller rollback context in one call; `helm_history` is skipped                                               |
| **`helm_history` tool**          | **Not implemented**                                                                                                   | Per design review — not needed                                                                                         |
| **Destructive guard**            | `helm_rollback` gated by `--allow-destructive` only (no `input_required`)                                             | Rolling back changes live release state, so it requires the `--allow-destructive` opt-in, but no per-call confirmation |
| **New package**                  | `internal/helm` wraps the Helm SDK; tool files stay thin                                                              | Keeps Helm SDK specifics out of the tool handlers                                                                      |
| **REST client getter**           | Reuses `internal/k8s` kubeconfig resolution so Helm talks to the same cluster/context/namespace                       | Single source of truth for cluster identity                                                                            |
| **RBAC**                         | Reader ClusterRole unchanged (secret read already granted); destructive ClusterRole gains `create, update` on secrets | Least privilege; rollback writes a new release revision                                                                |
| **Tool registration**            | One `Register*` per file, wired in central `register.go`                                                              | Consistent with earlier stages                                                                                         |
| **Formatting**                   | `format` param `text`/`json`, same conventions                                                                        | Consistent with earlier stages                                                                                         |
| **CLI / config**                 | Unchanged                                                                                                             | Additive stage, no new flags                                                                                           |

---

## 12. Open Questions

1. **`helm_list` `--all` flag.** Should `helm_list` support listing uninstalled/failed releases (the `--all` flag in `helm list`)? This design keeps it minimal with `namespace`, `label_selector`, and `status_filter`.
2. **`helm_status` notes.** Should `helm_status` optionally expose the rendered `notes` (not values) when explicitly requested? This design omits both `values` and `notes` by default.

---

This is the Stage-7 architecture for Helm tools. Two tools are read-only (`helm_list`, `helm_status`) and one is destructive (`helm_rollback`, gated by `--allow-destructive`). `helm_list` requires a `namespace` (error if missing) and supports `label_selector` and `status_filter` (enum `failed`/`deployed`); `helm_rollback` always rolls back exactly one revision with **no** `input_required` confirmation flow and is **non-blocking**; `helm_status` returns status metadata plus a **description message only** (no `values`/`notes`) and embeds the last 3 revisions of history so the caller has rollback context in a single call. It is purely additive to earlier stages — no CLI or config changes — and can be implemented by adding the `internal/helm` package, the three tool files, the `RESTClientGetter` helper on the k8s client, the `helm.sh/helm/v3` dependency, and the secret-write rule in the destructive ClusterRole.
