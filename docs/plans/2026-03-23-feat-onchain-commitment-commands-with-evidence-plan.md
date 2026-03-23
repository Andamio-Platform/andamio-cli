---
title: "feat: On-chain commitment commands with evidence for courses and projects"
type: feat
status: completed
date: 2026-03-23
origin: GitHub Issue #38
---

# On-Chain Commitment Commands with Evidence

## Overview

Add domain-specific commands that combine evidence preparation with the full Cardano transaction lifecycle for assignment commitments (courses) and task commitments (projects). Each command wires together existing building blocks (`wrapEvidence`, `tx run` orchestration, hash resolvers) into a single-command workflow that mirrors what the app does in `assignment-commitment.tsx`.

Two new commands:
- `course student commit-tx` -- on-chain assignment commitment with evidence
- `project contributor commit-tx` -- on-chain task commitment with evidence

Both replace a manual multi-step bash-script workflow with a single CLI invocation.

## Problem Statement / Motivation

The CLI has all the pieces but they aren't wired together:

| Building block | Location | Status |
|---------------|----------|--------|
| `wrapEvidence()` (markdown -> Tiptap + Blake2b-256) | `cmd/andamio/helpers.go:230` | Exists |
| `markdownToTiptap()` | `cmd/andamio/course_import.go:652` | Exists |
| `readEvidenceFlag()` | `cmd/andamio/helpers.go:314` | Exists |
| `resolveSltHash()` | `cmd/andamio/helpers.go:283` | Exists |
| `resolveTaskHash()` | `cmd/andamio/helpers.go:252` | Exists |
| `tx run` lifecycle (build->sign->submit->register->poll) | `cmd/andamio/tx_run.go` | Exists |
| `cardano.LoadSigningKey()` + `SignTransaction()` | `internal/cardano/sign.go` | Exists |

Today, users must: (1) convert evidence to Tiptap JSON, (2) compute Blake2b-256 hash, (3) construct a complex `--body` JSON with the hash as `assignment_info`, (4) pass evidence as `--metadata` on `tx run`. A bash script with embedded Python (`040-testing/xp/preprod/assignment-commit.sh` in orch) fills this gap. That script reimplements logic already in the CLI's Go code.

## Proposed Solution

### Command 1: `course student commit-tx`

Single command for on-chain assignment commitment with evidence. Two-phase: on-chain tx + off-chain evidence storage.

```bash
andamio course student commit-tx \
  --course-id abc123... \
  --module-code 101 \
  --evidence-file ./my-evidence.md \
  --skey ./payment.skey
```

**Flags:**

| Flag | Required | Description |
|------|----------|-------------|
| `--course-id` | yes | Course policy ID |
| `--module-code` | yes | Human-readable module code |
| `--evidence` | one of | Inline markdown evidence |
| `--evidence-file` | one of | Path to markdown evidence file |
| `--skey` | yes | Path to `.skey` file for signing |
| `--submit-url` | no | Override submit API URL |
| `--submit-header` | no | Additional submit headers (repeatable) |
| `--no-wait` | no | Exit after registration |
| `--timeout` | no | Max poll time (default 10m) |

**Execution flow:**

```
1. readEvidenceFlag(cmd)                    -> raw markdown string
2. wrapEvidence(raw)                        -> tiptapDoc, evidenceHash
3. Load config, create client
4. resolveSltHash(c, courseID, moduleCode)   -> sltHash
5. Resolve wallet address (see "Address Resolution" below)
6. Construct build body:
   {
     "alias":           cfg.UserAlias,
     "assignment_info": evidenceHash,       // 64-char hex, fits 140-char limit
     "course_id":       courseID,
     "slt_hash":        sltHash,
     "initiator_data": {
       "change_address": address,
       "used_addresses": [address]
     }
   }
7. POST /v2/tx/course/student/assignment/commit -> unsigned_tx
8. Sign with .skey                              -> signed_tx, tx_hash
9. Submit to Cardano network
10. Register as tx_type="assignment_submit", instance_id=courseID
    metadata: { slt_hash, course_module_code, evidence_hash }
11. Off-chain evidence submission:
    POST /api/v2/course/student/commitment/submit
    {
      "course_id":      courseID,
      "slt_hash":       sltHash,
      "evidence":       tiptapDoc,          // Tiptap JSON object
      "evidence_hash":  evidenceHash,
      "pending_tx_hash": txHash
    }
12. Poll for on-chain confirmation (unless --no-wait)
```

