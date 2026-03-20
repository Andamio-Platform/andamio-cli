---
title: "feat: add tx run command for full transaction lifecycle"
type: feat
status: completed
date: 2026-03-20
origin: docs/brainstorms/2026-03-20-tx-run-full-lifecycle-command-brainstorm.md
---

# feat: add `tx run` command for full transaction lifecycle

## Overview

A single `andamio tx run` command that executes the full Cardano transaction lifecycle: build, sign, submit, register with the gateway state machine, and poll for DB confirmation. Replaces the current 5-step manual copy-paste workflow.

(see brainstorm: docs/brainstorms/2026-03-20-tx-run-full-lifecycle-command-brainstorm.md)

## Problem Statement

The full transaction flow today requires 5 separate commands with manual plumbing:

```bash
andamio tx build /v2/tx/... --body '...' -o json | jq -r '.unsigned_tx' > unsigned.hex
andamio tx sign --tx-file unsigned.hex --skey payment.skey -o json > signed.json
TX_HASH=$(jq -r '.tx_hash' signed.json)
andamio tx submit --tx $(jq -r '.signed_tx' signed.json)
andamio tx register --tx-hash $TX_HASH --tx-type assessment_assess
andamio tx status $TX_HASH  # poll manually, repeatedly
```

This is error-prone, hard to teach, and forces users to manage intermediate state. The API already tracks the full lifecycle via a Redis-backed state machine — the CLI should orchestrate it end-to-end.

## Proposed Solution

```bash
andamio tx run /v2/tx/course/teacher/assignments/assess \
  --body '{"alias":"teacher-01","course_id":"abc123","assignment_decisions":[...]}' \
  --skey ./payment.skey \
  --tx-type assessment_assess
```

Output (step-by-step on stderr in text mode):
```
  ✓ Built unsigned TX (fee: 180000 lovelace)
  ✓ Signed with payment.skey
  ✓ Submitted to network (tx: abc123def456...)
  ✓ Registered as assessment_assess
  ⏳ Waiting for confirmation...
  ✓ Confirmed on-chain (slot 12345678)
  ✓ DB updated — complete!
```

## Technical Approach

### New file: `cmd/andamio/tx_run.go`

Single file, ~250-300 lines. Reuses existing packages — no new dependencies.

### Flags

```
andamio tx run <endpoint> [flags]

Required:
  --body <json>           Request body for the build endpoint
  --skey <path>           Path to Cardano .skey file for signing
  --tx-type <type>        Transaction type for registration (one of 17 valid types)

Optional:
  --body-file <path>      Read body from file (mutually exclusive with --body)
  --submit-url <url>      Override submit API URL (falls back to config)
  --submit-header <k:v>   Additional submit headers (repeatable)
  --instance-id <id>      Course or project ID for registration
  --metadata <k=v>        Metadata for registration (repeatable, e.g. --metadata task_hash=abc)
  --no-wait               Exit after registration without polling
  --timeout <duration>    Max poll time (default: 10m)
  -o, --output            Output format (text/json)
```

### Execution flow

```
1. Validate flags (--body/--body-file mutex, --skey exists, --tx-type valid)
2. Check JWT expiry — warn if <5min remaining, error if expired
3. Load config, create client
4. POST to build endpoint → extract unsigned_tx from response
5. Sign unsigned_tx with .skey → get signed_tx + tx_hash
6. Submit signed_tx to submit API → confirm acceptance
7. POST to /api/v2/tx/register with tx_hash, tx_type, instance_id, metadata
8. If --no-wait: print result, exit 0
9. Poll GET /api/v2/tx/status/{tx_hash} every 5s
10. Print state transitions as they change (stderr, text mode only)
11. On terminal state: print final result, exit
```

### RunResult struct

```go
type RunResult struct {
    TxHash        string                 `json:"tx_hash"`
    TxType        string                 `json:"tx_type"`
    State         string                 `json:"state"`
    Step          string                 `json:"step"`
    BuildResponse map[string]interface{} `json:"build_response,omitempty"`
    Error         string                 `json:"error,omitempty"`
}
```

Always populated to the extent known — on partial failure, `step` indicates where it stopped and `tx_hash` is included if signing completed.

### Error handling & partial failure recovery

