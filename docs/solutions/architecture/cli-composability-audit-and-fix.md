---
title: CLI Composability Audit and Fix
date: 2026-03-18
problem_type: architecture-issues
tags: [composability, stdout, stderr, exit-codes, json-output, scripting, ci-cd, auth, go, cobra]
symptoms:
  - Scripts piping --output json receive empty strings or mixed human text in stdout
  - Exit code is always 0 or 1; scripts cannot distinguish auth failures from not-found errors
  - Interactive pickers or stdin reads hang in CI pipelines
  - Commands require a TTY for non-interactive use cases
  - JSON mode returns non-JSON error text, breaking jq pipelines
  - auth status / config show / user status have no --output json support
  - Empty lists return hardcoded unindented JSON strings inconsistent with non-empty output
related_docs:
  - docs/solutions/architecture/non-interactive-cli-stdin-picker-removal.md
  - docs/solutions/feature-implementations/cli-output-format-flag.md
---

# CLI Composability Audit and Fix

## Problem

The Andamio CLI was not fully composable for scripting, agents, and CI pipelines. Eleven specific violations were identified across four categories:

1. **Stdout/stderr leakage** — progress text printed to stdout polluted pipes; empty-list edge cases emitted hardcoded unindented JSON strings
2. **Exit code uniformity** — all errors returned exit code 1; scripts could not distinguish auth failures (3) from not-found (2) from generic errors (1)
3. **JSON gaps** — `auth status`, `config show`, `user status` had no `--output json` mode; `spec fetch` emitted nothing in JSON mode; `spec paths` returned text-only output
4. **Headless auth** — `user login` requires a browser; CI pipelines had no way to supply a JWT non-interactively

Without these fixes, the two-step composable pattern was unreliable:

```bash
# This pattern failed in CI or when not logged in
PROJECT_ID=$(andamio project list --output json | jq -r '.data[0].project_id')
andamio project task list "$PROJECT_ID" --output json
```

## Root Cause

Several independent causes:

- No `SilenceErrors`/`SilenceUsage` on `rootCmd` — Cobra printed usage text to stderr on errors, polluting human-readable output
- `fmt.Print` / `fmt.Println` used for progress messages without checking `isJSON` or routing to stderr
- All errors were `error` interface returns from `fmt.Errorf`; no typed error hierarchy for exit code dispatch
- Status commands (`auth status`, `config show`, `user status`) predated the `--output` flag design
- Empty-list branches used `fmt.Println(\`{"data":[]}\`)` rather than `output.PrintJSON()`
- No env var fallback for JWT

## Solution

### 1. Typed Error Hierarchy (`internal/apierr/errors.go`)

Created a new package with sentinel error types that carry exit code semantics:

```go
package apierr

type NotFoundError struct{ Message string }
func (e *NotFoundError) Error() string { return e.Message }

type AuthError struct{ Message string }
func (e *AuthError) Error() string { return e.Message }
```

The HTTP client maps response codes to these types:

```go
switch resp.StatusCode {
case http.StatusUnauthorized, http.StatusForbidden:
    return &apierr.AuthError{Message: msg}
case http.StatusNotFound:
    return &apierr.NotFoundError{Message: msg}
}
return errors.New(msg)   // was: fmt.Errorf("%s", msg)
```

### 2. Exit Code Dispatch in `main.go`

`main.go` uses `errors.As` to unwrap typed errors and select the correct exit code:

```go
func init() {
    rootCmd.SilenceErrors = true
    rootCmd.SilenceUsage = true
}

func main() {
    if err := rootCmd.Execute(); err != nil {
        exitCode := 1
        var notFound *apierr.NotFoundError
        var authErr *apierr.AuthError
        switch {
        case errors.As(err, &notFound): exitCode = 2
        case errors.As(err, &authErr):  exitCode = 3
        }
        if output.GetFormat() == output.FormatJSON {
            b, _ := json.Marshal(map[string]string{"error": err.Error()})
            fmt.Println(string(b))
        } else {
            fmt.Fprintln(os.Stderr, err)
        }
        os.Exit(exitCode)
    }
}
```

**Exit code contract:**

