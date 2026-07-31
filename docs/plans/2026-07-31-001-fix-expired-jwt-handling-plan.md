---
title: "fix: Expired user JWT rides along on every request and bricks the CLI"
type: fix
status: completed
date: 2026-07-31
origin: https://github.com/Andamio-Platform/andamio-cli/issues/134
---

# fix: Expired user JWT rides along on every request and bricks the CLI

## Summary

Decode the stored JWT's `exp` claim locally (no signature verification) and act on it before any request leaves the machine: drop a known-expired user JWT from outgoing requests (with a single stderr warning), fail fast with exit 3 / `kind: auth` on JWT-required commands, let `user login` re-authenticate without a manual logout, and persist/report expiry truthfully for headless logins. Ships on `feat/cli-1-0-release-scope` before the 1.0 tag.

---

## Problem Frame

Found during the #133 preprod dry run. `internal/client` attaches `Authorization: Bearer <user_jwt>` unconditionally whenever the config holds a JWT. The gateway fails closed on any invalid credential, so an expired JWT poisons every request — including endpoints where the API key alone suffices and the headless login-session endpoint itself, making re-login require a manual `user logout`. Separately, headless login never records expiry (`cfg.JWTExpiresAt = ""`), so `user status` reports "active (no expiry info)" for a token that died months ago. Full reproduction: issue #134 and the #133 dry-run report.

---

## Assumptions

*This plan was authored in pipeline mode without synchronous user confirmation. The items below are agent inferences — review before or during implementation.*

