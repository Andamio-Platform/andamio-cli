---
title: "fix: dev-login browser callback POST+JSON contract alignment with andamio-app-v2#699"
type: fix
status: active
date: 2026-05-25
origin: end-to-end smoke test of PR #101 against preprod (2026-05-25)
---

# fix: dev-login browser callback POST+JSON contract alignment with andamio-app-v2#699

## Overview

PR #101 shipped the browser-wallet `andamio dev login` flow with a callback contract of **GET + query params** (mirroring the existing user-login flow). The companion app-side PR andamio-app-v2#699 shipped at nearly the same time with a callback contract of **POST + JSON body**. Both sides built consistently with their respective plans; the plans disagreed on the wire format; no pre-merge alignment happened. End-to-end smoke against preprod (2026-05-25) returned `callback_failed` — the gateway issued the JWT pair, the app POSTed it to `127.0.0.1:<port>/callback`, and the CLI listener returned 405 because the handler is GET-only.

This plan adopts the app's POST+JSON contract on the CLI side. The app's rationale is sound — a 30-day refresh token in the browser's URL history is a real privacy concern — and the headless flow gives operators a working fallback in the meantime.

User-login (`/auth/cli`) is **not** affected; the app-side comment in `cli-auth-flow.tsx` confirms user-login stays on GET-redirect. This is dev-login only.

## Problem Frame

End-to-end smoke after the #101 merge:

```
$ ./andamio dev login
Opening browser for developer authentication...
Waiting for authentication...
```

In the browser tab at `preprod.app.andamio.io/auth/dev-cli?...`:

> **Could not reach the CLI listener.** The credentials were issued, but the browser could not deliver them to the CLI.
> *Likely causes:* CORS / Private Network Access policy; mixed-content; listener exited.

Diagnostic from a second terminal confirmed the listener was alive on `127.0.0.1:50537` (manual `curl` returned the expected `Security validation failed` for a mismatched state). The block is browser-side — Chrome's PNA + CORS preflight against an HTTPS origin POSTing to a private-IP HTTP listener.

Code archaeology:

