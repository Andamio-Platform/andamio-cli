---
title: "feat: context propagation + bounded retries in internal/client"
type: feat
status: completed
date: 2026-04-23
issue: "#65"
deepened: 2026-04-23
---

# feat: context propagation + bounded retries in internal/client

## Overview

`internal/client` is the HTTP layer behind every gateway call in the CLI (60+ call
sites across `cmd/andamio/*.go`). Today it has two reliability gaps surfaced by
PR #63 review:

1. **No context propagation.** `client.Get`/`Post`/`Put` call `http.NewRequest`,
   not `http.NewRequestWithContext`. Cobra's `cmd.Context()` never reaches the
   HTTP layer. A Ctrl-C during a gateway POST cannot cancel the in-flight
   request — the user waits up to the 30s wall-clock `http.Client.Timeout`
   before anything happens. For `register-module`'s two-round-trip recovery
   path, worst case is ~60s of silent non-cancellable waiting.

2. **No bounded retries for transient failures.** A single transient 502/504 or
   TCP reset makes a recovery flow fatal. When an agent retries the full
   command in a tight loop, it hits the same conflict plus the same blip and
   never converges.

This plan adds `context.Context` to the three client methods, wires
`cmd.Context()` through at every call site, and introduces a small retry
helper applied opt-in to idempotent calls — starting with
`lookupTeacherModule` (the clearest motivating case per the issue).

## Problem Frame

The CLI is meant to be scripted and orchestrated by agents that may invoke it
in tight loops. Today:

- Users staring at a hanging register-module cannot interrupt it cleanly.
- Orchestrators that retry the outer command are fighting both the recovery
  path semantics (resurrect the conflict) and the network blip — compounding
  flakiness instead of smoothing it.
- The CLI is the wrong layer for this work to be absent — downstream retry at
  the shell level cannot distinguish a 5xx blip from a 409 "already exists,"
  so conservative scripts either fail fast (bad) or retry everything
  indiscriminately (worse).

**Do-nothing baseline.** No production incident has been filed against these
gaps; the motivation is forward-looking (agent-driven traffic will increase
as `andamio-cli` becomes the default developer entry point) plus the PR #63
manual-test observation that register-module's recovery path is
uncomfortably silent. Shipping this post-R3-content-sync is acceptable;
shipping before scaling agent workloads is strongly preferred.

## Requirements Trace

From issue #65 acceptance criteria:

- **R1.** Client methods accept `context.Context`; cobra handlers pass `cmd.Context()`.
- **R2.** Ctrl-C during a gateway POST cancels the request (verify manually).
- **R3.** Bounded retry wrapper available for idempotent calls; applied to
  `lookupTeacherModule`'s teacher-modules-list call at minimum.
- **R4.** Tests cover: (a) context cancellation propagates,
  (b) retry converges on eventual success, (c) retry gives up after N attempts.

## Scope Boundaries

- **In scope:** `internal/client/client.go`, all 60-ish call sites in
  `cmd/andamio/*.go`, tests in `internal/client/client_test.go` and
  `cmd/andamio/course_teacher_ops_test.go`.
- **Out of scope:**
  - `spec.go`'s standalone `specHTTPClient` (separate http.Client, already has
    its own timeout; not worth touching in this PR).
  - `course_export.go`'s image-download `httpClient.Get(imgURL)` (downloads
    from CDN, not gateway; separate retry semantics).
  - `internal/submit` Cardano submit API (different client, different risk
    profile; a separate issue if needed).
  - Circuit breaker. Issue explicitly excludes this.
  - Retry on 4xx. Issue explicitly excludes this.
  - Fine-grained `http.Transport` tuning (`DialContext`,
    `TLSHandshakeTimeout`, `ResponseHeaderTimeout`). Issue flags it as
    "consider also" — defer unless context work exposes real slow-header
    pain in practice. Noted as a deferred follow-up.

## Context & Research

### Relevant Code and Patterns

- `internal/client/client.go:44-75` — `Get`, uses `http.NewRequest`.
- `internal/client/client.go:88-135` — `Post`, same pattern.
- `internal/client/client.go:137-184` — `Put`, same pattern.
- `internal/client/client.go:60-72` — status→typed-error switch (401/403/404/409
  → typed; 5xx falls through to `errors.New`). Retry predicate needs to run
  *before* this mapping or operate on raw status from a pre-wrapped error.
- `cmd/andamio/main.go:73-76` — `PersistentPreRunE` is the only root-level
  wiring; no signal handling or context wiring is done at the root today. We
  will add `signal.NotifyContext` here so every subcommand inherits a
  cancellable `cmd.Context()`.
