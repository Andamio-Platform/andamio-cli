---
title: "Dev-portal gateway surfaces are dual-credential — never strip the JWT"
date: 2026-05-20
problem_type: integration-issues
module: auth, http client, dev-portal commands
symptoms:
  - 401 "Authorization header with Developer JWT required" against /api/v2/apikey/developer/*
  - 401 against /api/v2/keys when only one of (X-API-Key, dev JWT) is on the wire
  - CLI commands that previously worked stop working after a gateway middleware migration
  - Single-credential isolation helper (strip-the-JWT pattern) is reused for a new dev-portal surface and breaks on first contact
root_cause: "Gateway dev-portal surfaces sit behind dual middleware (V2AuthMiddleware requires X-API-Key, developerJWTAuth requires Bearer <devJWT>). CLI helpers that strip either credential — or fail to add the dev JWT — produce a 401 from whichever middleware is missing its header. The strip-the-JWT pattern was correct for the OLD gateway behavior on /v2/apikey/developer/* (which rejected JWTs) but is wrong now (the gateway flipped to requiring the dev JWT)."
severity: high
tags:
  - auth
  - gateway-middleware
  - dev-portal
  - dual-credential
  - recurrence
  - compounding
---

# Dev-portal gateway surfaces are dual-credential — never strip the JWT

## Problem statement

Two separate CLI commands shipped a 401 to users in successive releases because of the **same root cause**:

| Release | Commands | Symptom | PR |
|---------|----------|---------|----|
| 0.12.1 (2026-05-07) | `dev keys list\|create\|delete` | 401 against `/api/v2/keys` — clone cleared `APIKey`, sent dev JWT only | #89 |
| 0.12.2 (2026-05-20, this PR) | `apikey usage\|profile` | 401 "Authorization header with Developer JWT required" against `/api/v2/apikey/developer/*` — `getAPIKeyJSON` stripped the JWT, sent `X-API-Key` only | #96 |

In both cases the gateway moved the surface behind a **dual** middleware stack:

- `V2AuthMiddleware` validates `X-API-Key` for app-level auth, billing, and rate limits.
- `developerJWTAuth` validates `Authorization: Bearer <devJWT>` for the developer's identity.

Both must be on the wire. Sending only one returns 401 from whichever middleware is missing its header. The CLI's instinct each time was to reuse a single-credential isolation helper (a "clear the JWT before sending" or "clear the API key before sending" clone), which guaranteed the request would fail the missing-middleware check.

The second occurrence happened because the first had no solution doc — the CHANGELOG entry for 0.12.1 described the fix but the pattern was not in `docs/solutions/` where the next engineer (and future agents) would find it. This document is that record.

## Confirmed dual-credential surfaces

As of 2026-05-20, the following gateway surfaces require BOTH `X-API-Key` and `Authorization: Bearer <devJWT>`:

| Path | Middleware stack | CLI consumer |
|------|------------------|--------------|
| `/api/v2/keys` (GET, POST) | `V2AuthMiddleware` + `developerJWTAuth` | `cmd/andamio/dev_keys.go` |
| `/api/v2/keys/{id}` (DELETE) | `V2AuthMiddleware` + `developerJWTAuth` | `cmd/andamio/dev_keys.go` |
| `/api/v2/apikey/developer/profile/get` | `V2AuthMiddleware` + `developerJWTAuth` | `cmd/andamio/apikey.go` |
| `/api/v2/apikey/developer/usage/get` | `V2AuthMiddleware` + `developerJWTAuth` | `cmd/andamio/apikey.go` |
| `/api/v2/apikey/developer/key/{request,delete,rotate}` | `V2AuthMiddleware` + `developerJWTAuth` (+ verified email on some) | not yet exposed in CLI |
| `/api/v2/apikey/developer/account/delete` | `V2AuthMiddleware` + `developerJWTAuth` | not yet exposed in CLI |
| `/api/v2/auth/developer/{resend-verification,email-status}` | `V2AuthMiddleware` + `developerJWTAuth` | not yet exposed in CLI |
| `/api/v2/billing/{checkout,portal,status}` | `V2AuthMiddleware` + `developerJWTAuth` | not yet exposed in CLI |

Source of truth: `andamio-api/internal/router/main_router.go` (search for `developerJWTAuth` uses on `v2Group.Group(...)`).

**Any new endpoint added under these paths inherits the dual-credential requirement.** When wiring a new CLI command for one of them, route through `devKeysClient`.

## The rule

> **For any dev-portal gateway surface, use `cmd/andamio/dev_keys.go:devKeysClient(cfg)` as the routing helper. Never strip the JWT. Never clear the API key. Both headers must ride on every request.**

`devKeysClient` is the shared routing primitive:

```go
// devKeysClient clones cfg, preserves APIKey, and promotes DevJWT into
// the UserJWT slot so client.setHeaders emits BOTH X-API-Key and
// Authorization: Bearer <devJWT>. The wallet/user JWT is overwritten,
// not appended.
func devKeysClient(cfg *config.Config) (*client.Client, error) { ... }
```

Both `dev keys *` and `apikey usage`/`profile` now route through it. A new dev-portal command (e.g., `dev keys rotate`, `dev account delete`, `dev billing status`) should do the same. The package-level comment block above `devKeysClient` in `dev_keys.go` is the architectural anchor — read it before adding a new caller.

## Pre-merge checklist for new dev-portal CLI commands

Before shipping a CLI command that hits a path under `/api/v2/keys`, `/api/v2/apikey/developer/*`, `/api/v2/auth/developer/*`, or `/api/v2/billing/*`:

- [ ] Route through `devKeysClient(cfg)`, not `client.New(cfg)` directly and not via any "clear the JWT" or "clear the API key" config-clone helper.
- [ ] Pre-check `cfg.APIKey == ""` and `cfg.HasDevAuth() == false` separately, with distinct `*apierr.AuthError` messages naming the specific missing command (`auth login --api-key` vs `dev login`). A user with one credential and not the other deserves an actionable, surface-specific hint — not a raw gateway 401.
- [ ] Update both auth tables (`CLAUDE.md` "Complete Command Reference" and `docs/andamio-cli-context.md`) to show `api-key + dev-jwt` in the Auth column.
- [ ] Add a regression test that asserts BOTH headers on the wire AND asserts the wallet/user JWT does NOT leak as a second `Authorization` value (the tripwire pattern in `apikey_test.go:TestRunAPIKeyJSON_SendsDualCredential`).
- [ ] Add a regression test for each AuthError branch (missing API key, missing dev JWT) — without these, the next gateway middleware migration silently degrades the UX.

## Why this is worth a separate doc

The strip-the-JWT pattern was correct at one point — it was the right fix for the gateway behavior at the time of `cli-apikey-auth-isolation-and-content-404-ux.md` (2026-03-18). The pattern then became wrong when the gateway flipped on the same surfaces (the v2 dev-portal middleware migration). A solution doc that describes a fix is bound to the gateway state when it was written; this doc describes the **invariant** ("dev-portal surfaces are dual-credential, route through `devKeysClient`") that survives the next migration.

If the gateway changes again — adds a third middleware, splits dual into separate-but-related credentials, etc. — this doc should be updated, not replaced. The recurrence pattern (two commands, two releases, same root cause, no doc) is the real failure mode this captures.

## Related

- `docs/solutions/integration-issues/cli-apikey-auth-isolation-and-content-404-ux.md` — Issue #17 portion superseded by this doc and PR #96. Issue #18 (course content 404 UX) still current.
- `CHANGELOG.md` [0.12.1] — the dev-keys dual-credential fix that should have produced this doc but didn't.
- `CHANGELOG.md` [Unreleased] — the apikey dual-credential fix that prompted it.
- `andamio-api/internal/router/main_router.go` — source of truth for which paths sit behind `developerJWTAuth`.
- `cmd/andamio/dev_keys.go` — `devKeysClient` and its package-level comment.
- GitHub PRs: [#89](https://github.com/Andamio-Platform/andamio-cli/pull/89) (dev keys 0.12.1), [#96](https://github.com/Andamio-Platform/andamio-cli/pull/96) (apikey fix).
