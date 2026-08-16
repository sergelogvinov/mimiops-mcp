# Kubernetes MCP Server — Stage 2: Jobs & CronJobs

## Go-based Local Daemon, Scoped to Batch Workloads (Read-Only)

This document extends the revised architecture in `2-architect-pods-only.md` (Stage 1) with the **Jobs / CronJobs** tool set. It reuses the exact same CLI, MCP registration, formatting, and confirmation conventions established in Stage 1 — only the tool catalog, backend calls, and RBAC are new. The original tool specs for these tools live in `1-architect.md` (§5.1 `jobs_*` / `cronjobs_*`); this document refines them to match the Stage-1 implementation patterns.

**Scope for this stage:** the read-only batch tools (`list`, `get`, `describe`, `log`) **plus** four mutating batch tools — `cronjobs_suspend`, `cronjobs_resume`, `jobs_create` (create a Job from a CronJob template), and `jobs_delete`. All four are gated by `--allow-destructive` + the `input_required` confirmation flow.

---

## 1. Architectural Overview

The architecture is unchanged from Stage 1. The only delta is the tool registry content and the backend calls it makes against `batch/v1`.

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
│  │  CLI (cobra) — unchanged from Stage 1               │    │
│  │    mcp | server | version                            │    │
│  │    --kubeconfig --context --namespace --impersonate  │    │
│  │    --allow-destructive --log-level  (+ --port)       │    │
│  └──────────────────────────┬───────────────────────────┘    │
│                             │                                 │
│  ┌──────────────────────────▼───────────────────────────┐    │
│  │  MCP Server (mark3labs/mcp-go)                       │    │
│  │  Tool Registry — Stage 1 + Stage 2 additions:        │
│  │    jobs_list  jobs_get  jobs_describe  jobs_log      │
│  │    cronjobs_list  cronjobs_get  cronjobs_describe    │
│  │    cronjobs_suspend  cronjobs_resume  jobs_create     │
│  │    (mutating tools gated by --allow-destructive)     │    │
│  └───────────────────────┬────────────────────────────┘    │
│                          │                                  │
│  ┌───────────────────────▼────────────────────────────┐    │
│  │  Tool Handlers (internal/tools)                     │    │
│  │    one file per tool (see §2)                       │    │
│  └───────────────────────┬────────────────────────────┘    │
│                          │                                  │
│  ┌───────────────────────▼────────────────────────────┐    │
│  │  Backend: k8s typed client (client-go)             │    │
│  │    BatchV1().Jobs(ns) / CronJobs(ns)               │    │
│  │    CoreV1().Pods(ns)  (owned-pod lookup for logs)  │    │
│  └───────────────────────┬────────────────────────────┘    │
└──────────────────────────┼─────────────────────────────────┘
                           ▼
                  ┌─────────────────────────┐
                  │   Kubernetes API Server  │
                  │   (authenticated via     │
                  │    kubeconfig)           │
                  └─────────────────────────┘
```

No new transports, no new CLI flags, no new external dependencies. This stage is purely additive to the tool catalog.

---

## 2. Project Structure (Stage 2 additions)

The Stage-1 tree is unchanged; new tool files are added under `internal/tools/`, **one file per tool** (each exposing a single `Register*` function). No changes to `cmd/`, `internal/config`, `internal/k8s`, or `internal/formatter`.

```
mimiops/
├── internal/
│   ├── tools/
│   │   ├── register.go          # central wiring: calls every Register* below
│   │   ├── jobs_list.go         # jobs_list                              (NEW)
│   │   ├── jobs_get.go          # jobs_get                               (NEW)
│   │   ├── jobs_describe.go     # jobs_describe                          (NEW)
│   │   ├── jobs_log.go          # jobs_log (owned-pod log fetch)         (NEW)
│   │   ├── jobs_delete.go       # jobs_delete (mutating, gated)          (NEW)
│   │   ├── jobs_create.go       # jobs_create (from CronJob template)    (NEW, mutating)
│   │   ├── cronjobs_list.go     # cronjobs_list                          (NEW)
│   │   ├── cronjobs_get.go      # cronjobs_get                           (NEW)
│   │   ├── cronjobs_describe.go # cronjobs_describe                      (NEW)
│   │   ├── cronjobs_suspend.go  # cronjobs_suspend (mutating, gated)     (NEW)
│   │   └── cronjobs_resume.go   # cronjobs_resume (mutating, gated)      (NEW)
│   └── ...
└── deploy/
    └── rbac.yaml                # + batch read + mutating rules (see §8)