- Clock-skew handling is conservative: a token is treated as expired starting at `exp - 30s` (never send a token the gateway might reject; costs at most a 30s-early fail-fast).
- Undecodable tokens (not three dot-separated segments, bad base64, missing/non-numeric `exp`) are sent as-is, silently — fail open; the gateway remains the authority. This also keeps existing test fixtures (`"test-jwt"`) working.
- The one-line expiry warning is emitted on stderr in **all** output modes, matching the `dev keys create` WARNING precedent (scripts pipe `2>/dev/null`).
- The behavior flip on either-auth endpoints (expired JWT: previously exit 3, now exit 0 with API-key-only data) is accepted and documented in the CHANGELOG as a behavior change.
- The fix targets the user-JWT slot. Expired **dev** JWTs get a fail-fast with a `dev refresh` hint in `devKeysClient` (before slot promotion), never a silent drop — per the dual-credential learning doc.
- CHANGELOG entries go under the existing `## [1.0.0]` heading (the tag has not been cut; #133 blocks it), not a new Unreleased section.

---

## Requirements

- R1. A request whose user JWT is locally known-expired must not carry that JWT; the API key (when present) still rides. Either-auth commands succeed on API key alone.
- R2. JWT-required commands with an expired stored JWT fail fast client-side with exit 3 / `kind: auth` and a message naming the expiry time and the recovery command — no network round trip.
- R3. `user login` (both browser and headless flows) works while an expired JWT is stored — no manual `user logout` needed first.
- R4. Headless login persists the JWT expiry (decoded from the token) so `user status` can report EXPIRED; `user status` falls back to decoding `exp` when the stored expiry field is empty (pre-fix configs, `ANDAMIO_JWT`).
- R5. Dev-portal dual-credential surfaces never have their promoted dev JWT silently stripped; an expired dev JWT produces exit 3 with a `dev refresh` hint.
- R6. Env-sourced JWTs (`ANDAMIO_JWT`) get accurate messaging: the recovery hint says to update/unset the env var, not `user login`.
- R7. No config mutation side effects: dropping a JWT from a request must never cause a later `config.Save` to erase the stored JWT from disk.
- R8. Exit-code/kind contract unchanged: exit 3 = auth (existing meaning), all other codes untouched.

---

## Scope Boundaries

- No gateway changes — the gateway's fail-closed behavior on invalid credentials is correct and stays.
- No signature verification, issuer validation, or JWT library dependency — decode of the payload's `exp` claim only.
- No auto-clearing of the expired JWT from config (explicit state; warnings are idempotent).
- No change to the dev-slot refresh flow (`dev refresh` machinery) beyond the fail-fast gate in `devKeysClient`.
- No change to `session_remaining_seconds`'s `omitempty` in the `user status` JSON envelope (divergence from the dev-status convention noted, deferred).

### Deferred to Follow-Up Work

- Mid-command expiry in `tx run` (JWT valid at prerun but expiring before the register/poll steps; can orphan a submitted tx from tracking). A "remaining validity < timeout" stderr warning is the likely shape — separate issue after 1.0.
- `course import`'s `uploadImage` raw-HTTP path (`cmd/andamio/course_import.go` `uploadImage`) bypasses `internal/client` and sends `cfg.UserJWT` directly. Entry to import is covered by the U3 fail-fast; mid-command expiry there is the same follow-up as above.
- The root-chaining oddity in four bespoke PreRunEs (`course export/import/import-all/create-module` do not chain `rootCmd.PersistentPreRunE`, a latent `--output` handling issue) will not be fixed here — noted for a follow-up. All 7 bespoke-PreRunE files are still modified by U3's sweep; only their chaining behavior is left untouched.

---

## Context & Research

### Relevant Code and Patterns

- `internal/client/client.go` — `New` snapshots `cfg.UserJWT` into a private field (`setHeaders` attaches it). 48 production call sites across 24 files in `cmd/andamio` make the client constructor the right choke point; a call-site sweep is impractical.
- `cmd/andamio/helpers.go` — `jwtAuthPreRunE` (root chain + `HasUserAuth` check) gates 7 command groups; 7 more commands hand-roll the same check (`course_export.go`, `course_import.go`, `course_import_all.go`, `course_create_module.go`, `tx_build.go`, `tx_register.go`, `tx_run.go`).
- `cmd/andamio/user.go` — browser flow persists callback `expires_at`; headless flow sets `cfg.JWTExpiresAt = ""` with a comment anticipating this fix; browser `runUserLogin` refuses to run when `HasUserAuth()` is true (the other half of the re-login trap).
- `cmd/andamio/dev.go` — `parseExpiry` / `timeUntil` / `printExpiryLine` are the house RFC3339-expiry primitives; `devStatusResult` is the scriptable-expiry envelope model.
- `cmd/andamio/dev_keys.go` — `devKeysClient` clones cfg and promotes `DevJWT` into the `UserJWT` slot (dual-credential surfaces).
- `internal/config/config.go` — env snapshot (`ANDAMIO_JWT` et al.): `Save` strips fields still matching the Load-time env value; mutating the loaded cfg is hazardous (a `dev refresh` Save could persist a cleared user JWT).
- Test conventions: `cmd/andamio/exitcode_test.go` (`buildTestBinary` + temp-HOME config + `statusStub`), `dev_test.go` (`captureStderr`, flow-function unit tests against `httptest`), `internal/config/config_test.go` (`t.Setenv` HOME + env-credential persistence assertions). No test-JWT builder exists; the exitcode fixtures use non-decodable `"user_jwt": "test-jwt"`.

### Institutional Learnings

- `docs/solutions/integration-issues/cli-dev-portal-dual-credential-pattern.md` — two shipped 401 regressions came from stripping one credential on dual-credential surfaces. "Never strip the JWT" there; distinct recovery hints per credential; both-headers-on-wire regression tests are the tripwire pattern.
- `docs/solutions/architecture/cli-composability-audit-and-fix.md` — exit 3 covers local auth-guard failures, not just HTTP 401/403; return `*apierr.AuthError` so `main.go` dispatch lands exit 3 with no contract change; `SessionExpired *bool` tristate pattern; env override applied at `Load` (expiry logic must run after it).
- `docs/solutions/integration-issues/cli-apikey-auth-isolation-and-content-404-ux.md` — copy-the-config-before-mutating rule; JWT-conditional endpoint selection (`HasUserAuth()` picks teacher vs user endpoints) will interact with expiry semantics — test deliberately.
- `docs/solutions/logic-errors/fix-three-cli-issues-hex-encoding-lesson-merge-headless-login.md` — headless login's documented design always intended expiry to be stored; this fix restores that contract.
- `docs/solutions/integration-issues/gateway-status-code-drift-409-vs-400.md` — keep the local check conservative so a false-positive drop never strips a token the gateway would accept.

### External References

- None needed — stdlib `encoding/base64` + `encoding/json` decode of the JWT payload.

---

## Key Technical Decisions

- **Drop lives in `client.New`, not `config.Load` or per call site**: the constructor already snapshots the JWT into a private field, so blanking it there can never leak into a `config.Save` (R7), and all 48 call sites are covered at once — including the headless login-session request and `dev login`/`dev refresh` clients that currently carry a stale *user* JWT.
- **Slot-awareness by sequencing, not by flag**: `devKeysClient` gains a fail-fast on expired dev JWT *before* promoting it into the shared slot (R5). A JWT that reaches `client.New` via promotion is therefore known-fresh, and the client-level drop with its `user login`-flavored warning can only ever fire on genuine user JWTs. No new client API needed.
- **Expiry primitive in `internal/config`**: a package-level `TokenExpiry(token) (time.Time, bool)` plus `Config` accessors (`UserJWTExpired`, `UserJWTFromEnv`, `DevJWTExpired`). Config owns the JWT slots and both `cmd/andamio` and `internal/client` already import it; no new package, no import cycle.
- **Conservative skew**: expired means `now >= exp - 30s`. The bug being fixed is "we sent a token the gateway rejects"; the skew direction must never reopen that window.
- **Fail open on undecodable tokens**: send as-is, no warning. The gateway is the authority; a decoder bug must not lock users out, and test fixtures stay valid.
- **Warning channel**: one line, stderr, all output modes, once per process (`sync.Once`). Matches the `dev keys create` warning precedent and the "warnings go to stderr regardless of output mode" learning.
- **Shared fail-fast helper**: extract the auth check used by `jwtAuthPreRunE` into a helper that also checks expiry, and sweep the 7 bespoke PreRunEs onto it — without changing their root-chaining behavior (kept out of scope).
- **Browser login guard becomes expiry-aware**: `runUserLogin`'s "already authenticated" refusal treats an expired JWT as unauthenticated and proceeds (R3) — this guard, not the login endpoint, is what blocks browser users today.

---

## Open Questions

### Resolved During Planning

- Where does the drop live relative to `devKeysClient`'s slot promotion? — In `client.New`, with `devKeysClient` gating expired dev JWTs *before* promotion (see Key Technical Decisions).
- Skew direction? — Expire early (`exp - 30s`); never send a token the gateway might reject.
- Undecodable/missing-`exp` tokens? — Send as-is, silent, fail open.
- Warning in `--output json` mode? — Yes, stderr, matching `dev keys create`; pinned by a test.
- Does fail-fast extend beyond `jwtAuthPreRunE`? — Yes: shared helper swept across the 7 bespoke PreRunEs.
- Where do CHANGELOG entries go? — Under `## [1.0.0]` (tag not yet cut).

### Deferred to Implementation

- Exact wording of the two warning variants (stored vs env-sourced JWT) — settle in code review; must name the concrete recovery action.
- Whether `user status` text mode prints the decoded-fallback expiry with a "(from token)" qualifier — cosmetic, decide at implementation.

---

## Implementation Units

### U1. JWT expiry primitive in `internal/config`

**Goal:** One decoding primitive the whole codebase uses to answer "is this token expired?"

**Requirements:** R1, R2, R4, R5, R6 (foundation)

**Dependencies:** None

**Files:**
- Create: `internal/config/jwt.go`
- Test: `internal/config/jwt_test.go`

**Approach:**
- `TokenExpiry(token string) (time.Time, bool)`: split on dots, base64url-decode the middle segment (`RawURLEncoding`, tolerate padded), JSON-decode `{"exp": <number>}`. `ok=false` for anything undecodable or missing `exp` — "no expiry knowable", never "expired".
- `TokenExpired(token string, now time.Time) bool`: true only when expiry is knowable and `now >= exp - 30s` (skew constant defined here, documented).
- `Config` accessors: `UserJWTExpired(now)`, `DevJWTExpired(now)`, `UserJWTFromEnv() bool` (predicate over the existing unexported env snapshot), and `HasFreshUserAuth(now) bool` (`HasUserAuth() && !UserJWTExpired(now)`) for endpoint-routing decisions (see U8).
- Also a small exported test helper is NOT added here — tests build tokens inline (see test scenarios); production code never constructs JWTs.

**Patterns to follow:**
- `parseExpiry` in `cmd/andamio/dev.go` for the "ok=false means absent, not expired" tristate philosophy.

**Test scenarios:**
- Happy path: token with future `exp` → expiry returned, not expired.
- Happy path: token with past `exp` → expired.
- Edge case: `exp` exactly `now + 30s` boundary → expired (skew); `now + 31s` → not expired.
- Edge case: non-JWT string (`"test-jwt"`), empty string, two segments, four segments → `ok=false`, not expired.
- Edge case: valid base64 payload without `exp`; `exp` as string; `exp` non-numeric; `exp` absurdly large (no overflow panic) → `ok=false` or sane handling, never expired=true.
- Edge case: base64 payload with padding vs raw-url encoding → both decode.
- Happy path: `UserJWTFromEnv` true only when `ANDAMIO_JWT` supplied the value at Load (reuse `t.Setenv` pattern from `internal/config/config_test.go`).

**Verification:**
- `go test ./internal/config` passes; no production behavior change yet.

---

### U2. Drop expired user JWT at `client.New` + single stderr warning

**Goal:** No request ever carries a locally-known-expired user-slot JWT; users get one clear warning per process.

**Requirements:** R1, R3 (login-session leg), R6, R7

**Dependencies:** U1

**Files:**
- Modify: `internal/client/client.go`
- Test: `internal/client/client_test.go`

**Approach:**
- In `New`, when `cfg.UserJWT` is non-empty and `config.TokenExpired(cfg.UserJWT, time.Now())`, leave the client's `userJWT` field empty and emit the warning. The config struct is never mutated (R7).
- Warning: one line to `os.Stderr`, all output modes, deduped once per process. The dedup guard must be a **resettable package-level `var warnOnce sync.Once`** (not an inline once) with an unexported test reset — `go test` runs a package's tests in one process, and U2's three warning-asserting scenarios would otherwise be order-dependent. Two variants: stored JWT → "run 'andamio user login'"; env-sourced (`cfg.UserJWTFromEnv()`) → "update or unset ANDAMIO_JWT".
- `internal/client` already writes nothing to stdout and must stay free of `internal/output`; plain `fmt.Fprintln(os.Stderr, ...)` is fine (no new dependency).

**Patterns to follow:**
- `dev keys create` warning (`cmd/andamio/dev_keys.go`): stderr in both modes.
- Existing `client_test.go`/`httptest` style if present; otherwise `dev_test.go`'s `captureStderr`.

**Test scenarios:**
- Happy path: expired JWT + API key → outgoing request has `X-API-Key`, no `Authorization` header (assert via `httptest` echo server).
- Happy path: valid JWT → `Authorization` present, no warning.
- Edge case: undecodable token (`"test-jwt"`) → sent as-is, no warning (pins the fail-open contract and keeps `exitcode_test.go` fixtures working).
- Edge case: two `client.New` calls in one process → warning printed once. (Warning-asserting tests reset the package-level `warnOnce` guard in setup so scenarios stay order-independent — `client_test.go` is in-package.)
- Error path: expired env-sourced JWT → warning names `ANDAMIO_JWT`.
- Integration: JSON output mode — stdout contains only the JSON envelope; warning is on stderr (composability rule).

**Verification:**
- With an expired JWT in a temp-HOME config and a valid API key, an either-auth command exits 0 against a stub gateway; previously exit 3.

---

### U3. Fail-fast on JWT-required commands (shared helper + sweep)

**Goal:** Every JWT-required command rejects an expired session client-side with exit 3, a timestamp, and a recovery hint — no network round trip.

**Requirements:** R2, R6, R8

**Dependencies:** U1

**Files:**
- Modify: `cmd/andamio/helpers.go` (extract shared check into `requireUserAuth(cfg)`-style helper; `jwtAuthPreRunE` uses it)
- Modify: `cmd/andamio/course_export.go`, `cmd/andamio/course_import.go`, `cmd/andamio/course_import_all.go`, `cmd/andamio/course_create_module.go`, `cmd/andamio/tx_build.go`, `cmd/andamio/tx_register.go`, `cmd/andamio/tx_run.go` (bespoke PreRunEs call the shared helper; root-chaining behavior left exactly as it is per file)
- Test: `cmd/andamio/exitcode_test.go` (or a sibling test file following its pattern)

**Approach:**
- Helper returns `&apierr.AuthError{Message: ...}` (hand-built, HTTPStatus 0) so `apierr.Kind` lands exit 3 / `kind: auth` with zero contract change.
- Message includes the expiry timestamp (RFC1123 local, matching `printExpiryLine` style) and the correct recovery action (stored vs env-sourced).
- Missing-JWT message stays exactly as today ("not authenticated. Run 'andamio user login' first") — only the expired case is new.

**Patterns to follow:**
- `jwtAuthPreRunE` in `cmd/andamio/helpers.go`; exit-code table tests in `cmd/andamio/exitcode_test.go` (`buildTestBinary`, temp HOME, crafted config).

**Test scenarios:**
- Happy path: expired (properly-encoded) JWT in config + `course owner list` → exit 3, JSON `kind: auth`, message contains expiry time and `user login`; no HTTP request reaches the stub server.
- Happy path: valid JWT → command proceeds to the network as before.
- Edge case: undecodable JWT (`"test-jwt"`) → NOT failed fast; behaves exactly as today (fail open).
- Error path: each of the 7 bespoke-PreRunE commands with an expired JWT → exit 3 with the same message (table-driven over command argv).
- Integration: `--output json` emits the error envelope on stdout (`{"error": ..., "kind": "auth"}`), nothing else.

**Verification:**
- Table test proves exit-code parity across `jwtAuthPreRunE` commands and all 7 bespoke ones.

---

### U4. Dev-slot guard: fail fast before promotion, never strip

**Goal:** Dual-credential dev-portal surfaces keep both headers on the wire; an expired dev JWT yields exit 3 with a `dev refresh` hint instead of a silent drop or a generic gateway 401.

**Requirements:** R5, R8

**Dependencies:** U1, U2

**Files:**
- Modify: `cmd/andamio/dev_keys.go` (`devKeysClient`)
- Test: `cmd/andamio/dev_keys_test.go`

**Approach:**
- Before promoting `DevJWT` into the `UserJWT` slot, check `cfg.DevJWTExpired(now)`; if expired, return `&apierr.AuthError{Message: "developer session expired at <t>. Run 'andamio dev refresh' (or 'andamio dev login')"}`.
- Undecodable dev JWTs pass through (fail open) — same contract as U1.
- Because promotion only happens with a fresh token, U2's client-level drop can never strip a dev JWT (sequencing guarantees slot-correct messaging).

**Patterns to follow:**
- `docs/solutions/integration-issues/cli-dev-portal-dual-credential-pattern.md` — both-headers-on-wire regression tests (`TestRunAPIKeyJSON_SendsDualCredential` tripwire style).

**Test scenarios:**
- Happy path: valid dev JWT + API key → both `X-API-Key` and `Authorization: Bearer <devJWT>` on the wire (regression tripwire retained/extended).
- Error path: expired dev JWT → exit-3 AuthError naming `dev refresh`; no HTTP request issued.
- Edge case: expired *user* JWT + valid dev JWT → dev-portal command works; user-slot state is irrelevant to the promoted client.
- Edge case: undecodable dev JWT → request goes out with both headers (today's behavior).

**Verification:**
- `dev keys list` and `apikey usage` paths (both route through `devKeysClient`) behave per scenarios; existing dual-credential tests still green.

---

### U5. Login flows: re-login without logout; headless persists expiry

**Goal:** `user login` works over an expired session in both flows, and headless login records truthful expiry.

**Requirements:** R3, R4

**Dependencies:** U1 (U2 already fixes the login-session request leg)

**Files:**
- Modify: `cmd/andamio/user.go`
- Test: `cmd/andamio/user_test.go` (create if absent, following `dev_test.go`'s flow-function style)

**Approach:**
- Browser flow: the "already authenticated" guard treats an expired stored JWT as unauthenticated and proceeds to login.
- Headless flow: build the login-session client from a **copy of cfg with `UserJWT` blanked** (mirroring `devKeysClient`'s clone pattern) rather than relying on U2's expiry-conditional drop. Login never needs prior user auth; this keeps R3 true even for tokens the gateway rejects but the CLI cannot decode (truncated paste, corruption), and it suppresses the misleading "session expired — run 'andamio user login'" warning while the user is literally running `user login`. U2's drop remains as defense-in-depth.
- Headless flow: replace `cfg.JWTExpiresAt = ""` with the RFC3339 rendering of `config.TokenExpiry(jwt)` when decodable (empty when not); add `expires_at` to the headless JSON success envelope (mirrors `devSessionResult`).
- Browser flow fallback: when the callback's `expires_at` query param is empty, fall back to the decoded `exp`. Callback-provided value keeps precedence when present.

**Patterns to follow:**
- `runDevHeadlessLogin` in `cmd/andamio/dev.go` (persists expiry + emits it in the envelope); `devTestEnv` temp-HOME pattern.

**Test scenarios:**
- Happy path: headless login against a stub gateway returning a crafted JWT → config on disk has `jwt_expires_at` matching the token's `exp`; JSON envelope includes `expires_at`.
- Edge case: gateway returns an undecodable JWT → `jwt_expires_at` empty, login still succeeds (today's behavior).
- Happy path: browser-flow guard — expired JWT stored → login proceeds (guard does not refuse); valid JWT stored → guard refuses as today.
- Integration: headless login with an expired old JWT in config succeeds end-to-end against the stub (proves the U2 leg + this unit together kill the logout-first trap).

**Verification:**
- Full re-login flow over an expired session requires zero manual `user logout` in both flows.

---

### U6. `user status` decoded-exp fallback

**Goal:** `user status` reports truthful expiry even for pre-fix configs and env-sourced JWTs.

**Requirements:** R4

**Dependencies:** U1

**Files:**
- Modify: `cmd/andamio/user.go` (`runUserStatus`)
- Test: `cmd/andamio/user_test.go`

**Approach:**
- When `cfg.JWTExpiresAt` is empty and `cfg.UserJWT` decodes, use the decoded expiry for both text and JSON rendering (same `SessionExpiresAt` / `SessionExpired` / `SessionRemainingSeconds` fields — additive: fields that were previously absent now appear when derivable).
- Stored `JWTExpiresAt` keeps precedence when non-empty.
- `session_expired` is computed with the **same `config.TokenExpired` predicate (30s skew included)** that U2/U3 enforce — probe and enforcement must agree, or probe-then-act scripts flake inside the skew window.
- Update the EXPIRED-branch hint (currently "Run 'andamio user logout && andamio user login' to re-authenticate.") to "Run 'andamio user login' to re-authenticate." — R3 makes the logout step obsolete; env-sourced variant per R6.
- Do not change `session_remaining_seconds`'s `omitempty` (out of scope).

**Patterns to follow:**
- Existing `runUserStatus` branches; `printExpiryLine` for EXPIRED text rendering style.

**Test scenarios:**
- Happy path: config with JWT but empty `jwt_expires_at`, token `exp` in the past → text shows `Session: EXPIRED (...)`; JSON has `session_expired: true`.
- Happy path: token `exp` in the future → remaining time shown; JSON `session_expired: false`.
- Edge case: undecodable JWT, empty stored expiry → today's "active (no expiry info)" preserved; JSON omits expiry fields.
- Edge case: stored `jwt_expires_at` present and disagreeing with token `exp` → stored value wins (precedence pinned).

**Verification:**
- `user status` after a (simulated) headless login from a pre-fix config correctly shows EXPIRED for a dead token.

---

### U7. Documentation: CHANGELOG + CLAUDE.md

**Goal:** The behavior change is discoverable by script authors and future maintainers.

**Requirements:** R1, R2 (documentation of), R8

**Dependencies:** U2, U3 (final behavior settled)

**Files:**
- Modify: `CHANGELOG.md` (under `## [1.0.0]`: Fixed — expired-JWT poisoning incl. the re-login trap; Changed — behavior note that either-auth commands with an expired JWT now succeed on API key alone, exit 3 → 0 flip, and the new stderr warning; `user status` additive JSON fields, naming `session_expired` — not `user_authenticated`, which stays expiry-blind — as the canonical session-liveness field for scripts)
- Modify: `CLAUDE.md` (Auth Flow section: local `exp` decode, drop-and-warn, fail-fast, dev-slot sequencing guarantee, fail-open contract for undecodable tokens)

**Test expectation:** none — documentation only.

**Verification:**
- CHANGELOG explicitly flags the exit-code flip on either-auth endpoints as a behavior change per the file's own stated convention.

---

### U8. Freshness-aware endpoint routing in either-auth course commands

**Goal:** Either-auth course read commands actually succeed on API key alone when the JWT is expired — closing the gap where R1 would otherwise fail for exactly the commands the #133 dry run exercises.

**Requirements:** R1

**Dependencies:** U1, U2

**Files:**
- Modify: `cmd/andamio/course.go` (the five `HasUserAuth()` endpoint-selection branches: `runCourseModules`, `runCourseSlts`, and the lesson/intro/assignment teacher-endpoint preferences)
- Test: `cmd/andamio/course_test.go` (create if absent) or extend `cmd/andamio/exitcode_test.go`

**Approach:**
- Without this unit, an expired JWT + valid API key still exits 3 on `course modules`/`slts`/`lesson`/`intro`/`assignment`: `HasUserAuth()` (presence-only) routes them to teacher endpoints, U2 drops the JWT, and the gateway 401 propagates — the lesson/intro/assignment fallbacks catch only `NotFoundError`, not `AuthError`.
- Switch these routing decisions to `cfg.HasFreshUserAuth(now)` (U1) so a known-expired JWT routes to the user endpoints, where the API key suffices.
- Undecodable tokens keep today's routing (fail open — `HasFreshUserAuth` treats unknown expiry as fresh).

**Patterns to follow:**
- The existing `HasUserAuth()` branches themselves; `getJSONWithHint` fallback structure stays untouched.

**Test scenarios:**
- Happy path: expired JWT + API key → `course modules <id>` exits 0 via the user endpoint (stub gateway asserts which path was requested).
- Happy path: valid JWT → teacher endpoint still preferred (no routing regression).
- Edge case: undecodable JWT → teacher endpoint preferred (today's behavior pinned).

**Verification:**
- The planned CHANGELOG claim "either-auth commands with an expired JWT succeed on API key alone" is true for the five course read commands, not just `getJSON`-style ones.

---

## System-Wide Impact

- **Interaction graph:** All 48 `client.New` call sites inherit the drop (U2) with zero call-site edits; the 7 bespoke PreRunEs are the only swept surfaces (U3). `devKeysClient` consumers (`dev keys *`, `apikey usage/profile`) get the U4 gate.
- **Error propagation:** New failures are `*apierr.AuthError` → existing `apierr.Kind` mapper → exit 3 / `kind: auth`. No new kinds, no exit-code table change.
- **State lifecycle risks:** The drop never mutates `Config` (R7), so `Save` paths in `dev refresh` / `dev logout` / login flows cannot erase the user JWT. Expired JWT deliberately stays in config.
- **API surface parity:** JWT-conditional endpoint selection in `cmd/andamio/course.go` (teacher-vs-user endpoints keyed on `HasUserAuth()`) is made freshness-aware by U8 so expired-JWT + API-key requests route to user endpoints and succeed. Ungated commands outside those five branches fall back to the gateway's 401 (unchanged).
- **Integration coverage:** The end-to-end "expired session → headless re-login without logout" test (U5) is the scenario unit tests alone won't prove.
- **Unchanged invariants:** Exit codes 0–6 and their kinds; `--output json` envelope shapes except additive `user status` fields and the new headless-login `expires_at`; dual-credential both-headers-on-wire contract; fail-open on undecodable tokens preserves all existing test fixtures.

---

## Risks & Dependencies

| Risk | Mitigation |
|------|------------|
| Local clock far behind gateway → we send a token the gateway rejects (401 recurs) | 30s early-expiry skew; conservative direction chosen deliberately |
| Local clock far ahead → we drop/fail a token the gateway would accept | Bounded by clock error; warning names the perceived expiry time so the operator can spot clock issues; fail-open on undecodable tokens limits blast radius |
| Silent strip of a promoted dev JWT (regression class from 0.12.x) | U4 gates before promotion; both-headers-on-wire regression test retained; drop can only fire on genuine user JWTs |
| Scripts using either-auth commands as session-liveness probes see exit 3 → 0 flip | CHANGELOG behavior-change callout (U7); `user status --output json`'s `session_expired` field is the supported liveness probe, computed with the same skew-inclusive predicate as enforcement (U6) |
| `exitcode_test.go` fixtures (`"test-jwt"`) start failing | Fail-open contract (U1/U2) pinned by explicit tests |

---

## Documentation / Operational Notes

- Ships on `feat/cli-1-0-release-scope` (PR #130) before the 1.0 tag; #133's dry-run report documents the field failure this fixes.
- No rollout/monitoring concerns — client-side only.

---

## Sources & References

- **Origin document:** [issue #134](https://github.com/Andamio-Platform/andamio-cli/issues/134)
- Related: [issue #133 dry-run report](https://github.com/Andamio-Platform/andamio-cli/issues/133#issuecomment-5142208260) (field reproduction)
- Related code: `internal/client/client.go` (`New`, `setHeaders`), `cmd/andamio/helpers.go` (`jwtAuthPreRunE`), `cmd/andamio/user.go` (login flows, `runUserStatus`), `cmd/andamio/dev_keys.go` (`devKeysClient`), `internal/config/config.go` (env snapshot, `Save` stripping)
- Related PRs/issues: #130 (1.0 scope), #133 (dry run), #128 (tag gate)
