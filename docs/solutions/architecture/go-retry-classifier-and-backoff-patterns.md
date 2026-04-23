---
title: Go retry classifier and backoff patterns for internal HTTP client
date: 2026-04-23
problem_type: best_practice
tags: [retry, backoff, jitter, context, cancellation, errors-as, typed-errors, signal, go, internal-client]
applies_when:
  - Adding bounded retries to a Go HTTP client
  - Designing a retry predicate that must branch on HTTP status classes
  - Mixing root-level signal.NotifyContext with command-specific os.Exit handlers
  - A lower-layer package (like internal/client) needs to surface progress to a higher layer without taking a dependency on it
related_docs:
  - docs/solutions/architecture/cli-composability-audit-and-fix.md
  - docs/solutions/feature-implementations/cli-course-module-management-commands.md
  - docs/solutions/integration-issues/cli-apikey-auth-isolation-and-content-404-ux.md
---

# Go retry classifier and backoff patterns for internal HTTP client

## Context

Implementing issue #65 — context propagation + bounded retries for `internal/client` — surfaced four patterns that are not obvious from the retry literature and that review rounds repeatedly flagged. Capturing them so the next Go retry implementation in this repo (or another) gets these right the first time.

Team decision referenced: the CLI must be fully composable for bash scripting (2026-03-18) (auto memory [claude]). Several of these patterns fall out directly from that constraint.

## Guidance

### 1. Retry classifiers use typed errors via `errors.As`, never string matching

**Do this:**

```go
func isRetryable(err error) bool {
    if err == nil {
        return false
    }
    if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
        return false
    }
    // Semantic 4xx — never retry.
    var authErr *apierr.AuthError
    var notFound *apierr.NotFoundError
    var conflict *apierr.ConflictError
    if errors.As(err, &authErr) || errors.As(err, &notFound) || errors.As(err, &conflict) {
        return false
    }
    var serverErr *apierr.ServerError
    if errors.As(err, &serverErr) {
        return true  // 5xx
    }
    var backpressure *apierr.BackpressureError
    if errors.As(err, &backpressure) {
        return true  // 408/425/429
    }
    return isNetworkLayerError(err)
}
```

**Not this:**

```go
func isBackpressureError(err error) bool {
    msg := err.Error()
    for _, prefix := range []string{"API error 408", "API error 425", "API error 429"} {
        if strings.HasPrefix(msg, prefix) {
            return true
        }
    }
    return false
}
```

Both produce the same behavior today. The typed version stays correct after any of:
- Changing the error-message format in the HTTP client (e.g., including the request path in the message)
- Adding i18n or context to error messages
- Wrapping the error with `fmt.Errorf("operation X failed: %w", err)` (the string version breaks immediately; the typed version survives via `errors.As`)

If the HTTP client produces status errors, make each retry-relevant class a typed error: `ServerError` for 5xx, `BackpressureError` for 408/425/429, `AuthError` for 401/403, `NotFoundError` for 404, `ConflictError` for 409. The retry classifier then uses `errors.As` and is decoupled from message formatting forever.

### 2. `MaxBackoff` must be the last operation — cap AFTER jitter

**Do this:**

```go
func backoffDuration(cfg retryConfig, attempt int, err error) time.Duration {
    base := cfg.InitialBackoff << (attempt - 1)
    // ... optional Retry-After override ...
    if cfg.Jitter > 0 {
        factor := 1 + (rand.Float64()*2-1)*cfg.Jitter
        base = time.Duration(float64(base) * factor)
    }
    if base > cfg.MaxBackoff {
        base = cfg.MaxBackoff  // strict cap, last operation
    }
    return base
}
```

**Not this:**

```go
    if base > cfg.MaxBackoff {
        base = cfg.MaxBackoff  // cap first
    }
    if cfg.Jitter > 0 {
        factor := 1 + (rand.Float64()*2-1)*cfg.Jitter
        base = time.Duration(float64(base) * factor)  // jitter can push PAST cap
    }
```

With `MaxBackoff=5s` and `Jitter=0.2`, the buggy order yields actual sleeps up to 6s — the "cap" is a soft ceiling that the jitter can exceed by ±20%. The fixed order makes `MaxBackoff` a true strict upper bound.

Pin the invariant with a test:

```go
func TestBackoff_JitterRespectsMaxBackoff(t *testing.T) {
    cfg := retryConfig{InitialBackoff: 10 * time.Second, MaxBackoff: 5 * time.Second, Jitter: 0.2}
    for i := 0; i < 200; i++ {
        d := backoffDuration(cfg, 1, nil)
        if d > cfg.MaxBackoff {
            t.Fatalf("backoff %v exceeds MaxBackoff %v — cap must win over jitter", d, cfg.MaxBackoff)
        }
    }
}
```

