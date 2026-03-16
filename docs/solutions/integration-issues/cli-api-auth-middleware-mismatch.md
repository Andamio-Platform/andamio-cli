---
title: "CLI Authentication Headers Conflict in v1 Middleware"
date: 2026-03-16
category: integration-issues
tags: [authentication, headers, middleware, api-versioning]
components: [andamio-cli, andamio-api, auth-middleware]
symptom: "CLI commands (user me, user usage) fail with 400 Bad Request after successful OAuth login when user has both API key and JWT configured"
root_cause: "v1 AuthMiddleware explicitly rejects requests containing both X-API-Key and Authorization: Bearer headers simultaneously"
---

# CLI Authentication Headers Conflict in v1 Middleware

## Problem

After successfully authenticating with `andamio user login` (browser OAuth flow), CLI commands `andamio user me` and `andamio user usage` failed with:

```
Error: API error 400: {"status_code":400,"message":"Bad Request: Invalid input."}
```

The login worked perfectly - browser opened, wallet signed, JWT stored. But subsequent authenticated API calls failed.

## Investigation

1. **Verified CLI auth state** - `andamio user status` showed valid JWT and API key
2. **Checked endpoint existence** - `/api/v1/user/me` exists in API swagger docs
3. **Examined middleware code** - Found the rejection logic in `auth_middleware.go`:

```go
// auth_middleware.go:26-29
if apiKeyHeader != "" && authHeader != "" {
    return errors.NewHTTPError(http.StatusBadRequest, "Bad Request",
        "Provide either Authorization header or X-API-Key header, not both")
}
```

4. **Confirmed CLI sends both headers** - When user has both API key and JWT configured, the HTTP client sends both:
   - `X-API-Key: ant_xxxx...`
   - `Authorization: Bearer eyJ...`

## Root Cause

**Architectural mismatch between v1 and v2 auth models:**

| Version | Middleware | Dual Headers | Use Case |
|---------|------------|--------------|----------|
| v1 | `AuthMiddleware` | Rejected | Single auth (API key OR JWT) |
| v2 | `V2AuthMiddleware` | Accepted | Two-layer auth (app + user) |

The CLI was designed for the v2 two-layer auth model where:
- `X-API-Key` authenticates the app/developer
- `Authorization` authenticates the end-user

But `user me` was calling a v1 endpoint which only accepts one auth method.

## Solution

### 1. Migrate to v2 Dashboard Endpoint

Changed `user me` from `GET /api/v1/user/me` to `POST /api/v2/user/dashboard`:

```go
var userMeCmd = &cobra.Command{
    Use:   "me",
    Short: "Get current user dashboard",
    RunE:  runUserMe,
}

func runUserMe(cmd *cobra.Command, args []string) error {
    cfg, err := config.Load()
    if err != nil {
        return err
    }

    c := client.New(cfg)
    var result map[string]interface{}
    if err := c.Post("/api/v2/user/dashboard", nil, &result); err != nil {
        return err
    }

    if output.GetFormat() == output.FormatJSON {
        return output.PrintJSON(result)
    }

    data, ok := result["data"].(map[string]interface{})
    if !ok {
        return output.PrintJSON(result)
    }

    printDashboard(data)
    return nil
}
```

### 2. Remove `user usage` Command

Removed `user usage` as it exposed internal developer metrics not appropriate for public CLI v1.

### 3. Add `postJSON` Helper

Added helper in `course.go` for POST endpoints:

```go
func postJSON(path string) error {
    cfg, err := config.Load()
    if err != nil {
        return err
    }

    c := client.New(cfg)
    var result map[string]interface{}
    if err := c.Post(path, nil, &result); err != nil {
        return err
    }

    return output.PrintJSON(result)
}
```

### 4. Colorized Dashboard Output

Added ANSI color support for readable terminal output:

```go
const (
    cReset      = "\033[0m"
    cBold       = "\033[1m"
    cDim        = "\033[2m"
    cCyan       = "\033[36m"
    cGreen      = "\033[32m"
    cYellow     = "\033[33m"
    cMagenta    = "\033[35m"
    cBlue       = "\033[34m"
)

func printDashboard(data map[string]interface{}) {
    // Colorized sections for Summary, Teaching, Learning, Managing
    // Supports -o json for raw output
}
```

## Prevention

### Endpoint Selection Checklist

Before implementing CLI features:

1. **Does the request need both app AND user auth?** → Use v2
2. **Will the CLI send both X-API-Key and Authorization?** → Use v2
3. **Is it internal tooling only?** → v1 may be acceptable

### Quick Reference

| Headers Present | v1 Result | v2 Result |
|----------------|-----------|-----------|
| X-API-Key only | 200 OK | 200 OK |
| Authorization only | 200 OK | 401 (requires X-API-Key) |
| Both headers | **400 Bad Request** | 200 OK |
| Neither | 401 | 401 |

### Error to Watch For

```
"Provide either Authorization header or X-API-Key header, not both"
```

This always means: **switch to a v2 endpoint**.

## Related Documentation

- `andamio-api/.claude/skills/auth/SKILL.md` - Auth system architecture
- `andamio-api/docs/plans/2026-03-13-refactor-v1-routes-cleanup-and-migration-plan.md` - v1 deprecation
- `andamio-cli/docs/plans/2026-03-13-feat-browser-wallet-authentication-plan.md` - CLI auth design
- `andamio-api/internal/middleware/v2_auth_middleware.go:88-91` - v2 dual-header acceptance

## Files Changed

- `cmd/andamio/user.go` - New runUserMe, printDashboard, color constants, removed userUsageCmd
- `cmd/andamio/course.go` - Added postJSON helper
- `docs/plans/2026-03-14-test-wallet-auth-preprod-plan.md` - Updated test results
