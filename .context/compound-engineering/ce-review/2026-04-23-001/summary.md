---
run_id: 2026-04-23-001
date: 2026-04-23
mode: autofix
base: 2665965883feb9f7ae44c19f0ff49ed5d0aaabb6
plan: docs/plans/2026-04-23-001-feat-client-context-retries-plan.md
verdict: Ready with fixes (applied)
---

# ce:review run summary — issue #65 (client ctx + retries)

## Reviewer team

Always-on: correctness, testing, maintainability, project-standards, agent-native, learnings-researcher.
Conditional: reliability, api-contract, performance, adversarial.
Skipped: security (no auth/endpoint changes), schema-drift/deployment-verification (no migrations), stack-specific reviewers (Go CLI, no Rails/Python/TS).

## Finding counts

- Correctness: 0 findings (3 residual risks, 2 testing gaps)
- Testing: 4 findings (P2/P3), 4 residual risks, 10 testing gaps
- Maintainability: 9 findings (mix of P2/P3; several were non-findings confirmed)
- Project-standards: 1 P1 finding (OnRetry callback not wired — applied)
- Agent-native: 0 blocking findings (future recommendation for retry observability)
- Learnings-researcher: 3 past solutions referenced; all respected
- Reliability: 3 findings (P3/P3/P3 — low confidence)
- Api-contract: 0 findings (2 testing gaps)
- Performance: 0 findings
- Adversarial: 4 findings (advisory only — edge cases, documented asymmetries)

## Applied fixes (safe_auto)

1. **P1 — OnRetry wiring at register-module call site.** `runCourseTeacherRegisterModule` now calls `c.SetOnRetry(...)` before `registerOrRecoverModule`; callback prints "retrying in Xs (attempt N): <err>" to stderr in non-JSON mode. Matches CHANGELOG promise and plan design (`cmd/andamio/course_teacher_ops.go`).

2. **P2 — Eliminated string-prefix retry classification.** New `apierr.BackpressureError` type for 408/425/429, with `RetryAfterSeconds` field. `statusError` emits it (pulling Retry-After from body via `parseRetryAfterSeconds`). `isRetryable` now uses `errors.As(&BackpressureError)` — no string parsing of error messages anywhere in the retry core. Parallels the ServerError pattern already in place. (`internal/apierr/errors.go`, `internal/client/client.go`, `internal/client/retry.go`)

3. **P2 — Test coverage expansion.** Added:
   - `TestRetry_NonRetryable_401_403` — auth errors never retry
   - `TestRetry_NonRetryable_DeadlineExceeded` — caller deadline stops retry immediately
   - `TestRetry_BackpressureError_RetryAfter_Parsed` — six body shapes (integer, leading zero, zero, no hint, http-date, negative)
   - `TestRetry_Backoff_HonorsRetryAfter` — pins that positive Retry-After overrides exponential, still capped
   - `TestRetry_JitterScalesBackoff` — deterministic jitter verification (production ships with Jitter=0.2)
   - `TestRetry_NetworkError_UnexpectedEOF` — io.ErrUnexpectedEOF classified as retryable
   - `TestRegisterOrRecover_OnRetryCallbackFires` — end-to-end wiring test for the stderr message

4. **CHANGELOG entry** for BackpressureError and SetOnRetry additions.

## Not fixed — advisory only

| Finding | Severity | Reason |
|---------|----------|--------|
| 90s worst-case wall-clock (3 × 30s timeout) | P3 reliability | Known tradeoff; plan's scope boundary defers `http.Transport` tuning |
| Retry-After: 0 tight loop | P3 adversarial | Bounded by MaxAttempts=3 (<100ms total) |
| `postRegisterModule` doesn't retry 5xx | P2 adversarial | By design; documented in code and plan — register must see 409 on first try |
| `tx_lifecycle` `os.Exit(1)` bypasses deferred cancel | P3 reliability | Documented as accepted asymmetry in plan risks table |
| `lookupContributorTaskHash` fallback paths not retried | P2 maintainability | Out of scope; follow-up issue for sibling list-POST retries |
| HTTP-date Retry-After form not parsed | P3 maintainability | Deferred in plan; integer-second form covers observed gateway behavior |
| retryConfig struct abstraction | P3 maintainability | Test injection seam is the justification (disagreement resolved in favor of keeping) |
| Retry observability for agents | P3 agent-native | Future enhancement; structured retry metadata could land in a follow-up |

## Residual risks noted

- If `andamio-api` ever adds audit/telemetry writes to `POST /v2/course/teacher/course-modules/list`, the retry safety claim in Unit 6 erodes. Plan tracks this as an Open Question deferred to implementation.
- Concurrent `register-module` invocations each retry independently; if a gateway has tight rate limits, 3× hammering across N concurrent callers could compound. Jitter mitigates in-process; cross-process coordination is out of scope.
- Root-level `signal.NotifyContext` + `tx_lifecycle.go`'s own `signal.Notify` + `os.Exit(1)` race: the plan accepts this as intentional asymmetry (tx commands abrupt-exit is unchanged; non-tx commands get new cancellable behavior).

## Quality gates

- `go build ./...` — clean
- `go vet ./...` — clean
- `go test ./...` — 100% pass; new tests complete in under 12s total
