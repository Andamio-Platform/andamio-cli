---
title: "CLI v0.8.0-v0.8.1: On-chain commitment commands, address derivation, and slt-hash flag"
date: 2026-03-23
category: feature-implementations
problem_type: feature_implementation
severity: high
status: resolved
tags:
  - cli
  - on-chain-transactions
  - commitment-commands
  - evidence-pipeline
  - tiptap-json
  - blake2b-256
  - wallet-address-derivation
  - cardano
  - bech32
  - cip-19
  - slt-hash
  - chain-only-modules
  - tx-lifecycle
modules:
  - cmd/andamio/course_student.go
  - cmd/andamio/project_contributor.go
  - cmd/andamio/tx_lifecycle.go
  - cmd/andamio/tx_run.go
  - cmd/andamio/helpers.go
  - cmd/andamio/user.go
  - internal/cardano/sign.go
  - internal/cardano/address.go
  - internal/config/config.go
pull_requests:
  - "#39: feat: on-chain commitment commands with evidence"
  - "#41: fix: derive wallet address from skey during headless login"
  - "#43: fix: add --slt-hash flag for chain-only modules"
releases:
  - v0.8.0
  - v0.8.1
---

# On-Chain Commitment Commands, Address Derivation, and SLT Hash Resolution

## Summary

Added two on-chain commitment commands (`course student commit-tx`, `project contributor commit-tx`) that combine evidence preparation with the full Cardano transaction lifecycle in a single command. Fixed headless login to derive enterprise addresses from skey files. Added `--slt-hash` flag for chain-only modules that lack a `course_module_code`.