| Side | Plan | Implementation |
|------|------|----------------|
| CLI (#101) | `docs/plans/2026-05-22-001-feat-browser-based-dev-login-plan.md` line 82: "Callback transport: GET with query params, not POST/JSON" | `cmd/andamio/dev.go:501-504` — 405 for non-GET |
| App (#699) | `docs/plans/2026-05-22-001-feat-auth-dev-cli-route-plan.md` (in app-v2 repo) | `src/components/auth/dev-cli-auth-flow.tsx` — `fetch(redirectUri, { method: "POST", body: JSON.stringify(...) })` |

The CLI plan's stated rationale ("consistency with the existing user-login pattern") is weaker than the app plan's stated rationale ("refresh token would leak via URL history"). Picking the app side closes the security argument and keeps the user-login pattern unchanged.

Wire-format coordination was tracked as andamio-app-v2#700 (still open) but never gated either merge — both sides shipped on plan-internal consistency without cross-repo verification. Captured separately as a process learning; this plan addresses only the immediate code defect.

## Requirements Trace

- **R1.** CLI listener accepts `POST /callback` with `Content-Type: application/json` and a JSON body matching `DevCliSuccessPayload` from the app (`dev_jwt`, `dev_jwt_expires_at`, `dev_refresh_token`, `dev_refresh_token_expires_at`, `alias`, `address`, `state`).
- **R2.** CLI listener accepts `POST /callback` with a JSON body matching `DevCliErrorPayload` (`error`, `state`) and surfaces the gateway-side error.
- **R3.** CLI listener responds to `OPTIONS /callback` with a CORS preflight that allows the request: `Access-Control-Allow-Origin: <app-origin>`, `Access-Control-Allow-Methods: POST, OPTIONS`, `Access-Control-Allow-Headers: Content-Type`, `Access-Control-Allow-Private-Network: true`, `Access-Control-Max-Age: 60`.
- **R4.** Allowed origin is derived from `cfg.BaseURL` by the same `.api.` → `.app.` swap used by `buildAuthURL`. Exact-string match — no wildcard, no `*`. Both preprod and mainnet work without code changes.
- **R5.** State validation (CSRF) runs BEFORE payload parsing, mirroring the existing GET behavior. Mismatch returns 400, no enqueue, listener keeps waiting.
- **R6.** Required-field validation (`dev_jwt`, `dev_refresh_token`, `alias`) runs after state validation. Missing field returns 400 with a message naming the field; consistent with the existing GET behavior. (Empty `state` rejected at R5.)
- **R7.** GET method on `/callback` returns 405 (the contract is now POST-only). OPTIONS is the only other accepted method.
- **R8.** All existing token-leak guards still hold: tokens never on stdout, JSON envelope carries only metadata, state token never on stderr on the happy path.
- **R9.** User-login (`/auth/cli`) browser flow is unchanged. `buildAuthURL` stays generalized; the user-login call site still uses GET+query because that's what the app's `cli-auth-flow.tsx` continues to do.
- **R10.** `address` field from the new payload is **not persisted** (the CLI's dev slot already has `DevAlias`; the wallet address is informational and the dual-credential surfaces don't need it). It must be validated as non-empty for completeness and dropped after validation.
- **R11.** `dev_id`, `tier`, `key_hash` are **no longer part of the wire contract** (the app dropped them). The CLI's `devSessionResult` JSON envelope marks them `omitempty` and the headless flow continues to populate them; browser-flow JSON envelopes will omit those fields entirely. This is a `### Changed` for any agent reading the envelope.
- **R12.** CHANGELOG entry under `[Unreleased]` `### Fixed` (the previously-shipped browser flow was non-functional end-to-end) AND `### Changed` (envelope-shape change for `dev_id`/`tier`/`key_hash` in browser-flow JSON output).

## Scope Boundaries

- User-login browser flow stays GET-redirect. Not touched.
- Headless `dev login --skey/--alias/--address` not touched.
- `buildAuthURL` not touched.
- No new fields added to `internal/config.Config`; no schema changes to `~/.andamio/config.json`.
- Closing of the API contract drift for `dev_id`/`tier` (tracked as #110) is **partially superseded** by this plan — the app dropped them from the wire, so the CLI's matrix is now the one that's wrong; this plan updates the CLI side to match. #110 can be closed after this lands.

### Deferred to Separate Tasks

- **Optional `/v2/auth/developer/account/me` follow-up call** to populate `dev_id`/`tier`/`key_hash` after browser login. Adds a network round-trip and a failure mode; not justified for cosmetic envelope completeness. File as a separate enhancement if `dev status` UX needs it.
- **andamio-app-v2#700 wire-format contract issue.** Still open in app-v2. This plan adopts the app's contract; closing #700 is the app team's call.

## Context & Research

### Relevant Code

- `cmd/andamio/dev.go` — `runDevLoginBrowser` callback handler at lines 498-582. Change site.
- `cmd/andamio/dev.go` — `devAuthCallbackResult` struct at lines 50-60. Add `Address` field (validation-only, not persisted).
- `cmd/andamio/dev.go` — `devSessionResult` struct at lines 259-266. Add `omitempty` to `DevID`.
- `cmd/andamio/dev_test.go` — all `*Browser*` tests construct `GET /callback?...` requests via `successCallback`/`validCallbackParams`. Replace with `POST /callback` JSON-body helpers.

### App-Side Wire Contract (authoritative)

Sourced from `src/lib/cli-auth-params.ts` in andamio-app-v2:

```typescript
interface DevCliSuccessPayload {
  dev_jwt: string;
  dev_jwt_expires_at: string;
  dev_refresh_token: string;
  dev_refresh_token_expires_at: string;
  alias: string;
  address: string;
  state: string;
}

type CliCallbackErrorCode = "invalid_request" | "auth_failed" | "user_cancelled" | "no_access_token";

interface DevCliErrorPayload {
  error: CliCallbackErrorCode;
  state: string;
}
```

Allow-list policy (`cli-auth-params.ts`): hostname must be `localhost`/`127.0.0.1`/`[::1]`, scheme `http:`, port 1024-65535. Userinfo (`http://user:pass@host/`) rejected. State must be present (no entropy floor on the app side; CLI side generates a 32-byte URL-safe state via `generateState`).

### Browser Mechanics

Chrome's [Private Network Access (PNA)](https://wicg.github.io/private-network-access/) requires the listener to return `Access-Control-Allow-Private-Network: true` on the preflight when the request initiator is a public-network (HTTPS) origin and the destination is a private network (127.0.0.1). Without it, the preflight fails and the fetch is blocked. Safari and Firefox follow the same CORS rules but don't enforce PNA today — Chrome is the strictest deployment target.

The preflight is automatic for any `fetch()` with `Content-Type: application/json` because that's a non-simple header. The CLI listener cannot avoid the preflight by being clever about content-type — the JSON payload is required.

### Institutional Learnings

- `docs/solutions/security-issues/cli-security-hardening-input-validation.md` — established the GET-only callback pattern in user-login. The dev-login fix here is a deliberate divergence, justified by the refresh-token-in-URL-history argument. Worth a compounding update after this lands.
- `docs/solutions/integration-issues/cli-dev-portal-dual-credential-pattern.md` — dual-credential rule is preserved; this plan doesn't touch the X-API-Key + dev JWT routing.

## Key Technical Decisions

- **Adopt the app's POST+JSON contract, not the original GET+query plan.** Security argument (refresh token in URL history) is stronger than consistency argument (mirror user-login). User-login is unchanged; both flows can have different wire formats because they have different security properties (user JWT vs 30-day refresh token).
- **CORS allow-list derived from `cfg.BaseURL`, not wildcarded.** Exact-string match against the derived app URL (`.api.` → `.app.`). Prevents a hostile local web page from POSTing to the listener with a guessed state token (CSRF-without-state-knowledge attack). Origin header is browser-controlled; combined with state validation this gives defense-in-depth.
- **PNA header is non-negotiable.** Chrome rolling out PNA enforcement broadly through 2026; the CLI must include it to remain functional. Add it now even though Firefox/Safari don't require it — costs nothing.
- **OPTIONS preflight handler responds 204 No Content.** Standard pattern, smallest surface. `Access-Control-Max-Age: 60` keeps the preflight from re-firing within a single browser-flow ceremony but doesn't outlive the listener's 5-minute window.
- **State validation precedes body parsing.** Mirrors the existing GET handler's ordering. A POST without a valid state token gets 400 + listener-keeps-waiting (same as the GET path). Adversarial review #106's "keep listening on validation errors" lesson is preserved.
- **`address` is validated as non-empty but not persisted.** The CLI's dev slot already carries `DevAlias` which is the canonical identity. The wallet address is informational; persisting it would widen the config schema without a consuming path. Validate to catch protocol-level drift; drop after.
- **`Content-Type: application/json` is required.** Reject other content types with 415. The decoder is `json.NewDecoder(r.Body).Decode(&payload)` with `DisallowUnknownFields` so a future app-side payload extension that the CLI doesn't recognize fails loudly rather than silently dropping fields.
- **Request body size cap at 64 KiB.** Wrap `r.Body` in `http.MaxBytesReader(w, r.Body, 64*1024)` before decoding. Larger payloads are protocol violations or DoS attempts; 64 KiB is ~100× the realistic payload size (JWT + refresh token + alias + address + state).
- **GET on `/callback` returns 405.** The previous GET-handler is gone. Anyone still pointing the app at `/auth/dev-cli` from an old in-flight browser session will get a clear 405. The listener tears down on timeout so this is a transient window.
- **No backward-compat shim for the GET callback.** PR #101 was merged 2026-05-25 and the contract was never functional against preprod, so no operator has a working GET-based CLI in the wild. A shim would be code-debt for no operator benefit.

## Open Questions

### Resolved During Planning

- **Adopt app's contract, or change the app?** Adopt app's. Security argument is stronger; user-login pattern is unchanged either way.
- **Validate `address` field?** Yes, but don't persist. Catches contract drift; doesn't widen the config schema.
- **Drop `dev_id`/`tier`/`key_hash` from browser-flow envelope?** Yes (via `omitempty`). The app dropped them from the wire; we cannot manufacture them client-side without an extra network call. Document under `### Changed`.
- **Add backward-compat GET handler?** No. The browser flow has never worked end-to-end; no operator has a working GET-based dependency.
- **Support `Content-Type: application/json; charset=utf-8`?** Yes — parse the content-type header and accept any variant with `application/json` as the media type. The app may include the charset suffix.

### Deferred to Implementation

- **Exact wording of the 415 unsupported-media-type response.** Decide at implementation time; align with the existing 400/405 plain-text style.
- **Whether to log the parsed payload at debug level.** Probably no — tokens would land in debug logs. Decide based on the existing CLI debug-logging discipline.

## High-Level Technical Design

> *Directional guidance for review, not implementation specification.*

### New callback handler shape

```
mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
    // 1. Method gate.
    switch r.Method {
    case http.MethodOptions:
        writeCORSPreflight(w, r, allowedOrigin)  // see helper below
        return
    case http.MethodPost:
        // fall through
    default:
        http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
        return
    }

    // 2. Origin allow-list (defense-in-depth alongside state).
    if origin := r.Header.Get("Origin"); origin != "" && origin != allowedOrigin {
        http.Error(w, "Origin not allowed", http.StatusForbidden)
        return
    }

    // 3. Content-Type gate.
    if !isJSONContentType(r.Header.Get("Content-Type")) {
        http.Error(w, "Content-Type must be application/json", http.StatusUnsupportedMediaType)
        return
    }

    // 4. Read body with size cap.
    r.Body = http.MaxBytesReader(w, r.Body, 64*1024)

    // 5. Decode into one of two payload shapes. Try success first; on
    //    parse failure, retry as error payload. (Or decode into a union
    //    type — simpler is fine here.)
    var payload struct {
        // Success fields
        DevJWT                   string `json:"dev_jwt"`
        DevJWTExpiresAt          string `json:"dev_jwt_expires_at"`
        DevRefreshToken          string `json:"dev_refresh_token"`
        DevRefreshTokenExpiresAt string `json:"dev_refresh_token_expires_at"`
        Alias                    string `json:"alias"`
        Address                  string `json:"address"`
        // Common
        State string `json:"state"`
        // Error-only
        Error string `json:"error"`
    }
    dec := json.NewDecoder(r.Body)
    dec.DisallowUnknownFields()
    if err := dec.Decode(&payload); err != nil {
        http.Error(w, "Malformed JSON body", http.StatusBadRequest)
        return
    }

    // 6. State validation FIRST (CSRF).
    if payload.State != state {
        w.WriteHeader(http.StatusBadRequest)
        fmt.Fprint(w, "Security validation failed.")
        return  // do NOT enqueue; keep listening
    }

    // 7. Error path.
    if payload.Error != "" {
        result.Error = payload.Error
        sendResult(result)
        w.WriteHeader(http.StatusOK)
        fmt.Fprint(w, "Authentication failed. You can close this window.")
        return
    }

    // 8. Sanitize + validate success fields.
    // ... same logic as today's GET handler, but reading from payload not q ...

    // 9. Required-field check (do NOT enqueue invalid; keep listening).
    if len(missing) > 0 {
        http.Error(w, fmt.Sprintf("missing fields: %v", missing), http.StatusBadRequest)
        return
    }

    sendResult(result)
    w.WriteHeader(http.StatusOK)
    fmt.Fprint(w, "Authentication successful. You can close this window.")
})
```

### CORS preflight helper

```
func writeCORSPreflight(w http.ResponseWriter, r *http.Request, allowedOrigin string) {
    // Echo the allowed origin verbatim (exact-string match, not wildcard).
    // If the Origin header is missing or doesn't match, omit the headers —
    // browsers will reject the preflight, which is the correct behavior.
    if origin := r.Header.Get("Origin"); origin == allowedOrigin {
        w.Header().Set("Access-Control-Allow-Origin", origin)
        w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
        w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
        w.Header().Set("Access-Control-Allow-Private-Network", "true")
        w.Header().Set("Access-Control-Max-Age", "60")
        w.Header().Set("Vary", "Origin")
    }
    w.WriteHeader(http.StatusNoContent)
}
```

### Allowed-origin derivation

```
// runDevLoginBrowser, immediately after generating state, before mux setup.
appOrigin := strings.Replace(cfg.BaseURL, ".api.", ".app.", 1)
// Trim trailing slash if present, and any path component.
if u, err := url.Parse(appOrigin); err == nil {
    appOrigin = u.Scheme + "://" + u.Host
}
```

If `cfg.BaseURL` is malformed (covered separately by #108), `appOrigin` may be junk — that's #108's fix to land first or alongside, not this PR's.

## Implementation Units

- [ ] **Unit 1: Replace GET callback handler with POST+JSON + OPTIONS preflight (test-first)**

  **Goal:** Land the contract change in `runDevLoginBrowser`. GET→POST, add OPTIONS preflight, add Origin allow-list, add Content-Type gate, add body size cap.

  **Requirements:** R1, R2, R3, R4, R5, R6, R7, R8.

  **Files:**
  - Modify: `cmd/andamio/dev.go` — callback handler, `devAuthCallbackResult` struct (add `Address` field), helpers for CORS preflight + content-type check
  - Test: `cmd/andamio/dev_test.go` — replace `validCallbackParams` (URL.Values) + `successCallback` (HTTP GET) helpers with JSON-body + POST equivalents. All `*Browser*` tests cascade.

  **Approach:**
  - Define `validCallbackPayload` returning a `DevCliSuccessPayload`-shaped Go struct (or `map[string]string` for symmetry with current helper).
  - Define `successPostCallback` returning an `openURL` override that POSTs the payload to the redirect URI with `Content-Type: application/json` and the expected `Origin` header (set to the derived app URL — tests pass `https://preprod.app.andamio.io` since the test cfg uses preprod).
  - Replace every test's `successCallback` call with `successPostCallback`.
  - Replace `params.Set(...)` mutations with payload-struct mutations (e.g., `payload.DevJWT = "undefined"` instead of `params.Set("dev_jwt", "undefined")`).
  - Add new tests: `TestRunDevLoginBrowser_OPTIONSPreflight_AllowsApprovedOrigin`, `TestRunDevLoginBrowser_POSTFromDisallowedOrigin_403`, `TestRunDevLoginBrowser_NonJSONContentType_415`, `TestRunDevLoginBrowser_BodyTooLarge_413`, `TestRunDevLoginBrowser_MalformedJSON_400`, `TestRunDevLoginBrowser_UnknownJSONField_400`.

  **Test scenarios:**
  - Happy path (POST + valid JSON + matching Origin): persists all slots, returns 200 with the success HTML.
  - Pre-flight: OPTIONS with matching Origin returns 204 + all CORS+PNA headers, including `Access-Control-Allow-Private-Network: true`.
  - Pre-flight: OPTIONS with mismatched Origin returns 204 + NO CORS headers (browser will reject).
  - POST with disallowed `Origin` header returns 403 even with valid state + payload.
  - POST without `Origin` header (e.g., curl) is accepted iff state validates — same as today (Origin enforcement applies only to browser-originated requests; the loopback listener allows unauthenticated curl for local diagnostics).
  - POST with wrong Content-Type returns 415.
  - POST body > 64 KiB returns 413.
  - POST with malformed JSON returns 400.
  - POST with unknown JSON field returns 400 (DisallowUnknownFields catches contract drift).
  - State mismatch in POST body returns 400, no enqueue, listener keeps waiting.
  - Missing-field validation rejects + does NOT enqueue (carries over the lesson from issue #106).
  - GET on /callback returns 405.

  **Patterns to follow:**
  - Existing handler structure at dev.go:498-582 — keep the non-blocking `sendResult` pattern and the state-first ordering.
  - `internal/config` package for any helper extraction (avoid widening cmd/andamio).

  **Verification:**
  - `go test -race ./... -timeout 90s` clean.
  - `go vet ./...` clean.
  - Manual smoke against preprod: `./andamio dev login` → browser opens → sign in wallet → callback delivers → `dev status` shows the new session.

---

- [ ] **Unit 2: Adjust `devSessionResult` JSON envelope for browser-flow (close drift with #110)**

  **Goal:** Mark `dev_id`, `tier`, `key_hash` as `omitempty` in the envelope. Browser flow doesn't receive them; headless flow continues to populate them. Document in CHANGELOG under `### Changed`.

  **Requirements:** R11.

  **Dependencies:** Unit 1.

  **Files:**
  - Modify: `cmd/andamio/dev.go` — `devSessionResult` struct JSON tags (`Tier` and `KeyHash` already `omitempty`; add `omitempty` to `DevID`).
  - Modify: `CHANGELOG.md` — `### Changed` entry under `[Unreleased]`.

  **Approach:**
  - One-line tag change on `DevID` field. Update the comment near the struct to note that browser-flow envelopes omit these three fields, headless envelopes populate them.
  - Existing tests that assert `dev_id` is present in headless envelope shapes still pass (`omitempty` only suppresses zero values).
  - New test: `TestRunDevLoginBrowser_JSONOutputShape_OmitsServerOnlyFields` — assert browser-flow envelope has only `alias`, `jwt_expires_at`, `refresh_token_expires_at` (no `dev_id`, `tier`, `key_hash`).

  **Patterns to follow:**
  - Existing `devSessionResult` struct comment style.

  **Verification:**
  - `go test -race ./cmd/andamio/` clean.
  - Manual: `./andamio dev login --output json` (browser flow) emits envelope without those three fields; `./andamio dev login --skey ... --output json` (headless) still emits them.

---

- [ ] **Unit 3: Documentation sync**

  **Goal:** Update CLAUDE.md, CHANGELOG, and the round-1 plan to reflect the contract change.

  **Requirements:** R12.

  **Dependencies:** Unit 2.

  **Files:**
  - Modify: `CHANGELOG.md` — `### Fixed` (end-to-end break in v0.13.x's `dev login`) and `### Changed` (envelope-shape change).
  - Modify: `CLAUDE.md` — Auth Flow narrative for `dev login`. Mention POST+JSON contract specifically; cross-reference andamio-app-v2#699.
  - Modify: `docs/plans/2026-05-22-001-feat-browser-based-dev-login-plan.md` — append a "Post-Merge Correction" section noting the contract switch, link to this plan.

  **Approach:**
  - CHANGELOG: two entries.
    - `### Fixed`: "Browser-flow `dev login` callback now matches andamio-app-v2's POST+JSON contract. Previously shipped GET+query handler returned 405 to the app's POST and produced a `callback_failed` browser-side error; this fixes the end-to-end break."
    - `### Changed`: "Browser-flow `dev login --output json` envelope no longer includes `dev_id`, `tier`, or `key_hash` (the app dropped them from the callback contract). Headless `dev login --skey ... --output json` still populates all three. Scripts that need them on browser-flow accounts should call `andamio dev status --output json` after."
  - CLAUDE.md: minor edit to the Auth Flow paragraph and the `dev login` command-reference row. No table-shape change.
  - Round-1 plan correction: short section, link to this plan and to andamio-app-v2#699.

  **Verification:**
  - Manual readthrough; no behavioral changes.

## System-Wide Impact

- **Interaction graph:** Browser-flow `dev login` → app `/auth/dev-cli` → POST `127.0.0.1:<port>/callback`. The fix changes the third arrow only.
- **Error propagation:** Pre-flight errors stay as before. Browser-side error payload now arrives via POST JSON; CLI surfaces it via the same `fmt.Errorf("developer authentication failed: %s", code)` shape.
- **State lifecycle:** Read-modify-save pattern unchanged. Atomic file rename in `config.Save` unchanged.
- **API surface parity:** `devSessionResult` envelope changes for browser-flow only (three fields move to `omitempty`). Headless envelope unchanged.
- **Cross-system contract:** This plan ratifies andamio-app-v2#699's wire format. andamio-app-v2#700 (wire-format contract issue) can close once this lands.
- **Unchanged invariants:**
  - User-login (`/auth/cli`) browser flow.
  - Headless `dev login` byte-for-byte unchanged.
  - `internal/config.Config` schema.
  - `devKeysClient` dual-credential routing.

## Risks & Dependencies

| Risk | Mitigation |
|------|------------|
| Chrome PNA enforcement schedule changes; the `Access-Control-Allow-Private-Network` header becomes mandatory before this lands | Header is added in this PR. No-op for Firefox/Safari. |
| App-side payload shape evolves (new optional field) | `DisallowUnknownFields` makes contract drift loud rather than silent. App + CLI must coordinate any payload extension via andamio-app-v2#700. |
| Operator runs an old CLI binary against the new app route | App-side `dev-cli-auth-flow.tsx` will POST; old CLI returns 405; app surfaces `callback_failed` (same as today's situation). No new failure mode. Recommend operators upgrade to the version landing with this fix. |
| Operator runs the new CLI against the old app route (pre-#699) | The app's old route doesn't exist; browser would get 404 on `/auth/dev-cli`. Same as the pre-#699 situation. No new failure mode. |
| Origin allow-list rejects legitimate self-hosted Andamio deployments | The derivation uses `cfg.BaseURL` so any deployment with a parallel `.api.`/`.app.` pair works. Custom deployments without that naming get a 403 — they would already be broken because `buildAuthURL` makes the same assumption (covered by #108). |
| Test fixtures heavily coupled to the URL.Values-based callback helpers | Unit 1 explicitly replaces the helpers; the test churn is bounded by ~20 `_Browser*` tests in dev_test.go. |
| `DisallowUnknownFields` blocks a future app-side payload extension | This is the intended behavior — extending the wire format requires a coordinated PR on both sides. Add a `// TODO: coordinate wire-format extension via andamio-app-v2#700` comment near the decode call so a future maintainer doesn't relax this without thinking. |

## Sources & References

- **Origin:** End-to-end smoke test against preprod, 2026-05-25
- **Companion PR (app, MERGED):** [andamio-app-v2#699](https://github.com/Andamio-Platform/andamio-app-v2/pull/699)
- **Parent plan:** [`docs/plans/2026-05-22-001-feat-browser-based-dev-login-plan.md`](./2026-05-22-001-feat-browser-based-dev-login-plan.md) — the original GET+query plan this corrects
- **Wire format source of truth:** `andamio-app-v2/src/lib/cli-auth-params.ts` (`DevCliSuccessPayload`, `DevCliErrorPayload`)
- **Wire format contract issue (app, OPEN):** [andamio-app-v2#700](https://github.com/Andamio-Platform/andamio-app-v2/issues/700) — process learning; closes after this lands
- **Round-2 ce-review of #101:** four reviewer agents (correctness, security, adversarial, api-contract) — none caught the wire-format mismatch because each reviewed only the CLI side. Lesson: cross-repo contract changes need a verification step that spans both repos.
- **Related cleanup:** [#110](https://github.com/Andamio-Platform/andamio-cli/issues/110) is partially superseded by this plan; close after Unit 2 lands.