| Code | Meaning | Trigger |
|------|---------|---------|
| 0 | Success | Normal completion |
| 1 | Generic error | API errors, config missing, invalid args |
| 2 | Not found | HTTP 404 responses |
| 3 | Auth required | HTTP 401/403, local auth guard failures |

**Critical:** Use `json.Marshal` for the JSON error envelope — never `fmt.Sprintf(\`{"error":%q}\`, ...)`. The `%q` verb produces Go-quoted strings with non-standard escapes that break `jq`.

### 3. Auth Guards Using Typed Errors

All `PreRunE`/`PersistentPreRunE` hooks that check authentication locally must return `*apierr.AuthError` — not `fmt.Errorf` — so the exit code contract covers local failures too:

```go
// Before (exit code 1 — wrong)
return fmt.Errorf("not authenticated. Run 'andamio user login' first")

// After (exit code 3 — correct)
return &apierr.AuthError{Message: "not authenticated. Run 'andamio user login' first"}
```

Six locations were updated: `project_task.go`, `course_export.go`, `course_import.go`, `course_import_all.go`, `course_create_module.go`, `teacher.go`.

### 4. JSON Mode for Status Commands

`auth status`, `config show`, and `user status` now emit structured JSON when `--output json` is set:

```go
// auth status
if output.GetFormat() == output.FormatJSON {
    type authStatus struct {
        APIKeySet bool   `json:"api_key_set"`
        BaseURL   string `json:"base_url"`
    }
    return output.PrintJSON(authStatus{APIKeySet: cfg.APIKey != "", BaseURL: cfg.BaseURL})
}
```

For `user status`, `SessionExpired` uses `*bool` (pointer) to distinguish three states: nil (no JWT), false (valid session), true (expired). A plain `bool` with `omitempty` silently drops `false` from JSON output.

```go
// Correct tristate pattern
type userStatusResult struct {
    SessionExpired *bool `json:"session_expired,omitempty"`
}
expired := now.After(expiresAt)
result.SessionExpired = &expired
```

### 5. `spec` Command JSON Modes

**`spec fetch`** emits a confirmation JSON object instead of silent success:

```go
if isJSON {
    return output.PrintJSON(map[string]interface{}{
        "path":        outPath,
        "api_version": apiVersion,
        "api_title":   apiTitle,
    })
}
fmt.Fprintf(os.Stderr, "Saved to %s\n", outPath)
```

**`spec paths`** returns a sorted JSON array in scripting mode:

```go
type specPathEntry struct {
    Method  string `json:"method"`
    Path    string `json:"path"`
    Summary string `json:"summary"`
}
// collect entries, sort, then:
if output.GetFormat() == output.FormatJSON {
    return output.PrintJSON(entries)
}
```

### 6. Empty-List Branches

Replace all hardcoded `fmt.Println(\`{"data":[]}\`)` with `output.PrintJSON()`:

```go
// Before — hardcoded, unindented, bypasses output package
if output.GetFormat() == output.FormatJSON {
    fmt.Println(`{"data":[]}`)
}

// After — consistent, indented, routes through output package
if !ok || len(data) == 0 {
    if output.GetFormat() == output.FormatJSON {
        return output.PrintJSON(map[string]interface{}{"data": []interface{}{}})
    }
    fmt.Fprintln(os.Stderr, emptyMsg)
    return nil
}
```

### 7. `ANDAMIO_JWT` Environment Variable

CI pipelines and headless agents can now supply a JWT without running `user login`:

```go
// internal/config/config.go — in Load(), after reading the config file:
if jwt := os.Getenv("ANDAMIO_JWT"); jwt != "" {
    cfg.UserJWT = jwt
}
```

The env var takes precedence over the stored config value. This must be applied in both the file-exists path and the default-config fallback path.

```bash
# Usage in CI:
ANDAMIO_JWT="$CI_JWT_TOKEN" andamio project task list "$PROJECT_ID" --output json
```

### 8. `errors.New` vs `fmt.Errorf("%s", msg)`

Replace `fmt.Errorf("%s", msg)` with `errors.New(msg)` when constructing errors from plain strings. The `fmt.Errorf` form triggers staticcheck SA1006 ("redundant Sprintf") and will fail CI lint checks.

