# Kubernetes MCP Server — Stage 6: Log Tools Output Schema

## Go-based Local Daemon, Scoped to Log Tools

This document extends the architecture in `1-architect.md` / `2-architect-pods-only.md` (Stage 1) and the workload refinements in `5-architecture-workloads.md` (Stage 4). It redesigns the two log tools — `jobs_log` and `pods_log` — to return **structured output** via the MCP output-schema convention (`mcp.WithOutputSchema[...]()` + `mcp.NewToolResultStructured`), matching how the `*_get` and `*_list` tools already behave.

**Scope for this stage:** two read-only tools.

| Resource | Tools |
|----------|-------|
| Jobs | `jobs_log` |
| Pods | `pods_log` |

**Key change vs. Stage 1:** the log tools no longer return raw text or a hand-rolled `{"logs": [...]}` JSON blob. Instead each tool declares a typed output schema and returns a structured result. The `format` parameter (`"text"` / `"json"`) is **removed** — the MCP client receives the structured object directly, and the tool supplies a short human-readable fallback string for clients that only render text.

---

## 1. Architectural Overview

The architecture is unchanged from earlier stages. The only delta is the tool registry content, the output types, and the backend calls they make against `core/v1` (pods) and `batch/v1` (jobs).

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
│   │  jobs_log     │   │  pods_log     │   │  *_get/_list  │  │
│   │  (structured) │   │  (structured) │   │  (structured) │  │
│   └───────┬───────┘   └───────┬───────┘   └───────┬───────┘  │
│           └───────────────────┼───────────────────┘          │
│                               ▼                              │
│                 ┌───────────────────────────┐                │
│                 │  k8s typed client         │                │
│                 │  (core/v1, batch/v1)      │                │
│                 └───────────────────────────┘                │
└───────────────────────────┬──────────────────────────────────┘
                            │  HTTPS
                            ▼
                 ┌──────────────────────┐
                 │  Kubernetes API      │
                 └──────────────────────┘