**Output (stderr, text mode):**
```
  Preparing evidence...
  ✓ Evidence hashed (blake2b-256: abc123...)
  ✓ Resolved slt_hash for module 101
  ✓ Built unsigned TX
  ✓ Signed with payment.skey
  ✓ Submitted to network (tx: def456...)
  ✓ Registered as assignment_submit
  ✓ Evidence submitted to API
  ⏳ Waiting for confirmation...
  ✓ Confirmed on-chain
  ✓ DB updated — complete!
```

**Output (--output json):**
```json
{
  "tx_hash": "def456...",
  "tx_type": "assignment_submit",
  "state": "updated",
  "step": "complete",
  "evidence_hash": "abc123...",
  "slt_hash": "...",
  "course_id": "...",
  "course_module_code": "101"
}
```

### Command 2: `project contributor commit-tx`

Parallel implementation for on-chain task commitment with evidence.

```bash
andamio project contributor commit-tx \
  --project-id abc123... \
  --task-index 0 \
  --evidence-file ./my-evidence.md \
  --skey ./payment.skey
```

**Flags:** Same pattern, with `--project-id` and `--task-index` replacing `--course-id` and `--module-code`.

**Execution flow:**

```
1-2. Same evidence preparation
3.   Load config, create client
4.   resolveTaskHash(c, projectID, taskIndex)  -> taskHash
5.   Resolve contributor_state_id (see below)
6.   Construct build body:
     {
       "alias":                cfg.UserAlias,
       "task_info":            evidenceHash,
       "project_id":           projectID,
       "task_hash":            taskHash,
       "contributor_state_id": contributorStateID,
       "initiator_data": {
         "change_address": address,
         "used_addresses": [address]
       }
     }
7.   POST /v2/tx/project/contributor/task/commit -> unsigned_tx
8-10. Sign, submit, register as tx_type="task_submit", instance_id=projectID
11.  Off-chain: POST /api/v2/project/contributor/commitment/update
     { "task_hash": taskHash, "evidence": tiptapDoc, "evidence_hash": evidenceHash }
12.  Poll for confirmation
```

## Technical Considerations

### Address Resolution (Critical -- Blocks Both Commands)

The build endpoints require `initiator_data` with `change_address` (bech32) and `used_addresses` (bech32 array). The CLI only has a `.skey` file.

**Recommended approach: Save address from login, derive as fallback.**

Phase 1 -- Store from login:
- The headless login response already returns `cardano_bech32_addr` (`cmd/andamio/user.go:313`)
- Currently not saved to config. **Add `UserAddress string` to `Config` struct.**
- Save during both browser and headless login flows
- Use as `change_address` and sole `used_addresses` element

Phase 2 (optional) -- Derive from skey:
- Compute `blake2b-224(pubKey)` to get key hash (already have `blake2b224` in `internal/cardano/sign.go`)
- Detect network from `cfg.BaseURL`: `preprod.api.` -> testnet, `mainnet.api.` -> mainnet
- Encode as bech32 enterprise address: `addr_test1` (testnet header 0x60) or `addr1` (mainnet header 0x61)
- Requires adding a bech32 library dependency (or minimal implementation)

Phase 1 is sufficient for v1. The address is already returned by the API -- we just need to persist it.

### Two-Phase Commit: On-Chain + Off-Chain

The `SubmitAssignmentCommitmentV2Request` schema confirms a `pending_tx_hash` field. This reveals the pattern:

1. **On-chain**: Evidence hash goes into `assignment_info`/`task_info` as part of the Cardano transaction
2. **Off-chain**: Full Tiptap evidence document + `pending_tx_hash` are sent to the REST API for database storage

The CLI must make BOTH calls. The state machine tracks the on-chain tx, but it cannot reconstruct the full evidence document from just the hash. The off-chain call should happen **after registration** (the API needs the tx to be registered to link it).

**Timing:** Register first, then submit off-chain evidence with `pending_tx_hash`. The API associates the evidence with the pending transaction. When the state machine confirms the on-chain tx, it updates the DB record that already has the evidence.

### Contributor State ID (Project Commands)

`CommitTaskTxRequest` requires `contributor_state_id` -- a policy ID hash. The existing pattern in `project_task.go` resolves this via `findProjectPolicyID()` which fetches from the manager projects list endpoint.

**Question to verify:** Can a contributor access `contributor_state_id` from the contributor projects list endpoint (`/v2/project/contributor/projects/list`)? If not, can it be fetched from the public project endpoint (`/api/v2/project/user/project/{id}`)?

