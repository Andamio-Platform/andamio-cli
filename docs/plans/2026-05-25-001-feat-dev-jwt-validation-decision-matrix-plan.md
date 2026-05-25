---
title: "feat: defense-in-depth validation of received dev JWT — architectural decision matrix"
type: feat
status: active
date: 2026-05-25
deepened: 2026-05-25
origin: https://github.com/Andamio-Platform/andamio-cli/issues/102
---

# feat: defense-in-depth validation of received dev JWT — architectural decision matrix

## Overview

**This is a decision artifact, not an implementation plan.** Issue [#102](https://github.com/Andamio-Platform/andamio-cli/issues/102) proposed three approaches to defend the dev-JWT browser callback against token-injection attacks. Research surfaced a fourth approach (OAuth code-flow + PKCE — what every major CLI ships) and a critical pre-requisite (callback transport mismatch between the in-flight andamio-cli and andamio-app-v2 work that must be resolved first via [andamio-app-v2#700](https://github.com/Andamio-Platform/andamio-app-v2/issues/700)).

Output of this plan: a four-way decision matrix the platform team can use to pick the right tradeoff between security posture, cross-repo coordination cost, and time-to-ship — followed by an implementation-plan sketch for each approach so the work that follows the decision is scoped before the decision is made.

No code lands from this plan directly. Once a path is picked, the chosen approach gets its own `feat:` plan and follows the standard `ce:plan` → `ce:work` workflow.

## Problem Frame

The browser-wallet `dev login` flow shipped in PR #101 receives a 60-min dev JWT + 30-day refresh token + alias + tier via an ephemeral localhost GET callback. CSRF state validation closes the basic CSRF class. The residual concern surfaced during ce-review:

**A local process that can observe the CSRF state token** — from `ps` output for the `open`/`xdg-open` subprocess that opens the browser, from browser history, or from stderr capture in CI logs if `openURL` fails — can craft a `GET 127.0.0.1:<port>/callback?state=<observed>&dev_jwt=<attacker.jwt>&dev_refresh_token=<attacker.refresh>&alias=attacker&...` during the 5-minute window. The CLI's callback handler validates state (passes — attacker has the real state), parses fields, runs sanitization, and persists. **The user is now authenticated as the attacker** — subsequent `andamio apikey usage`, `dev keys list`, etc. operate against the attacker's developer account (confused-deputy).

### Threat model — what is in and out of scope

External research (RFC 6819 §4.1.4, RFC 9700 §4.6) makes the boundary explicit:

- **In scope** (what this plan defends against): an adversary who can observe the CSRF state token (CI log capture, `ps` output) but who does **not** have file-system read access to `~/.andamio/config.json`. This is a real CI/CD scenario — shared CI worker, ephemeral home directory, observable process args.
- **Out of scope** (architectural acceptance, per RFC 6819 §4.1.4): an adversary with same-UID code execution on the user's machine. Such an adversary can read `~/.andamio/config.json` (0600) directly and steal credentials at rest — defending only the inbound callback path while leaving at-rest credentials exposed at 0600 closes a narrow window the adversary doesn't need. OAuth specs treat this as a platform-level problem ("It is not the task of the authorization server to protect the end-user's device from malicious software").

**The plan recommendation will be honest about this asymmetry.** Defending the callback path adds value specifically for the CI-log-leakage scenario. Anyone considering "and let's also defend against malware-on-the-machine" is asking a different question that needs different defenses (sender-constrained tokens, hardware-backed key storage, etc.) — out of scope.

## Critical Prerequisite (Not Part of This Plan's Scope, But Must Resolve First)

**Repo research surfaced a shipped contract mismatch:** the andamio-cli callback handler (shipped in PR #101) accepts **GET only with query params**; the andamio-app-v2 implementation in flight for [#698](https://github.com/Andamio-Platform/andamio-app-v2/issues/698) currently uses **POST with JSON body** (verified by reading `andamio-app-v2/src/app/auth/dev-cli/dev-cli-auth-flow.tsx`, the `fetch(..., { method: "POST", body: JSON.stringify(...) })` call).

If both ship as-is, the basic happy path is broken — the CLI returns 405 to every POST and the App's JSON body never reaches the listener.

The reconciliation is tracked at [andamio-app-v2#700](https://github.com/Andamio-Platform/andamio-app-v2/issues/700) (the wire-format pin filed during PR #101 review). Until #700 is resolved, no defense-in-depth approach in this plan is implementable — every approach hooks into the callback-parsing path, and that path's input shape is in dispute.

**This plan assumes the GET + query params contract from #700 wins.** If the App side comes back with a strong reason to switch to POST + JSON, the approach sketches in this plan stay valid but the integration points in each one (where exactly we hook into the callback parsing) shift correspondingly. The decision matrix below is approach-shape-agnostic and is not affected.

The recommended sequencing is therefore:

1. Resolve [andamio-app-v2#700](https://github.com/Andamio-Platform/andamio-app-v2/issues/700) (settle GET + query params or POST + JSON).
2. Use this plan to pick a defense-in-depth approach.
3. Write the implementation plan for the chosen approach (`ce:plan`).
4. Execute (`ce:work`).

## Requirements Trace

What any approach must satisfy, regardless of which one is chosen:

- **R1.** Reject callbacks that deliver attacker-controlled tokens (tokens the attacker generated rather than tokens the legitimate auth server minted for this user).
- **R2.** Detect tokens minted for a different developer (cross-user injection) and reject — specifically, the alias the auth server attests in the JWT must match the alias the callback advertised.
- **R3.** Preserve the dual-credential routing invariant from [`docs/solutions/integration-issues/cli-dev-portal-dual-credential-pattern.md`](../solutions/integration-issues/cli-dev-portal-dual-credential-pattern.md). Any new gateway endpoint call (Approach A or D) must route through `devKeysClient`.
- **R4.** Composability contract from [`docs/solutions/architecture/cli-composability-audit-and-fix.md`](../solutions/architecture/cli-composability-audit-and-fix.md): validation failure returns `*apierr.AuthError` (exit code 3); `--output json` emits structured error envelope to stderr; token bodies never appear on stdout or stderr.
- **R5.** No regression on the headless `dev login --skey` path. The validation layer applies to the browser flow only, OR applies to both with no observable behavior change for the headless path (its JWT comes from the same gateway endpoint and is already trusted by definition).
- **R6.** No regression on the existing test suite shipped in PR #101 — the 23 `dev_test.go` browser-flow + dispatcher tests must continue to pass with the new validation in place.
- **R7.** The chosen approach must be honest about its threat model — defending the inbound callback path while accepting same-UID at-rest credential exposure is the explicit posture documented in the Problem Frame.

## Scope Boundaries

- **No implementation lands from this plan.** This is the decision document; implementation gets its own plan after the decision.
- **Does not re-litigate PR #101's scope.** The flow itself (listener, CSRF state, `sanitizeCallbackValue`, `persistDevSession`, read-modify-save) stays as shipped. Validation is additive.
- **Does not address at-rest credential protection** — see the threat model framing above. That's a different plan.
- **Does not address SIGINT/listener-teardown hardening** ([#103](https://github.com/Andamio-Platform/andamio-cli/issues/103)), concurrent-`dev login` clobbering ([#105](https://github.com/Andamio-Platform/andamio-cli/issues/105)), or expired-JWT auto-refresh hint ([#97](https://github.com/Andamio-Platform/andamio-cli/issues/97)). Those are independent follow-ups that may or may not bundle with the chosen approach's PR.

### Deferred to Separate Tasks

- **Callback transport reconciliation:** [andamio-app-v2#700](https://github.com/Andamio-Platform/andamio-app-v2/issues/700). Pre-requisite to ANY work in this plan landing.
- **Per-approach implementation plan:** written after the decision in this plan is made.

## Context & Research

### The current state of `runDevLoginBrowser` (post-PR #101)

`cmd/andamio/dev.go` — `runDevLoginBrowser` shape relevant to where validation hooks in:

1. Pre-flight: API key + HasDevAuth.
2. Listener + CSRF state.
3. GET callback handler — state validation FIRST (SEC-001 fix), then field parsing with `sanitizeCallbackValue`, then required-field validation (`dev_jwt`, `dev_refresh_token`, `alias` non-empty).
4. Success result delivered via capacity-1 channel.
5. Read-modify-save: fresh `config.Load()`, build synthetic `secureLoginResponse` via field-assignment, call `persistDevSession`.

**The natural validation hook lives between step 4 (callback success result received, all fields sanitized and non-empty) and step 5 (read-modify-save persistence)** — at the equivalent of dev.go around lines 660-678. The implementation sketches below all reference this insertion point.

### Gateway state (relevant to Approaches A, B, D)

- `/.well-known/jwks.json` **already exists** on andamio-api — `internal/router/router.go:156` registers it as unauthenticated, single-key (kid `andamio-api-attestation-key`, alg RS256), standard JWKSResponse with base64url-encoded N+E. This is a complete, usable JWKS endpoint for offline verification today.
- `auth.ValidateJWT(tokenString string) (*Claims, error)` at `andamio-api/internal/auth/jwt.go:171` — full RS256 parse + claims validation. The `Claims` struct carries `UserID`, `Alias`, `Tier`, `TierID`, plus standard registered claims (`iss: andamio-api`, `aud: paid-api`, `sub`, `exp`).
- **`wallet_address` is NOT in the JWT claims** — the JWT was minted from the wallet-signed nonce flow but does not carry the wallet address back. Approach C (which wants to bind to the wallet address) needs to source the address from the callback `address` field, NOT from the JWT.
- **`SecureLoginResponse` also does not carry wallet_address** — only `UserID`, `Alias`, `Tier`, `JWT`, `RefreshToken`. The App-side `dev-cli-auth-flow.tsx` sends `address` in the callback payload but the CLI's `devAuthCallbackResult` struct (shipped in PR #101) silently drops it (no `Address` field).

### Library landscape

- **andamio-cli has zero JWT-parsing code today** — no imports of `golang-jwt/jwt`, `lestrrat-go/jwx`, `coreos/go-oidc`. Adding any of Approach A, B, or D requires a new dependency.
- **andamio-api uses `github.com/golang-jwt/jwt/v5`** for RS256 sign+verify.
- **External research recommends `lestrrat-go/jwx`** (v2 stable, v4 released Apr 2026 with generics and quantum-ready algorithms) for the CLI side. `jwk.Fetch` for a one-shot CLI pattern is the natural fit. Auth0 CLI uses this library.
- **`github.com/auth0/go-jwt-middleware/v2/jwks`** pairs with `golang-jwt/jwt/v5` if matching the andamio-api dependency is preferred — but the JWKS layer is thinner.

### Institutional learnings

- **[`cli-dev-portal-dual-credential-pattern.md`](../solutions/integration-issues/cli-dev-portal-dual-credential-pattern.md):** any new gateway endpoint added under `/v2/auth/developer/*` or `/v2/apikey/developer/*` inherits the dual-credential middleware stack and must route through `devKeysClient`. Applies directly to Approach A.
- **[`gateway-status-code-drift-409-vs-400.md`](../solutions/integration-issues/gateway-status-code-drift-409-vs-400.md):** the gateway does not always return REST-conventional status codes. Before any approach writes `errors.As(err, &authErr)` dispatch on a new endpoint's failure path, the actual HTTP status code must be cross-checked against the gateway handler implementation — not assumed.
- **[`cli-composability-audit-and-fix.md`](../solutions/architecture/cli-composability-audit-and-fix.md):** `*apierr.AuthError` for validation failures; `--output json` envelope contract; token-leak guard on stdout AND stderr (the plan for PR #101 already enforces this; the new validation step inherits the guard).
- **[`cli-security-hardening-input-validation.md`](../solutions/security-issues/cli-security-hardening-input-validation.md):** `ValidateBaseURL` allowlist already covers `*.andamio.io` + localhost. A JWKS fetch from `preprod.api.andamio.io` is allowed by default. An App endpoint at `*.app.andamio.io` is also allowed.

### External best-practice consensus

From RFC 8252 (2017), RFC 6819 §4.1.4, RFC 9700 §4.6 (January 2025), and prior art in `gh`, `aws sso login`, Auth0 CLI:

- **The IETF position:** local-machine adversaries are out of scope for OAuth. CLI tools defending against this class are doing belt-and-braces beyond what specs require.
- **PKCE solves authorization code interception, not direct token injection.** It only applies if the flow returns a `code` (not a JWT) in the callback. Inapplicable to the current PR #101 shape; would require restructuring to Approach D.
- **No major CLI does client-side JWT signature verification of received tokens.** `gh`, `aws sso login`, Auth0 CLI all trust the token endpoint response (and they all use code flow, which doesn't have the direct-token-injection class because the token is fetched server-to-server post-callback).
- **HMAC binding (Approach C variant) is a real defense if the secret is the API key, not a hardcoded string** — hardcoded or fetchable secrets are theater because the attacker who can read the state token can also extract/fetch the secret. API key as HMAC input makes the threat model coherent: attacker has `state` from CI logs but not the API key (different exposure paths).

## Key Technical Decisions

These are the decisions framing the matrix below — not the matrix itself.

- **Defense-in-depth is honest, not redundant.** Even though same-UID adversaries can read the at-rest config directly, the CI-log-leakage threat model is real, narrow, and worth defending against if the cost is reasonable.
- **Code flow + PKCE (Approach D) is the structurally correct answer.** Industry consensus is unambiguous. Every alternative is defense-in-depth on a fundamentally fragile callback pattern. **This plan should not pretend otherwise.** The matrix below makes the case for D explicitly.
- **Approach B (CLI-side JWKS verification) is the right tactical answer if D is too expensive to ship now.** JWKS already exists; one dependency add; no cross-repo coordination. Catches the "attacker injects a self-signed JWT" class which is the most likely low-sophistication attack.
- **Approach C is only meaningful with the API key as the HMAC input.** Any other secret-management story (hardcoded, fetched-on-demand) is theater. The API-key-as-secret version is genuinely useful for the CI-log-leakage threat model.
- **Approach A standalone (introspection endpoint) is the weakest of the four.** It requires andamio-api work AND a network round-trip per login AND doesn't close the threat class structurally (attacker still controls what's in the callback; the endpoint just re-validates). Only worth it if combined with another approach OR if introspection is wanted for other reasons (audit, observability).
- **The decision is not purely technical** — it depends on andamio-api roadmap, willingness to ship cross-repo work, and how seriously the platform team weights the CI-log-leakage threat. The matrix surfaces these as decision criteria, not the answer.

## High-Level Technical Design

> *This illustrates the four approaches as a comparative decision matrix. Each row is a candidate approach; columns are the dimensions the platform team should weigh. Sketches further below show what the implementation plan would look like for each.*

### Decision matrix: four approaches to dev-JWT defense-in-depth

| Dimension | A: Introspection endpoint | B: CLI-side JWKS verification | C: HMAC binding (API key as secret) | D: Code flow + PKCE |
|---|---|---|---|---|
| **Threat closed** | Attacker-forged JWT (server says no) | Attacker-forged JWT (signature fails) | Attacker can't forge `?sig=` without API key | **Token injection class entirely** (no token in callback) |
| **Repos that change** | andamio-cli + andamio-api | **andamio-cli only**¹ | andamio-cli + andamio-api + andamio-app-v2 | andamio-cli + andamio-api + andamio-app-v2 |
| **New deps in CLI** | None | `lestrrat-go/jwx/v2` or pair `golang-jwt/jwt/v5` + JWKS fetcher | `crypto/hmac` + `crypto/sha256` (stdlib) | `crypto/sha256` + base64url (stdlib); PKCE codes |
| **New endpoints on gateway** | 1 new: `POST /v2/auth/developer/token/introspect` | None — JWKS already exists | None (HMAC computed during existing `/login/complete`) | 1 new + 1 modified (breaking): new `POST /v2/auth/developer/token/exchange`; existing `/login/complete` becomes dual-mode (legacy direct-token shape for headless `--skey` callers, new code-only shape for browser callers) |
| **New routes on App** | None | None | Pass `?sig=` in callback (additive to current callback construction) | Change callback to deliver `code` not JWT — substantial App refactor |
| **CI/CD-log threat closed?** | Yes (server says JWT is invalid) | Partial — only if attacker forges signature; if attacker has a valid JWT for another user, B detects it via alias mismatch | Yes (attacker doesn't have the API key to forge `?sig=`) | Yes (code is single-use, exchanged server-to-server) |
| **Same-UID malware threat closed?** | No (out of scope per threat model — adversary reads `~/.andamio/config.json` directly) | No (same as above) | No (same as above) | No (same as above) |
| **Per-login latency** | +1 network round-trip | +1 network round-trip (first call), then 0 (cached JWKS) | 0 (HMAC computed inline) | +1 round-trip (code exchange) |
| **Key-rotation friction** | None (server is source of truth) | JWKS cache invalidation on `kid` mismatch (lestrrat-go handles) | Rotating the API key invalidates in-flight callbacks | None (PKCE verifier is per-login) |
| **Effort: andamio-cli** | Small (one POST call, AuthError dispatch) | Medium (new dep, JWKS fetch, signature verify, claim cross-check) | Medium (HMAC computation, callback field addition, claim cross-check) | Large (code-flow refactor, exchange call, PKCE verifier generation) |
| **Effort: andamio-api** | Medium (new endpoint, handler, tests) | Zero | Small (compute `H(apiKey, state\|alias\|jwt)` in `/login/complete` response) | Large (code generation + storage, exchange endpoint, expire codes after 60s) |
| **Effort: andamio-app-v2** | Zero | Zero | Small (echo `?sig=` from gateway response in callback URL) | Large (callback construction overhaul, no longer carries JWT) |
| **Wait time (cross-repo coordination)** | Weeks (andamio-api roadmap) | **Days (CLI-only)** | Weeks (3-repo coordination) | Weeks-to-months (3-repo coordination, biggest scope) |
| **Industry precedent** | Common (Auth0, AWS Cognito, Okta all expose introspection) | Common (most JWT libraries support JWKS verification) | Less common (mostly used in webhook signing; rare in CLI auth) | **Industry standard** (`gh`, `aws sso login`, Auth0 CLI, Snyk, 1Password) |
| **Closes problem structurally vs defense-in-depth?** | Defense-in-depth | Defense-in-depth | Defense-in-depth | **Structural** |
| **Net recommendation** | Standalone: weak. Combined with B: redundant. Combined with C: belt-and-braces. | **Tactical winner if D is too expensive now.** Catches low-sophistication attack; cheap to ship; CLI-only.¹ | Genuinely strong against the CI-log threat model. Requires the most coordination of the defense-in-depth options. | **Architecturally correct.** Worth the cost if the team is willing to invest in cross-repo work. Closes the class for good. |

¹ Approach B is "CLI-only" assuming the callback transport question in [andamio-app-v2#700](https://github.com/Andamio-Platform/andamio-app-v2/issues/700) resolves in favor of GET + query params (the contract this plan assumes). If POST + JSON wins instead, B's verification hook still has the same shape, but the CLI's callback parsing path changes — work that the PR shipping B would have to absorb. Approach B remains the least coordination-heavy of the four under either resolution, but it is not fully insulated from #700.

### The honest recommendation

If we had unlimited time: **Approach D** (code flow + PKCE) is what every comparable CLI does and is the only approach that closes the token-injection class structurally rather than incrementally.

If we have limited time and want to ship something useful in the next release: **Approach B** (CLI-side JWKS verification). It's CLI-only, the JWKS endpoint already exists, and it catches the most likely attack (attacker signing a JWT with their own key). It does NOT catch an attacker who has a valid JWT for their own account and is injecting it cross-user; for that, alias cross-check (also part of B's implementation) is necessary.

The matrix surfaces the trade-offs but does not pick. **The decision is the platform team's** — it depends on andamio-api roadmap signal, willingness to invest in App-side work, and how the security-vs-velocity tradeoff is weighted right now.

## Implementation Sketches (Once a Decision Is Made)

For each approach, the implementation plan that would follow:

### If Approach A is chosen

**Scope:** CLI + andamio-api.

**Implementation units:**
1. andamio-api: Add `POST /v2/auth/developer/token/introspect` endpoint. Handler calls existing `auth.ValidateJWT` (already in place). Response: `{valid: bool, alias: string, user_id: string, tier: string, expires_at: string}`. Auth: dual-credential (`X-API-Key` + dev JWT being introspected) — sit it behind `developerJWTAuth` so attackers can't probe arbitrary JWTs. Tests + Swagger annotations.
2. andamio-cli: Add introspection call in `runDevLoginBrowser` between callback success and `persistDevSession`. Route through `devKeysClient`. Cross-check `response.alias == callbackResult.Alias`. Mismatch → `*apierr.AuthError`. Tests pinning the cross-check + the new gateway 4xx error contract.
3. CLAUDE.md auth narrative + CHANGELOG.

**Risk to call out in the eventual implementation plan:** the introspection endpoint's HTTP status codes on validation failure must be verified against the actual handler implementation, not assumed (gateway-status-code-drift learning).

### If Approach B is chosen

**Scope:** CLI only.

**Implementation units:**
1. Add `lestrrat-go/jwx/v2` to `go.mod`. Justification in CLAUDE.md.
2. New `internal/jwt` package: `Verify(rawJWT, jwksURL, allowedKid string) (*Claims, error)` — wraps `jwk.Fetch` + `jws.Verify` + claim extraction. JWKS fetched per-CLI-invocation (CLI process is short-lived; no point caching across invocations). Cross-check `iss == "andamio-api"` and `aud` includes `"paid-api"`.
3. Add verification call in `runDevLoginBrowser` between callback success and `persistDevSession`. Cross-check JWT claims (`alias`, `sub`) against callback fields (`Alias`, `DevID`). Mismatch → `*apierr.AuthError` mentioning the specific mismatch.
4. Test scenarios: valid JWT happy path, invalid signature, expired token, alias mismatch, JWKS endpoint unreachable, malformed JWT. Token-leak guards across all.
5. CLAUDE.md auth narrative + CHANGELOG.

**Risk to call out:** JWKS-endpoint-unreachable failure mode. If preprod JWKS is down at login time, the entire `dev login` flow fails. Decide: hard-fail (strict security posture) or soft-fail with a warning (availability over hardness). Recommend hard-fail for security consistency; document as a known operational coupling.

### If Approach C is chosen (API-key-as-HMAC-secret variant)

**Scope:** CLI + andamio-api + andamio-app-v2.

**Implementation units:**
1. andamio-api: In `/v2/auth/developer/login/complete` handler, after minting the dev JWT, compute `sig = HMAC-SHA256(api_key, "v1|" + state + "|" + alias + "|" + jwt_hash + "|" + expiry)`. Return `sig` as a new field in `secureLoginResponse`. The API key here is the developer's API key from the request's `X-API-Key` header.
2. andamio-app-v2: Echo the `sig` field into the callback URL as `&sig=...`. No App-side cryptography needed.
3. andamio-cli: Re-compute the same HMAC using the API key from the CLI's config; constant-time compare against `?sig=` from callback. Mismatch → `*apierr.AuthError`. Add `sig` field to `devAuthCallbackResult` struct + callback handler parsing.
4. Tests: valid HMAC happy path, mismatched HMAC, missing `sig` field, expired (HMAC includes expiry to make replay harder). Token-leak guards.
5. CLAUDE.md auth narrative + CHANGELOG. Cross-repo coordination notes.

**Risk to call out:** if the API key is rotated between callback issuance and callback receipt (5-minute window), the HMAC fails to verify and the user has to retry. Acceptable trade-off.

### If Approach D is chosen (code flow + PKCE — the structural fix)

**Scope:** CLI + andamio-api + andamio-app-v2. Substantial refactor of all three.

**Implementation units:**
1. andamio-api: Add `POST /v2/auth/developer/token/exchange` endpoint. Takes `{code, code_verifier}`. Looks up the code in a short-TTL store (Redis or in-memory), verifies the code_verifier against the stored code_challenge per RFC 7636, mints+returns the JWT pair. Codes expire after 60 seconds and are single-use.
2. andamio-api: Modify `/v2/auth/developer/login/complete` to NOT return the JWT pair anymore. Instead, return a `code` + the alias + tier. The wallet-signed nonce gates the code issuance; the code gates the JWT issuance.
3. andamio-app-v2: Modify callback construction to deliver `code` (not `dev_jwt`/`dev_refresh_token`) in the callback URL.
4. andamio-cli: Generate `code_verifier` (random 43-128 char string) + `code_challenge` (`base64url(SHA256(code_verifier))`). Pass `code_challenge` and `code_challenge_method=S256` in the auth URL. On callback, receive `code`. Exchange via the new endpoint with the `code_verifier`. Persist the returned JWT pair.
5. Backward compatibility: the headless `dev login --skey` flow still calls `/v2/auth/developer/login/complete` and still receives the JWT pair directly (skey flow doesn't go through the browser). Keep the old response shape supported on the endpoint with a query param or content-negotiation header indicating "legacy direct-token mode."
6. Tests: PKCE happy path, code_verifier mismatch, code reuse (single-use enforcement), code expiry, the legacy headless path still works.
7. CLAUDE.md auth narrative substantial rewrite + CHANGELOG `### Changed` (breaking change for any third-party tool that was consuming the legacy callback shape — though there shouldn't be any).

**Risk to call out:** this is a significant architectural change with a long coordination window. The legacy headless flow has to keep working through the transition. Worth doing only if the team is committed to the security investment.

## Open Questions

### Resolved During Planning

- **Is the threat model coherent given at-rest exposure?** Yes — for the CI-log-leakage scenario specifically. Same-UID malware is out of scope per RFC 6819 §4.1.4 and is documented as accepted. The plan recommendation will not pretend otherwise.
- **Does PKCE solve direct-JWT injection without restructuring?** No — PKCE binds code exchange. Direct-JWT-in-callback has no exchange step for PKCE to gate. PKCE only applies if we restructure to code flow (Approach D).
- **Does andamio-api already expose JWKS?** Yes — `/.well-known/jwks.json` is live, single-key, unauthenticated. Approach B doesn't need any API changes.
- **Is `wallet_address` in the dev JWT?** No — claims are `UserID`, `Alias`, `Tier`, `TierID` + registered. Approach C cannot bind to the wallet address from the JWT; the address is only in the callback's `address` field (which the CLI currently drops).
- **Are we sure no other CLI does client-side JWT verification of received tokens?** Verified — `gh`, `aws sso login`, Auth0 CLI all trust the token endpoint response. They all use code flow, which is why they don't need to. We do not have code flow → we're the outlier in even considering Approach B.

### Deferred to Implementation (per-approach)

- **Approach A:** exact endpoint path (`/v2/auth/developer/token/introspect` vs `/introspect` vs nested elsewhere); whether the endpoint requires the dev JWT itself or accepts any JWT; rate-limiting.
- **Approach B:** JWKS cache strategy for in-process state if a CLI invocation makes multiple verifications (unlikely; default to per-call fetch); choice of `lestrrat-go/jwx/v2` vs `golang-jwt/jwt/v5` + companion JWKS fetcher; library version pin.
- **Approach C:** exact HMAC input format (`v1|...` prefix for versioning, separator choice); whether to bind expiry into the HMAC; how to expose the secret to andamio-api (it has the API key from the request already, no rotation issues).
- **Approach D:** code storage backend (Redis vs in-memory map with expiry), code length, code_challenge_method support beyond S256.

### Decision Question for the Platform Team

**The one question this plan exists to answer:** Which approach (or combination) do we ship, and on what timeline?

The matrix surfaces the trade-offs. The decision criteria are:

1. **Security posture target.** Defending the inbound callback path for the CI-log-leakage threat = any of A/B/C/D. Closing the token-injection class structurally = only D.
2. **Velocity.** B ships fastest (days, CLI-only). A and C ship in weeks (cross-repo). D ships in months.
3. **Long-term architecture.** D aligns with industry consensus and is the only approach that doesn't need to be revisited.
4. **Resource availability.** Does andamio-api have bandwidth for A/D? Does andamio-app-v2 have bandwidth for C/D?

The plan recommendation, with the honest caveats above: **B as the tactical winner for the next release; D as the strategic target for a future release.** A standalone is weaker than B for similar cost. C is genuinely strong but requires the most coordination.

## System-Wide Impact

- **Interaction graph:** The validation hook lives inside `runDevLoginBrowser`'s callback-success path. Downstream consumers (`devKeysClient`, every `dev keys *` and `apikey usage|profile` command) inherit the validated JWT — no changes to their code, but the JWT they read from disk is now provably gateway-attested (or rejected before persistence).
- **Error propagation:** All approaches funnel validation failures through `*apierr.AuthError` (exit code 3). `--output json` error envelope already specified in PR #101's plan.
- **State lifecycle risks:** None — validation happens BEFORE `persistDevSession`. A rejected JWT is never written to disk; the slot stays whatever it was before the failed `dev login`.
- **API surface parity:** Approaches A and D add new gateway endpoints. The dual-credential routing learning means any CLI call to these endpoints must go through `devKeysClient`.
- **Integration coverage:** The validation step has a clear contract (input: parsed callback result; output: validated `*secureLoginResponse` or `*apierr.AuthError`). Unit tests cover each approach's specifics; the integration with `persistDevSession` is unchanged.
- **Unchanged invariants:** The headless `dev login --skey` flow does not go through this validation layer (Approaches A, B, C) or continues to use the legacy direct-token path (Approach D). The user-login browser flow is completely separate and unaffected. The dual-credential routing through `devKeysClient` is unchanged.

## Risks & Dependencies

| Risk | Mitigation |
|------|------------|
| Callback transport mismatch (CLI uses GET + query; App in flight uses POST + JSON) — pre-requisite to any approach | Resolved via [andamio-app-v2#700](https://github.com/Andamio-Platform/andamio-app-v2/issues/700). This plan assumes #700's GET + query params win. If POST + JSON wins, every approach's integration point shifts but the matrix stays valid. |
| Approach B JWKS endpoint unreachable at login time → hard-fail blocks all `dev login` invocations | Decide at implementation time: hard-fail (strict security) vs soft-fail with warning (availability). Recommendation: hard-fail; document the operational coupling. |
| Approach C: API key rotation between callback issuance and receipt invalidates HMAC | Accept — 5-min window is short; user retries with new API key. Document. |
| Approach D: substantial cross-repo coordination cost; may take months | Accept if chosen — D is the strategic investment, not a tactical fix. Maintain B (or interim defense) in parallel until D ships. |
| Defending callback path while leaving at-rest credentials at 0600 → asymmetric posture | Explicit acceptance per RFC 6819 §4.1.4. Threat model documented in Problem Frame. This is consistent with industry practice. |
| Gateway HTTP status code drift on new endpoints (A, D) | Cross-check the gateway's actual status codes against the handler implementation before writing CLI-side `errors.As(err, &authErr)` dispatch. Per [`gateway-status-code-drift-409-vs-400.md`](../solutions/integration-issues/gateway-status-code-drift-409-vs-400.md). |
| Adding `lestrrat-go/jwx` (Approach B) is a new dependency surface | Vetted library (widely used in Go OAuth/OIDC stack, used by auth0-cli). Version pin in go.mod; review v4 migration when natural opportunity arises. |
| Approach D legacy-path compatibility: headless `dev login --skey` must keep working through the transition | Endpoint serves both shapes (legacy direct-token vs new code-only) via content-negotiation or query param. Documented as part of D's plan. |
| Cross-repo plans can drift — App ships before CLI is ready, or vice versa | Approach B is the least cross-repo-coupled (CLI-only assuming #700 resolves to GET + query params; see footnote ¹ on the matrix). For A/C/D, the implementation plan must include a sequencing section with explicit "land in this order" steps. |

## Documentation / Operational Notes

- After a decision is made, this plan should be referenced from the chosen approach's implementation plan with `(see decision matrix: docs/plans/2026-05-25-001-feat-dev-jwt-validation-decision-matrix-plan.md)` so the trade-off context isn't lost.
- The accepted threat model (CI-log-leakage in scope; same-UID malware out of scope) should be added to CLAUDE.md's Auth Flow section once an approach ships — it's currently unstated but important for future contributors.
- If Approach D is chosen, the existing [`cli-dev-portal-dual-credential-pattern.md`](../solutions/integration-issues/cli-dev-portal-dual-credential-pattern.md) solution doc will need an update — the new exchange endpoint becomes a third surface in the dual-credential family.

## Sources & References

- **Origin document:** GitHub issue [andamio-cli#102](https://github.com/Andamio-Platform/andamio-cli/issues/102)
- **Pre-requisite:** [andamio-app-v2#700](https://github.com/Andamio-Platform/andamio-app-v2/issues/700) (callback transport reconciliation)
- **Parent feature plan:** [`docs/plans/2026-05-22-001-feat-browser-based-dev-login-plan.md`](2026-05-22-001-feat-browser-based-dev-login-plan.md) (PR #101)
- **Related issues:** [#103](https://github.com/Andamio-Platform/andamio-cli/issues/103) (listener double-close), [#105](https://github.com/Andamio-Platform/andamio-cli/issues/105) (concurrent `dev login` flock), [#104](https://github.com/Andamio-Platform/andamio-cli/issues/104) (`dev refresh` test gap), [#97](https://github.com/Andamio-Platform/andamio-cli/issues/97) (expired dev JWT auto-refresh hint)
- **Code references (andamio-cli):** `cmd/andamio/dev.go` (`runDevLoginBrowser`, `devAuthCallbackResult`, `persistDevSession`); `cmd/andamio/dev_test.go` (23 browser-flow + dispatcher tests to preserve)
- **Code references (andamio-api):** `internal/router/router.go:156` (JWKS endpoint registration); `internal/auth/jwt.go:171` (`ValidateJWT`); `internal/auth/jwt.go:15-21` (`Claims` struct); `internal/middleware/developer_jwt_middleware.go:55-83` (current JWT validation in middleware)
- **Code references (andamio-app-v2):** `src/app/auth/dev-cli/dev-cli-auth-flow.tsx:154` (current POST callback construction — to be reconciled via #700); `src/types/cli-auth-params.ts:60-68` (current `DevCliSuccessPayload` shape)
- **Institutional learnings:**
  - [`docs/solutions/integration-issues/cli-dev-portal-dual-credential-pattern.md`](../solutions/integration-issues/cli-dev-portal-dual-credential-pattern.md)
  - [`docs/solutions/integration-issues/cli-apikey-auth-isolation-and-content-404-ux.md`](../solutions/integration-issues/cli-apikey-auth-isolation-and-content-404-ux.md)
  - [`docs/solutions/integration-issues/gateway-status-code-drift-409-vs-400.md`](../solutions/integration-issues/gateway-status-code-drift-409-vs-400.md)
  - [`docs/solutions/architecture/cli-composability-audit-and-fix.md`](../solutions/architecture/cli-composability-audit-and-fix.md)
  - [`docs/solutions/security-issues/cli-security-hardening-input-validation.md`](../solutions/security-issues/cli-security-hardening-input-validation.md)
- **External references:**
  - [RFC 8252: OAuth 2.0 for Native Apps](https://www.rfc-editor.org/rfc/rfc8252.html) — §7.3 loopback redirects, §8.3 same-machine adversaries
  - [RFC 9700: Best Current Practice for OAuth 2.0 Security (Jan 2025)](https://datatracker.ietf.org/doc/rfc9700/) — §4.6 access token injection, §4.15 confused-deputy
  - [RFC 6819: OAuth 2.0 Threat Model](https://datatracker.ietf.org/doc/html/rfc6819) — §4.1.4 platform-vs-OAuth boundary
  - [RFC 7636: PKCE](https://datatracker.ietf.org/doc/html/rfc7636) — applicable only if restructuring to code flow (Approach D)
  - [`cli/oauth` (GitHub CLI)](https://github.com/cli/oauth) — reference implementation of code-flow + loopback in Go
  - [AWS CLI PKCE migration (Nov 2024)](https://aws.amazon.com/blogs/developer/aws-cli-adds-pkce-based-authorization-for-sso/) — case study of moving from device flow to code flow + PKCE
  - [`lestrrat-go/jwx`](https://github.com/lestrrat-go/jwx) — recommended JWKS library for Approach B