This work spans 3 PRs (#39, #41, #43), 2 releases (v0.8.0, v0.8.1), and ~2,200 lines added across 15 files.

## Key Patterns Established

### 1. `executeTxLifecycle` -- Reusable Transaction Orchestration

**File:** `cmd/andamio/tx_lifecycle.go:36-225`

Extracted from `tx run` into a standalone function that accepts `TxLifecycleParams` and returns `*RunResult`. The 5-step pipeline (build, sign, submit, register, poll) is now shared by `tx run`, `course student commit-tx`, and `project contributor commit-tx`.

- Accepts `*client.Client` and `*config.Config` as injected dependencies
- Returns partial progress in `RunResult` on any failure (`Step` shows where it stopped)
- The `fail()` closure (line 58) uses `ReportedError` in JSON mode to prevent double error printing
- SIGINT handler (line 72) prints recovery guidance if a tx hash is known
- `--no-wait` path prints JSON directly; callers check `result.State != "registered"` to avoid double-print

**Rule:** The lifecycle function returns data. Only the top-level Cobra RunE handler prints to stdout.

### 2. Two-Phase Commit (On-Chain TX + Off-Chain Evidence)

**Files:** `course_student.go:480-505`, `project_contributor.go:384-408`

The on-chain transaction carries only the evidence hash (64-char hex in `assignment_info`/`task_info`). The full Tiptap evidence document is sent to the REST API separately with `pending_tx_hash` to link the two.

**Execution order:**
1. Execute full tx lifecycle (build, sign, submit, register)
2. POST off-chain evidence with `pending_tx_hash: result.TxHash`
3. Poll for on-chain confirmation

**Error handling:** Off-chain failure is non-fatal (the on-chain tx is already submitted). The error is captured in `offchainError` string and surfaced in `CommitTxResult.OffchainError` for JSON consumers. Text mode prints a warning with a recovery command.

### 3. Evidence Pipeline (Markdown to Tiptap to Blake2b-256)

**File:** `helpers.go:230-248` (`wrapEvidence`)

```
readEvidenceFlag (--evidence or --evidence-file, max 1MB)
  -> markdownToTiptap(text, nil)
    -> normalizeForHashing(tiptapDoc)  // trim strings, Go sorts map keys
      -> json.Marshal(normalized)
        -> cardano.Blake2b256(jsonBytes)  // 32-byte hash, 64-char hex
```

The normalized doc (not the original) is sent to the off-chain API. The hash matches `@andamio/core`'s `computeCommitmentHash` function.

### 4. `CommitTxResult` Embedding Pattern

**File:** `course_student.go:367-374`

```go
type CommitTxResult struct {
    RunResult                           // embedded, inherits all fields
    EvidenceHash  string `json:"evidence_hash,omitempty"`
    SltHash       string `json:"slt_hash,omitempty"`
    TaskHash      string `json:"task_hash,omitempty"`
    OffchainError string `json:"offchain_error,omitempty"`
}
```

Go struct embedding gives flat JSON output. When `RunResult` gains fields, `CommitTxResult` inherits them automatically. Never duplicate fields -- embed.

### 5. Address Derivation from Skey (CIP-19 Enterprise Address)

**File:** `internal/cardano/address.go:16-42`

```
enterprise_address = bech32_encode(hrp, header_byte || blake2b_224(pubkey))
```

- Testnet: header `0x60`, HRP `addr_test`
- Mainnet: header `0x61`, HRP `addr`
- Uses `btcsuite/btcd/btcutil/bech32` (already transitive via Bursa)
- Network detected from `cfg.IsMainnet()` which checks `strings.Contains(c.BaseURL, "mainnet")`
- Called as fallback in headless login when API returns null for `cardano_bech32_addr`

### 6. `--slt-hash` / `--module-code` Mutual Exclusion

**File:** `helpers.go:284-301` (`resolveSltHashFromFlags`)

Chain-only modules have no `course_module_code`. The `--slt-hash` flag bypasses resolution by accepting the hash directly. Returns `(sltHash, moduleCode, error)` where `moduleCode` may be empty.

Not using Cobra's `MarkFlagRequired` because it doesn't support "one of two is required". Manual validation with discovery hints in error messages.

### 7. `warnSkeyMismatch` Pre-Flight Check

**File:** `helpers.go:399-416`

Compares the skey's key hash against `cfg.UserKeyHash` (stored during login). Warns but does not abort -- the mismatch might be intentional (multi-sig flows). Uses `cardano.PubKeyHash` (blake2b-224) for comparison, which works regardless of address type.

## Review Findings and Fixes

PR #39 was reviewed by 7 agents. 10 findings were identified and fixed:

| # | Finding | Severity | Fix |
|---|---------|----------|-----|
| 1 | Double JSON output in `--no-wait` path | P1 | Added `result.State != "registered"` guard |
| 2 | Missing `pending_tx_hash` in project off-chain payload | P1 | Added to payload |
| 3 | `CommitTxResult` duplicated `RunResult` fields | P2 | Embedded `RunResult` |
| 4 | `resolveContributorStateID` in wrong file | P2 | Moved to `helpers.go` |
| 5 | Ignored `checkJWTExpiry` return in `tx_run.go` | P2 | Now checked |
| 6 | No skey-to-address validation | P2 | Added `warnSkeyMismatch` |
| 7 | Unbounded evidence file read | P3 | Added 1MB size guard |
| 8 | Off-chain failure invisible in JSON | P3 | Added `OffchainError` field |
| 9 | `task-index` flag String instead of Int | P3 | Changed to Int for `commit-tx` |
| 10 | Redundant hash preview length guard | P3 | Simplified |

## Prevention Strategies

### Output Ownership: Caller Prints, Callee Returns

When extracting shared functions, the function returns data and the Cobra RunE handler owns stdout output. Search new functions for `output.Print` or `fmt.Print` calls -- if the function is not a RunE, those calls are a bug.

### Struct Extension: Embed, Never Duplicate

If struct B extends struct A, embed A. Manual field copying is a maintenance trap that silently drops new fields.

### File Placement: Resolvers in helpers.go

Any function used by multiple command files, or any `resolve*` function, belongs in `helpers.go`. Command files contain only Cobra definitions and their RunE functions.

### Pre-Flight Validation Before Network Calls

Check JWT expiry, skey-address match, and flag validation before making any API call. Always check `checkJWTExpiry`'s return value.

### Defense in Depth for File Reads

All `--*-file` flags should enforce a size limit before `os.ReadFile`. Current limit: 1MB for evidence files.

### JSON Output Completeness

Every failure path must be representable in JSON output. Use `omitempty` error fields (like `OffchainError`) for partial failures so scripts can detect and handle them.

## Related Documentation

### Solution Docs
- `docs/solutions/integration-issues/evidence-submission-payload-format-and-field-alignment.md` -- the `wrapEvidence` pattern
- `docs/solutions/security-issues/tx-signing-code-review-witness-drop-url-validation.md` -- signing security
- `docs/solutions/feature-implementations/cli-course-import-markdown-to-tiptap-conversion.md` -- goldmark pipeline
- `docs/solutions/architecture/cli-composability-audit-and-fix.md` -- stderr/stdout rules

### Brainstorms
- `docs/brainstorms/2026-03-18-cli-wallet-signing-brainstorm.md` -- signing architecture
- `docs/brainstorms/2026-03-20-tx-run-full-lifecycle-command-brainstorm.md` -- tx run design

### Dependency Chain
```
brainstorm: wallet-signing (03-18)
  -> brainstorm: tx-run-lifecycle (03-20)
    -> PR #35: off-chain API coverage
      -> PR #37: evidence format fix (v0.7.0)
        -> PR #39: on-chain commit-tx commands (v0.8.0)
          -> PR #41: address derivation fix (v0.8.1)
            -> PR #43: --slt-hash flag (pending)
```