### 3. Surface lower-layer progress via a caller-supplied callback, not a cross-layer import

When `internal/client` wants to log "retrying..." to stderr, it must not import `internal/output` (the package that knows about `--output json` gating). That would be an architectural regression.

**Do this:**

```go
// internal/client/client.go
type Client struct {
    // ...
    onRetry func(attempt int, wait time.Duration, err error)
}

func (c *Client) SetOnRetry(cb func(attempt int, wait time.Duration, err error)) {
    c.onRetry = cb
}

// internal/client/retry.go — reads c.onRetry into the retry config
func (c *Client) PostWithRetry(ctx context.Context, ...) error {
    cfg := defaultRetryConfig()
    cfg.OnRetry = c.onRetry
    return c.doWithRetry(ctx, cfg, ...)
}
```

```go
// cmd/andamio/course_teacher_ops.go — cobra layer wires in the formatted output
func runCourseTeacherRegisterModule(cmd *cobra.Command, args []string) error {
    isJSON := output.GetFormat() == output.FormatJSON
    c := client.New(cfg)
    if !isJSON {
        c.SetOnRetry(func(attempt int, wait time.Duration, err error) {
            fmt.Fprintf(os.Stderr, "  retrying in %s (attempt %d): %v\n",
                wait.Round(time.Millisecond), attempt, err)
        })
    }
    // ...
}
```

The cobra layer owns the output decisions (when to emit, what format, JSON gating); the client layer owns the retry mechanics. Neither needs to know about the other's internals. Testing is also easier — tests inject their own `OnRetry` to count callbacks without scraping stderr.

### 4. Document signal-handler asymmetry; don't try to unify

When root-level `signal.NotifyContext` exists and individual commands install their own `signal.Notify` that calls `os.Exit(1)`, the `os.Exit` wins the race against any deferred cleanup or `ctx.Done()` mid-retry. This is NOT a bug — the two handlers serve different needs:

- Root-level handler: cancellable context for normal commands (HTTP call aborts when Ctrl-C pressed).
- Command-level handler in `tx_lifecycle.go`: prints the tx hash before exit so the user can run `tx status <hash>` after interruption. Requires `os.Exit(1)` to skip normal error output.

Don't try to merge them. The asymmetry is intentional. Document it explicitly in code comments AND in the plan's risks table, so the next maintainer doesn't spend an hour tracing "why doesn't my `defer cancel()` run."

```go
// main.go — root wiring
ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
defer cancel()
if err := rootCmd.ExecuteContext(ctx); err != nil { /* ... */ }

// tx_lifecycle.go — command-specific signal handler, intentionally uses os.Exit
sigCh := make(chan os.Signal, 1)
signal.Notify(sigCh, os.Interrupt)
go func() {
    <-sigCh
    if txHash != "" {
        fmt.Fprintf(os.Stderr, "\nInterrupted. Transaction may have been submitted. Check: andamio tx status %s\n", txHash)
    }
    cancel()
    os.Exit(1)  // deliberate: skips error path so we don't print the interrupt twice
}()
```

## Why This Matters

**Pattern 1 (typed errors):** Already established in this repo for exit-code dispatch (`cli-composability-audit-and-fix.md`) and sentinel errors for not-found/auth classification (`cli-course-module-management-commands.md`). Extending it to retry classification closes a gap — a future refactor to error-message format would silently break retry behavior under the string-matching approach. The typed-error approach makes the retry predicate a compile-time contract.

**Pattern 2 (cap after jitter):** Reviewers flagged this at 0.95 confidence. It sounds like a style preference but materially affects reliability contracts. If code downstream makes latency budgets based on `MaxBackoff`, a 20% overshoot breaks SLAs. The fixed ordering costs nothing — just swap two `if` blocks.

**Pattern 3 (OnRetry callback):** Upholds the repo's composability invariant (JSON output must be pure; stderr noise suppressed in JSON mode). Without the callback, either (a) `internal/client` imports `internal/output` creating a cross-layer dep, or (b) the client emits raw stderr always, breaking JSON scripts. The callback is a small API surface that lets the higher layer own the policy.

**Pattern 4 (signal asymmetry):** Without documentation, a future maintainer sees root-level `signal.NotifyContext` and assumes deferred `cancel()` always runs. They refactor the `tx_lifecycle` handler to drop `os.Exit(1)`, and suddenly Ctrl-C during a tx leaves the user with no tx hash to recover from. The asymmetry is load-bearing.

## When to Apply

