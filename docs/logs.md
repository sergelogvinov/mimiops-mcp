# Log Sanitizer Design

This document specifies the second shared utility for the mimiops-mcp server: a
**log sanitizer** that masks secrets inside pod/job log output before it is
surfaced to the MCP client.

It complements the key-removal util in `docs/utils.md`. That util _removes_
whole keys from `map[string]string` metadata; this one _masks values_ inside
free-form log text, where secrets appear as:

- structured fields (`key=value`, logfmt, JSON),
- raw values (credit-card numbers, phone numbers, tokens, keys).

## 1. Motivation

`pods_log` (`internal/tools/pods_log.go`) and `jobs_log`
(`internal/tools/jobs_log.go`) return raw container output via
`fetchPodLogStream` (`internal/tools/pods.go`). Container logs routinely
contain sensitive data:

| Data                | Example                              |
| ------------------- | ------------------------------------ |
| API keys / tokens   | `sk-...`, `ghp_...`, `Bearer eyJ...` |
| Credit-card numbers | `4111 1111 1111 1111`                |
| CVV                 | `cvv=123`                            |
| Phone numbers       | `+1 (555) 123-4567`                  |
| Emails              | `user@example.com`                   |
| Private keys        | `-----BEGIN RSA PRIVATE KEY-----`    |
| AWS keys            | `AKIA...`                            |

There is currently no filtering, so anything the app logs is passed straight to
the MCP client. This util centralizes masking in one testable component.

## 2. Package & Location

- Package: `utils`
- Path: `internal/utils/sanitize.go`
- Tests: `internal/utils/sanitize_test.go`

Like the obfuscator, it depends only on the Go standard library (`regexp`,
`strings`, `encoding/json`).

## 3. Semantics

- **Mode: mask.** Sensitive values are replaced with a mask string (default
  `***`). The surrounding structure (keys, separators, quotes) is preserved so
  the log remains readable.
- **Mask style: full replacement.** The entire sensitive value is replaced
  (e.g. `4111 1111 1111 1111` → `***`). Partial reveal (keeping the last 4
  digits) is **not** used.
- **Always on.** Redaction is unconditional; there is no `--no-redact`
  escape hatch. Every log line returned through `fetchPodLogStream` is masked.
- **Pure function.** `Sanitize` returns a new string; the input is never mutated.
- **Line-oriented.** Input is processed line by line. This keeps regexes simple
  (no cross-line matching), bounds memory, and matches how logs are naturally
  structured. Multi-line secrets (e.g. PEM blocks) are handled by a dedicated
  block pattern that is applied across the whole input first.
- **Two detection strategies** (see §4):
    1. **Key-based** — redact the _value_ of a known-sensitive key.
    2. **Value-based** — redact any value matching a known-sensitive _format_.
- **Conservative defaults.** Value-based patterns only match high-confidence
  formats (Luhn-valid card numbers, known token prefixes, PEM blocks). Noisy
  short patterns (bare 3–4 digit numbers) are key-based only, to avoid mangling
  legitimate log content.

## 4. Detection Strategies

### 4.1 Key-based (structured logs)

Recognizes sensitive keys in common structured formats and masks their values:

- **logfmt / `key=value`**: `password=secret`, `token=abc`, `cvv=123`
- **JSON**: `{"password": "secret", "card_number": "4111..."}`
- **Quoted values**: `password="secret"`

Sensitive keys are matched by **exact name** (fast, precise) or by a
**key regex** (catches variants like `db_password`, `access_token`).

Default exact keys:

`password`, `passwd`, `secret`, `token`, `api_key`, `apikey`, `access_key`,
`access_token`, `refresh_token`, `auth_token`, `client_secret`,
`client_id`, `card_number`, `cardnumber`, `cc_number`, `ccn`, `cvv`, `cvc`,
`pin`, `ssn`, `phone`, `telephone`, `mobile`, `email`, `authorization`,
`cookie`, `private_key`, `privatekey`, `aws_secret_access_key`,
`connection_string`, `dsn`.

