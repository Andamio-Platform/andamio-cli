---
title: "fix: task hash mismatch for content > 64 bytes (PlutusTx chunked encoding)"
type: fix
status: completed
date: 2026-04-08
---

# fix: task hash mismatch for content > 64 bytes (PlutusTx chunked encoding)

## Overview

`ComputeTaskHash` produces incorrect hashes when `ProjectContent` exceeds 64 bytes because it uses definite-length CBOR encoding instead of PlutusTx's indefinite-length chunked encoding. The fix already exists in the same package — `encodePlutusBuiltinByteString` in `slt_hash.go` handles chunking correctly.

## Problem Frame

Off-chain task hashes don't match on-chain hashes for tasks with content > 64 bytes. This causes task records to remain `chain_only` (can't merge with DB records), breaks `project_join` and `project_credential_claim` flows, and `verify-hash` reports false mismatches. See GitHub issue #58.

## Requirements Trace

- R1. `ComputeTaskHash` must produce hashes matching PlutusTx's `hash_project_data` for all valid content lengths (0–140 chars)
- R2. Content > 64 bytes must use indefinite-length chunked CBOR encoding (64-byte chunks) matching PlutusTx's `stringToBuiltinByteString`
- R3. A test vector with content > 64 bytes must exist to prevent regression
- R4. Existing test vectors must continue to pass (content ≤ 64 bytes is unaffected)

## Scope Boundaries

- Only `task_hash.go` encoding is affected — `slt_hash.go` already handles chunking correctly
- No CLI command changes needed — only the internal hash computation
- No API changes

## Context & Research

### Relevant Code and Patterns

- `internal/cardano/slt_hash.go:39-58` — `encodePlutusBuiltinByteString()` already implements the correct chunked encoding with the 64-byte boundary
- `internal/cardano/task_hash.go:85` — the call site that needs to change from `encodeCBORBytes` to `encodePlutusBuiltinByteString`
- `internal/cardano/task_hash.go:148-165` — `encodeCBORBytes()` always produces definite-length encoding (correct for ≤64 bytes, wrong for >64)

### Key Insight

The fix is a one-line change: replace `encodeCBORBytes(contentBytes)` with `encodePlutusBuiltinByteString(contentBytes)` on line 85 of `task_hash.go`. The `encodePlutusBuiltinByteString` function delegates to `encodeCBORBytes` for data ≤64 bytes, so existing behavior is preserved.

## Key Technical Decisions

- **Reuse `encodePlutusBuiltinByteString`**: The function already exists in the same package and is battle-tested by SLT hash computation. No need to duplicate chunking logic.
- **Test-first approach**: Write a failing test with the known >64-byte vector from the issue before applying the fix, to confirm the test catches the bug and then passes after.

## Open Questions

### Resolved During Planning

- **Should `encodeTokensList` also use chunked encoding for policy IDs / token names?** No — policy IDs are exactly 28 bytes and token names max 32 bytes, both well under 64 bytes. Only `ProjectContent` can exceed 64 bytes (up to 140 chars = up to ~560 bytes for multi-byte UTF-8).

### Deferred to Implementation

- **Exact on-chain hash for the 73-byte test vector**: The issue provides both the CLI (wrong) and on-chain (correct) hash. The on-chain hash `395a410edd42e5cfa9c56f4304b690193caecbe81f02150075bb32b9ce327d57` will be used as the test vector.

## Implementation Units

- [x] **Unit 1: Add failing test for >64-byte content**

**Goal:** Prove the bug exists with a test that fails before the fix

**Requirements:** R3, R4

**Dependencies:** None

**Files:**
- Modify: `internal/cardano/task_hash_test.go`

**Approach:**
- Add a test case using the exact reproduction data from issue #58: 73-byte content, lovelace 5000000, expiration 2026-12-31 (Unix ms)
- Expected hash: `395a410edd42e5cfa9c56f4304b690193caecbe81f02150075bb32b9ce327d57` (on-chain value)
- Also add a `DebugTaskBytes` test to verify the CBOR encoding contains `0x5f` (indefinite start) for content > 64 bytes
- Run tests — the new test should fail

**Execution note:** Write this test first. Verify it fails before proceeding to Unit 2.

**Patterns to follow:**
- `TestComputeTaskHash_OnChainVectors` for the hash assertion pattern
- `TestDebugTaskBytes` for the CBOR byte-level verification pattern

**Test scenarios:**
- 73-byte content produces hash `395a410e...` (currently fails, will pass after fix)
- Content at exactly 64 bytes uses definite-length encoding (boundary check)
- Content at 65 bytes uses chunked encoding (boundary check)

**Verification:**
- Test fails with hash mismatch before the fix is applied

- [x] **Unit 2: Fix `encodeTaskAsPlutusData` to use chunked encoding**

**Goal:** Replace `encodeCBORBytes` with `encodePlutusBuiltinByteString` for content encoding

**Requirements:** R1, R2

**Dependencies:** Unit 1

**Files:**
- Modify: `internal/cardano/task_hash.go`

**Approach:**
- Change line 85: `encodeCBORBytes(contentBytes)` → `encodePlutusBuiltinByteString(contentBytes)`
- No other changes needed — `encodePlutusBuiltinByteString` already handles both short (≤64) and long (>64) byte strings correctly

**Patterns to follow:**
- `slt_hash.go:25` — uses `encodePlutusBuiltinByteString` for SLT strings

**Test scenarios:**
- All existing `onChainVectors` still pass (content ≤22 bytes, well under 64)
- New >64-byte test from Unit 1 now passes
- `DebugTaskBytes` for short content remains unchanged

**Verification:**
- `go test ./internal/cardano/...` passes all tests including the new one
- The 73-byte content hash matches the on-chain value

## System-Wide Impact

- **`compute-hash` command**: Will now produce correct hashes for long content — no code change needed, it already calls `ComputeTaskHash`
- **`verify-hash` command**: Will stop reporting false mismatches for tasks with content > 64 bytes
- **`task import`**: Task hash lookups will match correctly for long-content tasks
- **No API surface change**: The fix is entirely internal to the CBOR encoding

## Risks & Dependencies

- **Low risk**: The fix reuses an existing, tested function. The only change is one function call substitution.
- **Backward compatibility**: Short content (≤64 bytes) produces identical CBOR encoding through both code paths, so no existing hashes change.

## Sources & References

- Related issue: #58
- Existing chunked encoding: `internal/cardano/slt_hash.go:39-58`
