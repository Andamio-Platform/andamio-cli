---
title: "CLI tx state machine pattern enforcement and ComputeTaskHash verification"
date: 2026-03-24
category: architecture
problem_type: architecture_decision
severity: high
status: resolved
tags:
  - cli
  - architecture
  - tx-state-machine
  - task-hash
  - plutus-data
  - cbor
  - blake2b-256
  - commit-tx-removal
modules:
  - cmd/andamio/course_student.go
  - cmd/andamio/project_contributor.go
  - cmd/andamio/tx_lifecycle.go
  - cmd/andamio/project_task.go
  - cmd/andamio/helpers.go
  - internal/cardano/task_hash.go
pull_requests:
  - "#47: ComputeTaskHash, verify-hash, --task-hash flag, remove commit-tx"
---

# TX State Machine Pattern Enforcement and ComputeTaskHash

## Problem

Two issues emerged in the same session:

1. **Architectural violation**: The `commit-tx` commands (added in PR #39) violated the Andamio tx state machine pattern by combining on-chain transactions with off-chain API calls in a single command. This created a dual-write path that bypassed the state machine.

2. **Task hash mismatch** (#44): Task submissions produced wrong hashes because the CLI had no way to compute or verify task hashes locally. The on-chain validator includes native assets in the hash, but the API's submission flow was omitting them.

## Root Cause

### Commit-TX Violation

The `commit-tx` commands did two things in one handler:
1. Ran the tx lifecycle (build → sign → submit → register) — this IS the state machine
2. Made a separate `POST /api/v2/course/student/commitment/submit` with `pending_tx_hash` — this BYPASSES the state machine

The Andamio transaction pattern (matching the app) is:
1. API tx endpoint returns unsigned CBOR
2. User signs and submits
3. Register with state machine (tx_hash + tx_type + metadata)
4. Gateway watches for confirmation, syncs DB
5. On-chain and off-chain data are synced

No step should make direct off-chain API calls alongside the tx flow. Evidence reaches the DB through the register metadata or through separate off-chain commands (`course student submit`, `project contributor update`).

### Task Hash Mismatch

The CLI did not compute task hashes — it trusted `task_hash` from the API. When the API's tx builder omitted native assets from the Plutus Data datum, the on-chain validator computed a different hash. Without local hash computation, the CLI had no way to detect or diagnose this.

## Solution

### 1. Remove commit-tx, enforce tx run as the only tx path

Removed `course student commit-tx` and `project contributor commit-tx` entirely. All on-chain transactions use `tx run` (or the individual `tx build` → `tx sign` → `tx submit` → `tx register` steps).

`tx run` remains as the composable convenience — it follows the state machine exactly. No one-off commands that add extra API calls.

### 2. Implement ComputeTaskHash matching @andamio/core

Ported the `computeTaskHash` function from `@andamio/core/hashing/task-hash.ts` to Go. The algorithm:

```
Blake2b-256(
  tag(121) + 0x9f + [
    cbor_bytes(NFC_normalize(project_content)),
    cbor_uint(expiration_time),
    cbor_uint(lovelace_amount),
    cbor_list(native_assets)
  ] + 0xff
)
```

Key encoding details:
- Tag 121 = `0xd8 0x79` = Plutus Data Constructor 0
- Indefinite-length arrays (`0x9f ... 0xff`) for constructors
- Empty tokens list = definite empty array `0x80`
- Non-empty tokens = indefinite array of Constructor 0 entries (policyId, tokenName, quantity)
- Content is NFC-normalized before UTF-8 encoding
- Manual CBOR encoding (not fxamacker/cbor) — needed for exact control over indefinite vs definite arrays

All 7 on-chain test vectors from `@andamio/core` pass.

### 3. Add diagnostic verify-hash command

`project task verify-hash <project-id>` fetches all tasks, computes hashes locally, and reports mismatches. This diagnoses where assets or other fields are missing.

### 4. Add --task-hash flag for chain-only tasks

Same pattern as `--slt-hash` from #42. Chain-only tasks have no `task_index`, so contributor commands now accept `--task-hash` as alternative. Both flags validate as 64-char hex strings.

## Prevention Strategies

### The TX State Machine Rule

**Every transaction must follow the same composable pattern:**
1. `tx build` — API returns unsigned CBOR
2. `tx sign` — local .skey signing
3. `tx submit` — submit to Cardano network
4. `tx register` — register with state machine + metadata
5. Gateway watches for confirmation, syncs DB

`tx run` composes steps 1-5. No CLI command should add extra API calls alongside this flow. Off-chain operations (evidence submission, evidence update) are separate commands that run before or after the tx flow, not during it.

**Test:** If a new command calls both `executeTxLifecycle` AND a separate API POST, it violates the pattern.

### Hash Verification for On-Chain Data

When the CLI sends a hash to a build endpoint, verify it matches a locally-computed hash before signing. This catches API-side bugs where the datum construction omits fields (like native assets).

The `ComputeTaskHash` function provides this for tasks. A similar `ComputeSltHash` could be added for course modules if needed.

### Flag Validation Prevents Panics

Always validate `--*-hash` flags as 64-char hex before using them. A short string passed to `hash[:16]` for progress messages will panic. The `isHex()` helper already exists in the codebase.

## Key Files

| File | Purpose |
|------|---------|
| `internal/cardano/task_hash.go` | `ComputeTaskHash` — Plutus Data CBOR + Blake2b-256 |
| `internal/cardano/task_hash_test.go` | 7 on-chain vectors + CBOR encoding tests |
| `cmd/andamio/project_task.go` | `verify-hash` diagnostic command |
| `cmd/andamio/helpers.go` | `resolveTaskHashFromFlags`, `resolveTaskData` |
| `cmd/andamio/tx_lifecycle.go` | `executeTxLifecycle` (used only by `tx run`) |

## Related Documentation

- `docs/solutions/feature-implementations/cli-onchain-commitment-commands-and-address-derivation.md` — the earlier solution doc (pre-removal)
- `docs/brainstorms/2026-03-20-tx-run-full-lifecycle-command-brainstorm.md` — tx run design
- `@andamio/core/src/utils/hashing/task-hash.ts` — reference TypeScript implementation
- Issue #44: task hash mismatch due to missing assets
- Issue #45: --task-hash flag for chain-only tasks
