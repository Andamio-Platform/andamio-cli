---
title: "feat: browser-based dev login (no args) — mirror user login for the developer JWT slot"
type: feat
status: active
date: 2026-05-22
deepened: 2026-05-22
origin: https://github.com/Andamio-Platform/andamio-cli/issues/100
---

# feat: browser-based `dev login` (no args) — mirror user login for the developer JWT slot

## Overview

Today `andamio dev login` is headless-only — all three flags (`--skey`, `--alias`, `--address`) are `MarkFlagRequired`. The typical developer who claims their API key through the browser wallet at app.andamio.io has no `.skey` file on disk and cannot complete `dev login`, which makes the entire dev-portal CLI surface (`dev keys *`, `apikey usage`, `apikey profile`) unreachable for the persona the v0.13.0 fix was supposed to unblock.

This plan adds a browser-flow branch to `andamio dev login` (no args) that mirrors the existing `andamio user login` browser flow but writes the resulting JWT + 30-day refresh token + tier into the developer JWT config slot. The `--skey` headless path stays untouched for CI/CD and ops users.

The API endpoints already exist (`/v2/auth/developer/login/session` + `/v2/auth/developer/login/complete`, shipped in andamio-api #410). The App side lands separately as [andamio-app-v2#698](https://github.com/Andamio-Platform/andamio-app-v2/issues/698). This plan is the CLI half.

## Problem Frame

External developer (Andrew, 2026-05-19) hit a 401 on `apikey usage` he could not recover from. The chain of fixes:

1. PR #96 + v0.13.0 — fixed the dual-credential routing bug for users with a local `.skey` (CI/CD, ops, devkit). Real fix, ships value to those users.
2. **This plan** — closes the actual gap that blocked Andrew's persona by giving browser-wallet developers a way to complete `dev login` end to end.

The browser wallet (Eternl, Lace, Nami) holds the signing key inside encrypted extension storage by design — exporting a `.skey` is not a viable path for a typical developer journey. The CLI must support the same browser-wallet sign-in path it already supports for `user login`. (See origin: [#100](https://github.com/Andamio-Platform/andamio-cli/issues/100).)

## Requirements Trace

- **R1.** `andamio dev login` (no args, no flags) opens a browser, prompts wallet sign-in at `{appURL}/auth/dev-cli`, receives the dev JWT + 30-day refresh token + alias + tier via an ephemeral localhost callback, and persists them in the developer config slot.
- **R2.** `andamio dev login --skey <path> --alias <name> --address <bech32>` continues to work identically to today — no behavioral change to the headless path.
- **R3.** A user with only one of the three flags set (partial invocation) gets a clear error explaining the all-or-nothing rule for headless mode and the no-args alternative for browser mode.
- **R4.** `cfg.APIKey == ""` pre-check fires before the browser opens — empty API key returns `*apierr.AuthError` mentioning `auth login --api-key` (per the dual-credential learning).
- **R5.** CSRF state token is generated, sent in the redirect URL, and validated on callback. Mismatch returns an error and persists nothing.
- **R6.** Callback handler accepts only `GET` (405 otherwise), matches the existing user-login callback semantics.
- **R7.** Hard 5-minute timeout on the listener; matches the gateway's session window.
- **R8.** Browser-open failures are non-fatal — the URL is printed to stderr for manual opening; the listener keeps waiting.
- **R9.** All progress messages go to stderr and are gated on `!isJSON`. `--output json` success returns a structured envelope (alias, expiry timestamps, tier, dev_id, key_hash) — never the token bodies themselves.
- **R10.** Pre-flight `HasDevAuth()` check: if already authenticated, print a message and exit early without opening the browser. Mirror the user-login pattern.
- **R11.** Tokens never appear on stdout in any mode — pinned by a `SECRET.SHOULD-NOT-LEAK` test guard on both stdout AND stderr.
- **R12.** Relaxation of `MarkFlagRequired` on `--skey`/`--alias`/`--address` is a visible behavior change for scripts that relied on cobra's exit-1 flag-validation error; CHANGELOG must call it out under `### Changed` (not just `### Added`) so consumers can audit their automation before upgrading.

## Scope Boundaries

- The `andamio user login` browser flow is **not** modified or refactored.
- The headless `dev login --skey` path is **not** modified beyond removing the three `MarkFlagRequired` calls.
- No new fields are added to `internal/config.Config` — every dev-slot field already exists from the headless flow.
- No new andamio-api endpoints are added or modified.
- No CLI-side feature flag — gating behavior on the App route's existence is handled by sequencing the release after #698 ships.

### Deferred to Separate Tasks

- **App route `/auth/dev-cli`**: [andamio-app-v2#698](https://github.com/Andamio-Platform/andamio-app-v2/issues/698). Independent PR in a separate repo. End-to-end manual verification waits for both this CLI plan and #698 to land.
- **Characterization tests for the existing `runUserLogin` browser flow**: it has no coverage today (only `TestSanitizeCallbackValue` exists). Worth a separate plan; widening scope here would couple this PR to a refactor of a different command.
- **Auto-refresh on expired dev JWT**: tracked at [#97](https://github.com/Andamio-Platform/andamio-cli/issues/97). Separate fix in `devKeysClient`.

## Context & Research

### Relevant Code and Patterns

- `cmd/andamio/user.go` — `runUserLogin` is the structural reference. Browser path lives inline (no separately named function). Lines of interest: state generation (87–93), listener setup (138–145), `buildAuthURL` (622–633), callback handler (152–200), timeout block (224–237), persistence (244–258), pre-flight `HasUserAuth` guard (125–129).
- `cmd/andamio/dev.go` — `devLoginCmd` flag registration (133–138), `runDevLogin` dispatcher (141–156), `runDevHeadlessLogin` (205–329), `persistDevSession` (337–352). Both files are in `package main` — `generateState`, `buildAuthURL`, `sanitizeCallbackValue`, and the `authCallbackResult` struct are directly callable from `dev.go` without imports.
- `internal/config/config.go` — dev slot fields (27–34): `DevJWT`, `DevJWTExpiresAt`, `DevRefreshToken`, `DevRefreshTokenExpiresAt`, `DevAlias`, `DevID`, `DevTier`, `DevKeyHash`. Env-snapshot logic at lines 179–192 + Save() stripping at 265–277 — handled automatically by `Load`/mutate/`Save` cycle; new code does not need to touch it.
- `cmd/andamio/dev_test.go` — `devGatewayStub` (28–45), `devTestEnv` (80–98), `captureStdout` (977–1006), and the `SECRET.SHOULD-NOT-LEAK` pattern (in JSON output shape tests) are the model to follow for the new browser-flow tests.

### Institutional Learnings

- `docs/solutions/integration-issues/cli-dev-portal-dual-credential-pattern.md` — the just-compounded learning. Mandates: pre-check `cfg.APIKey == ""` and return `*apierr.AuthError` with the `auth login --api-key` hint before opening the browser. The dev JWT alone is useless against any subsequent dev-portal endpoint.
- `docs/solutions/security-issues/cli-security-hardening-input-validation.md` — established the GET-only callback handler (405 otherwise) and the loopback-allowlist exemption for `ValidateBaseURL`. Both apply unchanged to the new flow.
- `docs/solutions/architecture/cli-composability-audit-and-fix.md` — AuthError exit code 3, `--output json` contract, stderr discipline, ephemeral env-credential semantics. All apply.
- `docs/solutions/logic-errors/fix-three-cli-issues-hex-encoding-lesson-merge-headless-login.md` — the headless `user login --skey` doc; structural template for the JSON output shape (alias, expires_at, key_hash).

### External References

None used. The local pattern in `user.go` is strong (single direct reference, recently touched, in active use) and external research adds no value for a copy-adapt of a known shape.

## Key Technical Decisions

- **Single cobra command with runtime branching, not split subcommands.** Mirror `user login`'s pattern: relax the three `MarkFlagRequired` declarations, branch inside `runDevLogin`. Splitting into `dev login browser` / `dev login headless` would diverge from `user login` for no gain.
- **Branch discriminator: `cmd.Flags().Changed("skey")` etc., NOT empty-string equality.** A user passing `--skey ""` (from a shell variable that's unset) sets the flag to empty but `Changed()` still returns true. Using empty-string equality would misclassify this case as "no flags → browser mode" when the user's intent is clearly headless-with-an-empty-skey-path. Use `Changed()` for all three flags; the three-way matrix becomes (none-changed → browser) / (all-three-changed → headless) / (partial → error).
- **Callback transport: GET with query params, not POST/JSON.** The issue body (#100) proposed POST/JSON. The plan locks in GET+query to match the existing user-login pattern and reuse `sanitizeCallbackValue`. The 30-day refresh token transiting in a loopback URL with an ephemeral OS-assigned port is acceptable — the URL is never seen outside the browser process, the listener tears down within seconds, and there is no shared infrastructure that could log it. This decision must also be reflected in andamio-app-v2#698 — the App callback POSTs back via `GET /callback?…` not POST/JSON.
- **Generalize `buildAuthURL` with `path` as the second positional argument.** Today's signature is `buildAuthURL(baseURL, redirectURI, state string) string` (user.go:622). The new signature is `buildAuthURL(baseURL, path, redirectURI, state string) string` — `path` inserted at position 2. The single existing call site at user.go:210 becomes `buildAuthURL(cfg.BaseURL, "/auth/cli", redirectURI, state)`. New dev-login call: `buildAuthURL(cfg.BaseURL, "/auth/dev-cli", redirectURI, state)`. Putting `path` last would change the URL shape — do not.
- **Reuse `generateState` and `sanitizeCallbackValue` as-is.** Both are package-level and apply identically.
- **Pre-flight: `cfg.APIKey == ""` AND `cfg.HasDevAuth()`.** Two distinct guards, two distinct AuthError messages, two distinct hints. Order: API-key check first (the gateway's `V2AuthMiddleware` runs first too — mirror the gateway's enforcement order in the CLI's error surface).
- **HasDevAuth early-return message goes to STDERR, gated on `!isJSON`.** The user-login pattern at user.go:125–128 uses `fmt.Printf` / `fmt.Println` which write to stdout — that violates the `--output json` contract and the project's stderr-discipline rule. Do NOT mirror that detail; explicitly route the message to `fmt.Fprintf(os.Stderr, ...)` and gate on `!isJSON`. **In `--output json` mode the early-return contract is: exit 0, empty stdout, nothing on stderr either.** A script that needs to distinguish "freshly authenticated" from "already authenticated" should call `andamio dev status --output json` after — that command is the right surface for state inspection. Do NOT invent a synthetic `{"already_authenticated": true}` envelope just for this path; adding a new JSON shape for a no-op success creates an API surface without justification.
- **`persistDevSession` is the persistence path, with `fallbackAlias = ""` and a pre-call guard for empty alias.** `secureLoginResponse` uses anonymous nested structs (`JWT struct{Token, ExpiresAt string}`, `RefreshToken struct{Token, ExpiresAt string}`) — the implementer cannot use composite literal syntax across the nested boundary; field assignment is required. **Field mapping from callback param → `secureLoginResponse`:** `dev_jwt → resp.JWT.Token`, `dev_jwt_expires_at → resp.JWT.ExpiresAt`, `dev_refresh_token → resp.RefreshToken.Token`, `dev_refresh_token_expires_at → resp.RefreshToken.ExpiresAt`, `alias → resp.Alias`, `dev_id → resp.UserID` (note: `persistDevSession` reads `resp.UserID` and writes to `cfg.DevID` — the source param is `dev_id`, the response field is `UserID`, the config field is `DevID`; all three names coexist), `tier → resp.Tier`. `key_hash` is NOT a `secureLoginResponse` field — pass it as the `keyHash` argument to `persistDevSession` directly. Pass `""` as `fallbackAlias` (no flag alias exists in browser mode). Guard: if `resp.Alias == ""` after sanitization, return a meaningful error BEFORE calling `persistDevSession` rather than persisting a session with a blank alias.
- **Read-modify-save to avoid clobbering concurrent slot writes.** The browser flow may wait up to 5 minutes before persisting. A concurrent shell running `user login` during that window could write a new `UserJWT`; this flow's stale in-memory `cfg` would overwrite that on `Save`. Fix: after the callback succeeds, do a fresh `config.Load()`, apply ONLY the dev-slot fields (via `persistDevSession` on the freshly-loaded cfg), then `config.Save`. Atomic file rename in `config.Save` only prevents byte-interleaving; it does NOT prevent stale read-modify-write.
- **Refresh token must be validated after sanitization, not just JWT.** The user-login pattern only validates `JWT != ""`. The dev flow has a second mandatory token (the 30-day refresh token). Add a parallel guard: if `dev_refresh_token == ""` after `sanitizeCallbackValue`, fail the callback (HTTP 400 + error channel) with a message naming the missing field. Otherwise `dev refresh` will silently break on first use with a confusing error.
- **Injectable `var openURL = browser.OpenURL` for testability.** `pkg/browser`'s `OpenURL` is a direct package function; today's user.go calls it as `browser.OpenURL(...)` with no indirection, which is why the user-login browser flow has zero tests. The new dev-login flow introduces a package-level `var openURL = browser.OpenURL` indirection at `cmd/andamio/dev.go` (or a shared spot if it's cleaner), called as `openURL(authURL)`. Tests override it. Do not retrofit user-login to use the same indirection in this PR (scope).
- **No HTTP body parsing in the callback.** GET-only, all data in query params. `io.LimitReader` is moot for empty bodies; document the choice in the test.
- **5-minute timeout via `context.WithTimeout(context.Background(), 5*time.Minute)`** — mirror the user-login deadline exactly, not the cobra command context. Trade-off documented: SIGINT (Ctrl-C) during the wait will not gracefully shut down the listener — the user-visible behavior is "process exits but the goroutine serving the listener and `defer listener.Close()` may not fire cleanly." Long help text should mention this so users aren't surprised.
- **Test-first execution posture for the browser-flow function.** The user-login browser flow has zero tests today. We will not repeat that pattern for `dev login`. Tests come first; implementation makes them pass. Specifically targets the CSRF, sanitization, timeout, token-leak guard, and dual-validation scenarios.

## Open Questions

### Resolved During Planning

- **GET vs POST callback?** GET + query params. Mirror user login; loopback ephemeral-port makes the marginal POST benefit not worth the divergence.
- **Single command or split subcommands?** Single command with runtime branching on `cmd.Flags().Changed(...)` per flag.
- **Do we need a separate `buildDevAuthURL`?** No. Generalize the existing helper — `path` is the new 2nd positional argument.
- **Should `--alias` and `--address` become optional even in headless mode?** No. The all-or-nothing rule stays for headless mode (all three or none); partial invocation returns an error. The error message points at both the all-three headless form and the no-args browser form.
- **Should we characterize the existing `runUserLogin` browser flow as part of this PR?** No. Out of scope — separate plan.
- **How does the browser-open call become testable?** Via a package-level `var openURL = browser.OpenURL` indirection introduced in this PR (scoped to dev.go). Tests override it; the user-login call site is unchanged.
- **How is concurrent config.Save handled?** Read-modify-save: fresh `config.Load()` after callback success, apply dev-slot fields, then `Save`. Documented as part of the persistence step.
- **Should the HasDevAuth early-return print to stdout or stderr?** Stderr, gated on `!isJSON`. User-login mirrors stdout — known defect we do NOT replicate.
- **Does `--skey ""` mean "use empty path" or "no flag"?** "Flag was provided with empty value" — use `cmd.Flags().Changed("skey")`, not empty-string equality.
- **What does `fallbackAlias` get in browser mode?** Empty string. The flag alias doesn't exist in browser mode; if the callback's alias is also empty after sanitization, the flow errors out rather than persisting.

### Deferred to Implementation

- **Exact wording of the partial-flags error message.** Resolved at implementation time; the constraint is that it names both modes and the specific missing flag(s).
- **Whether the success message in non-JSON mode should print the tier.** Mirror the headless dev login's success message and decide at implementation time based on the exact lines `runDevHeadlessLogin` prints today.
- **Test helper extraction.** If `apikeyTestEnv` and `devTestEnv` share enough setup with the new `devBrowserTestEnv`, consider extracting a common helper. Decide once the test code is in front of you — premature extraction is worse than mild duplication.

## High-Level Technical Design

> *This illustrates the intended approach and is directional guidance for review, not implementation specification. The implementing agent should treat it as context, not code to reproduce.*

### Decision matrix: callback URL fields

The new `runDevLoginBrowser` callback handler accepts a GET to `/callback` with the following query parameters. Field names align with `internal/config.Config`'s `dev_*` JSON tags so persistence is a direct field copy.

| Query param | Required? | Maps to | Notes |
|-------------|-----------|---------|-------|
| `state` | yes | (validated, not stored) | CSRF token; exact-string compare against CLI-side `state`. Mismatch → 400 + plain-text body, no persistence. |
| `error` | (if present, success params ignored) | (returned to user as error) | App route reports gateway errors via this param. |
| `dev_jwt` | yes | `cfg.DevJWT` | 60-min RS256 JWT. Sanitized via `sanitizeCallbackValue`. |
| `dev_jwt_expires_at` | yes | `cfg.DevJWTExpiresAt` | RFC3339. Sanitized. |
| `dev_refresh_token` | yes | `cfg.DevRefreshToken` | 30-day rotation token. Sanitized. |
| `dev_refresh_token_expires_at` | yes | `cfg.DevRefreshTokenExpiresAt` | RFC3339. Sanitized. |
| `alias` | yes | `cfg.DevAlias` | Sanitized. |
| `dev_id` | yes | `cfg.DevID` | Sanitized. |
| `tier` | yes | `cfg.DevTier` | Sanitized. |
| `key_hash` | optional | `cfg.DevKeyHash` | App may not have this; tolerate empty. |

andamio-app-v2#698's callback construction must match this shape exactly. The wire format goes in the App issue's contract section before merging either side.

### Flag-branch decision

```
runDevLogin(cmd, args):
  // Use Changed(), NOT empty-string equality on GetString.
  // Rationale: `--skey ""` sets the value to empty but Changed() returns true,
  // which correctly routes to headless mode (where it fails with the existing
  // "missing skey" error). Empty-string equality would misroute it to browser
  // mode and trap a script with an unset $SKEY_PATH variable.
  skeyProvided  := cmd.Flags().Changed("skey")
  aliasProvided := cmd.Flags().Changed("alias")
  addrProvided  := cmd.Flags().Changed("address")

  switch {
  case !skeyProvided && !aliasProvided && !addrProvided:
      return runDevLoginBrowser(ctx, cfg)              // new browser path
  case skeyProvided && aliasProvided && addrProvided:
      return runDevHeadlessLogin(ctx, cfg, ...)        // unchanged
  default:
      return fmt.Errorf("dev login requires either no flags (browser mode) or all three of --skey/--alias/--address (headless mode); missing or partial flags are not accepted")
  }
```

Directional only — the implementing agent decides exact naming and error wording. **Do NOT copy the discriminator pattern from `runUserLogin`** if that pattern uses empty-string equality; the `Changed()` form is required for the reasons in Key Technical Decisions.

### devAuthCallbackResult struct shape

```
type devAuthCallbackResult struct {
    DevJWT                   string  // from `dev_jwt` query param, post-sanitize
    DevJWTExpiresAt          string  // from `dev_jwt_expires_at`
    DevRefreshToken          string  // from `dev_refresh_token`
    DevRefreshTokenExpiresAt string  // from `dev_refresh_token_expires_at`
    Alias                    string  // from `alias`
    DevID                    string  // from `dev_id`
    Tier                     string  // from `tier`
    KeyHash                  string  // from `key_hash`, optional
    Error                    string  // populated on any failure branch
}
```

This is a new struct distinct from the user-login flow's 4-field `authCallbackResult` — do NOT reuse `authCallbackResult` for the dev flow; the shape is wrong.

## Implementation Units

- [ ] **Unit 1: Relax `MarkFlagRequired` on `dev login` and add browser/headless RunE branch (with stub for browser path)**

**Goal:** Open up `devLoginCmd` to accept either no-args or all-three flags; route correctly inside `runDevLogin`. The browser branch returns a clear "not yet implemented" error to keep the unit landable on its own — Unit 2 replaces it.

**Requirements:** R2, R3.

**Dependencies:** None.

**Files:**
- Modify: `cmd/andamio/dev.go`
- Test: existing `cmd/andamio/dev_test.go` (regression coverage for the headless path)

**Approach:**
- Remove the three `devLoginCmd.MarkFlagRequired(...)` lines in `init()`.
- Update flag help strings to say `(required for headless mode)` instead of `(required)`.
- In `runDevLogin`, replace the unconditional flag-read + headless dispatch with a three-way branch using `cmd.Flags().Changed("skey")`, `Changed("alias")`, `Changed("address")` — NOT empty-string equality. Branches: (none changed → browser stub) / (all three changed → `runDevHeadlessLogin`) / (partial → error).
- The browser branch returns `fmt.Errorf("browser-flow dev login not yet implemented in this commit — use --skey/--alias/--address for now")`. This is replaced in Unit 2.
- The partial-flag error message names the specific missing flag(s) AND the no-args alternative, so the user can fix forward in either direction.
- `cobra.NoArgs` stays as-is on the command.

**Execution note:** Sanity-check the partial-flag error path manually after applying. Verify both ordinary partial cases AND the `--skey ""` (explicit empty value) case routes to headless (because `Changed()` returns true for explicit empty), not browser.

**Patterns to follow:**
- `cmd/andamio/user.go` — `runUserLogin`'s flag-read + branch at lines 110–135.

**Test scenarios:**
- Happy path (regression): `dev login --skey … --alias … --address …` continues to call `runDevHeadlessLogin` and lands a session. Existing `dev_test.go` tests must pass unchanged.
- Error path: invocation with `--skey` only (no `--alias`, no `--address`) returns the partial-flag error mentioning both modes. Equivalent for `--alias` only and `--address` only.
- Error path: invocation with two-of-three flags returns the partial-flag error (all three two-flag permutations).
- Edge case: `dev login --skey ""` (explicit empty string for skey) routes to the **headless** branch (because `cmd.Flags().Changed("skey")` returns true even for explicit empty values), where `runDevHeadlessLogin` then fails with the existing "missing skey" error. NOT routed to browser. This pins the Changed()-based discriminator and prevents the empty-shell-variable trap.
- Happy path: invocation with no flags returns the "not yet implemented" stub error (verifies the branch dispatched to the new path).

**Verification:**
- All existing `dev_test.go` tests pass without modification.
- `andamio dev login` with no args returns the stub error.
- `andamio dev login --skey foo` returns the partial-flag error.
- `andamio dev login --skey ""` returns the headless-mode `LoadSigningKey` error, NOT the browser stub.

---

- [ ] **Unit 2: Implement `runDevLoginBrowser` + tests (test-first)**

**Goal:** Replace the Unit 1 stub with a full browser-flow implementation. New function that mirrors `runUserLogin`'s browser path but writes to the dev slot and persists both the JWT and the 30-day refresh token. Tests come first; implementation makes them pass.

**Requirements:** R1, R4, R5, R6, R7, R8, R9, R10, R11.

**Dependencies:** Unit 1.

**Files:**
- Modify: `cmd/andamio/dev.go` (new `runDevLoginBrowser` function; replace Unit 1's stub)
- Modify: `cmd/andamio/user.go` (generalize `buildAuthURL` to accept a path parameter — `/auth/cli` for user, `/auth/dev-cli` for dev; update the single existing call site)
- Test: `cmd/andamio/dev_test.go` (new test block for the browser flow)

**Approach:**
- Generalize `buildAuthURL` — new signature `buildAuthURL(baseURL, path, redirectURI, state string) string`, with `path` inserted at position 2. Update the existing call inside `runUserLogin`'s browser branch (in user.go) to pass `"/auth/cli"` explicitly: `buildAuthURL(cfg.BaseURL, "/auth/cli", redirectURI, state)`. The compiler enforces the call-site update. Do NOT create a second `buildAuthURL` in dev.go — both files are in `package main` and share the helper.
- Introduce a package-level browser-opener indirection. Declare `var openURL = browser.OpenURL` once at package level in **`cmd/andamio/user.go`** (where `pkg/browser` is already imported), so the next PR that characterizes user-login can swap its existing `browser.OpenURL(authURL)` call to `openURL(authURL)` without a new declaration. In this PR, dev.go calls `openURL(authURL)`; user.go's existing `browser.OpenURL` call is untouched (strict scope). Package-level Go variables are exempt from the unused-variable check, so this compiles cleanly. Tests override `openURL` to mock browser-open behavior (success and failure).
- New `runDevLoginBrowser(ctx context.Context, cfg *config.Config) error`:
  1. Pre-flight: `cfg.APIKey == ""` → `*apierr.AuthError` with `auth login --api-key` hint. Exit before opening browser.
  2. Pre-flight: `cfg.HasDevAuth()` → print "already authenticated as <alias>" message to **stderr** via `fmt.Fprintf(os.Stderr, ...)`, gated on `!isJSON`, and return `nil`. In JSON mode the function returns `nil` with NO output on stdout — JSON consumers detect "no action needed" via empty stdout + zero exit. (Do NOT mirror user.go:125–128's `fmt.Printf` — that writes to stdout and breaks the JSON contract.)
  3. `generateState()` for CSRF.
  4. `net.Listen("tcp", "127.0.0.1:0")`, derive `redirectURI`.
  5. Build callback handler: GET-only (405 otherwise), validate `state` exact-match (mismatch → HTTP 400 + plain-text body, send error result, no persistence). Parse all required query params, run each through `sanitizeCallbackValue`. After sanitization, validate that BOTH `dev_jwt` AND `dev_refresh_token` are non-empty — empty either field → HTTP 400 with a message naming the missing field, send error result. (Do NOT mirror user-login's "only check JWT" pattern.) Also validate that `alias` is non-empty after sanitization (the persistence guard depends on it). Send a buffered `devAuthCallbackResult` into a capacity-1 channel on either success or error.
  6. `buildAuthURL(cfg.BaseURL, "/auth/dev-cli", redirectURI, state)`.
  7. `openURL(authURL)` — non-fatal failure (print URL to stderr for manual open, keep waiting). In `--output json` mode, the URL print goes to stderr only.
  8. `context.WithTimeout(context.Background(), 5*time.Minute)`, select on result or timeout. (SIGINT note — see Long help below.)
  9. **Read-modify-save** for persistence: on callback success, call `config.Load()` again to get the latest on-disk state (avoids clobbering a concurrent shell's `user login` write to the user slot). Construct a `secureLoginResponse` from the sanitized callback fields using field-assignment (the nested `JWT` and `RefreshToken` structs are anonymous — composite literal syntax does NOT compose across them; use `resp.JWT.Token = ...; resp.JWT.ExpiresAt = ...; resp.RefreshToken.Token = ...; resp.RefreshToken.ExpiresAt = ...; resp.UserID = ...; resp.Alias = ...; resp.Tier = ...`). Call `persistDevSession(freshCfg, &resp, /*keyHash=*/sanitizedKeyHash, /*fallbackAlias=*/"")`. If both `resp.Alias` and `fallbackAlias` are empty, return a meaningful error rather than persisting a blank alias.
  10. `config.Save(freshCfg)`.
  11. Print success message to stderr (gated on `!isJSON`). If `--output json`, emit a structured envelope via `output.PrintJSON` carrying only non-secret metadata: `alias`, `dev_id`, `tier`, `dev_jwt_expires_at`, `refresh_token_expires_at`, `key_hash`. Never the token bodies themselves.
- Update Long help text on `devLoginCmd`: explain both modes, name BOTH preconditions (API key AND wallet/skey), AND mention that Ctrl-C during the 5-minute wait will exit but may leave the listener goroutine running until process termination (the OS releases the port on exit).

**Execution note:** Test-first. Write `dev_test.go` test cases for all listed scenarios first; let them fail; then implement `runDevLoginBrowser` until they pass. The existing `runUserLogin` browser flow has zero tests — do not repeat that pattern.

**Technical design:** *(see High-Level Technical Design above — the decision matrix pins the callback URL contract that this function consumes.)*

**Patterns to follow:**
- `cmd/andamio/user.go:104–261` — `runUserLogin`'s browser branch (structure, listener lifecycle, timeout pattern, browser-open fallback).
- `cmd/andamio/dev.go` — `persistDevSession` for field writes; `runDevHeadlessLogin` for the `!isJSON` / stderr gating pattern; `runDevHeadlessLogin`'s JSON envelope shape.
- `cmd/andamio/dev_test.go` — `devGatewayStub` adaptation, `devTestEnv` setup with `t.Setenv("HOME", t.TempDir())`, `captureStdout`, and the `SECRET.SHOULD-NOT-LEAK` token-leak guard.

**Test scenarios:**
- Happy path: callback delivers valid params → `cfg.DevJWT`, `cfg.DevRefreshToken`, `cfg.DevAlias`, `cfg.DevID`, `cfg.DevTier`, both expiry fields populated; `config.Load()` after the call reads the same values from disk.
- Happy path: `--output json` mode emits a structured object with `alias`, `dev_id`, `tier`, `dev_jwt_expires_at`, `refresh_token_expires_at` — and NO field containing the raw JWT or refresh token body. Stderr carries no human prose either (the success print is also gated).
- Happy path: browser-open failure (via `openURL` override returning a non-nil error) prints the URL to stderr; listener keeps waiting; subsequent callback succeeds.
- Edge case: `key_hash` query param absent → `cfg.DevKeyHash` is empty (not set to literal `"undefined"`); persistence succeeds.
- Edge case: `dev_jwt` query param is the literal string `"undefined"` → `sanitizeCallbackValue` drops it; the callback returns HTTP 400 and the function reports a meaningful error rather than persisting an empty JWT.
- Edge case: `dev_refresh_token` query param is empty / `"undefined"` after sanitization → callback returns HTTP 400 with a message naming the missing refresh token; function returns an error; no persistence. (Parallel to the `dev_jwt` guard — this is the second-token validation the user-login pattern does NOT cover.)
- Edge case: `alias` query param empty after sanitization → callback returns HTTP 400; function returns an error before calling `persistDevSession`; no blank-alias persistence.
- Error path: state mismatch in callback → 400 + plain-text body, returns error, NO config write (verify via `config.Load()` after returning).
- Error path: callback uses POST method → 405; listener keeps waiting; verify via second valid GET callback completing successfully (the wrong method doesn't break the session).
- Error path: `error` query param present in callback → returned as a wrapped error mentioning the gateway's message; no config write.
- Error path: 5-minute timeout fires (test sends no callback) → returns "authentication timed out after 5 minutes" or equivalent; listener torn down; no config write.
- Pre-flight error path: `cfg.APIKey == ""` → returns `*apierr.AuthError` mentioning `auth login --api-key`; `openURL` is NOT called (override fails loudly if invoked); no listener started; no config write.
- Pre-flight early-return: `cfg.HasDevAuth() == true` → in text mode prints "already authenticated" to **stderr**, returns nil; in `--output json` mode emits NOTHING on stdout AND returns nil; `openURL` not called; no config write.
- Concurrent slot safety (disk-state form, not live concurrency): seed `~/.andamio/config.json` with only an API key and a user JWT. Start `runDevLoginBrowser`. AFTER `runDevLoginBrowser` calls into the listener-wait phase but BEFORE the callback fires, overwrite `~/.andamio/config.json` directly to add a new `UserJWT` value (different from the seeded one — simulates a concurrent `user login` having completed). Then trigger the callback. Assert that after `runDevLoginBrowser` returns, the on-disk config carries BOTH the new dev-slot fields from the callback AND the post-overwrite `UserJWT` — NOT the original seeded one. This pins the read-modify-save invariant via deterministic disk state rather than a true live race. (A live-concurrency simulation would be inherently flaky; this disk-state form is what production behavior actually depends on.)
- Token-leak guard: across every test case above, captured stdout AND captured stderr must NOT contain the literal `dev_jwt` or `dev_refresh_token` body (use a `SECRET.SHOULD-NOT-LEAK` marker in stub responses). Stderr gets human prose; tokens never reach either stream.
- Integration scenario: a config seeded with BOTH `APIKey: "test-api-key"` AND the browser-flow-populated dev creds is passed to a subsequent `runAPIKeyJSON` call against a stubbed gateway; assert the wire carries both `X-API-Key: test-api-key` AND `Authorization: Bearer <dev_jwt>` headers. (Verifies the persisted slot integrates with `devKeysClient`. The APIKey MUST be populated for the assertion to be non-vacuous — `apikey_test.go`'s `apikeyTestEnv` pattern is the model.)

**Verification:**
- All new tests pass.
- Manual smoke: run `andamio dev login` (no args) against a local andamio-app-v2 stub that mimics `/auth/dev-cli` per the contract; confirm browser opens, callback fires, `dev status` shows the new session.
- `cmd/andamio/user_test.go` and existing `dev_test.go` tests still pass (no regression in user-login or headless dev-login).

---

- [ ] **Unit 3: Update CLAUDE.md auth narrative + CHANGELOG entry**

**Goal:** Sync the project's source-of-truth docs to the new behavior.

**Requirements:** N/A (documentation).

**Dependencies:** Unit 2.

**Files:**
- Modify: `CLAUDE.md` — Auth Flow narrative (around line 80, the `Developer JWT` bullet) and the `dev login` row in the Complete Command Reference table.
- Modify: `CHANGELOG.md` — `[Unreleased]` `### Added` entry.

**Approach:**
- `CLAUDE.md` Auth Flow bullet: add a sentence noting that `dev login` (no args) now supports a browser-wallet sign flow that mirrors `user login`, with the same `--skey` headless variant for CI/CD. Reference `/auth/dev-cli` and andamio-app-v2#698 as the App-side dependency.
- `CLAUDE.md` command-reference table row for `dev login`: split the cell into two forms (browser vs headless) like the user-login row does.
- `CHANGELOG.md` — **two entries** under `[Unreleased]`:
  - `### Added`: the new browser-flow `dev login` (no args), the App-side dependency, the prereq (`auth login --api-key`), and the cross-link to issue #100 / andamio-app-v2#698. Closes #100.
  - `### Changed`: removal of `MarkFlagRequired` on `--skey`/`--alias`/`--address`. Scripts in headless environments that previously relied on cobra's flag-required error to short-circuit must now pass all three flags explicitly OR omit all three (browser mode). Partial-flag invocations now return an application-level error explaining both modes. This is a backward-compatible behavioral change for callers passing all three flags; it IS a visible change for callers relying on cobra's exit-1 flag-validation error.

**Test scenarios:**
Test expectation: none — documentation-only unit; no behavioral change.

**Verification:**
- `andamio --help dev login` matches the new Long text (set in Unit 2).
- Manual readthrough: CLAUDE.md narrative matches the implementation; no stale claims about `dev login` being headless-only.
- CHANGELOG entry is under `### Added` and follows the prose density convention of recent entries.

## System-Wide Impact

- **Interaction graph:** `dev login` writes to the dev config slot → downstream consumers (`dev keys *`, `apikey usage`, `apikey profile`, `dev refresh`, `dev status`, `dev logout`) read from the same slot. No new entry points; the new flow extends the front door to a slot that already has stable readers. The CLI ↔ App ↔ API call chain is the cross-system path — both this PR and andamio-app-v2#698 must land before the chain is functional.
- **Error propagation:** Pre-flight errors → `*apierr.AuthError` exit code 3 (composability contract). Timeout → plain `fmt.Errorf` with explicit "timed out" text. Gateway errors propagated via the existing `apierr.AuthError` chain through `client.Post` to `/v2/auth/developer/login/{session,complete}`. The callback's `error` query param produces a wrapped error mentioning the gateway's message verbatim.
- **State lifecycle risks:** Partial config write if `config.Save` fails mid-flow is covered by `internal/config.Save`'s atomic tempfile+rename. Separately — and NOT covered by that atomicity — a concurrent shell writing to the same `config.json` mid-flow could be clobbered by this flow's stale read-modify-write. Mitigation captured in Key Technical Decisions and Unit 2 approach: re-`Load` immediately before the dev-slot write so only the dev slot is overwritten. Atomic file rename only prevents byte interleaving; the read-modify-save pattern narrows the racy window from "up to 5 minutes" (between initial Load and final Save) to "milliseconds" (between the fresh re-Load and the final Save), but does not close it entirely — a concurrent write that lands inside that millisecond window will still be lost. Acceptable for the realistic operator threat model; full elimination would require file locking and is out of scope.
- **API surface parity:** None required. The dev JWT slot reader path is shared between this new flow and the existing headless flow.
- **Integration coverage:** Cross-process flow (CLI listener ↔ browser ↔ andamio-app-v2 `/auth/dev-cli` ↔ andamio-api `/v2/auth/developer/login/*`). Unit tests cover the CLI portion with stubs; joint end-to-end manual verification waits for andamio-app-v2#698. A note in the PR description should call this out so reviewers don't expect the CLI PR to demonstrate an end-to-end browser run on its own.
- **Unchanged invariants:**
  - The `--skey` headless `dev login` path is byte-for-byte unchanged in `runDevHeadlessLogin` and `persistDevSession`.
  - `internal/config.Config`'s dev slot shape is unchanged — no new fields.
  - `devKeysClient`'s dual-credential routing is unchanged — this plan just provides another way to populate the slot it reads from.
  - The env-snapshot logic for `ANDAMIO_DEV_JWT` / `ANDAMIO_DEV_REFRESH_TOKEN` continues to work — `Load`/mutate/`Save` cycle is intact.
  - The `user login` browser flow is completely untouched (only `buildAuthURL` is generalized; the one existing call site is updated in place).

## Risks & Dependencies

| Risk | Mitigation |
|------|------------|
| andamio-app-v2#698 lands later → CLI ships a flow that opens a browser to a 404 | Sequence the CLI release after the App route is live in preprod. The PR description for this plan's PR explicitly notes the dependency. End-to-end smoke before tagging the next CLI release confirms the App route exists. |
| Callback payload field-name drift between this plan's contract and #698's implementation | The decision-matrix table in this plan is the wire-format source of truth. Mirror it into #698's body. Pre-merge manual joint smoke confirms field names match. |
| Generalizing `buildAuthURL` introduces a regression in `user login` | One existing call site, single-line change (path → parameter). Covered by manual verification — `andamio user login` still produces `…/auth/cli?…` exactly as today. |
| Existing `runUserLogin` browser flow has no tests, so the pattern we're mirroring is unverified | Accept and acknowledge — characterizing user-login is out of scope (see Deferred to Separate Tasks). The new flow's own tests pin the equivalent properties for the dev path, which is the meaningful coverage. |
| 30-day refresh token transits the loopback URL in plaintext (query string) | Accept — loopback only, ephemeral port, listener torn down in seconds, no shared infrastructure between browser and listener that could log it. The marginal POST/JSON benefit doesn't justify diverging from the user-login pattern. Documented explicitly in Key Technical Decisions. |
| User has dev creds already; new browser login could silently overwrite them | Pre-flight `HasDevAuth()` guard: print "already authenticated" to stderr (gated) and return early. User must `dev logout` first. Mirrors `user login`'s pattern (with the stdout→stderr correction noted in Key Technical Decisions). |
| Tokens accidentally appear on stdout in some output mode | `SECRET.SHOULD-NOT-LEAK` test guard across every test case. The headless dev login already uses this pattern; new flow inherits it. Tests assert on BOTH stdout and stderr. |
| SIGINT (Ctrl-C) during 5-min listener wait does not gracefully shut down the listener — process exits abruptly | Accept and document. Trade-off comes from using `context.Background()` for the timeout instead of the cobra command context (so cancellation only fires on the actual deadline, mirroring user-login). Long help text mentions the behavior so users aren't surprised. Separate hardening pass could introduce a signal-aware select; out of scope. |
| Concurrent shell writes user-slot fields while browser flow is waiting → naive `Save` clobbers them | Read-modify-save pattern: fresh `config.Load()` immediately before applying dev-slot fields, then `Save`. Documented in Key Technical Decisions; pinned by the "concurrent slot safety" test scenario in Unit 2. |
| `secureLoginResponse` shape mismatch with flat callback query params → compile error or persistence bug | Field-assignment pattern (the nested `JWT`/`RefreshToken` are anonymous structs, no cross-field composite literal). Explicit in Unit 2 approach. |
| `fallbackAlias=""` + empty callback alias → silent blank-alias persistence and confusing `dev status` output | Explicit guard before `persistDevSession`: if both are empty, return a meaningful error instead of persisting. Test scenario pins it. |
| Two simultaneous `dev login` processes both complete and clobber each other's dev-slot writes | Accept as known limitation. Both processes Load, both write the same logical fields, the last `Save` rename wins; the first process's tokens are silently lost from disk even though it exited 0. Atomic rename in `config.Save` prevents JSON corruption but not logical clobbering. Implementing file-level advisory locking (flock) would close it but widens scope significantly; deferred to a separate hardening pass. Realistic operator usage rarely runs two concurrent `dev login`s. Surfaced by adversarial review (ADV-003). |
| Spoofed callback with bogus `?error` could DoS the listener without knowing the state token | Fixed: state validation now precedes `?error` processing. Any request without a matching state token receives 400 + plain-text body and is silently dropped — the listener keeps waiting for a legitimate callback. Pinned by `TestRunDevLoginBrowser_StateMismatch_SilentlyIgnoredKeepsListening`. Surfaced by security review (SEC-001). |
| CSRF state token leaks to stderr via preemptive "if browser doesn't open, visit:" print → CI logs capture it | Fixed: the manual-open URL is now printed ONLY on actual `openURL` failure, not preemptively. The state token stays in process memory on the happy path. Pinned by `TestRunDevLoginBrowser_StateTokenNotInStderr_OnHappyPath`. Surfaced by security review (SEC-002). |
| Two simultaneous valid callbacks deadlock the flow (capacity-1 channel + blocking send + unbounded `server.Shutdown(context.Background())`) | Fixed: handler uses non-blocking `select { case resultChan <- result: default: }`; `shutdownServer` helper bounds `server.Shutdown` with a 2-second timeout context. Pinned by `TestRunDevLoginBrowser_TwoSimultaneousCallbacks_NoDeadlock`. Surfaced by adversarial review (ADV-001). |
| `HasDevAuth` pre-flight uses stale cfg from dispatcher → concurrent `dev logout` race produces silent no-op | Fixed: `runDevLoginBrowser` re-loads the config immediately before the pre-flight check (`preflightCfg, err := config.Load()`). Cost: one extra file read. Surfaced by adversarial review (ADV-005). |

## Documentation / Operational Notes

- After both this plan and andamio-app-v2#698 land, update `andamio-docs` CLI install/auth pages to document the new `dev login` no-args path. Out of scope for this PR; track separately.
- The verification task note at `~/projects/02-areas/andamio/000-task-notes/Tasks/2026-05-22-verify-andamio-cli-013-before-andrew-handoff.md` should be revised to cover the browser flow once both halves ship — the current checklist only verifies the headless path (i.e., the v0.13.0 fix, not the actual Andrew-blocker).
- The staged reply to Andrew at `~/projects/02-areas/andamio/000-inbox/2026-05-20-andrew-cli-auth-401-reply.md` should be revised to point at this plan / the new commands, not the headless `dev login --skey` path it currently describes.

## Sources & References

- **Origin document:** GitHub issue [andamio-cli#100](https://github.com/Andamio-Platform/andamio-cli/issues/100)
- **App-side companion:** [andamio-app-v2#698](https://github.com/Andamio-Platform/andamio-app-v2/issues/698)
- **API endpoints (already shipped):** [andamio-api#410](https://github.com/Andamio-Platform/andamio-api/issues/410)
- **Predecessor CLI fix (dual-credential routing):** [PR #96](https://github.com/Andamio-Platform/andamio-cli/pull/96), v0.13.0 release
- **Related issues:** [#97](https://github.com/Andamio-Platform/andamio-cli/issues/97) (expired dev JWT), [#98](https://github.com/Andamio-Platform/andamio-cli/issues/98) (stdout error envelope), [#99](https://github.com/Andamio-Platform/andamio-cli/issues/99) (whitespace API key)
- **Reference code:** `cmd/andamio/user.go` (browser flow pattern), `cmd/andamio/dev.go` (headless dev login + persistence), `cmd/andamio/dev_test.go` (test patterns)
- **Institutional learnings:**
  - `docs/solutions/integration-issues/cli-dev-portal-dual-credential-pattern.md`
  - `docs/solutions/security-issues/cli-security-hardening-input-validation.md`
  - `docs/solutions/architecture/cli-composability-audit-and-fix.md`
  - `docs/solutions/logic-errors/fix-three-cli-issues-hex-encoding-lesson-merge-headless-login.md`