| Step fails | Side effects | CLI behavior |
|-----------|-------------|-------------|
| Build | None | Exit with API error |
| Sign | None | Exit with skey/CBOR error |
| Submit | TX may be in mempool | Print tx_hash to stderr + JSON. Exit with error. User can retry register manually. |
| Register | TX is on-chain | Print tx_hash + warning. Exit with error. User runs `andamio tx register` manually. |
| Poll timeout | TX is registered | Print tx_hash + last state. Exit with error. User runs `andamio tx status` later. |
| Poll → "failed" | TX confirmed but DB failed | Print tx_hash + failure reason. Exit 1. |
| Poll → "expired" | TX never confirmed | Print tx_hash + "expired". Exit 1. |

**Key principle:** Once signing completes, always print the tx_hash — even on failure. This is the user's recovery handle.

### SIGINT handling

Install a `context.WithCancel` + signal handler. On Ctrl+C:
- If tx_hash is known: print `"Interrupted. Transaction may have been submitted. Check: andamio tx status <hash>"` to stderr
- If tx_hash is not yet known: exit silently

### Exit codes

| Outcome | Exit code |
|---------|-----------|
| Success (state: "updated") | 0 |
| Build/sign/submit failure | 1 |
| Auth error (401/403) | 3 (via apierr.AuthError) |
| Register failure | 1 |
| Poll timeout | 1 |
| Terminal "failed" | 1 |
| Terminal "expired" | 1 |

### Progress output (text mode)

All progress lines go to stderr, gated by `if !isJSON`. Each step prints on completion:

```
  ✓ Built unsigned TX
  ✓ Signed with payment.skey (tx: abc123de...)
  ✓ Submitted to network
  ✓ Registered as assessment_assess
  ⏳ Waiting for confirmation... (pending)
  ✓ Confirmed on-chain
  ✓ DB updated — complete!
```

State transitions during polling are printed as they change (not every 5s).

### Poll behavior

- Interval: 5 seconds
- On HTTP error from status endpoint: retry up to 3 consecutive failures with warning to stderr, then abort
- On `--timeout` exceeded: exit with tx_hash and last known state
- Default timeout: 10 minutes
- `--timeout 0`: no CLI-side timeout (poll until API returns terminal state)

### Metadata flag

`--metadata key=value` parsed via `strings.SplitN(v, "=", 2)`. Repeatable. Passed as `map[string]string` in the register payload's `metadata` field. The API accepts this field per the `PendingTx` struct (verified in `tx_state_machine_service.go:133`).

### JWT expiry pre-check

Read `jwt_expires_at` from config. If expired, return `apierr.AuthError`. If <5 minutes remaining, print warning to stderr: `"Warning: JWT expires in Xm — pipeline may fail at register step. Run 'andamio user login' to refresh."`

### Tx-type validation

Validate `--tx-type` against the known 17 types client-side before making any API calls. Fail fast with: `"invalid --tx-type %q. Run 'andamio tx types' to see valid values."`

## Implementation Steps

### Step 1: Create `cmd/andamio/tx_run.go` with command skeleton (~30 min)

- [x] Define `txRunCmd` with Use, Short, Long, Args, PreRunE (JWT check), RunE
- [x] Register all flags in `init()`
- [x] Add `RunResult` struct
- [x] Register command: `txCmd.AddCommand(txRunCmd)`
- [x] ~~Validate `--tx-type` against hardcoded list~~ — deferred; API validates. See QoL note below.

### Step 2: Implement the pipeline (~45 min)

- [x] `runTxRun()` function with step-by-step execution
- [x] Reuse `tx_build.go` build logic (POST to endpoint, extract unsigned_tx)
- [x] Reuse `internal/cardano` signing (load skey, sign, get tx_hash + signed_tx)
- [x] Reuse `internal/submit` for CBOR submission
- [x] Reuse `tx_register.go` register logic (POST to /api/v2/tx/register)
- [x] Parse `--metadata key=value` flags into map
- [x] JWT expiry pre-check from config

### Step 3: Implement polling (~30 min)

- [x] `pollTxStatus()` function: GET /api/v2/tx/status/{tx_hash} every 5s
- [x] Print state transitions to stderr (only when state changes)
- [x] Respect `--timeout` with `time.After`
- [x] Handle consecutive poll failures (3 retries then abort)
- [x] Return on terminal states: "updated", "failed", "expired"

