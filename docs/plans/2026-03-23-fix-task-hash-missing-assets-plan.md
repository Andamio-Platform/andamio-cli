---
title: "fix: Task hash computation missing assets — implement ComputeTaskHash in CLI"
type: fix
status: active
date: 2026-03-23
origin: GitHub Issue #44
---

# Task Hash Computation Missing Assets

## Problem

Task submissions from the CLI produce incorrect on-chain hashes because the assets array is missing from the Plutus Data datum. The managed task hash (correct, includes assets) differs from the submitted task hash (wrong, `assets: null`). This breaks PM assessment flow.

## Root Cause

The CLI does not compute task hashes locally — it trusts `task_hash` from the API via `resolveTaskHash()`. The API's `/v2/tx/project/contributor/task/commit` endpoint builds the on-chain datum, but it appears to omit the assets array from the Plutus Data, causing the on-chain validator to compute a different hash.

## Proposed Solution

Implement `ComputeTaskHash` in the CLI's Go code, matching the `@andamio/core` TypeScript implementation exactly. Use it to:

1. **Verify** the API-returned `task_hash` before submitting (pre-flight check in `commit-tx`)
2. **Diagnose** where the mismatch originates (CLI can log expected vs actual hash)

### The Algorithm (from `@andamio/core/hashing/task-hash.ts`)

Task hash = Blake2b-256 of Plutus Data CBOR encoding:

```
tag(121) + indefinite_array_start(0x9f) + [
  cbor_bytes(NFC_normalize(project_content)),   // ByteArray
  cbor_uint(expiration_time),                    // Int (milliseconds)
  cbor_uint(lovelace_amount),                    // Int (micro-ADA)
  cbor_list(native_assets),                      // List<FlatValue>
] + break(0xff)
```

**Native assets encoding:**
- Empty list: definite empty array `0x80`
- Non-empty: indefinite array of Constructor 0 entries, each containing `[policyId_bytes, tokenName_bytes, quantity_uint]`

**CBOR uint encoding:** Standard major type 0 (1/2/4/8 byte big-endian based on value)

**CBOR bytes encoding:** Standard major type 2 (header + raw bytes)

### Key Details from `@andamio/core`

- Uses **indefinite-length arrays** (`0x9f ... 0xff`) for Plutus Data constructors (matches Haskell's `serialiseData . toBuiltinData`)
- Tag 121 = CBOR tag `0xd8 0x79` = Plutus Data Constructor 0
- `project_content` is **NFC normalized** before encoding to UTF-8 bytes
- Empty tokens list is `0x80` (definite empty array), non-empty uses indefinite array
- Each FlatValue (asset) is also Constructor 0: `tag(121) + indef[policyId_bytes, tokenName_bytes, quantity_uint]`

### Test Vectors (from `@andamio/core` tests)

```
"Introduce Yourself", deadline=1782792000000, lovelace=5000000, assets=[]
→ b1e5c9234e8a4481da7cb3fb525fc54430f8df127ab9f10464ddc8a4e7560614

"Review the Docs", deadline=1782792000000, lovelace=8000000, assets=[]
→ 9d113eafdbe599d624c1ae3e545083e3ec7a053e14ebb6cb730eb3fb59eb3363

"Find a Typo", deadline=1782792000000, lovelace=5000000, assets=[]
→ c79b778c46a26148c5a33ad669b3452ecf0263539270513003abef73c5858cb2
```

Debug bytes for `"Hi", deadline=1, lovelace=2, assets=[]`:
```
d8799f424869010280ff
```

## Implementation

### File: `internal/cardano/task_hash.go` (new)

Port the `@andamio/core` `computeTaskHash` to Go:

```go
type TaskData struct {
    ProjectContent string
    ExpirationTime uint64
    LovelaceAmount uint64
    NativeAssets   []NativeAsset
}

type NativeAsset struct {
    PolicyID  string // 56 hex chars
    TokenName string // hex encoded
    Quantity  uint64
}

func ComputeTaskHash(task TaskData) (string, error)
```

Uses the existing `fxamacker/cbor/v2` for CBOR encoding (already a dependency) OR manual byte construction matching the TypeScript (simpler, no CBOR library quirks with indefinite arrays).

**Recommendation:** Manual byte construction (matching the TypeScript implementation directly) is safer than relying on `fxamacker/cbor` to produce the exact same indefinite-length encoding. The algorithm is simple enough (~80 lines).

### File: `internal/cardano/task_hash_test.go` (new)

Use all 7 on-chain test vectors from `@andamio/core` plus the `debugTaskBytes` vectors.

### File: `cmd/andamio/project_contributor.go` (modify)

In `runProjectContributorCommitTx`, after resolving the task:

```go
// Verify task_hash matches computed hash (diagnose mismatches)
taskData := resolveFullTaskData(c, projectID, taskIndex)
computedHash, _ := cardano.ComputeTaskHash(taskData)
if computedHash != taskHash {
    fmt.Fprintf(os.Stderr, "Warning: API task_hash does not match computed hash\n")
    fmt.Fprintf(os.Stderr, "  API hash:      %s\n", taskHash)
    fmt.Fprintf(os.Stderr, "  Computed hash:  %s\n", computedHash)
    fmt.Fprintf(os.Stderr, "  Using computed hash (includes assets)\n")
    taskHash = computedHash  // Use the correct one
}
```

### File: `cmd/andamio/helpers.go` (modify)

Add `resolveFullTaskData` that fetches the full task data (including assets) from the task list:

```go
func resolveFullTaskData(c *client.Client, projectID string, taskIndex int) (*cardano.TaskData, error)
```

This reads from `/api/v2/project/user/tasks/list`, extracts `content.title` (or `on_chain_content`), `expiration_posix`, `lovelace_amount`, and `assets` for the matching task.

## Acceptance Criteria

- [ ] `ComputeTaskHash` in Go matches all 7 `@andamio/core` test vectors
- [ ] `debugTaskBytes` output matches the TypeScript for `"Hi", 1, 2, []` → `d8799f424869010280ff`
- [ ] Empty assets list encodes as `0x80`
- [ ] Non-empty assets list encodes as indefinite array of Constructor 0 entries
- [ ] NFC normalization applied to project_content
- [ ] `commit-tx` verifies hash before submitting and warns on mismatch
- [ ] Preprod task with XP tokens produces matching hash

## Sources

- GitHub Issue #44
- `@andamio/core/src/utils/hashing/task-hash.ts` — reference implementation
- `@andamio/core/src/utils/hashing/task-hash.test.ts` — 7 on-chain test vectors
- `internal/cardano/sign.go:168` — existing `Blake2b256()` function
- `cmd/andamio/helpers.go:252` — existing `resolveTaskHash()`