## Files Changed

| File | Change |
|------|--------|
| `internal/apierr/errors.go` | NEW — typed error sentinel types |
| `internal/client/client.go` | Returns typed errors on 401/403/404; `errors.New` for generic |
| `cmd/andamio/main.go` | `SilenceErrors`/`SilenceUsage`; exit code dispatch; JSON error envelope |
| `cmd/andamio/auth.go` | JSON mode for `auth status` |
| `cmd/andamio/config.go` | JSON mode for `config show` |
| `cmd/andamio/user.go` | JSON mode for `user status`; `*bool` for `SessionExpired` |
| `cmd/andamio/course.go` | `output.PrintJSON` for empty list branches |
| `cmd/andamio/spec.go` | JSON confirmation for `spec fetch`; JSON array for `spec paths` |
| `cmd/andamio/course_create_module.go` | Progress to stderr; `apierr.AuthError` auth guard |
| `cmd/andamio/course_export.go` | `apierr.AuthError` auth guard |
| `cmd/andamio/course_import.go` | `apierr.AuthError` auth guard |
| `cmd/andamio/course_import_all.go` | `apierr.AuthError` auth guard |
| `cmd/andamio/teacher.go` | `apierr.AuthError` auth guard; removed unused `fmt` import |
| `cmd/andamio/project_task.go` | `apierr.AuthError` auth guard |
| `internal/config/config.go` | `ANDAMIO_JWT` env var override in `Load()` |

## Verification

Smoke tests for the composability contract:

```bash
# T1: JSON error envelope on auth failure
andamio project task list fake-id --output json 2>/dev/null
# → {"error":"not authenticated..."}, exit 3

# T2: Not-found exit code
andamio course get does-not-exist --output json 2>/dev/null
# → {"error":"API error 404:..."}, exit 2

# T3: Spec paths scripting
andamio spec paths --output json | jq '.[0].method'
# → "GET" (or "POST")

# T4: Spec fetch confirmation
andamio spec fetch --output json | jq .path
# → "openapi.json"

# T5: Empty list JSON consistency
andamio course list --output json | jq .data
# → [] (indented, same shape as non-empty)

# T6: Status commands in JSON mode
andamio auth status --output json | jq .api_key_set
andamio config show --output json | jq .base_url
andamio user status --output json | jq .user_authenticated

# T7: Headless auth via env var
ANDAMIO_JWT="$TOKEN" andamio project task list "$PROJECT_ID" --output json

# T8: No stdout pollution from progress messages
andamio course export "$COURSE_ID" 2>/dev/null | jq .
# → Valid JSON, no human-readable text mixed in
```

## Prevention

**Checklist for every new command:**

- [ ] All progress/status messages use `fmt.Fprintf(os.Stderr, ...)` gated with `if !isJSON`
- [ ] All structured output uses the `output` package — no `fmt.Println` for data
- [ ] If command can fail with HTTP 401/403 → local auth guard returns `&apierr.AuthError{}`
- [ ] If command performs a write operation → `--output json` emits a confirmation object
- [ ] No hardcoded JSON string literals anywhere in command files
- [ ] No `fmt.Errorf("%s", msg)` — use `errors.New(msg)` for plain string errors
- [ ] No reading from stdin in command handlers

**Code review red flags:**

```go
fmt.Println(`{"...}`)          // hardcoded JSON — use output.PrintJSON
fmt.Printf("...")              // progress to stdout — use os.Stderr
fmt.Errorf("not auth...")      // plain error — use &apierr.AuthError{}
fmt.Errorf("%s", msg)          // redundant — use errors.New(msg)
SessionExpired bool `omitempty` // bool with omitempty drops false — use *bool
```

## Related

- [Non-interactive CLI stdin picker removal](non-interactive-cli-stdin-picker-removal.md) — companion fix for interactive picker anti-pattern
- [CLI output format flag](../feature-implementations/cli-output-format-flag.md) — foundation: how `--output` flag and format detection work
- PR #15: `fix/composability-gaps` — implementation PR
- Todos 011–017 in `todos/` — individual review findings addressed in this work
- `CLAUDE.md` Composability Rules section — codified rules derived from this audit