### Step 4: SIGINT handler and partial failure output (~20 min)

- [x] `context.WithCancel` wired through all HTTP calls
- [x] Signal handler prints tx_hash if known
- [x] Each failure point includes tx_hash in both stderr message and JSON output
- [x] `--output json` always returns RunResult struct (even on failure)

### Step 5: Tests (~20 min)

- [x] `TestParseMetadataFlags` — key=value parsing, edge cases
- [x] ~~`TestValidateTxType`~~ — deferred (no client-side validation in v1)
- [x] Build and verify help output shows all flags
- [ ] Manual: run against preprod with a real assessment_assess transaction

### Step 6: Documentation (~15 min)

- [x] Update CLAUDE.md command reference table
- [ ] Add to andamio-docs CLI transaction-signing page
- [x] Update `--help` Long text with examples

## Acceptance Criteria

### Functional

- [x] `tx run` builds, signs, submits, registers, and polls in one command
- [x] Step-by-step progress on stderr in text mode
- [x] `--output json` returns clean RunResult on stdout, no stderr noise
- [x] `--no-wait` exits after registration
- [x] `--timeout` controls max poll duration (default 10m)
- [x] `--metadata key=value` passed to register endpoint
- [x] `--tx-type` passed to API (API validates; see QoL note)

### Error recovery

- [x] tx_hash always printed once signing completes, even on subsequent failure
- [x] SIGINT during poll prints tx_hash and recovery command
- [x] Register failure includes tx_hash for manual recovery
- [x] Poll timeout includes tx_hash and last known state

### Composability (from CLAUDE.md)

- [x] No stdin reads
- [x] Progress to stderr only
- [x] `--output json` is the scripting surface
- [x] Works without a TTY
- [x] Exit code 0 on success, 1 on failure, 3 on auth error

### Backwards compatibility

- [x] All 5 existing tx commands unchanged
- [x] `tx run` is purely additive

## Out of Scope (v1)

- Multi-signature (`--skey` repeatable) — single signer only
- `--dry-run` flag — users can use `tx build` directly
- SSE streaming — polling is sufficient for CLI
- Auto-inference of `--tx-type` from endpoint path — keep explicit for v1
- Auto-inference of `--instance-id` from build response — keep explicit for v1

### QoL improvement: client-side `--tx-type` validation

Currently `--tx-type` is passed directly to the API, which validates it. A future improvement could validate client-side against the known 17 types (fetched from `andamio tx types` or hardcoded) to fail fast before the build step. This would give a better error message: `"invalid --tx-type %q. Run 'andamio tx types' to see valid values."` vs waiting for the API to reject it at the register step (after build+sign+submit have already succeeded). Trade-off: a hardcoded list can drift from the API; fetching types adds a network round-trip.

## Dependencies & Risks

- **Risk:** Poll loop adds a long-running command to a CLI designed for one-shot operations. Mitigated by `--timeout` and `--no-wait`.
- **Risk:** JWT expiry during pipeline. Mitigated by pre-check with 5-minute warning.
- **Risk:** Submit succeeds but register fails → on-chain state diverges from DB. Mitigated by always printing tx_hash for manual recovery.
- **No new dependencies.** Reuses existing `internal/cardano`, `internal/submit`, `internal/client`, `internal/output`.

## Sources

- **Origin brainstorm:** [docs/brainstorms/2026-03-20-tx-run-full-lifecycle-command-brainstorm.md](../brainstorms/2026-03-20-tx-run-full-lifecycle-command-brainstorm.md) — key decisions: `tx run` command name, `--tx-type` explicit flag, poll-not-SSE, wait-by-default
- **TX state machine:** `andamio-api/internal/service/tx_state_machine_service.go` — states, polling, DB update queue
- **Existing tx commands:** `cmd/andamio/tx_build.go`, `tx_sign.go`, `tx_submit.go`, `tx_register.go`
- **Security learnings:** `docs/solutions/security-issues/tx-signing-code-review-witness-drop-url-validation.md` — validate URLs at point of use, bound response reads, never discard marshal errors
- **Composability:** `docs/solutions/architecture/cli-composability-audit-and-fix.md` — typed errors, stderr separation, JSON error envelopes
