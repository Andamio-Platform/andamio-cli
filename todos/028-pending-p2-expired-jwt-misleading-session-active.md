---
status: pending
priority: p2
issue_id: "028"
tags: [auth, ux, user-login, jwt]
dependencies: []
---

# Expired JWT Shows Misleading "Session: active (no expiry info)"

## Problem

When a user JWT expires (24hr lifetime), `andamio user status` still reports
the session as active. Any command that requires auth silently hits a 401:

```
API error 401: {"status_code":401,"message":"Unauthorized: Invalid or missing credentials."}
```

The user has no indication their session is expired until they hit this error.
They must then discover for themselves that `andamio user logout && andamio user login`
is the fix.

### Root cause

`HasUserAuth()` (`internal/config/config.go`) only checks `c.UserJWT != ""` —
it does not check expiry. When `JWTExpiresAt` is empty (headless `.skey` login
never stores it — `user.go:336` explicitly sets it to `""`), `runUserStatus`
falls through to `"Session: active (no expiry info)"`.

Even when `JWTExpiresAt` is stored (browser login), no other code path reads
it — commands proceed with the expired token and get a 401 from the gateway.

## Fix

### 1. Add `jwtExpiry()` helper (suggested location: `cmd/andamio/helpers.go`)

Parse expiry directly from the JWT payload — no extra dependencies. JWTs are
`header.payload.signature` where each part is base64url-encoded. The payload
contains an `exp` Unix timestamp claim.

```go
// jwtExpiry extracts the expiry time from a JWT payload without verifying
// the signature. Used as a fallback when JWTExpiresAt is not stored in config
// (e.g. headless login). Returns (time, true) on success, (zero, false) if
// the token is malformed or has no exp claim.
func jwtExpiry(token string) (time.Time, bool) {
    parts := strings.Split(token, ".")
    if len(parts) != 3 {
        return time.Time{}, false
    }
    payload, err := base64.RawURLEncoding.DecodeString(parts[1])
    if err != nil {
        return time.Time{}, false
    }
    var claims struct {
        Exp int64 `json:"exp"`
    }
    if err := json.Unmarshal(payload, &claims); err != nil || claims.Exp == 0 {
        return time.Time{}, false
    }
    return time.Unix(claims.Exp, 0), true
}
```

### 2. Expand `PersistentPreRunE` (`cmd/andamio/main.go:79`)

Warn to stderr before every command if the JWT is expired. Stderr keeps
`--output json` scripting clean. Skip warning for `user login` and
`user logout` (those are the remedy).

```go
PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
    if err := output.SetFormat(outputFormat); err != nil {
        return err
    }
    name := cmd.Name()
    if name == "login" || name == "logout" {
        return nil
    }
    cfg, err := config.Load()
    if err != nil {
        return nil
    }
    if cfg.UserJWT != "" {
        if expiry, ok := jwtExpiry(cfg.UserJWT); ok && time.Now().After(expiry) {
            fmt.Fprintf(os.Stderr, "Warning: your session has expired. Run 'andamio user logout && andamio user login' to re-authenticate.\n")
        }
    }
    return nil
},
```

### 3. Fix `user login` re-login block (`cmd/andamio/user.go:125`)

Currently blocks re-login if any JWT is present, even expired. Should allow
re-login when the session has expired without requiring a manual logout first.

```go
if cfg.HasUserAuth() {
    if expiry, ok := jwtExpiry(cfg.UserJWT); ok && time.Now().After(expiry) {
        cfg.ClearUserAuth()
    } else {
        fmt.Printf("Already authenticated as: %s\n", cfg.UserAlias)
        fmt.Println("Run 'andamio user logout' first to re-authenticate.")
        return nil
    }
}
```

## Files to change

- `cmd/andamio/main.go` — expand `PersistentPreRunE`
- `cmd/andamio/user.go` — fix re-login block
- `cmd/andamio/helpers.go` — add `jwtExpiry()` helper

## Notes

- No new dependencies needed — `encoding/base64`, `encoding/json`, `strings`,
  `time` are all already imported in the package.
- `jwtExpiry()` does NOT verify the JWT signature — only reads the exp claim.
  Signature verification is the gateway's job. We only need the timestamp for UX.
- The headless login flow (`user.go:336`) could also be updated to extract and
  store `JWTExpiresAt` from the JWT itself after login, so expiry is visible in
  `user status`. Lower priority — the `PersistentPreRunE` warning covers the gap.