**Fallback:** Add `--contributor-state-id` flag for manual input if API resolution isn't available to contributors.

### Extracting Shared TX Lifecycle

The `tx run` command's orchestration logic (`tx_run.go:93-370`) should be extracted into a reusable function:

```go
// cmd/andamio/tx_lifecycle.go (new file)

type TxLifecycleParams struct {
    Endpoint    string
    Body        interface{}
    SkeyPath    string
    TxType      string
    InstanceID  string
    Metadata    map[string]string
    NoWait      bool
    Timeout     time.Duration
    SubmitURL   string
    Headers     []string
}

func executeTxLifecycle(cmd *cobra.Command, c *client.Client, params TxLifecycleParams) (*RunResult, error)
```

Both `tx run` and the new commands call this. `tx run` constructs params from its flags. The new commands construct params programmatically with evidence-derived values.

### Evidence Is Optional

The build schemas show `assignment_info` and `task_info` are not required fields. Users might want to commit first and submit evidence later (matching the two-step off-chain flow).

If `--evidence`/`--evidence-file` is omitted:
- Skip Tiptap conversion and hashing
- Omit `assignment_info`/`task_info` from build body
- Skip off-chain evidence submission
- Just execute the on-chain commitment tx

This keeps the command useful for commit-only flows.

### Error Handling

Same model as `tx run` with domain-specific additions:

| Stage | Error | Action |
|-------|-------|--------|
| Evidence prep | Markdown parse failure | Exit with parse error |
| SLT/task resolution | Module not on-chain | Exit: "Module X has no slt_hash. Is it published?" |
| SLT/task resolution | Module/task not found | Exit: "Module code X not found. Run `andamio course modules <id>`" |
| Build | Insufficient funds | Exit with balance hint |
| Build | No access token | Exit: "Mint an access token first: `andamio tx run ...`" |
| Build | Commitment already exists | Exit: "Active commitment exists. Use `update-tx` or `leave` first" |
| Sign | Key mismatch | Exit: "Signing key does not match authenticated user" |
| Off-chain submit | API error | Warning (tx is already on-chain). Print tx_hash for manual recovery |
| Poll | Timeout | Exit with tx_hash for `andamio tx status` follow-up |

The `fail()` closure pattern from `tx run` captures partial progress in `RunResult` for JSON output.

## System-Wide Impact

- **API surface parity**: The app (`assignment-commitment.tsx`) does this flow in the browser. CLI parity means any commitment flow works headlessly.
- **State lifecycle**: Two-phase commit means partial failure is possible (on-chain succeeds, off-chain fails). The `pending_tx_hash` pattern handles this -- the state machine will still confirm the on-chain tx, and evidence can be re-submitted via the existing `course student submit` command.
- **Integration test scenarios**: (1) Full commit-tx with evidence end-to-end on preprod, (2) commit-tx without evidence (commit-only), (3) off-chain failure recovery, (4) skey/JWT mismatch detection.

## Acceptance Criteria

### Course Student Commit-TX

- [x] `course student commit-tx` builds on-chain tx with evidence hash as `assignment_info`
- [x] Evidence markdown is converted to Tiptap JSON and Blake2b-256 hashed automatically
- [x] `slt_hash` is resolved from `--module-code` automatically
- [x] Full tx lifecycle: build -> sign -> submit -> register -> poll
- [x] Off-chain evidence submitted with `pending_tx_hash` after registration
- [x] `--output json` produces structured result with `tx_hash`, `evidence_hash`, `slt_hash`
- [x] Evidence is optional -- omitting `--evidence`/`--evidence-file` does commit-only
- [x] Works without TTY (composable, no interactive prompts)
- [x] Progress messages to stderr, suppressed in JSON mode

### Project Contributor Commit-TX

- [x] `project contributor commit-tx` mirrors the course flow for task commitments
- [x] `task_hash` resolved from `--task-index` automatically
- [x] `contributor_state_id` resolved automatically (from contributor projects list)
- [x] Tx type `task_submit`, instance_id = project_id
- [x] Same evidence handling, same lifecycle, same output contract

### Infrastructure

- [x] `UserAddress` saved to config during headless login
- [x] TX lifecycle extracted into reusable `executeTxLifecycle()` function
- [x] `tx run` refactored to use `executeTxLifecycle()` (no behavior change)
- [x] Extended `CommitTxResult` struct with domain-specific fields

## Dependencies & Risks