Default key regexes:

| Regex                                    | Matches                         |
| ---------------------------------------- | ------------------------------- |
| `(?i)(password\|passwd\|pwd)\b`          | `db_password`, `user_password`  |
| `(?i)(token\|secret\|apikey\|api_key)\b` | `access_token`, `client_secret` |
| `(?i)(card\|cc\|cvv\|cvc)\b`             | `card_number`, `ccv`            |
| `(?i)(phone\|telephone\|mobile\|tel)\b`  | `phone_number`, `mobile_no`     |

### 4.2 Value-based (raw formats)

Catches secrets that appear without a sensitive key. Only high-confidence
formats are enabled by default:

| Pattern                                                        | Example                                | Confidence |
| -------------------------------------------------------------- | -------------------------------------- | ---------- |
| Credit card (Luhn-valid, 13–19 digits, optional spaces/dashes) | `4111 1111 1111 1111`                  | High       |
| JWT / Bearer token                                             | `eyJhbGciOi...`                        | High       |
| GitHub token                                                   | `ghp_...`, `gho_...`, `github_pat_...` | High       |
| Slack token                                                    | `xoxb-...`, `xoxp-...`                 | High       |
| AWS access key                                                 | `AKIA[0-9A-Z]{16}`                     | High       |
| Stripe key                                                     | `sk_live_...`, `pk_live_...`           | High       |
| PEM private key block                                          | `-----BEGIN ... PRIVATE KEY-----`      | High       |
| Generic long hex/base64 secret (≥ 32 chars)                    | `a3f9...`                              | Medium     |
| Email                                                          | `user@example.com`                     | Medium     |
| Phone (E.164 / grouped)                                        | `+1 (555) 123-4567`                    | Medium     |

**Deliberately NOT value-matched by default** (too noisy): bare 3–4 digit
numbers (CVV), bare 16-digit numbers without Luhn, IP addresses, UUIDs. These
are only redacted when they appear under a sensitive key.

## 5. API

```go
package utils

// Sanitizer masks sensitive values in log text.
// It is safe for concurrent use after construction.
type Sanitizer struct {
    exactKeys     map[string]struct{}
    keyPatterns   []*regexp.Regexp
    valuePatterns []*regexp.Regexp
    mask          string
}

// NewDefaultSanitizer returns a Sanitizer with the built-in defaults (§4).
func NewDefaultSanitizer() *Sanitizer

// Sanitize returns a copy of input with sensitive values masked.
func (s *Sanitizer) Sanitize(input string) string

// AddSensitiveKeys adds exact key names to sanitize.
func (s *Sanitizer) AddSensitiveKeys(keys ...string)

// AddKeyPatterns compiles and adds key regexes. Returns an error on bad regex.
func (s *Sanitizer) AddKeyPatterns(patterns ...string) error

// AddValuePatterns compiles and adds value regexes. Returns an error on bad regex.
func (s *Sanitizer) AddValuePatterns(patterns ...string) error
```

### Implementation notes

- **Precompiled regexes.** All patterns are compiled once at construction.
  `Sanitize` only runs `ReplaceAllString`, so per-call cost is minimal.
- **Order of operations in `Sanitize`:**
    1. Mask multi-line blocks (PEM) across the whole input.
    2. Split into lines; for each line run key-based masking, then value-based
       masking.
    3. Join lines back.
- **Key-based masking** uses a single combined regex built from the exact keys
  and key patterns, so each line is scanned once:
  `(?i)(key1|key2|pattern1|pattern2)\s*=\s*("?[^"\s,}]+"?)` plus a JSON variant
  `"(key)"\s*:\s*("?[^",}]+"?)`.
- **Masking preserves the key and separator**, replacing only the value:
  `password=secret` → `password=***`.
- **Quoted values** keep their quotes: `"password": "secret"` →
  `"password": "***"`.