```

**File ownership rule (from Stage 1, enforced here):** each tool = one `Register*` function in its **own** file, with the `mcp.NewTool(...)` description/schema and the handler colocated. `internal/tools/register.go` is the only place that names every tool. No two tools share a file in this stage — `jobs_list`, `jobs_get`, `jobs_describe`, `jobs_log`, `jobs_delete`, `jobs_create`, `cronjobs_list`, `cronjobs_get`, `cronjobs_describe`, `cronjobs_suspend`, and `cronjobs_resume` each live in their own file.

---

## 3. Core Dependencies

**No new dependencies.** `batch/v1` (Jobs, CronJobs) is part of the already-present `k8s.io/client-go` / `k8s.io/api` typed clientset. `jobs_log` reuses the existing pod-log plumbing from Stage 1 (`pods_log`).

```go
module github.com/sergelogvinov/mimiops-mcp

go 1.26

    github.com/spf13/pflag v1.0.10        // Flags
    github.com/spf13/cobra v1.10.2        // CLI command dispatch
    k8s.io/client-go                      // typed clientset (BatchV1, CoreV1)
    k8s.io/api                            // batch/v1.Job, batch/v1.CronJob types
```

Helm, metrics, and the dynamic client remain out of scope for this stage.

---

## 4. CLI Interface

**Unchanged from Stage 1.** No new flags or subcommands. The `--namespace` global flag already scopes batch reads; empty means all namespaces. `--allow-destructive` gates the four mutating tools (`cronjobs_suspend`, `cronjobs_resume`, `jobs_create`, `jobs_delete`) — they are only registered when it is set.

---

## 5. Tool Specifications (Stage-2 Catalog)

All tools follow the Stage-1 conventions: `format` param (`"text"` default / `"json"`), typed client-go, and the central `register.go` wiring.

### 5.1 Read Tools

<details>
<summary><b>jobs_list</b></summary>

| Field | Value |
|-------|-------|
| **name** | `jobs_list` |
| **description** | List Jobs in a namespace (or all namespaces) |
| **params** | `namespace` (string, optional), `label_selector` (string, optional), `format` (string, optional, default: `"text"`) |
| **response** | Text: markdown table `name, namespace, completions, duration, age, status` / JSON: `[]JobSummary` |
| **status derivation** | `Complete` if `status.succeeded == spec.completions`; `Failed` if `status.failed > 0` (or backoffLimit exceeded); otherwise `Running` (active pods > 0) or `Pending` |
| **completions** | `"succeeded/completions"` (e.g. `1/1`); `completions` defaults to 1 when unset |
| **duration** | `status.completionTime - status.startTime`; `-` if not finished |
| **RBAC** | `get`, `list` jobs |
</details>

<details>
<summary><b>jobs_get</b></summary>

| Field | Value |
|-------|-------|
| **name** | `jobs_get` |
| **description** | Get a single Job's full spec and status |
| **params** | `name` (required), `namespace` (required), `format` (optional, default: `"text"`) |
| **response** | Text: key-value describe format / JSON: full `batch/v1.Job` object |
| **RBAC** | `get` jobs |
</details>

<details>
<summary><b>jobs_describe</b></summary>

| Field | Value |
|-------|-------|
| **name** | `jobs_describe` |
| **description** | Human-readable Job summary (conditions, parallelism, completions, backoff, pod selector, active pods) |
| **params** | `name` (required), `namespace` (required), `format` (optional, default: `"text"`) |
| **response** | Rich formatted description: parallelism, completions, backoffLimit, activeDeadlineSeconds, selector, labels, conditions (Complete/Failed/Suspended), start/completion time, active/succeeded/failed counts, owned pods (name + status) |
| **notes** | Lists pods owned by the Job (via `metav1.GetControllerOf` + `batch/v1.Job` owner ref) to show per-pod status, mirroring `kubectl describe job`. The owned-pod listing is **capped at 5** — if there are more, show the first 5 and append `... and N more`. |
| **RBAC** | `get` jobs, `get`/`list` pods |
</details>

<details>
<summary><b>jobs_log</b></summary>

| Field | Value |
|-------|-------|
| **name** | `jobs_log` |
| **description** | Fetch logs from a Job's pods |
| **params** | `name` (required), `namespace` (required), `container` (string, optional), `tail` (int, optional, default: `20`, max: `5000`), `previous` (bool, optional, default: `false`), `all_pods` (bool, optional, default: `false`), `format` (optional, default: `"text"`) |
| **response** | Text: log lines as plain text (prefixed with pod name when `all_pods=true`) / JSON: `[]LogLine` |
| **implementation** | 1. `Get` the Job. 2. List pods in the namespace and filter by `metav1.GetControllerOf(pod)` matching the Job's UID. 3. If `all_pods=false`, pick the most recently created pod (`creationTimestamp`); if `true`, fetch all owned pods. 4. Reuse the Stage-1 `pods_log` fetch path per pod, including its **multi-container behavior**: if a pod has multiple containers and `container` is omitted, return an error listing the available containers. |
| **notes** | If the Job has no pods yet (not started / already cleaned up), return a clear message rather than an error. `previous=true` reads the crashed container's logs. When `all_pods=true`, the `container` param applies to every pod fetched; if any pod is multi-container and `container` is omitted, error listing that pod's containers. |
| **RBAC** | `get` jobs, `get`/`list` pods, `get` pods/log |
</details>

<details>
<summary><b>cronjobs_list</b></summary>

| Field | Value |
|-------|-------|
| **name** | `cronjobs_list` |
| **description** | List CronJobs in a namespace (or all namespaces) |
| **params** | `namespace` (string, optional), `format` (optional, default: `"text"`) |
| **response** | Text: markdown table `name, namespace, schedule, suspend, status, last_schedule, age` / JSON: `[]CronJobSummary` |
| **status derivation** | `Suspended` if `spec.suspend=true` **and** the CronJob has no running jobs; `Running (x/y)` if `spec.suspend=true` **and** it has `x` active jobs out of `y` total owned jobs (a suspended CronJob can still have in-flight jobs from before suspension); otherwise `Active` (not suspended) |
| **notes** | `last_schedule` from `status.lastScheduleTime`; `suspend` shown as `True`/`False`; `age` from `creationTimestamp`. The `schedule` column shows the **raw cron expression** (e.g. `0 2 * * *`) — no humanized next-run time. |
| **RBAC** | `get`, `list` cronjobs |
</details>

<details>
<summary><b>cronjobs_get</b></summary>

| Field | Value |
|-------|-------|
| **name** | `cronjobs_get` |
| **description** | Get a single CronJob's full spec and status |
| **params** | `name` (required), `namespace` (required), `format` (optional, default: `"text"`) |
| **response** | Text: key-value describe format / JSON: full `batch/v1.CronJob` object |
| **RBAC** | `get` cronjobs |
</details>

<details>
<summary><b>cronjobs_describe</b></summary>

| Field | Value |
|-------|-------|
| **name** | `cronjobs_describe` |
| **description** | Human-readable CronJob summary (schedule, suspend, concurrency policy, active jobs, last schedule, job template) |
| **params** | `name` (required), `namespace` (required), `format` (optional, default: `"text"`) |
| **response** | Rich formatted description: schedule, suspend, concurrencyPolicy, startingDeadlineSeconds, successful/failed jobs history limits, active jobs (names), lastScheduleTime, and a summary of the job template (image, restartPolicy, backoffLimit) |
| **notes** | Lists active Jobs owned by the CronJob (via owner reference) to show what is currently running. The owned-Job listing is **capped at 5** — if there are more, show the first 5 and append `... and N more`. If the CronJob is suspended but still has in-flight jobs, surface that explicitly (e.g. `Suspended — running 1/2 jobs`). |
| **RBAC** | `get` cronjobs, `get`/`list` jobs |
</details>

> **Note on `cronjobs_log`:** CronJobs do not produce logs directly — they create Jobs, which create Pods. To inspect a CronJob's output, the model should call `cronjobs_get`/`cronjobs_describe` to find the active/last Job, then `jobs_log` on that Job. A dedicated `cronjobs_log` tool is intentionally **not** added (see §10).

### 5.2 Mutating Tools (gated by `--allow-destructive` + `input_required`)

<details>
<summary><b>cronjobs_suspend</b></summary>

| Field | Value |
|-------|-------|
| **name** | `cronjobs_suspend` |
| **description** | Suspend a CronJob (stops future scheduled runs) |
| **params** | `name` (required), `namespace` (required), `format` (optional, default: `"text"`) |
| **destructive** | Yes — gated by `--allow-destructive` + `input_required` confirmation. Rationale: pausing scheduled jobs can have business impact. |
| **action** | Patches the CronJob's `spec.suspend` to `true` |
| **response** | Confirmation of the suspend + the updated `spec.suspend` value |
| **RBAC** | `patch` cronjobs |
</details>

<details>
<summary><b>cronjobs_resume</b></summary>

| Field | Value |
|-------|-------|
| **name** | `cronjobs_resume` |
| **description** | Resume a suspended CronJob (re-enables future scheduled runs) |
| **params** | `name` (required), `namespace` (required), `format` (optional, default: `"text"`) |
| **destructive** | Yes — gated by `--allow-destructive` + `input_required` confirmation. Rationale: resuming a CronJob that was intentionally suspended can have production impact. |
| **action** | Patches the CronJob's `spec.suspend` to `false` |
| **response** | Confirmation of the resume + the updated `spec.suspend` value |
| **RBAC** | `patch` cronjobs |
</details>

<details>
<summary><b>jobs_create</b></summary>

| Field | Value |
|-------|-------|
| **name** | `jobs_create` |
| **description** | Create a one-off Job from a CronJob's job template (CLI equivalent: `kubectl create job --from=cronjob/<name>`) |
| **params** | `cronjob` (required, the CronJob to source the template from), `namespace` (required), `job_name` (string, optional — default `<cronjob>-manual-<random4>`), `format` (optional, default: `"text"`) |
| **destructive** | Yes — gated by `--allow-destructive` + `input_required` confirmation. Rationale: running the Job executes the template's containers, which can have side effects (migrations, reports, external calls). |
| **action** | 1. `Get` the CronJob. 2. Copy `spec.jobTemplate.spec` into a new `batch/v1.Job`. 3. Set `job.metadata.name` to `job_name` (or generate `<cronjob>-manual-<random4>`). 4. Set `job.metadata.namespace` and copy the CronJob's labels/annotations. 5. Clear `spec.suspend` (a manual run executes regardless of the CronJob's suspend state) and reset `spec.selector`/`spec.template.metadata.labels` to a fresh, unique label set so the Job is not adopted by the CronJob. 6. `Create` the Job. |
| **response** | Confirmation of creation + the created Job's name/namespace |
| **RBAC** | `get` cronjobs, `create` jobs |
| **notes** | Mirrors `kubectl create job --from=cronjob/<name> <job-name>`. The generated suffix is a **4-character** random alphanumeric string (`<cronjob>-manual-<random4>`). Before creating, the server checks whether the generated name already exists in the namespace; if it does, it **retries with a new random suffix** (up to a bounded number of attempts, e.g. 10) before falling back to an error. An explicit `job_name` is used as-is and a collision is reported as an error. |
</details>

<details>
<summary><b>jobs_delete</b></summary>

| Field | Value |
|-------|-------|
| **name** | `jobs_delete` |
| **description** | Delete a Job (cascading — also deletes owned pods) |
| **params** | `name` (required), `namespace` (required), `propagation_policy` (string, optional, enum: `"Background"`, `"Foreground"`, default: `"Background"`), `format` (optional, default: `"text"`) |
| **destructive** | Yes — gated by `--allow-destructive` + `input_required` confirmation. Rationale: deleting a Job removes its pods and any completed run history. |
| **action** | Deletes the Job via `client.BatchV1().Jobs(ns).Delete(ctx, name, opts)` with the requested `propagation_policy`. `Background` (default) lets the Job controller delete owned pods asynchronously; `Foreground` blocks until owned pods are deleted first. |
| **response** | Confirmation of deletion + the Job's name/namespace |
| **RBAC** | `delete` jobs |
</details>

All mutating tools in this stage are `cronjobs_suspend`, `cronjobs_resume`, `jobs_create`, and `jobs_delete`.

---

## 6. Destructive Confirmation Flow (input_required)

Applies to the four mutating tools (`cronjobs_suspend`, `cronjobs_resume`, `jobs_create`, `jobs_delete`). Reuses the Stage-1 two-phase `input_required` flow verbatim: registration gated by `--allow-destructive`, then a confirmation prompt on the first call, and execution only when the client calls again with `confirm=true`. Read tools are unaffected.

```
Client calls jobs_create(cronjob="backup", namespace="default")
                     │
                     ▼