### Dependencies
- Headless login must return `cardano_bech32_addr` (it does -- `user.go:313`)
- Off-chain submit endpoints must accept `pending_tx_hash` (confirmed in OpenAPI: `SubmitAssignmentCommitmentV2Request`)
- `contributor_state_id` must be resolvable by contributors (needs verification)

### Risks
- **Two-phase failure**: On-chain tx succeeds but off-chain evidence call fails. Mitigated by `pending_tx_hash` pattern -- evidence can be re-submitted.
- **Address mismatch**: Config address doesn't match skey. Mitigated by comparing key hash against stored address before building.
- **Schema drift**: Build endpoint schemas may change. Mitigated by checking OpenAPI spec before implementation.

## Implementation Order

### Phase 1: Infrastructure (1 PR)
1. Add `UserAddress` to `Config` struct, save from login responses
2. Extract `executeTxLifecycle()` from `tx run` into `tx_lifecycle.go`
3. Refactor `tx run` to use `executeTxLifecycle()` -- zero behavior change, all existing tests pass

### Phase 2: Course Student Commit-TX (1 PR)
1. Add `course student commit-tx` command in `course_student.go`
2. Wire: readEvidence -> wrapEvidence -> resolveSltHash -> build body -> executeTxLifecycle -> off-chain submit
3. Test against preprod with the existing bash script as reference

### Phase 3: Project Contributor Commit-TX (1 PR)
1. Add `project contributor commit-tx` command in `project_contributor.go`
2. Wire: readEvidence -> wrapEvidence -> resolveTaskHash -> resolve contributor_state_id -> build body -> executeTxLifecycle -> off-chain update
3. Test against preprod

### Future (out of scope)
- `course student update-tx` (on-chain evidence update via `/v2/tx/course/student/assignment/update`)
- `course student claim-tx` (on-chain credential claim)
- `project contributor action-tx` (on-chain task action)
- Address derivation from skey (Phase 2 of address resolution)
- `--dry-run` flag to preview build body without executing

## Questions to Verify Before Implementation

1. **`contributor_state_id` access**: Does `/v2/project/contributor/projects/list` or `/api/v2/project/user/project/{id}` return `contributor_state_id`? If not, add `--contributor-state-id` flag.
2. **`fee_tier` for project commit**: Is it optional? What are valid values? Try omitting it against preprod.
3. **Off-chain timing**: Does the off-chain submit work immediately after registration, or does the tx need to be in a specific state first? Test with preprod.
4. **`alias` in build body**: Is it the `cfg.UserAlias` or can the API infer from JWT? Test by omitting.

## Sources & References

### Issue
- GitHub Issue #38: `course student create: accept evidence content for assignment commitment tx`

### Brainstorms
- `docs/brainstorms/2026-03-20-tx-run-full-lifecycle-command-brainstorm.md` -- tx run design decisions
- `docs/brainstorms/2026-03-18-cli-wallet-signing-brainstorm.md` -- signing architecture, build endpoint inventory
- `docs/brainstorms/2026-03-20-full-api-coverage-brainstorm.md` -- command naming, role-based nesting

### Institutional Learnings
- `docs/solutions/integration-issues/evidence-submission-payload-format-and-field-alignment.md` -- wrapEvidence pattern, field name gotchas
- `docs/solutions/security-issues/tx-signing-code-review-witness-drop-url-validation.md` -- never discard crypto errors
- `docs/solutions/feature-implementations/cli-course-import-markdown-to-tiptap-conversion.md` -- goldmark pipeline

### OpenAPI Schemas (from `openapi.json`)
- `CommitAssignmentTxRequest` -- build body for course assignment commit
- `CommitTaskTxRequest` -- build body for project task commit
- `SubmitAssignmentCommitmentV2Request` -- off-chain evidence with `pending_tx_hash`
- `RegisterPendingTxRequest` -- tx_type enum: `assignment_submit`, `task_submit`
- `WalletData` -- `change_address` + `used_addresses`

### Key Files
- `cmd/andamio/tx_run.go` -- existing lifecycle to extract
- `cmd/andamio/helpers.go:230` -- `wrapEvidence()`, `resolveSltHash()`, `resolveTaskHash()`
- `cmd/andamio/course_student.go` -- existing off-chain student commands
- `cmd/andamio/project_contributor.go` -- existing off-chain contributor commands
- `internal/cardano/sign.go` -- signing, Blake2b256
- `internal/config/config.go` -- Config struct (needs `UserAddress`)
- `cmd/andamio/user.go:313` -- `cardano_bech32_addr` from login response