```

---

## 2. Project Structure (Stage 6 additions)

No new files are required. The redesign touches the two existing log tools and reuses shared types already defined in `types.go`.

```
internal/tools/
├── jobs_log.go     # jobs_log tool — rewritten to return JobLogResult
├── pods_log.go     # pods_log tool — rewritten to return PodLogResult
├── types.go        # JobSummary, PodSummary, LogLine (existing); LogStream added
└── ...
```

---

## 3. Core Dependencies

Unchanged from earlier stages. The log tools continue to use the `k8s` typed client and the `mcp-go` server library. No new dependencies are introduced.

---

## 4. Shared Output Types

The two tools share a common "stream" shape so that a log entry is always `{pod, container, logs}` regardless of whether it came from a Job or a Pod.

### 4.1 `LogStream`

A single stream of raw log output for one pod/container pair.

```go
// LogStream is one pod/container log stream.
type LogStream struct {
    Pod       string `json:"pod" jsonschema:"Name of the pod"`
    Container string `json:"container" jsonschema:"Name of the container"`
    Logs      string `json:"logs" jsonschema:"Raw log output from the container"`
}
```

- `pod` — the pod name.
- `container` — the container name the logs were fetched from.
- `logs` — the **raw** log output from the container, exactly as returned by the Kubernetes API. It is **not** split into lines and no timestamp is parsed or added; the content is preserved verbatim (including any embedded newlines).

---

## 5. `jobs_log` Output Schema

### 5.1 Result type

```go
// JobLogResult is the structured result of jobs_log.
type JobLogResult struct {
    JobSummary

    Streams []LogStream `json:"streams" jsonschema:"Log streams from the Job's pods"`
}
```

- Embeds `JobSummary` (namespace, name, completions, duration, age, status) so the caller gets Job context alongside the logs.
- `streams` — one `LogStream` per pod/container fetched.

### 5.2 Tool registration

```go
tool := mcp.NewTool("jobs_log",
    mcp.WithReadOnlyHintAnnotation(true),
    mcp.WithDestructiveHintAnnotation(false),
    mcp.WithIdempotentHintAnnotation(true),
    mcp.WithToolTitle("Get Job Logs"),
    mcp.WithDescription("Fetch logs from a Job's pods"),
    mcp.WithString("name", mcp.Description("Job name"), mcp.Required()),
    mcp.WithString("namespace", mcp.Description("namespace"), mcp.Required()),
    mcp.WithString("container", mcp.Description("container name (optional)")),
    mcp.WithInteger("tail", mcp.Description("number of lines to show from end of logs"), mcp.DefaultNumber(20)),
    mcp.WithBoolean("previous", mcp.Description("return previous terminated container logs"), mcp.DefaultBool(false)),
    mcp.WithBoolean("all_pods", mcp.Description("fetch logs from all owned pods"), mcp.DefaultBool(false)),
    mcp.WithOutputSchema[JobLogResult](),
)
```

- The `format` parameter is removed.
- `all_pods=false` (default) fetches only the most recently created pod; `all_pods=true` fetches every pod owned by the Job.

### 5.3 Behavior

1. Resolve the Job by `name`/`namespace`.
2. List pods and filter by the Job's owner reference (existing logic).
3. If no pods exist, return a structured result with an empty `streams` array and a fallback text explaining the Job has no pods yet.
4. For each pod to fetch, resolve the container name (explicit `container`, else the `kubectl.kubernetes.io/default-container` annotation, else the first container — existing logic).
5. Fetch the log stream and store the **raw** output verbatim (no line splitting, no timestamp parsing).
6. Append a `LogStream{Pod, Container, Logs}` to `streams`.
7. Return `mcp.NewToolResultStructured(result, fallbackText)`.

### 5.4 Example output

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

---

## 6. `pods_log` Output Schema

### 6.1 Result type

```go
// PodLogResult is the structured result of pods_log.
type PodLogResult struct {
    PodSummary

    Streams []LogStream `json:"streams" jsonschema:"Log streams from the pod's containers"`
}
```

- Embeds `PodSummary` (namespace, name, ready, restarts, age, status, node, owner references).
- `streams` — one `LogStream` per container fetched. For a single-container pod this is a one-element array; the shape is identical to `jobs_log` so clients can treat both tools uniformly.

### 6.2 Tool registration

```go
tool := mcp.NewTool("pods_log",
    mcp.WithReadOnlyHintAnnotation(true),
    mcp.WithDestructiveHintAnnotation(false),
    mcp.WithIdempotentHintAnnotation(true),
    mcp.WithToolTitle("Get Pod Logs"),
    mcp.WithDescription("Fetch pod logs"),
    mcp.WithString("name", mcp.Description("pod name"), mcp.Required()),
    mcp.WithString("namespace", mcp.Description("namespace"), mcp.Required()),
    mcp.WithString("container", mcp.Description("container name (optional)")),
    mcp.WithInteger("tail", mcp.Description("number of lines to show from end of logs"), mcp.DefaultNumber(20)),
    mcp.WithBoolean("previous", mcp.Description("return previous terminated container logs"), mcp.DefaultBool(false)),
    mcp.WithInteger("since_seconds", mcp.Description("only return logs newer than N seconds"), mcp.DefaultNumber(0)),
    mcp.WithOutputSchema[PodLogResult](),
)
```

- The `format` parameter is removed.
- `container` remains optional. When omitted, the tool resolves the default container (annotation, else first container — existing logic) and returns a single stream. When a container is supplied, that container's logs are returned.

### 6.3 Behavior

1. Resolve the Pod by `name`/`namespace`.
2. Resolve the container name (existing logic).
3. Fetch the log stream and store the **raw** output verbatim (no line splitting, no timestamp parsing).
4. Build a single `LogStream{Pod, Container, Logs}`.
5. Return `mcp.NewToolResultStructured(result, fallbackText)`.

### 6.4 Example output

```json
{
  "namespace": "default",
  "name": "nginx-7d8f9",
  "ready": "1/1",
  "restarts": 0,
  "age": "5d",
  "status": "Running",
  "node": "node-1",
  "ownerReferences": [
    { "apiVersion": "apps/v1", "kind": "ReplicaSet", "name": "nginx-7d8f9" }
  ],
  "streams": [
    {
      "pod": "nginx-7d8f9",
      "container": "nginx",
      "logs": "GET / 200\nGET /health 200\n"
    }
  ]
}
```

---

## 7. Output Formatting

Both tools follow the Stage-4 convention: `mcp.NewToolResultStructured(result, fallbackText)`.

- **Structured payload** — the typed result object (Job/Pod summary + `streams`). This is what MCP clients consume as the machine-readable output.
- **Fallback text** — a short human-readable sentence used only by clients that render text. Examples:

  - `jobs_log`: `"Job 'pi' in namespace 'default' has 1 log stream(s)."`
  - `pods_log`: `"Pod 'nginx-7d8f9' in namespace 'default' has 1 log stream(s)."`

The `format` parameter and the old `formatPodLog` / `formatPodLogJSON` helpers are removed. Log output is stored **raw** — exactly as returned by the Kubernetes API, not split into lines and without any parsed timestamps — and delivered as a string inside `streams`.

---

## 8. Backend: K8s Typed Client (log additions)

Unchanged from earlier stages. The tools use:

- `client.CoreV1().Pods(namespace).Get(...)` — resolve the pod.
- `client.CoreV1().Pods(namespace).List(...)` — list pods owned by a Job.
- `client.CoreV1().Pods(namespace).GetLogs(podName, logOpts).Stream(ctx)` — fetch log bytes.
- `client.BatchV1().Jobs(namespace).Get(...)` — resolve the Job.

The existing `fetchPodLogs` helper is retained (and may be shared by `pods_log`) but is refactored to return the **raw** log string (or a `LogStream`) instead of a formatted/parsed value.

---

## 9. RBAC Template

Unchanged from earlier stages. The log tools require read access to pods and their logs:

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: mimiops-mcp-reader
rules:
  - apiGroups: [""]
    resources: ["pods", "pods/log"]
    verbs: ["get", "list"]
  - apiGroups: ["batch"]
    resources: ["jobs"]
    verbs: ["get", "list"]
```