Server: checks --allow-destructive (error if false)
  → Validates params
  → confirm is absent / false
  → Returns CallToolResult (isError=false) with:
      — content: "This will create Job 'backup-manual-abc12' from CronJob
                  'backup' in namespace 'default'. Call again with
                  confirm=true to proceed."
      — inputRequired: true
                     │
                     ▼
Client presents confirmation to user
                     │
                     ▼
Client calls jobs_create again with confirm=true
                     │
                     ▼
Server: sees confirm=true → creates the Job → returns result
```

If `confirm=true` is missing, the confirmation prompt is returned again. All four mutating tools are also gated by `--allow-destructive`; if unset, the handler returns an error.

---

## 7. Output Formatting

Same `format` param convention as Stage 1.

### Text

- Lists → markdown tables.
- Describes → key-value blocks.
- Logs → plain text.

Example (`jobs_list` text):

```
NAMESPACE     NAME              COMPLETIONS   DURATION   AGE   STATUS
default       db-migrate-01     1/1           42s        5d    Complete
default       nightly-report    0/1           -          2h    Running
default       cleanup-xyz       0/1           3m         1d    Failed
```

Example (`cronjobs_list` text):

```
NAMESPACE     NAME              SCHEDULE        SUSPEND   STATUS         LAST SCHEDULE   AGE
default       backup            "0 2 * * *"     False     Active         2026-08-15T02:00Z   30d
default       nightly-report    "0 0 * * *"     True      Suspended      -               2h
default       paused-migrate    "0 0 * * *"     True      Running (1/2)  2026-08-15T00:00Z   1d
```

### JSON

```json
{
  "jobs": [
    {
      "namespace": "default",
      "name": "db-migrate-01",
      "completions": "1/1",
      "duration": "42s",
      "age": "5d",
      "status": "Complete"
    }
  ]
}
```

```json
{
  "cronjobs": [
    {
      "namespace": "default",
      "name": "backup",
      "schedule": "0 2 * * *",
      "suspend": false,
      "status": "Active",
      "last_schedule": "2026-08-15T02:00:00Z",
      "age": "30d"
    },
    {
      "namespace": "default",
      "name": "paused-migrate",
      "schedule": "0 0 * * *",
      "suspend": true,
      "status": "Running (1/2)",
      "last_schedule": "2026-08-15T00:00:00Z",
      "age": "1d"
    }
  ]
}
```

---

## 8. Backend: K8s Typed Client (batch additions)

The Stage-1 `k8s.Client` (typed clientset + resolved identity) is reused unchanged. Stage 2 only adds calls against `BatchV1()` and reuses `CoreV1()` for owned-pod lookups.

```go
// internal/k8s/client.go — no structural change; new call sites:

