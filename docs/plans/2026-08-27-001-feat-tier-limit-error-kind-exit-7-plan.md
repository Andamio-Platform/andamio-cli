---
title: "feat: `tier_limit` error kind + exit 7 for plan/entitlement refusals"
type: feat
status: active
date: 2026-08-27
---

# feat: `tier_limit` error kind + exit 7 for plan/entitlement refusals

## Summary

Add a ninth error kind, `tier_limit` (exit **7**), for refusals whose remedy is billing-side — revoke, upgrade, subscribe — rather than retry or re-auth. Classification keys on the gateway's body code `tier_limit_exceeded`, decoded from the error envelope on any 4xx *before* the status switch, so the ruled 429→403 change (product-circle#304) needs no CLI release. `dev keys create` gains a CLI-authored remedy line in text mode; every other 429 stays `backpressure`. The exit-code table is updated in every place it is documented, and the stale comment / test stub the issue names are fixed.

---

## Problem Frame

Issue #159 (realizing product-circle#321, decisions 1–6) blocks #128: 1.0 publishes the exit-code table as a stability contract, and today a tier-capped `dev keys create` returns 429, which `internal/client.statusError` maps to `BackpressureError` — documented as "retry later". That can never come true: no amount of waiting frees a key slot. A script branching on `kind` retries a hard cap; a human reads "backpressure" and waits. The fix must survive the gateway changing the status for this condition, which is why classification keys on the body code, not the status.

Verified against `andamio-api` source during planning: `checkTierCap` in `internal/handlers/v2/keys_handlers/keys_handlers.go` runs **before** the mainnet/preprod env switch and emits `NewHTTPErrorWithCode(429, "tier_limit_exceeded", "maximum API key limit (N) reached for your tier; revoke an existing key or upgrade your subscription")`. `WriteErrorResponse` (`internal/errors/errors.go`) renders it as `{"error":{"code":"tier_limit_exceeded","message":"...","details":""}}`. The legacy `GenerateNewAPIKey` cap re-check on the mainnet path (`customErrors.TooManyRequestsError`, no code) can only fire on a race between the upstream check and the insert — see Scope Boundaries.

---

## Assumptions

*This plan was authored without synchronous user confirmation. The items below are agent inferences that fill gaps in the input — un-validated bets that should be reviewed before implementation proceeds.*