- `cmd/andamio/tx_lifecycle.go:75-88` — existing ad-hoc signal handling for tx
  lifecycle. Keep for the tx-progress-specific side effect (print the hash
  before exit), but the new root-level context will be the primary signal path
  for plain HTTP work.
- `cmd/andamio/tx_run.go:196` — `pollTxStatus(ctx, ...)` already takes a
  context but never hands it to `c.Get`. After this plan it finally flows.
- `cmd/andamio/course_teacher_ops.go:348` — `lookupTeacherModule`, the
  retry-target case from the issue.
- `cmd/andamio/course_teacher_ops.go:212-275` — `registerOrRecoverModule`;
  plumbs into its two HTTP calls.
- `internal/client/client_test.go` — `httptest.NewServer`-based tests. Pattern
  extends naturally to context cancellation and retry scenarios.

### Institutional Learnings

- `docs/solutions/` has no prior retry or context-propagation pattern in this
  repo. This is greenfield for the CLI.
- Go standard-library idiom: `context.Context` is always the **first**
  parameter, never stored in a struct, never threaded via globals. Enforced
  by `go vet` and `staticcheck` (`contextcheck`).

### External References

Go standard-library guidance (already internalized in the codebase style —
see `tx_run.go`'s existing ctx-first signature): propagate via parameters,
cancel via `ctx.Done()`, and never block the request path on a non-cancellable
timer. No external docs are load-bearing for this plan.

## Key Technical Decisions

- **Signature change, not overloads.** Add `ctx context.Context` as the first
  parameter of `Get`, `Post`, `Put`. Do not add `GetContext`/`GetCtx` parallel
  methods. The package is `internal/client` — Go enforces that no repo outside
  `andamio-cli` can import it, so signature churn has zero external blast
  radius. The codebase is small, all call sites are internal, and Go
  convention is strong here. A one-time mechanical update beats two flavors of
  every method forever.

- **Retry lives inside the client, behind an opt-in method.** Add
  `PostWithRetry` that wraps `Post` with bounded
  exponential-backoff-plus-jitter. Do not auto-retry every call. The
  default remains single-attempt so POSTs with side effects are never silently
  retried. Callers opt in per endpoint.

  Rationale for scope: only one call site (`lookupTeacherModule`) opts in to
  retries in this PR — the one named by issue #65. Applying the same
  "zero production call sites = speculation" bar the plan uses for `Put`,
  neither `GetWithRetry` nor `PutWithRetry` is added here. The underlying
  `doWithRetry` helper is written generically so a future `GetWithRetry`
  or `PutWithRetry` is a one-line addition when a real caller appears.

- **Retry predicate.** Retry only on:
  - Network-layer errors returned from `httpClient.Do` — identified by
    `errors.As(err, &*url.Error{})` or by `errors.Is(err, syscall.ECONNRESET)`
    / `io.ErrUnexpectedEOF` / similar transport-layer signals. JSON
    marshal errors (pre-`Do`) and JSON decode errors after a 2xx response
    are **not** retryable — marshal means bad input and a 2xx decode
    failure means the write happened. (Implementation decides the exact
    type-check; see Unit 5 test scenarios.)
  - HTTP 5xx responses (500–599) surfaced as `*apierr.ServerError`.
  - HTTP 408 (Request Timeout), 425 (Too Early), 429 (Too Many Requests)
    — transient backpressure signals agents will hit in tight loops. For
    429, honor `Retry-After` when present (cap at `MaxBackoff` to bound
    latency); otherwise use the standard schedule.

  Never retry other 4xx (including 409, which
  register-module's recovery path explicitly observes and branches on; 401/403
  which mean "go re-auth"; and 404 which is resource semantics).

- **Retry schedule.** 3 attempts total (initial + 2 retries). Backoff
  250 ms → 1 s → 2 s, each with ±20% jitter. Implemented as a small private
  `retryConfig` struct (`MaxAttempts`, `InitialBackoff`, `MaxBackoff`,
  `Jitter`, `Rand *rand.Rand`) with production defaults and a test-only
  constructor/accessor so test scenarios can inject 1 ms / 5 ms / 10 ms and
  a seeded RNG. Not exposed as a public knob. Without this seam, the Unit 5
  exhaustion test alone takes ~3 s per run and jitter makes assertions
  flaky.

- **Retry respects context.** Between attempts, if `ctx.Done()` fires, return
  the underlying network error wrapped with `ctx.Err()` so callers can still
  see what was being retried. The backoff sleep uses a `select` on
  `ctx.Done()`.

- **Root-level signal handling.** In `main.go`, replace the bare
  `rootCmd.Execute()` with `signal.NotifyContext(context.Background(),
  os.Interrupt)` wired through `rootCmd.ExecuteContext(ctx)`. This makes
  `cmd.Context()` actually cancellable on Ctrl-C across every subcommand.

- **Helper call sites.** A few helpers in `cmd/andamio/helpers.go` currently
  take `*client.Client` but not a context. They are internal, small, and
  called only from cobra handlers — so extend their signatures to take
  `context.Context` as the first parameter (same convention as the client).
  This is where most of the mechanical diff lives.

## Open Questions

### Resolved During Planning

- **Q: Retry on POST — universal or opt-in?** Opt-in. `PostWithRetry` is a
  separate method. Specific call sites decide. `lookupTeacherModule`'s list
  POST opts in (issue names it as the required case); others stay as-is for
  this PR.
- **Q: `Put` retry?** Not added in this PR. Zero production call sites make
  it pure speculation.
- **Q: Where to add signal handling?** Root-level
  `signal.NotifyContext` in `main.go`. Avoids duplicating per-command
  signal wiring and gives Ctrl-C uniform behavior.
- **Q: Retry visible in logs?** Yes, but quietly. A single stderr line per
  retry when not `--output json`, gated to avoid noise. Suppressed in JSON
  mode (same convention as other CLI progress output).

### Deferred to Implementation

- **Exact error classification for "is this network-level or HTTP-level?"**
  Unit 5 test scenarios (marshal-error, decode-error-after-2xx,
  connection-refused) pin the boundary behaviorally. Precise type checks
  (`*url.Error`, `net.Error`, `syscall.ECONNRESET`, etc.) finalized during
  implementation so they match what the current Go stdlib actually surfaces.
  Context cancel → `context.Canceled`/`DeadlineExceeded` → never retried
  (pinned).
- **Whether to tune `http.Transport` with explicit `DialContext` /
  `TLSHandshakeTimeout` / `ResponseHeaderTimeout`.** Deferred per scope
  boundary; revisit if manual verification finds slow-header conditions still
  eat the full 30s budget.
- **Exact naming of stderr progress line for retries.** Keep it short
  ("retrying in 1s (attempt 2/3)...") but final wording during implementation.
- **Verify `POST /v2/course/teacher/course-modules/list` is truly
  side-effect-free** on the andamio-api side before landing. The retry
  safety claim rests on this — if the gateway handler writes an audit log
  row, increments a per-user rate-limit counter, or emits telemetry per
  call, a triple-fire on 502 distorts those downstream systems. Resolution
  path: read the handler source in `andamio-api` (repo alias `api`) or
  confirm with a maintainer; record the finding in a comment near
  `lookupTeacherModule`. If any side effect is found, retain retry only if
  the side effect is idempotent-safe (e.g., a log row); otherwise revert
  Unit 6 to plain `Post` and close the issue with a note.

## High-Level Technical Design

> *This illustrates the intended approach and is directional guidance for
> review, not implementation specification. The implementing agent should
> treat it as context, not code to reproduce.*

Client surface after this plan:

```
Client.Get(ctx, path, result)
Client.Post(ctx, path, body, result)
Client.Put(ctx, path, body, result)

Client.PostWithRetry(ctx, path, body, result) // wraps Post with bounded retry
```

(`GetWithRetry` / `PutWithRetry` deferred — no caller in this PR.)

Internal flow for `*WithRetry`:

```
for attempt := 1..maxAttempts:
    err := c.Get/Post(ctx, ...)
    if err is nil:
        return nil
    if !retryable(err):
        return err          # 4xx, auth, not-found, conflict, context-cancel
    if attempt == maxAttempts:
        return err
    sleep(backoff(attempt) ± jitter, cancellable by ctx)
```

Retry classifier:

```
retryable(err):
    if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded):
        return false    # caller cancelled — propagate, don't mask
    var serverErr *apierr.ServerError
    if errors.As(err, &serverErr):
        return true     # 5xx
    var authErr *apierr.AuthError
    var notFound *apierr.NotFoundError
    var conflict *apierr.ConflictError
    if errors.As(...) any of the above:
        return false    # 401/403/404/409 — never retry
    if isRetryableStatus(err, 408, 425, 429):
        return true     # backpressure; for 429 honor Retry-After
    if isNetworkLayerError(err):  # *url.Error, ECONNRESET, unexpected EOF, etc.
        return true
    return false        # default-safe: unclassified = don't retry
```

To make 5xx classification reliable, introduce `apierr.ServerError{Status int, Message string}`
for 500–599 (currently falls through to `errors.New`). `Get`/`Post`/`Put`
return `*ServerError` on 5xx; retry classifier inspects via `errors.As`.

Call-site shape at a cobra handler:

```
func runXxx(cmd *cobra.Command, args []string) error {
    ...
    c := client.New(cfg)
    ctx := cmd.Context()
    return c.Post(ctx, "/api/v2/...", payload, &resp)
}
```

Root-level wiring in `main.go`:

```
ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
defer cancel()
rootCmd.ExecuteContext(ctx)
```

## Implementation Units

- [ ] **Unit 1: Introduce `apierr.ServerError` for 5xx**

**Goal:** Give the retry classifier a typed handle on 5xx responses, same
way it has handles on 401/403/404/409 today.

**Requirements:** R3 (enables clean retry predicate).

**Dependencies:** None.

**Files:**
- Modify: `internal/apierr/errors.go` (verified — package already hosts
  `AuthError`, `NotFoundError`, `ConflictError`, `ReportedError` in this file).
- Test: assertion added to `internal/client/client_test.go` (no existing
  `apierr` test file).

**Approach:**
- Add a `ServerError` type mirroring the existing error types:
  `{Status int; Message string}` with `Error()` method.
- Keep error-message format stable: `"API error %d: %s"`. Only the type changes.
- Grep `cmd/andamio/` before landing for any string-match on `"API error 5"`;
  expect zero production matches. `course_teacher_ops_test.go:44` has a
  fixture using `errors.New("API error 500: ...")` — update that fixture to
  use `&apierr.ServerError{...}` so the test mirrors the new type surface
  (see Unit 6).

**Alternative considered:** Use an unexported sentinel or a boolean inside
`internal/client` to drive retry classification without adding a public type.
Rejected because: (a) typed 5xx lets future callers branch on server vs.
client errors via `errors.As` — same pattern proven for 409/404/401/403
in PR #63; (b) the taxonomy stays complete (every HTTP class has a typed
form) rather than leaving 5xx as the one untyped stragger.