// jobs_list      → client.BatchV1().Jobs(ns).List(ctx, opts)
// jobs_get       → client.BatchV1().Jobs(ns).Get(ctx, name, opts)
// jobs_describe  → GET job + list owned pods (CoreV1().Pods(ns).List + GetControllerOf)
// jobs_log       → GET job → owned pods → CoreV1().Pods(ns).GetLogs(pod, opts).Stream(ctx)
// cronjobs_list  → client.BatchV1().CronJobs(ns).List(ctx, opts)
// cronjobs_get   → client.BatchV1().CronJobs(ns).Get(ctx, name, opts)
// cronjobs_describe → GET cronjob + list owned jobs (BatchV1().Jobs(ns).List + GetControllerOf)
// cronjobs_suspend → client.BatchV1().CronJobs(ns).Patch(ctx, name, types.MergePatchType, `{"spec":{"suspend":true}}`)
// cronjobs_resume  → client.BatchV1().CronJobs(ns).Patch(ctx, name, types.MergePatchType, `{"spec":{"suspend":false}}`)
// jobs_create       → GET cronjob → build batchv1.Job from spec.jobTemplate → client.BatchV1().Jobs(ns).Create(ctx, job, opts)
// jobs_delete      → client.BatchV1().Jobs(ns).Delete(ctx, name, opts) with propagation policy
```

**Owned-resource lookup helper** (shared by `jobs_describe`, `jobs_log`, `cronjobs_describe`): a small helper in `internal/k8s` that lists a resource type and filters by `metav1.GetControllerOf(obj)` matching a given owner UID. This mirrors the Stage-1 `pods_log` owner-resolution approach and keeps the logic in one place.

```go
// internal/k8s/owner.go (NEW)
// OwnerPods(ctx, ns, ownerUID) []v1.Pod
// OwnerJobs(ctx, ns, ownerUID) []batchv1.Job
```

---

## 9. RBAC Template (batch additions)

The Stage-1 `deploy/rbac.yaml` gains batch read rules in the **reader** ClusterRole, and batch mutating verbs (`create`/`delete` jobs, `patch` cronjobs) in the **destructive** ClusterRole.

```yaml
# deploy/rbac.yaml  (reader ClusterRole — additions shown)
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: mimiops-mcp-reader
rules:
# Pods — Stage 1 (unchanged)
- apiGroups: [""]
  resources: [pods, pods/log, pods/status]
  verbs: [get, list, watch]