- The issue's remedy line names `andamio dev keys revoke <id>`; the shipped command is `dev keys delete <id>`. The plan uses `delete`. Adding a `revoke` alias is deferred, not in scope.
- "Gateway message verbatim" is satisfied by carrying the decoded `error.message` inside the existing `API error <status>: …` prefix convention, with the stable code string kept in the text (`API error 429 (tier_limit_exceeded): maximum API key limit …`). This keeps `TestRunDevKeysCreate_TierLimitExceededBubbles`'s substring contract and the CHANGELOG 0.13 promise that stable codes are "preserved verbatim in error messages". The JSON `error` field therefore reads `create developer key failed: API error 429 (tier_limit_exceeded): <message>` — prefixed exactly like every other kind — not the bare message. If a bare machine-readable message is wanted later, that is a new envelope field and belongs with #136.
- The CLI-authored remedy line is appended at the **command layer** (`dev keys create` only) and **in every output mode except JSON** (mirroring `main.go`'s own JSON-vs-everything-else branch, so `--output csv`/`markdown` users get it too), so the JSON `error` value carries no CLI prose and the typed error stays surface-agnostic (a future `tier_limit` on a different surface must not tell people to revoke keys).

---

## Requirements

- R1. New kind `tier_limit` / exit 7; semantics documented in `andamio help exit-codes` as: the account's current plan does not permit this action; remedy is billing-side (revoke, upgrade, subscribe) — not retry, not re-auth.
- R2. Classification keys on the body code only: `error.code == "tier_limit_exceeded"` on **any 4xx** classifies `tier_limit` before the status switch. Both 429 and 403 are covered by tests using the real nested envelope.
- R3. Every other 429 (no code, or a different code) keeps `backpressure` / exit 1 and remains retryable.
- R4. `tier_limit` is never retried by `PostWithRetry`.
- R5. Text output contains the gateway's remedy sentence **and** one CLI-authored line naming `andamio dev keys list`, `andamio dev keys delete <id>`, and upgrading. JSON output is `{"error": …, "kind": "tier_limit"}` with no new envelope fields.
- R6. `andamio help exit-codes` lists `7  tier_limit`; no row describes a tier cap as retryable. README, `docs/andamio-cli-context.md`, `CLAUDE.md` Failure Contract, and the `main.go` comment block agree.
- R7. `CHANGELOG.md` `[1.0.0]` gains an Added entry for the kind + exit code, and notes that a coded 429 from `dev keys create` moves from exit 1/`backpressure` to exit 7.
- R8. Stale comment in `cmd/andamio/dev_keys.go` (claims 422) fixed; `TestRunDevKeysCreate_TierLimitExceededBubbles` rewritten to the nested envelope; mainnet-path behaviour stated in the PR.
- R9. Existing guards keep passing: `exitcode_test.go`, `retired_test.go`, `TestExitCodes_TextModeCarriesNoKind`, dual-credential tripwires in `dev_keys_test.go`.

---

## Scope Boundaries

- No new JSON envelope fields (`code`, `message`, `remedy`) — structured fields are #136.
- No CLI-side handling of uncoded plan refusals the gateway still emits as bare 429/403 bodies: the legacy mainnet race-path 429 (`too_many_requests`, no code), the legacy 403 "subscription expired", and the tier **monthly/daily quota** 429s from `andamio-api/internal/middleware/tier_rate_and_quota_limit.go` (`{"error":"Monthly quota exceeded"}` / `{"error":"Daily quota exceeded"}`, flat, uncoded, on every authenticated route). Under this plan those still classify `backpressure` and are retried — the same wrong remedy for a billing problem. All are gateway-side gaps; the plan records them and a gateway follow-up under product-circle#304 should code them. Do **not** string-match message text to compensate.
- No status whitelist inside 4xx; no treatment of the code on 2xx/5xx (a 5xx body carrying the code stays `server`).
- No `dev keys revoke` alias.
- No environment-aware wording in the remedy line (the tier cap is a union across mainnet + preprod per the gateway comment, so "upgrade" is correct in both envs).

### Deferred to Follow-Up Work

- Gateway: emit `tier_limit_exceeded` (or a sibling plan-refusal code — a product call) from the legacy `GenerateNewAPIKey` cap re-check, the monthly/daily quota middleware, and the subscription-expired 403 — andamio-api, under product-circle#304. That follow-up must also state that `keys_viewmodels.ErrCodeTierLimitExceeded` is consumed verbatim by the CLI exit-code contract and cannot be renamed without a coordinated CLI release.
- `andamio-docs` `content/docs/apps-tooling/cli/index.mdx` exit-code section — tracked with the post-1.0 docs update (andamio-docs#64).

---

## Context & Research

### Relevant Code and Patterns

- `internal/apierr/errors.go` — `Kind*` constants, `Kind(err)` single mapper (unwraps `%w` and `ReportedError`), one struct per kind. `RemovedCommandError{Command, Guidance}` is the precedent for a typed error carrying more than a message.
- `internal/client/client.go` — `statusError(status, body)` receives the **full raw body** (`io.ReadAll` in all four verbs); `truncateErrorBody` (500 bytes) is applied only when building `Message`. `parseRetryAfterSeconds(body)` is the precedent for parsing the body at construction time. Message format `API error %d: %s` is shared by every branch.
- `internal/client/retry.go` — `isRetryable` checks semantic 4xx types (`AuthError`, `NotFoundError`, `ConflictError`) via `errors.As` first, then `BackpressureError` → retry. Comment says semantic types are listed explicitly so nothing leaks into the retry path.
- `cmd/andamio/main.go` — exit switch on `apierr.Kind`; JSON envelope is `map[string]string{"error","kind"}`; text mode prints `err` to stderr. Never inspects error fields.
- `cmd/andamio/dev_keys.go` — `runDevKeysCreateFlow` wraps with `create developer key failed: %w`; the 404 rewrite in the delete flow (`dev_keys.go` ~line 369) is the precedent for a command-layer, surface-specific error message.
- `cmd/andamio/helpers.go` — home for shared cmd-layer helpers (per `docs/solutions/architecture/cmd-package-helper-placement-and-output-consistency.md`).
- Tests: `internal/apierr/errors_test.go` (`TestKind_ClassifiesEachTypedError` table), `internal/client/client_test.go` (`TestClient_StatusCodeToTypedError` table mirrors the switch order), `internal/client/retry_test.go` (`TestRetry_408_425_429_AllRetry`), `cmd/andamio/exitcode_test.go` (`buildTestBinary` + `statusStub` + `runCLI`, `TestExitCodes_AndKindsAgree` table drives `course list --output json`), `cmd/andamio/dev_keys_test.go` (`devKeysGatewayStub{createRespStatus, createRespBody}`, `devKeysTestEnv`).
- Documentation surfaces for the table: `cmd/andamio/exitcodes_help.go`, `cmd/andamio/main.go` doc comment, `README.md` "Exit codes and error kinds", `docs/andamio-cli-context.md` (table + second mention ~line 381), `CLAUDE.md` Failure Contract, `CHANGELOG.md`.
- Gateway (read-only reference, sibling repo `andamio-api`): `internal/handlers/v2/keys_handlers/keys_handlers.go` (`checkTierCap`), `internal/viewmodels/keys_viewmodels/keys_viewmodels.go` (`ErrCodeTierLimitExceeded`), `internal/errors/errors.go` (`WriteErrorResponse`, nested envelope; `code` falls back to a status-derived value when no explicit code is set).

### Institutional Learnings

- `docs/solutions/integration-issues/gateway-status-code-drift-409-vs-400.md` — a typed gate built on an assumed status never fired in production; tests passed because they hand-built the typed error. **Constraint:** at least one test must drive a real HTTP body through `httptest` → `client.Post` → `errors.As`, and the wire shape must be verified against gateway source (done — see Problem Frame).
- `docs/solutions/architecture/go-retry-classifier-and-backoff-patterns.md` — classify with `errors.As` on typed errors, never string-match; parse the body at construction time; add new non-retryable types to the explicit semantic-4xx list; `internal/client` must not import `internal/output` (user-facing hint text lives in the cmd layer).
- `docs/solutions/architecture/cli-composability-audit-and-fix.md` — exit code + `kind` are a public scripting contract; `apierr.Kind` is the only mapper; JSON goes through `json.Marshal`.
- `docs/solutions/integration-issues/cli-dev-portal-dual-credential-pattern.md` — `dev keys create` is dual-credential; the rewritten stub must keep both headers on the wire and keep pre-flight auth failures at exit 3.
- `docs/solutions/architecture/typed-output-envelope-with-gateway-state-fallbacks.md` — tolerant lookup of drifting gateway fields; fall through on any decode miss so nothing currently classified changes unless the code matches exactly.
- `todos/023-pending-p3-typed-error-body-truncation-boundary.md` — decoding the raw body (not the truncated message) sidesteps the truncation-boundary concern it records.

### External References

- None — local patterns are strong; no external research run.

---

## Key Technical Decisions

- **Decode the envelope in `statusError`, before the status switch, on 400–499 only.** This is the one placement that guarantees a coded 429 never becomes `BackpressureError` (and so is never retried) and that a future coded 403 never becomes `AuthError`. Outside 4xx the code is ignored: a 5xx body carrying it stays `server`/retryable.
- **Tolerant decoder, exact code match.** A small helper decodes `error` as *either* an object (`{code,message,details}`, the gateway's current shape) *or* a string (the flat legacy shape seen in `internal/client/testdata`), returning `ok=false` on empty/non-JSON/null/whitespace. Code comparison is exact (`tier_limit_exceeded`); the gateway constant is fixed and case-folding invites false positives. Decode from the raw `body`, never from `truncateErrorBody(body)` — the gateway pretty-prints, and a long `details` could push the closing braces past 500 bytes.
- **`TierLimitError{Status, Code, Message, Details}` with `Error()` = `API error <status> (<code>): <message>[: <details>]`.** Keeps the shared prefix convention, keeps the stable code string in the text (existing test + CHANGELOG contract), and carries the gateway's own remedy sentence verbatim. `Details` appended when non-empty, mirroring the gateway's `HTTPError.Error()`.
- **Remedy line is command-layer and non-JSON-only.** A `helpers.go` helper appends the line to the returned error whenever `output.GetFormat()` is anything other than JSON (the same predicate `main.go` uses to choose stderr prose over the JSON envelope); in JSON mode the error passes through unchanged. Rationale: `main.go` prints `err.Error()` identically in both modes and has no per-type branching — adding one would be a first and would erode `TestExitCodes_TextModeCarriesNoKind`'s "text mode is unchanged" posture. Keeping the hint in `dev_keys.go` also means `TierLimitError` stays surface-agnostic; a future `tier_limit` from another endpoint gets its own hint (or none).
- **`Kind` switch order: `TierLimitError` checked before `auth`/`backpressure`/`conflict`.** Never both in practice (statusError returns one type), but the ordering documents intent and protects against a future wrapper.
- **`isRetryable` lists `TierLimitError` explicitly** in the semantic-4xx block rather than relying on fall-through, per the retry-classifier learning; plus a test asserting zero retries for a coded 429.
- **Exit 7 is additive; no existing kind or code changes.** The only observable behaviour change is a coded 429 moving from exit 1/`backpressure` to exit 7/`tier_limit`, called out in CHANGELOG.

---

## Open Questions

### Resolved During Planning

- Does the mainnet path emit the same code at cap? **Yes** — `checkTierCap` gates both envs before the switch. Only a race can reach the legacy uncoded 429. State this in the PR.
- What does the wire envelope look like? Nested object: `{"error":{"code":"tier_limit_exceeded","message":"…","details":""}}` (verified in `andamio-api/internal/errors/errors.go`).
- Should the decoder also accept the flat shape? **Yes**, tolerantly — it costs nothing and the CLI's own testdata shows the gateway has emitted flat bodies on other routes.
- `revoke` vs `delete`? Use `delete` — it is the shipped command.

### Deferred to Implementation

- Exact helper names (`decodeGatewayErrorCode`, `withTextRemedy`, etc.) and whether the remedy helper takes the line as a string or a func.
- Whether `TestExitCodes_AndKindsAgree` grows a body column or a sibling `statusStubWithBody` helper is added — whichever keeps the table readable.
- Golden files: `surface_test.go` goldens cover command surface and output-struct schemas, not help text; regen only if the build proves otherwise.

---

## High-Level Technical Design

> *This illustrates the intended approach and is directional guidance for review, not implementation specification. The implementing agent should treat it as context, not code to reproduce.*

```mermaid
flowchart TD
    A[HTTP response status + raw body] --> B{status in 400..499?}
    B -- no --> S[existing switch: 5xx → ServerError, else plain]
    B -- yes --> C[decode envelope tolerantly<br/>object or string 'error']
    C -- code == tier_limit_exceeded --> T[TierLimitError<br/>Status, Code, Message, Details]
    C -- anything else / decode miss --> S2[existing switch:<br/>401/403 auth · 404 · 409 · 408/425/429 backpressure]
    T --> K[apierr.Kind → tier_limit]
    K --> M[main.go: exit 7<br/>JSON {error, kind} / text stderr]
    T -. dev keys create, text mode only .-> R[append remedy line:<br/>dev keys list · dev keys delete id · upgrade]
    T --> N[isRetryable → false]
```

---

## Implementation Units

### U1. `TierLimitError` type and `tier_limit` kind

**Goal:** Add the typed error and the kind constant to the taxonomy so exit code and `kind` derive from one mapper.

**Requirements:** R1, R9

**Dependencies:** None

**Files:**
- Modify: `internal/apierr/errors.go`
- Test: `internal/apierr/errors_test.go`

**Approach:**
- New `KindTierLimit = "tier_limit"` constant with a doc comment carrying the ruled semantics (plan does not permit; remedy is billing-side; first member is the API-key cap; future plan-gated refusals reuse it).
- New `TierLimitError{Status int; Code, Message, Details string}`; `Error()` renders `API error <status> (<code>): <message>` plus `: <details>` when non-empty.
- `Kind()` gains the case, placed before `auth`.

**Patterns to follow:**
- `RemovedCommandError` (struct with composed `Error()`), `BackpressureError` (status-carrying), per-type comment convention "main.go maps this to exit code N".

**Test scenarios:**
- Happy path: `TestKind_ClassifiesEachTypedError` table gains `{"tier limit", &TierLimitError{…}, KindTierLimit}`.
- Happy path: `TestKind_UnwrapsThroughErrorfWrapping` gains a wrapped `TierLimitError` → `tier_limit`.
- Edge case: `Error()` with empty `Details` has no trailing `: `; with details, appends them.
- Integration: `Kind` through `ReportedError{TierLimitError}` → `tier_limit`.

**Verification:**
- `apierr` tests pass; the constant exists; no other kind string changed.

---

### U2. Envelope decode in `statusError`, before the status switch

**Goal:** Classify `tier_limit` from the body code on any 4xx, leaving every other classification byte-for-byte unchanged.

**Requirements:** R2, R3

**Dependencies:** U1

**Files:**
- Modify: `internal/client/client.go`
- Test: `internal/client/client_test.go`

**Approach:**
- Add a tolerant decoder over the **raw** body returning `(code, message, details, ok)`. Accept `error` as object `{code,message,details}` or as string (then code = the string, message empty); trim whitespace; `ok=false` for empty, non-JSON, `null`, or blank code.
- In `statusError`: if `400 <= status < 500` and decode ok and code equals `tier_limit_exceeded` exactly → return `TierLimitError{Status, Code, Message (fallback to truncated raw body when empty), Details}`. Otherwise fall through to the existing switch untouched.
- Decoding reads the raw body, but the decoded `Message` and `Details` are still passed through `truncateErrorBody` before being stored — the 500-byte cap exists for log flooding / info leakage and this is the first typed error built from untruncated bytes. Doc comment on the decoder cites the gateway constant `keys_viewmodels.ErrCodeTierLimitExceeded`.
- Update the `statusError` doc comment to name the new precedence.

**Execution note:** Land the decoder with its table test first; then wire it into `statusError`.

**Patterns to follow:**
- `parseRetryAfterSeconds` (body parsed at construction time); `TestClient_StatusCodeToTypedError` table ordering mirrors the switch.

**Test scenarios:**
- Happy path: 429 + nested envelope with the code → `*TierLimitError`, `Message` equals the gateway message, `Code` preserved.
- Happy path: 403 + nested envelope with the code → `*TierLimitError` (forward-compat with product-circle#304).
- Happy path: 429 + flat body `{"error":"tier_limit_exceeded"}` → `*TierLimitError`.
- Edge case: 429 + `{"message":"stub"}` (no code) → `*BackpressureError` (unchanged).
- Edge case: 429 + nested envelope with a **different** code (`too_many_requests`) → `*BackpressureError`.
- Edge case: 422 + `invalid_environment` code → plain error (unchanged).
- Edge case: empty body, HTML body, `{"error":null}`, `{"error":{"code":"  "}}` on 429 → `*BackpressureError`; no panic.
- Edge case: `Tier_Limit_Exceeded` (case differs) → not classified.
- Edge case: body > 500 bytes with a long `details` and the code → still `*TierLimitError` (decode is on the raw body) and the rendered `Error()` text is capped by `truncateErrorBody`.
- Error path: 503 + nested envelope with the code → `*ServerError` (code ignored outside 4xx).
- Integration: an `httptest` server returning 429 + nested envelope, driven through `client.Post` → `errors.As(&TierLimitError)` succeeds (the status-drift learning's mandatory round-trip test).

**Verification:**
- All existing `client_test.go` cases pass unchanged; the new table cases pass.

---

### U3. Retry classifier excludes `tier_limit`

**Goal:** Guarantee `PostWithRetry` never retries a tier cap.

**Requirements:** R4

**Dependencies:** U1, U2

**Files:**
- Modify: `internal/client/retry.go`
- Test: `internal/client/retry_test.go`

**Approach:**
- Add `*apierr.TierLimitError` to the explicit semantic-4xx non-retryable block in `isRetryable`; update the doc comment listing.

**Patterns to follow:**
- Existing `errors.As` checks for `AuthError`/`NotFoundError`/`ConflictError`; `TestRetry_408_425_429_AllRetry` as the sibling shape.

**Test scenarios:**
- Happy path: server returns 429 + coded envelope on every attempt → exactly one request made, error is `*TierLimitError`.
- Edge case: server returns 429 + `{"message":"stub"}` → still retried (regression guard for R3).

**Verification:**
- Retry tests pass; a coded 429 makes one attempt.

---

### U4. `dev keys create` remedy line, stale comment, test stub rewrite

**Goal:** Text mode adds the CLI-authored remedy; JSON is untouched; the mechanical items from the issue are done.

**Requirements:** R5, R8, R9

**Dependencies:** U1, U2

**Files:**
- Modify: `cmd/andamio/dev_keys.go`, `cmd/andamio/helpers.go`
- Test: `cmd/andamio/dev_keys_test.go`

**Approach:**
- Helper in `helpers.go`: given an error and a remedy line, if `errors.As(err, *TierLimitError)` and output format is not JSON, return an error whose text is the original followed by a newline and the line (still wrapping the original with `%w` so `Kind` is unchanged); otherwise return the error unchanged.
- `runDevKeysCreateFlow` error branch: wrap as today, then pass through the helper with the line naming `andamio dev keys list`, `andamio dev keys delete <id>`, and upgrading the subscription.
- Fix the comment at the `Post` error branch: the tier cap is a 429 (ruled to become 403) with a coded envelope, classified by body code to `tier_limit`; `invalid_environment` is 422 and stays a plain error.
- Rewrite `TestRunDevKeysCreate_TierLimitExceededBubbles`: stub body becomes the nested envelope; assert `errors.As(*TierLimitError)`, the code substring still present in `err.Error()`, and that the gateway message text reaches `err.Error()`.

**Patterns to follow:**
- The delete flow's 404 message rewrite in `dev_keys.go` (command-layer, surface-specific wording); "progress to stderr / data to stdout" composability rules; `devKeysGatewayStub` + `devKeysTestEnv`.

**Test scenarios:**
- Happy path (text mode): 429 + nested envelope → error text contains the gateway message, `tier_limit_exceeded`, `dev keys list`, and `dev keys delete`.
- Happy path (JSON mode): same stub with format set to JSON → error text contains the gateway message and **not** `dev keys delete`; `apierr.Kind` is `tier_limit`.
- Happy path: 403 + nested envelope → same classification as 429.
- Edge case: `--output csv` (and markdown) with the coded 429 → error text contains `dev keys delete` (the gate is not-JSON, not text-only).
- Edge case: helper given a non-tier-limit error (e.g. `AuthError`) returns it unchanged in both modes.
- Integration: dual-credential tripwire assertions on the create request remain green (both headers on the wire); pre-flight missing-dev-JWT still yields `AuthError`.

**Verification:**
- `go test ./cmd/andamio -run DevKeys` passes; stale comment gone; the `tier_limit_exceeded` substring contract holds.

---

### U5. End-to-end exit-code guard cases

**Goal:** Lock exit 7 / `tier_limit` into the contract guard, alongside proof that plain 429 still exits 1.

**Requirements:** R2, R3, R9

**Dependencies:** U1, U2

**Files:**
- Modify: `cmd/andamio/exitcode_test.go`

**Approach:**
- Add a stub variant that returns a caller-supplied body (or add a body column to the `TestExitCodes_AndKindsAgree` table with `{"message":"stub"}` as the default).
- New rows: `{"tier limit on 429", 429 + coded body, 7, "tier_limit"}`, `{"tier limit on 403", 403 + coded body, 7, "tier_limit"}`; keep the existing `{"backpressure", 429, 1, "backpressure"}` row so the two 429s are visibly distinct in one table.

**Patterns to follow:**
- `TestExitCodes_UnreachableServiceIsFive` / `TestExitCodes_RemovedCommandIsFour` as the single-code precedent; `runCLI` with the `test-jwt` fixture.

**Test scenarios:**
- Happy path: coded 429 via `course list --output json` → exit 7, `kind` `tier_limit`, non-empty `error`.
- Happy path: coded 403 → exit 7, `tier_limit` (not 3/`auth`).
- Edge case: uncoded 429 → exit 1, `backpressure` (existing row).
- Edge case: text mode with coded 429 → stdout empty, stderr non-empty, no `kind` substring (extends `TestExitCodes_TextModeCarriesNoKind` posture).

**Verification:**
- `go test ./cmd/andamio -run ExitCodes` passes.

---

### U6. Documentation: help topic, README, context doc, CLAUDE.md, main.go comment, CHANGELOG

**Goal:** Every place the table is documented agrees, and the release notes carry the entry.

**Requirements:** R1, R6, R7, R8

**Dependencies:** U1–U5 (wording must match shipped behaviour)

**Files:**
- Modify: `cmd/andamio/exitcodes_help.go`, `cmd/andamio/main.go` (doc comment + switch), `README.md`, `docs/andamio-cli-context.md`, `CLAUDE.md`, `CHANGELOG.md`
- Test: `cmd/andamio/retired_test.go` (`TestExitCodesHelpTopic_IsReachable` substring checks — extend)

**Approach:**
- `main.go`: `case apierr.KindTierLimit: exitCode = 7`; update the comment block listing codes.
- Help topic: add `7  tier_limit  plan does not permit this action — revoke, upgrade or subscribe; not retryable`; keep the `backpressure` row's "retry later" wording (uncoded quota 429s still land there until the gateway codes them — see Scope Boundaries); STABILITY paragraph notes 7 is new and that a coded 429 no longer reports `backpressure`.
- PR description (R8): state the mainnet-path finding — `checkTierCap` gates both envs upstream of the env switch, so mainnet key creation at cap receives the coded envelope; the legacy `GenerateNewAPIKey` re-check is race-only and uncoded.
- README, `docs/andamio-cli-context.md` (both mentions), `CLAUDE.md` Failure Contract table + the `dev keys create` row's error-code list: add the row / mapping.
- CHANGELOG `[1.0.0]` Added: `tier_limit` kind + exit 7 with the ruled semantics, first member is the API-key cap, classification by body code so the ruled status change needs no release, and the behaviour change for a coded 429 (1 → 7). Reference #159 and product-circle#321.

**Test scenarios:**
- Happy path: `TestExitCodesHelpTopic_IsReachable` asserts the `tier_limit` row and `7` appear in the help output.
- Test expectation for prose docs: none — Markdown only; reviewed by eye for agreement across the six surfaces.

**Verification:**
- `andamio help exit-codes` shows the row; grep for `backpressure` across the docs shows no row describing a tier cap as retryable.

---

## System-Wide Impact

- **Interaction graph:** `statusError` is shared by every verb and every command; the pre-switch decode runs on every 4xx body. Only an exact `tier_limit_exceeded` match changes any outcome.
- **Error propagation:** `TierLimitError` → `%w` wrapping in the command layer → `apierr.Kind` → `main.go` exit 7. The remedy line wraps with `%w`, so `Kind` still resolves.
- **State lifecycle risks:** none — no persistence, no config mutation.
- **API surface parity:** `kind` and exit code are additive; JSON envelope schema unchanged. Any surface that later emits the code gets exit 7 automatically but no remedy hint until its command adds one.
- **Integration coverage:** the `httptest` round-trip in U2 and the binary-level cases in U5 cover what unit tests of `Kind` alone would not.
- **Unchanged invariants:** exit codes 0–6 and their kinds; empty result = exit 0; text mode carries no `kind`; retry semantics for uncoded 408/425/429; dual-credential routing in `devKeysClient`; all retired-command guards.

---

## Risks & Dependencies

| Risk | Mitigation |
|------|------------|
| Gateway envelope shape drifts (flat vs nested) and the gate never fires — the exact failure recorded in the 409/400 learning | Decoder accepts both shapes; wire shape verified against gateway source; `httptest` round-trip test with the real nested body |
| Legacy mainnet race path or subscription-expired 403 still classify as `backpressure`/`auth` | Out of CLI scope; stated in PR and CHANGELOG; gateway follow-up under product-circle#304 |
| Remedy line leaks into JSON `error` | Helper is text-mode-gated; U4 JSON-mode test asserts absence |
| A future kind added to `Kind` after `backpressure` for a wrapped tier error | Ordering decision documented in the switch comment |
| Gateway renames `tier_limit_exceeded` alongside the 429→403 change → exact-match gate stops firing and a tier cap regresses to exit 3/`auth` | Deferred gateway follow-up must declare the constant a CLI contract; the CLI decoder's doc comment cites `keys_viewmodels.ErrCodeTierLimitExceeded` |
| Uncoded quota 429s (monthly/daily) still classify `backpressure` and are retried | Out of CLI scope; listed as a gateway gap; help-topic wording does not claim every 429 is transient |
| Doc surfaces drift (six of them) | U6 lists every surface; help-topic guard test extended |

---

## Documentation / Operational Notes

- Scripts branching on `kind == "backpressure"` from `dev keys create` will now see `tier_limit`; called out in CHANGELOG. This is the intended contract correction before the 1.0 tag.
- PR description must state the mainnet-path finding (cap gate is env-agnostic and upstream; legacy re-check is race-only and uncoded).

---

## Sources & References

- Issue: Andamio-Platform/andamio-cli#159; product ruling product-circle#321; status-change ruling product-circle#304; blocks #128; structured fields deferred to #136.
- Related code: `internal/apierr/errors.go`, `internal/client/client.go` (`statusError`), `internal/client/retry.go` (`isRetryable`), `cmd/andamio/main.go`, `cmd/andamio/dev_keys.go`, `cmd/andamio/exitcodes_help.go`, `cmd/andamio/exitcode_test.go`.
- Gateway reference (sibling repo `andamio-api`): `internal/handlers/v2/keys_handlers/keys_handlers.go`, `internal/errors/errors.go`, `internal/viewmodels/keys_viewmodels/keys_viewmodels.go`.
- Learnings: `docs/solutions/integration-issues/gateway-status-code-drift-409-vs-400.md`, `docs/solutions/architecture/go-retry-classifier-and-backoff-patterns.md`, `docs/solutions/architecture/cli-composability-audit-and-fix.md`, `docs/solutions/integration-issues/cli-dev-portal-dual-credential-pattern.md`, `docs/solutions/architecture/typed-output-envelope-with-gateway-state-fallbacks.md`.
- Prior plans: `docs/plans/2026-04-23-001-feat-client-context-retries-plan.md`, `docs/plans/2026-07-27-001-feat-cli-1-0-release-scope-plan.md`.