- **Value-based masking** runs `ReplaceAllString` for each value pattern.
- **Luhn check** for card numbers is implemented in Go (not regex) to avoid
  false positives; the regex first extracts candidate digit runs, then Luhn
  validates them.

## 6. Wiring

Sanitizing is **always on** with the built-in defaults — there is no
per-deployment configuration, no CLI flags, and no env vars. The sanitizer is
constructed once at startup from the default key/value patterns
(`NewDefaultSanitizer`) and passed to the log tools.

### Tool wiring

Sanitizing is applied **only** inside `fetchPodLogStream`
(`internal/tools/pods.go`), on the raw log content read from the stream,
before it is wrapped in the returned `LogStream`. This single point covers
both `pods_log` and `jobs_log`, since both call `fetchPodLogStream`:

```go
san := utils.NewDefaultSanitizer()
return LogStream{
    Pod:       podName,
    Container: container,
    Logs:      san.Sanitize(string(logContent)),
}, nil
```

Because both log tools share this helper, no changes are needed in
`pods_log.go` or `jobs_log.go` themselves.

## 7. Affected Call Sites

| File                         | Change                                                                           |
| ---------------------------- | -------------------------------------------------------------------------------- |
| `internal/tools/pods.go`     | Mask `string(logContent)` in `fetchPodLogStream` before building the `LogStream` |
| `internal/tools/register.go` | Construct `san *utils.Sanitizer` once and thread it into the log tools           |

## 8. Testing

`internal/utils/sanitize_test.go` covers:

- **Key-based logfmt:** `password=secret` → `password=***`.
- **Key-based JSON:** `{"token": "abc", "level": "info"}` → token masked, level
  preserved.
- **Key regex variants:** `db_password=secret` masked via `(?i)password\b`.
- **Value-based:** Luhn-valid card number masked; Luhn-invalid 16-digit number
  left intact.
- **Noisy-value guard:** bare `cvv=123`-style short numbers only masked under a
  key; a standalone `123` is untouched.
- **PEM block:** multi-line private key fully masked.
- **Preservation:** keys, separators, quotes, and non-sensitive content intact.
- **Empty / no-match input:** returned unchanged.
- **Bad regex:** an invalid pattern in the defaults fails construction.
- **Concurrency:** a constructed `Sanitizer` can be read from multiple goroutines
  (guarded by `go test -race`).

## 9. Out of Scope (v1)

- **Streaming redaction.** Logs are fetched fully via `io.ReadAll` today; the
  sanitizer operates on the whole string. Streaming line-by-line redaction can
  be added later if logs grow large.
- **Contextual CVV.** CVV is only redacted under a key; detecting a bare
  `123`-style CVV next to a card number is left for a future heuristic.
- **Encryption / reversible masking.** Masking is one-way; no way to recover the
  original value.
- **Per-namespace / per-pod policies.** One global sanitizer applied at
  `fetchPodLogStream`.
- **Partial reveal.** Only full replacement (`***`) is supported; keeping the
  last 4 digits is out of scope.

## 10. Decisions

The following design choices have been settled:

1. **Mask style: full replacement.** Sensitive values are replaced entirely
   with `***`; no partial reveal.
2. **Always on.** Redaction is unconditional with no `--no-redact` escape
   hatch.
3. **Apply point:** `fetchPodLogStream` in `internal/tools/pods.go` only. Since
   both `pods_log` and `jobs_log` call this helper, redaction is applied
   consistently to both without changing either tool file.
4. **Email / phone defaults:** on by default. They catch common contact PII;
   the risk of over-redaction is accepted for v1.
5. **Structured-log parsing:** regex-based flat approach for v1 (no full JSON
   parse / recursive masking of nested objects).

## 11. Remaining Questions

1. **Structured-log parsing depth** — is regex-based flat masking sufficient,
   or is full JSON parsing with recursive nested-object masking wanted later?
2. **Describe tools** — should `pods_describe` / `jobs_describe` (which may
   embed env/secret references) also route through the sanitizer, or stay as-is?