**Patterns to follow:**
- Mirror `ConflictError` exactly (same fields, same `Error()` shape).

**Test scenarios:**
- Happy path: `ServerError.Error()` renders `"API error 500: ..."`.
- Integration: `errors.As` on a wrapped `ServerError` returns the unwrapped
  pointer (same property proven for `ConflictError` in
  `TestClient_TypedErrorsSurviveWrapChain`).

**Verification:** `go vet ./...` clean; existing tests still pass.

- [ ] **Unit 2: Add `context.Context` to `Get`/`Post`/`Put`**

**Goal:** Propagate cancellation from cobra into the HTTP layer.

**Requirements:** R1, R2.

**Dependencies:** None (Unit 1 not required but fits the same file).

**Files:**
- Modify: `internal/client/client.go`.
- Modify: `internal/client/client_test.go` (all test call sites).

**Approach:**
- Change signatures to `Get(ctx context.Context, path string, result interface{}) error`
  and analogous for `Post`/`Put`.
- Replace `http.NewRequest` with `http.NewRequestWithContext(ctx, ...)`.
- Surface 5xx as `*apierr.ServerError` (uses Unit 1's type).
- Do not change error types for other status codes or the happy path.

**Patterns to follow:**
- Three switch blocks in `client.go` — `Get` (60–72), `Post` (117–128), `Put`
  (166–177) — all must gain the 5xx arm returning
  `&apierr.ServerError{Status: resp.StatusCode, Message: msg}`. The switch
  bodies are structurally identical today; the fix is identical three times.

**Test scenarios:**
- Happy path: context-bearing call still succeeds with 200 and decodes body.
- Error path: 5xx response returns `*apierr.ServerError` (new; asserts via
  `errors.As`).
- Edge case: passing `nil` for ctx panics at runtime (Go convention — not a
  test, but documented in `// Get ...` doc comment as "ctx must not be nil").
  Optional: add a guard that returns a clear error if nil.
- Integration: pre-existing `TestClient_StatusCodeToTypedError` and
  `TestClient_TypedErrorsSurviveWrapChain` continue to pass once signatures
  are updated — these assert the contract for 401/403/404/409 and that
  unchanged.
- Integration (new): server that sleeps 10s before sending headers; client
  call with `context.WithTimeout(ctx, 50ms)` returns an error whose
  `errors.Is(err, context.DeadlineExceeded)` is true, and the `httptest`
  server observes the request being cancelled (verify via the server side
  receiving `r.Context().Done()`).
- Integration (new): server sends 200 + headers, then stalls 10s before the
  first body chunk (chunked response with a delayed chunk); client call
  with 50ms ctx timeout returns with `errors.Is(err,
  context.DeadlineExceeded)` true. Covers the mid-body-read cancellation
  path, not just the pre-response stall.

**Verification:** New context-cancellation test passes; all pre-existing
tests still pass after their signatures are updated to pass
`context.Background()`.

- [ ] **Unit 3: Wire `signal.NotifyContext` at the root**

**Goal:** Make Ctrl-C actually cancel the context every command inherits
through `cmd.Context()`.

**Requirements:** R2.

**Dependencies:** Unit 2.

**Files:**
- Modify: `cmd/andamio/main.go`.

**Approach:**
- In `main`, build `ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)`
  before `rootCmd.Execute()`, defer `cancel()`, and call
  `rootCmd.ExecuteContext(ctx)`.
- Leave `tx_lifecycle.go`'s custom signal handler in place — it has a
  useful side effect (printing the tx hash on interrupt so the user can run
  `tx status` after). Its own `signal.Notify` + `cancel()` on a local context
  is compatible with the root-level one because the channel is already
  scoped to that function.

**Patterns to follow:**
- Standard `signal.NotifyContext` usage from Go stdlib docs; no novel
  pattern needed.

**Test scenarios:**
- Test expectation: none — single wiring line in main, best verified by
  manual smoke in Unit 6 (verification) rather than a unit test that would
  have to fork a process to deliver SIGINT.

**Verification:** `andamio <any long command>` interrupted with Ctrl-C
during a sleeping `httptest` gateway returns promptly (<1s after signal)
instead of hanging 30s.

- [ ] **Unit 4: Thread `cmd.Context()` through all call sites**

**Goal:** Pass context from cobra handlers down to `client.*` calls.

**Requirements:** R1.

**Dependencies:** Unit 2.

**Files (modify — grep finds ~60 call sites):**
- `cmd/andamio/course.go`
- `cmd/andamio/course_export.go`
- `cmd/andamio/course_import.go`
- `cmd/andamio/course_create_module.go`
- `cmd/andamio/course_owner.go`
- `cmd/andamio/course_teacher_ops.go` (including the `registerOrRecoverModule`
  internal chain — threads ctx through `postRegisterModule`,
  `postUpdateModuleStatus`, `lookupTeacherModule`)
- `cmd/andamio/course_student.go`
- `cmd/andamio/course_credential.go`
- `cmd/andamio/project_owner.go`
- `cmd/andamio/project_task.go`
- `cmd/andamio/project_task_import.go`
- `cmd/andamio/project_contributor.go`
- `cmd/andamio/tx_build.go`
- `cmd/andamio/tx_register.go`
- `cmd/andamio/tx_run.go` (includes handing ctx from existing `pollTxStatus`
  into `c.Get`)
- `cmd/andamio/tx_lifecycle.go`
- `cmd/andamio/teacher_assignments.go`
- `cmd/andamio/helpers.go` — signatures of `getJSON`, `postJSON`,
  `getJSONWithHint`, `resolveTaskHash`, `resolveTaskData`, `resolveSltHash`,
  `resolveTaskHashFromFlags`, `resolveSltHashFromFlags` extend to take
  `ctx context.Context` first. (No `postJSONList` helper exists today —
  prior plan draft was wrong.)
- `cmd/andamio/apikey.go`, `cmd/andamio/token.go`, `cmd/andamio/user.go`

**Approach:**
- For every cobra `RunE`/`Run` handler, capture `ctx := cmd.Context()` early.
- Pass `ctx` as the first argument of `c.Get`/`c.Post`/`c.Put`.
- For internal helpers that wrap client calls, extend their signatures to
  take ctx first. Don't forget nested helpers in `tx_lifecycle.go` like
  `extractTaskHash` → `lookupContributorTaskHash` (two `c.Post` calls at
  lines 294 and 326) — the outer `executeTxLifecycle` signature is not
  enough; every internal `c.*` call must receive a real ctx.
- Where there is no cobra command in scope (e.g., the auth-callback HTTP
  server in `user.go` or a request issued during cleanup), pass
  `context.Background()` with a `// TODO(#65): wire ctx when handler
  accessible` comment. `user.go`'s `runHeadlessLogin` path *does* have `cmd`
  in scope — prefer `cmd.Context()` there. The escape hatch is for truly
  non-cobra paths only.
- **Authoritative file list:** at implementation start, run
  `grep -l 'c\.\(Get\|Post\|Put\)(' cmd/andamio/*.go` and verify the
  enumerated list above matches. The listing is a snapshot; the grep is
  the source of truth.

**Execution note:** Mechanical per-file sweep. No behavior change beyond
threading. Do not reorder code, rename variables, or touch error-handling.

**Patterns to follow:**
- `pollTxStatus(ctx, c, ...)` in `cmd/andamio/tx_run.go:196` is the existing
  in-repo example of ctx-first internal signatures.

**Test scenarios:**
- Test expectation: none for this unit in isolation. The change is purely
  mechanical and regression-covered by the existing test suite continuing to
  pass once signatures are updated. Behavioral coverage lives in Unit 2
  (cancellation propagation) and Unit 6 (retry applied to lookup).

**Verification:**
- `go build ./...` clean.
- `go test ./...` passes with existing coverage once call sites are updated.
- Manual: run a command with long latency (an httptest backend), Ctrl-C, and
  confirm the process exits promptly.

- [ ] **Unit 5: Add `GetWithRetry` / `PostWithRetry` + retry core**

**Goal:** Opt-in bounded-retry wrappers for idempotent gateway calls.

**Requirements:** R3.

**Dependencies:** Unit 1 (ServerError), Unit 2 (context-aware client).

**Files:**
- Modify: `internal/client/client.go`.
- Modify: `internal/client/client_test.go`.

**Approach:**
- Add a private `doWithRetry(ctx context.Context, cfg retryConfig,
  fn func() error) error` helper. `retryConfig` carries `MaxAttempts`,
  `InitialBackoff`, `MaxBackoff`, `Jitter` fraction, and a `*rand.Rand` (or
  equivalent goroutine-safe source). Production default built by
  `defaultRetryConfig()`; tests build a fast config with small backoffs and
  a seeded RNG for determinism.
- Classifier: see the Retry predicate under Key Technical Decisions. In
  code: `errors.As` against typed errors, then `errors.Is` for context
  errors, then status-based checks for 408/425/429, then a
  network-layer-error check using `errors.As(err, &*url.Error{})` or
  equivalent. Unknown errors are **not** retried (default-safe).
- Sleep implemented as `select { case <-time.After(d): case <-ctx.Done(): }`
  so a cancel during backoff returns promptly.
- Jitter: symmetric ±20%. Use `math/rand/v2` (Go 1.22+, goroutine-safe
  top-level functions — no mutex needed) rather than `math/rand`; the
  CLI currently uses Go 1.22+ per `go.mod`. If v2 is unavailable for any
  reason, fall back to `math/rand` protected by `sync.Mutex`. Do not touch
  global state.
- **Stderr progress line on retry**: implemented via a caller-supplied
  `OnRetry func(attempt int, wait time.Duration, err error)` callback
  field on `retryConfig`. Keeps `internal/client` free of a dependency on
  `internal/output`. The cobra layer (in the call sites that opt in to
  retry) supplies a callback that logs to stderr when not `--output json`
  and no-ops otherwise. Not passing an `OnRetry` means no logging.

**Technical design:** See High-Level Technical Design.

**Patterns to follow:**
- Error typing mirrors existing `apierr` package.
- Progress-line gating mirrors the pattern used in
  `cmd/andamio/course_teacher_ops.go:231-233` and elsewhere.

**Test scenarios:** (all tests use a fast `retryConfig` with 1ms/5ms/10ms
backoffs and a seeded RNG, injected via a test-only constructor or
package-private setter — wall-clock cost is <50ms per exhaustion test.)
- Happy path: first attempt succeeds → no sleep, no retry output.
- Retry path: server returns 500 twice, then 200 → `PostWithRetry` succeeds
  with attempt 3; total attempts = 3.
- Retry path: server returns 429 with `Retry-After: 1` (capped to `MaxBackoff`
  in test config) then 200 → succeeds, one retry, respects cap.
- Retry path: server returns 408 / 425 / 429 once each in separate cases then
  200 → succeeds. Pins the extended predicate.
- Exhaustion: server returns 503 every time → error is `*apierr.ServerError`
  after `MaxAttempts`; attempt count matches.
- Non-retryable: server returns 404 → first attempt, no retry, returns
  `*apierr.NotFoundError`. Verifies general 4xx is never retried.
- Non-retryable: server returns 409 → first attempt, no retry, returns
  `*apierr.ConflictError`. Critical: preserves `register-module`'s
  recovery semantics (the branch must see the 409 immediately).
- Edge case: context cancelled mid-backoff → returns promptly with
  `errors.Is(err, context.Canceled)` true (not `ServerError`).
- Edge case: network error (connection refused on a closed listener) is
  retried up to `MaxAttempts` then returned.
- Edge case: pre-`Do` JSON marshal error (bad `body`) → first attempt, no
  retry. Pins that classifier does not over-retry marshal failures.
- Edge case: post-`Do` JSON decode error on a 2xx response → first attempt,
  no retry. Pins that classifier does not over-retry success-then-decode
  failures.
- Integration: `fmt.Errorf("x: %w", err)` wrap chain on the final returned
  error still unwraps to the typed error via `errors.As` for both
  `*apierr.ServerError` (exhaustion) and `*apierr.ConflictError` (no-retry).

**Verification:** Full retry path exercised by tests; no global state leaks
between tests (each test builds its own `httptest` server).

- [ ] **Unit 6: Apply `PostWithRetry` to `lookupTeacherModule`'s list call**

**Goal:** Make the register-module recovery path resilient to transient
gateway blips — the specific motivation named in issue #65.

**Requirements:** R3 (targeted application).

**Dependencies:** Unit 5.

**Files:**
- Modify: `cmd/andamio/course_teacher_ops.go` — `lookupTeacherModule`.
- Modify: `cmd/andamio/course_teacher_ops_test.go` — add retry-behavior tests.

**Approach:**
- `lookupTeacherModule` currently calls
  `c.Post("/api/v2/course/teacher/course-modules/list", ..., &resp)`.
  Replace with `c.PostWithRetry(ctx, ...)`.
- Thread `ctx` through `registerOrRecoverModule` and its helpers
  (`postRegisterModule`, `postUpdateModuleStatus`) to reach
  `lookupTeacherModule`. `postRegisterModule` itself stays on plain `Post`
  — a register call is where recovery branches run, retrying it would
  re-trigger the "already exists" conflict and defeat idempotency handling.
- Docstring update: note that the list call is retryable because it is a
  pure GET-shaped POST (list endpoints are read-only despite the verb).
- **Fixture update**: `cmd/andamio/course_teacher_ops_test.go` line ~44
  contains a test case using `errors.New("API error 500: ...")` to
  assert that `isModuleAlreadyExistsError` returns false on 5xx. After
  Unit 2, real 5xx paths return `*apierr.ServerError`, not
  `errors.New`. Update the fixture to
  `&apierr.ServerError{Status: 500, Message: "..."}` and confirm the
  predicate still returns false.
- **Scope narrowing** (explicit): Other sibling list POSTs are structurally
  identical candidates for `PostWithRetry` — `course.go:119,242,344`,
  `course_export.go:213,238`, `course_import.go:1125`,
  `project_task.go:194,260,696`, `teacher_assignments.go:71,121`,
  `tx_lifecycle.go:294,326`, `helpers.go:303`. Not converted in this PR:
  scope tracks issue #65's single named call site. Each sibling needs a
  per-endpoint idempotency review before opting in. File a follow-up
  issue titled "Extend PostWithRetry to read-only list endpoints" with
  this enumeration.

**Patterns to follow:**
- The rest of `registerOrRecoverModule`'s existing structure is preserved;
  this is a localized swap of one call.

**Test scenarios:**
- Retry: `httptest` server returns 502 on first list call then full list on
  second → `registerOrRecoverModule` succeeds and routes to the correct
  branch (DRAFT advance or already_registered).
- Non-retry preserved: server returns 409 on `register` and then 200 on
  `list` first try → existing recovery behavior unchanged (list was never
  retryable in practice here anyway).
- Exhaustion: server returns 502 on every list call → lookup returns a
  wrapped `*apierr.ServerError` after `MaxAttempts`; recovery error matches
  existing "could not locate it for recovery" message shape.
- **Wrap-chain preservation**: exhaustion case above — assert
  `var se *apierr.ServerError; errors.As(err, &se)` returns `true` on the
  final `registerOrRecoverModule` error, which is double-wrapped
  (`fmt.Errorf("failed to list modules...: %w", ...)` inside
  `fmt.Errorf("could not locate it for recovery: %w", ...)`). Pins the
  invariant that future callers (main.go exit-code mapping, upstream
  callers) can still branch on 5xx via `errors.As` through the double
  wrap.

**Verification:**
- `go test ./cmd/andamio/... -run TestLookupTeacherModule` passes.
- Manual: preprod smoke against a known DRAFT module — register-module
  survives at least one forced retry (can simulate via local proxy, or
  just rely on the unit tests).

## System-Wide Impact

- **Interaction graph:** Every cobra handler that touches the gateway is
  updated. No external callers of `internal/client` exist outside
  `cmd/andamio/` (confirmed by grep).
- **Error propagation:** New `*apierr.ServerError` enters the error taxonomy.
  `cmd/andamio/main.go`'s exit-code mapping does not currently branch on 5xx
  — no change needed there. Wrapped `%w` chains preserved by design.
- **State lifecycle risks:** Retry is opt-in and applied only to a pure list
  read. Nothing in scope performs writes under retry, so no risk of duplicate
  mutations, duplicate credits, duplicate tx submissions, or partial writes.
- **API surface parity:** Public CLI surface unchanged. No new flags, no
  changed flags, no changed output envelopes. The retry is invisible on
  success; on retry it prints one extra stderr line per attempt (suppressed
  in JSON mode).
- **Integration coverage:** Cancellation and retry tests use `httptest` — a
  real HTTP server boundary rather than mocks, so they exercise the full
  request path including transport and decoding.
- **Unchanged invariants:**
  - `apierr` typed errors for 401/403/404/409 and their `errors.As`
    unwrapping behavior — `TestClient_StatusCodeToTypedError` and
    `TestClient_TypedErrorsSurviveWrapChain` must continue to pass.
  - `register-module`'s idempotency contract (PR #63) — the 409 → recovery
    branch must not be retried.
  - Exit codes in `main.go` — no new exit code introduced.

## Risks & Dependencies

| Risk | Mitigation |
|------|------------|
| Mechanical call-site sweep introduces a typo or accidental behavior change | Trust the compiler: signature change will fail-fast at every missed site. Follow with `go test ./...` and `go vet ./...`. |
| Retry accidentally masks a real bug by smoothing over consistent 5xx | Stderr progress line per retry (non-JSON mode) surfaces that retries happened. `MaxAttempts` bounded at 3 so the latency floor is visible. |
| `PostWithRetry` on the list call double-fires if the "error" was actually a successful write from the gateway's perspective | List endpoint is read-only. Only retrying list endpoints in this PR. Any future opt-in to retry on a write POST must document idempotency explicitly at the call site. |
| Root-level `signal.NotifyContext` interacts badly with `tx_lifecycle.go`'s own `signal.Notify` | Both install signal handlers; Go multiplexes signal channels so each handler sees SIGINT. `tx_lifecycle` calls `os.Exit(1)` — this bypasses the root-level `defer cancel()` and any in-flight retry `<-ctx.Done()`. Accepted asymmetry: `tx run` Ctrl-C semantics unchanged (abrupt exit after printing tx hash); non-tx commands get new cancellable behavior. Document this explicitly as intended; do not try to unify in this PR. |
| `os.Exit(1)` in `tx_lifecycle` wins the race against a mid-retry backoff `select { case <-ctx.Done() }` | Same accepted asymmetry. If future `tx run` internals opt in to `PostWithRetry`, revisit by replacing `os.Exit` with a return so deferred cleanup and retry cancellation can run. |
| 5xx typed error changes unwrap behavior somewhere we don't expect | Grep before landing for any `errors.Is`/`errors.As` consumer of "API error %d" string matching (expect zero — current callers use typed errors). |

## Documentation / Operational Notes

- No user-facing CHANGELOG entry required for context propagation; it's a
  transparent reliability improvement.
- Add a brief CHANGELOG.md `## [Unreleased]` note for the retry behavior:
  "register-module now retries the teacher-list lookup up to 3 times on
  transient 5xx/network errors during recovery." This is a reliability
  upgrade visible in stderr output; scripts that match on stderr content
  (unlikely but possible) deserve a heads-up.
- No monitoring changes. No migration. No feature flag.

## Sources & References

- **Issue:** [#65](https://github.com/Andamio-Platform/andamio-cli/issues/65)
- **Upstream review:** PR #63 review, findings P2 #12 (context propagation)
  and #13 (retries). Merged via #69.
- **Related code:**
  - `internal/client/client.go`
  - `cmd/andamio/course_teacher_ops.go` (registerOrRecoverModule,
    lookupTeacherModule)
  - `cmd/andamio/main.go` (root wiring)
  - `cmd/andamio/tx_run.go` (pre-existing ctx signature that lacked a
    ctx-aware client to hand off to)
