# fallbackText — Design: Go struct → Markdown conversion

This document defines the rules for a function that converts a Go struct value
into a **markdown raw string** (fallback text) for old versions of agents that
cannot consume JSON/structured output.

> **Scope note:** This is a _design_ document only. No code is implemented here.
> The rules below are the contract the future function must satisfy.

---

## 1. Purpose

Given a Go struct value (one of the types in `internal/tools/types.go`), produce
a human/agent-readable markdown string that describes the data. The output is
used as a fallback when an agent cannot parse the structured (JSON) response.

---

## 2. Input / Output

- **Input:** a single Go struct value (e.g. `PodSummary`, `NodeSummary`,
  `EventSummary`, …).
- **Output:** a markdown string.

### Example (from the draft)

```go
PodSummary{
    Namespace: "default",
    Name: "my-pod",
    Roles: []string{"web", "api"},
    OwnerReferences: []OwnerReference{
        {
            APIVersion: "v1",
            Kind:       "ReplicaSet",
            Name:       "my-replicaset",
        },
    },
}
```

Renders to:

```markdown
Namespace of the Pod: default
Name of the Pod: my-pod
Roles of the Pod: web, api

### Owner references of the Pod

| API version | Kind       | name          |
| ----------- | ---------- | ------------- |
| v1          | ReplicaSet | my-replicaset |
```

---

## 3. Field selection

Iterate over the struct fields **in declaration order**.

A field is **printable** only if it carries a `jsonschema` tag. Fields without
a `jsonschema` tag are **skipped entirely** — no label, no value, no line.

---

## 4. Label source

The **label** (the text before the colon, or the heading text) is the
`jsonschema` tag value. Because fields without a `jsonschema` tag are skipped
entirely (§3), the label always exists for any printed field.

---

## 5. Value formatting by Go type

The rendering of a field's value depends on its Go type:

| Go type                                                     | Rendering                                         |
| ----------------------------------------------------------- | ------------------------------------------------- |
| Scalar (`string`, `int`, `int32`, `bool`, `float*`)         | Render the value as-is (`bool` → `true`/`false`). |
| Slice of scalars (`[]string`, `[]int32`, …)                 | Join elements with `", "`.                        |
| Slice of structs (`[]OwnerReference`, `[]ContainerInfo`, …) | Render as a markdown table (see §7).              |
| Single struct (`NodeCapacityInfo`, `Replicas`, …)           | Render as a heading block (see §6).               |
| Map (`map[string]string`, `map[string]any`, …)              | Render as `key=value` pairs (see §8).             |

---

## 6. Nested structs → headings

A field whose type is a single struct (not a slice) is rendered as a **markdown
heading** followed by the struct's fields as plain text lines. **No indentation
or padding is used** for nested structures.

The heading level depends on the nesting depth of the struct:

- A struct nested directly under the top-level struct → `###` (3 hashes).
- A struct nested one level deeper → `####` (4 hashes).
- Each additional nesting level adds one more `#`.

```markdown
### Capacity of the node

CPU capacity of the node: 4
Memory capacity of the node: 8Gi
Maximum number of pods the node can run: 110
```

A deeper example:

```markdown
### Capacity of the node

CPU capacity of the node: 4

#### Some deeper struct

Field of the deeper struct: value
```

The nested fields follow the same rules (§3–§5) recursively.

---

## 7. Slices of structs → markdown table

When a field is a slice of structs, render it as a markdown table:

- **Heading:** the field label as a heading (same level rules as §6).
- **Header row:** the labels of the element struct's printable fields
  (in the element struct's declaration order).
- **Separator row:** `| --- | --- | … |` (one column per field).
- **Body:** one row per element, values in the same column order as the header.

```markdown
### Owner references of the Pod

| API version | Kind       | name          |
| ----------- | ---------- | ------------- |
| v1          | ReplicaSet | my-replicaset |
```

Rules:

- An **empty slice** omits the field entirely (no heading, no table).
- If the element struct itself contains nested structs/slices/maps, those cells
  are rendered with the same inline rules as §5 (e.g. a slice-of-scalars cell is
  comma-joined).

---

## 8. Maps

A `map` field is rendered as a comma-separated list of `key=value` pairs on a
single plain text line:

```markdown
Labels: app=web, env=prod
```

Rules:

- Keys and values are rendered with `key=value` (no surrounding spaces).
- Pairs are comma-separated on a single line.
- Map iteration order is non-deterministic; sort keys for stable output.

---

## 9. `omitempty` and zero values

- A field tagged `,omitempty` whose value is the zero value is **omitted**.
- A field **not** tagged `,omitempty` is always printed, even if zero-valued
  (e.g. an empty string prints an empty value).
- An empty slice / empty map is omitted regardless of `omitempty`.

---

## 10. Top-level shape

The output is a flat list of plain text lines in the form `Label: value`. No
bullet markers and no indentation are used at the top level. Nested structures
are introduced by headings (§6, §7).

---

## 11. Edge cases

- **Nested structs inside table cells** — render with inline rules.
- **Deeply nested slices of structs** — recurse; a slice of structs inside a
  struct inside a slice becomes a nested table under a deeper heading.
- **`map[string]any`** (e.g. `WorkloadDetails.Spec` / `.Status`) — values are
  arbitrary; render each `key=value` pair with the value stringified.
- **`nil` pointers / nil maps / nil slices** — render as empty (omitted).
- **Empty slice / empty map** — omitted entirely (no heading, no table).

---

## 12. Implementation notes

- The function is implemented using the Go standard library `reflect` package.
- It is **generic/reflection-based**: it inspects the struct type at runtime and
  handles any struct automatically, so no per-type hand-written code is needed.
- The type switch on `reflect.Kind` mirrors the table in §5:
    - `String`, `Int*`, `Uint*`, `Float*`, `Bool` → scalar
    - `Slice`/`Array` → recurse on element kind (scalar → join, struct → table)
    - `Struct` → heading block (§6)
    - `Map` → `key=value` list (§8)
    - `Ptr` → dereference (nil pointer renders as empty)
    - `Interface` → unwrap to the concrete value
- Heading level is derived from nesting depth: `###` at depth 1, adding one `#`
  per deeper level (§6).
- Unknown/unhandled kinds are skipped.
</content>
</write_file>