# Events — Stage 1 (unchanged)
- apiGroups: [""]
  resources: [events]
  verbs: [get, list, watch]
# Jobs + CronJobs — Stage 2 (NEW)
- apiGroups: ["batch"]
  resources: [jobs, jobs/status, cronjobs, cronjobs/status]
  verbs: [get, list, watch]
---
# Destructive ClusterRole — Stage 2 additions (NOT bound by default)
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: mimiops-mcp-destructive
rules:
# Pods — Stage 1 (unchanged)
- apiGroups: [""]
  resources: [pods]
  verbs: [delete]
# Jobs + CronJobs — Stage 2 (NEW)
- apiGroups: ["batch"]
  resources: [jobs]
  verbs: [create, delete]   # jobs_create, jobs_delete
- apiGroups: ["batch"]
  resources: [cronjobs]
  verbs: [patch]            # cronjobs_suspend / cronjobs_resume
```

**Why `pods`/`pods/log` are required for batch tools:** `jobs_describe` and `jobs_log` must list owned pods and read their logs; `cronjobs_describe` must list owned jobs. These are already granted by Stage 1, so no additional pod rules are needed.

---

## 10. Error Handling Strategy

Extends the Stage-1 table with batch-specific cases.

| Scenario | Behavior |
|----------|----------|
| Job not found | `Job 'foo' not found in namespace 'bar'` |
| CronJob not found | `CronJob 'foo' not found in namespace 'bar'` |
| K8s API failure (network/auth/RBAC) | `Failed to list jobs: <cause>` |
| Permission denied | `Permission denied: user cannot get jobs in namespace 'bar'` |
| `jobs_log` with no owned pods | Informational message: `Job 'foo' has no pods (not started or already cleaned up)` |
| `jobs_log` `all_pods=true` with no pods | Same informational message, not an error |
| `jobs_create` CronJob not found | `CronJob 'foo' not found in namespace 'bar'` |
| `jobs_create` name collision (generated) | Retry with a new 4-char random suffix (up to 10 attempts); if exhausted, `Could not generate a unique name for Job from CronJob 'foo' in namespace 'bar'` |
| `jobs_create` name collision (explicit `job_name`) | `Job 'foo-manual-abc1' already exists in namespace 'bar'` |
| `jobs_delete` Job not found | `Job 'foo' not found in namespace 'bar'` |
| Mutating tool without `--allow-destructive` | `Destructive tool disabled. Restart with --allow-destructive.` |
| Mutating tool with `confirm=false` | Return confirmation prompt (inputRequired=true) |
| Invalid tool params | `Invalid parameter 'name' ...` |

---

## 11. Summary of Key Design Decisions (Stage 2)

| Decision | Choice | Rationale |
|----------|--------|-----------|
| **Scope** | Read-only Jobs/CronJobs (`list`, `get`, `describe`, `log`) + mutating `cronjobs_suspend`, `cronjobs_resume`, `jobs_create`, `jobs_delete` | Read tools always registered; mutating tools gated by `--allow-destructive` |
| **K8s client** | client-go **typed** `BatchV1()` (no dynamic client) | Type-safe, minimal; `batch/v1` is a stable built-in API |
| **New dependencies** | None | `batch/v1` ships with existing client-go/api |
| **`cronjobs_log`** | Not added | CronJobs produce no logs directly; route through `jobs_log` |
| **Owned-resource lookup** | Shared `internal/k8s/owner.go` helper (`GetControllerOf` + UID filter) | Reused by `jobs_describe`, `jobs_log`, `cronjobs_describe` |
| **Tool registration** | One `Register*` per file (`jobs_list.go`, `jobs_get.go`, `jobs_describe.go`, `jobs_log.go`, `jobs_delete.go`, `jobs_create.go`, `cronjobs_list.go`, `cronjobs_get.go`, `cronjobs_describe.go`, `cronjobs_suspend.go`, `cronjobs_resume.go`), wired in central `register.go` | Consistent with Stage 1; each tool isolated in its own file |
| **Formatting** | `format` param `text`/`json`, same conventions | Consistent with Stage 1 |
| **RBAC** | Batch read rules in reader ClusterRole; `create`/`delete` jobs + `patch` cronjobs in destructive ClusterRole | Least privilege; destructive role not bound by default |
| **CLI / config** | Unchanged | Additive stage, no new flags |
| **`jobs_create` source** | Copies `spec.jobTemplate.spec` from a CronJob, resets selector/labels, clears `suspend`; generated name `<cronjob>-manual-<random4>` with collision-retry | Mirrors `kubectl create job --from=cronjob/<name>`; manual run executes even if suspended |

---

## 12. Open Questions

1. **`jobs_log` default pod selection.** **Resolved:** the default pod selection stays as the most recently created owned pod (`all_pods=false`). The multi-container handling adopts the `pods_log` behavior — a `container` param, and an error listing available containers if it is omitted on a multi-container pod.
2. **Job status edge cases.** **Resolved:** a suspended CronJob that still has running jobs is shown as `Running (x/y)` (x active jobs out of y total owned jobs); a suspended CronJob with no running jobs is shown as `Suspended`. This applies to the `cronjobs_list` `status` column and is surfaced in `cronjobs_describe`.

---

This is the Stage-2 architecture for Jobs/CronJobs. It is purely additive to Stage 1 — no CLI, config, or dependency changes — and can be implemented by adding three tool files plus the shared owner-lookup helper and the batch RBAC rules.