- **Pattern 1:** Every time you add a retry loop that needs to branch on status class. Skip it only for trivial "retry any error up to N" loops where classification doesn't matter.
- **Pattern 2:** Any time your backoff has both a cap and jitter, regardless of the codebase. Universal Go retry gotcha.
- **Pattern 3:** When a lower-layer package needs to emit progress or diagnostic output, and the formatting policy lives in a higher-layer package. Alternative: pass a logger interface. The callback is lighter.
- **Pattern 4:** When a CLI has both root-level signal wiring and command-specific cleanup that needs to run before exit. Common for long-running commands that partially commit state before interruption (tx submission, multi-step imports).

## Examples

### Retry-After parsing as a typed-field extraction

When a backpressure response carries a `Retry-After` header, don't parse it inside the retry classifier. Parse it once when the error is constructed, store on the typed error:

```go
// internal/apierr/errors.go
type BackpressureError struct {
    Status            int
    Message           string
    RetryAfterSeconds int  // 0 means "not supplied or unparseable"
}

// internal/client/client.go — statusError construction
case http.StatusRequestTimeout, http.StatusTooEarly, http.StatusTooManyRequests:
    return &apierr.BackpressureError{
        Status:            status,
        Message:           msg,
        RetryAfterSeconds: parseRetryAfterSeconds(body),
    }

// internal/client/retry.go — classifier uses the parsed value
func backoffDuration(cfg retryConfig, attempt int, err error) time.Duration {
    base := cfg.InitialBackoff << (attempt - 1)
    var backpressure *apierr.BackpressureError
    if errors.As(err, &backpressure) && backpressure.RetryAfterSeconds > 0 {
        base = time.Duration(backpressure.RetryAfterSeconds) * time.Second
    }
    // ...
}
```

This keeps the parser near the error construction (where the body is already in hand) and keeps the classifier focused on "what to retry and for how long," not "how to parse headers."

### Boundary tests for tolerant parsers

Any hand-rolled integer parser needs boundary coverage:

```go
{"boundary: exactly 1<<30 accepted",   "Retry-After: 1073741824 boundary", 1073741824},
{"boundary: 1<<30+1 rejected",         "Retry-After: 1073741825 nope",     0},
{"way past boundary rejected",         "Retry-After: 9999999999999 nope",  0},
{"negative (rejected)",                "Retry-After: -5 nope",             0},
{"whitespace only",                    "Retry-After:     ",                0},
{"http-date form (unsupported)",       "Retry-After: Wed, 21 Oct 2015 07:28:00 GMT", 0},
```

If the input is untrusted or hand-rolled parsing is involved, table-drive the boundary cases. The cost is one more test; the benefit is catching regressions when the `> N` becomes `>= N` by accident.

### Deterministic assertion alongside probabilistic check

For jitter tests, a `len(seen) >= 5` over 50 samples is mostly reliable but theoretically flaky. Pair it with a deterministic check:

```go
const unjittered = 100 * time.Millisecond
foundJittered := false
for i := 0; i < 50; i++ {
    d := backoffDuration(cfg, 1, nil)
    if d != unjittered {
        foundJittered = true
    }
    seen[d] = struct{}{}
}
if !foundJittered {
    t.Errorf("jitter produced no samples that differed from base %v — jitter math may be dead", unjittered)
}
if len(seen) < 5 {
    t.Errorf("expected varied durations, got %d distinct", len(seen))
}
```

The deterministic check fails clean if jitter is accidentally disabled; the probabilistic check is defense-in-depth.

## Prevention

Incorporate into new retry implementations:

1. **Define typed errors for each HTTP status class that affects retry.** Don't parse error messages in the classifier.
2. **Order backoff computation: exponential → override → jitter → cap.** Put the `MaxBackoff` check last.
3. **For lower-layer-to-higher-layer progress reporting, use a caller-supplied callback.** Don't import the higher layer downward.
4. **When mixing signal handlers, document the asymmetry in code AND in the plan.** `os.Exit` in a command handler is load-bearing when preceded by state-recovery output.
5. **Test the jitter/cap interaction with a 200-sample invariant test.** Catches cap-before-jitter regressions in one run.
6. **Hand-rolled integer parsers get boundary-case table tests.** One test row per edge; cheap and durable.

## Sources

- Plan: `docs/plans/2026-04-23-001-feat-client-context-retries-plan.md`
- Issue: [#65](https://github.com/Andamio-Platform/andamio-cli/issues/65)
- PR: [#71](https://github.com/Andamio-Platform/andamio-cli/pull/71)
- Review rounds surfacing these patterns: two `/ce:review` passes, both autofix + one interactive, 10 personas total.
