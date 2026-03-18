---
status: complete
priority: p3
issue_id: "017"
tags: [code-review, agent-native, auth, ci-cd, composability]
dependencies: []
---

# Add `ANDAMIO_JWT` Env Var for Headless/CI Authentication

## Problem Statement

The `user login` command requires a browser and a 5-minute interactive wait. There is no mechanism for agents, CI pipelines, or Docker containers to supply a JWT token non-interactively.

All JWT-gated write commands (`project task create/update/delete`, `course export`, `course import`) are therefore inaccessible to agents in headless environments. An agent can read all data (API key covers read operations) but cannot create or modify anything.

This is the standard pattern for CLI tools that support both human and machine auth:
- GitHub CLI: `GITHUB_TOKEN`
- Fly.io CLI: `FLY_API_TOKEN`
- Vercel CLI: `VERCEL_TOKEN`
- Andamio could follow with: `ANDAMIO_JWT`

## Findings

- **Source**: Agent-native reviewer (P2 #4)
- **Location**: `internal/config/config.go` (config load), `cmd/andamio/user.go` (login flow)

## Proposed Solutions

### Option A: Read `ANDAMIO_JWT` env var in `config.Load()` (Recommended)

```go
// In config.Load(), after reading the file:
if jwt := os.Getenv("ANDAMIO_JWT"); jwt != "" {
    cfg.UserJWT = jwt
    // optionally also read ANDAMIO_JWT_EXPIRES_AT
}
```

Env var takes precedence over the stored config value. Zero changes to command code — any command that reads `cfg.UserJWT` will automatically benefit.

**Pros:** Standard pattern. Works with CI secrets. No change to command code. Reversible (just unset the env var).
**Cons:** The JWT from env is not validated before use — an expired or malformed JWT will fail on the first API call with exit code 3.
**Effort:** Small (3-5 lines in config.go)
**Risk:** Low

### Option B: Add `user login --jwt <token>` flag

```bash
andamio user login --jwt "$CI_JWT_TOKEN"
```

**Pros:** Explicit — user intent is clear.
**Cons:** Persists the JWT to disk (not appropriate for ephemeral CI environments). Requires an extra command to invoke.
**Effort:** Small
**Risk:** Low

### Option C: Document the limitation only

Add to `user login --help`: "For CI/CD environments, see..."

**Pros:** Zero code change.
**Cons:** Leaves the gap open.
**Effort:** Trivial
**Risk:** None

## Recommended Action

Option A. The env var pattern is the industry standard and requires minimal code. Can be deferred from PR #15 but should be tracked as a near-term follow-up.

## Technical Details

- **Affected files**: `internal/config/config.go`
- **PR**: Related to #15 exit-code work

## Acceptance Criteria

- [ ] `ANDAMIO_JWT=<token> andamio project task list <id>` works without prior `user login`
- [ ] Env var JWT takes precedence over stored config JWT
- [ ] `ANDAMIO_JWT` is documented in `--help` output for auth-required commands

## Work Log

- 2026-03-18: Flagged by agent-native reviewer during PR #15 review. P3 — not blocking merge of #15 but high-value follow-up.