---

## 10. Error Handling Strategy

- Missing required `name` / `namespace` → `mcp.NewToolResultError`.
- Job/Pod not found → `mcp.NewToolResultErrorf` with the underlying error.
- Log stream fetch failure → `mcp.NewToolResultErrorf` naming the pod and container.
- Job with no pods yet → structured result with empty `streams` (not an error), so the caller can distinguish "no logs yet" from a real failure.

---

## 11. Summary of Key Design Decisions (Stage 6)

1. **Structured output everywhere** — both log tools use `mcp.WithOutputSchema[...]()` and `mcp.NewToolResultStructured`, dropping the `format` param.
2. **Shared `LogStream` shape** — `jobs_log` and `pods_log` both emit `{pod, container, logs}`, so clients parse logs identically across the two tools.
3. **Embedded summaries** — `JobSummary` / `PodSummary` are embedded so the caller gets resource context alongside logs without a second call.
4. **`LogStream.Logs` is raw** — the container output is stored verbatim as a single string; it is not split by newline and no timestamp is parsed or synthesized.
5. **Empty streams, not errors** — a Job with no pods returns an empty `streams` array rather than failing.

---

## 12. Open Questions

1. Should `pods_log` support fetching **all containers** of a multi-container pod (returning multiple `LogStream` entries) when `container` is omitted, or keep resolving a single default container?
2. Should `jobs_log` expose `since_seconds` (currently only `pods_log` has it) for parity?