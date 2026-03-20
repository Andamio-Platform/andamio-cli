# TX Run — Full Lifecycle Command

**Date:** 2026-03-20
**Status:** Brainstorm
**Author:** James + Claude

## What We're Building

A single `andamio tx run` command that executes the full Cardano transaction lifecycle: build, sign, submit, register with the gateway state machine, and wait for DB confirmation. One command replaces the current 5-step manual flow.

### The Problem Today

```bash
# 5 separate commands, manual copy-paste between each:
andamio tx build /v2/tx/... --body '...' -o json | jq -r '.unsigned_tx' > unsigned.hex
andamio tx sign --tx-file unsigned.hex --skey payment.skey -o json | jq -r '.signed_tx' > signed.hex
TX_HASH=$(andamio tx sign --tx-file unsigned.hex --skey payment.skey -o json | jq -r '.tx_hash')
andamio tx submit --tx-file signed.hex
andamio tx register --tx-hash $TX_HASH --tx-type assessment_assess
andamio tx status $TX_HASH  # poll manually
```

### After

```bash
andamio tx run /v2/tx/course/teacher/assignments/assess \
  --body '{"alias":"teacher-01","course_id":"abc123","assignment_decisions":[...]}' \
  --skey ./payment.skey \
  --tx-type assessment_assess
```

Output (step-by-step on stderr):
```
  ✓ Built unsigned TX
  ✓ Signed with payment.skey
  ✓ Submitted to network (tx: abc123def...)
  ✓ Registered as assessment_assess
  ⏳ Waiting for confirmation...
  ✓ Confirmed on-chain
  ✓ DB updated — complete!
```

## Why This Approach

1. **Humans first** — this is for teachers, managers, developers running transactions interactively. The step-by-step progress gives confidence that each phase succeeded.
2. **One command** — reduces the 5-step flow to a single invocation. No copy-paste, no intermediate files.
3. **Existing commands stay** — `tx build`, `tx sign`, `tx submit`, `tx register`, `tx status` remain for advanced use, scripting, and debugging. `tx run` is a convenience layer on top.
4. **Wait by default** — the command blocks and streams state machine updates until terminal state (updated/failed/expired). `--no-wait` exits after registration for scripts that don't want to block.

## Key Decisions

### 1. Command: `tx run` (new subcommand)

All 5 existing tx commands remain untouched. `tx run` is additive. Power users and scripts can still use the individual commands.

### 2. TX type: explicit `--tx-type` flag

User passes `--tx-type assessment_assess`. Simple, explicit, no magic mapping table to maintain. `andamio tx types` lists all 17 valid values.

### 3. Wait behavior: wait by default, `--no-wait` to skip

The command streams SSE events from `/api/v2/tx/stream/{tx_hash}` to show real-time state transitions. On `--no-wait`, it exits after registration with the tx_hash.

### 4. Progress display: step-by-step status lines to stderr

Each phase prints a checkmark line to stderr as it completes. In `--output json` mode, all progress is suppressed and the final result goes to stdout.

### 5. Signing key: `--skey` required flag

The command needs the signing key to complete the loop. Uses the existing `internal/cardano` signing logic.

### 6. Submit URL: same resolution as `tx submit`

`--submit-url` flag > `config.SubmitURL` > error. Same pattern, same flag.

## Proposed Flags

```
andamio tx run <endpoint> [flags]

Required:
  --body <json>         Request body for the build endpoint
  --skey <path>         Path to Cardano .skey file for signing
  --tx-type <type>      Transaction type for registration (e.g. assessment_assess)

Optional:
  --body-file <path>    Read body from file instead of --body
  --submit-url <url>    Override submit API URL
  --submit-header <k:v> Additional submit headers (repeatable)
  --instance-id <id>    Course or project ID for registration
  --metadata <k=v>      Metadata for registration (repeatable, e.g. --metadata task_hash=abc123)
  --no-wait             Exit after registration without waiting for confirmation
  --timeout <duration>  Max wait time (default: 10m)
  -o, --output          Output format (text/json)
```

## Execution Flow (internal)

```
1. Parse flags and validate
2. Load config, create client
3. POST to build endpoint → get unsigned_tx
4. Sign unsigned_tx with .skey → get signed_tx + tx_hash
5. Submit signed_tx to submit API → confirm acceptance
6. POST to /api/v2/tx/register → register with state machine
7. If --no-wait: print tx_hash, exit
8. Poll GET /api/v2/tx/status/{tx_hash} every 5s
9. Print state transitions as they change
10. On terminal state (updated/failed/expired): print final result, exit
```

### Error handling

- Build fails → exit with API error
- Sign fails → exit with skey/CBOR error
- Submit fails → exit with submit API error
- Register fails → print warning but still print tx_hash (TX is on-chain, just not tracked)
- Poll fails → retry with backoff, give up after --timeout
- Timeout → exit with "timed out waiting for confirmation" + tx_hash for manual follow-up

### JSON output (--output json)

```json
{
  "tx_hash": "abc123...",
  "tx_type": "assessment_assess",
  "state": "updated",
  "build_response": { ... },
  "submitted_at": "2026-03-20T12:00:00Z",
  "confirmed_at": "2026-03-20T12:00:45Z",
  "updated_at": "2026-03-20T12:00:46Z"
}
```

## Resolved Questions

- **How to determine tx_type?** → Explicit `--tx-type` flag. No inference from endpoint path.
- **Wait or not?** → Wait by default, `--no-wait` to skip.
- **Command name?** → `tx run`. New subcommand, existing commands untouched.
- **Progress UX?** → Step-by-step checkmark lines to stderr.
- **Audience?** → Humans first, but `--output json` + `--no-wait` makes it script-friendly too.

## Open Questions

(None — all resolved.)

## Additionally Resolved

- **Metadata field** → Yes, add `--metadata key=value` (repeatable). Needed for `project_join` and `project_credential_claim` which require `task_hash`.
- **SSE vs polling** → Poll `tx status` every 5s. Simpler, works through all proxies, good enough for 30-60s waits.
- **Instance ID inference** → No inference. Always require explicit `--instance-id` flag. Keep it simple.
