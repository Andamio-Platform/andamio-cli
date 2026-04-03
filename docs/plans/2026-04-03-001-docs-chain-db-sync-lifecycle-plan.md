---
title: "docs: document chain/DB sync model and course/project lifecycle workflows"
type: feat
status: completed
date: 2026-04-03
---

# docs: document chain/DB sync model and course/project lifecycle workflows

## Overview

Create user-facing documentation for the chain/DB synchronization model and correct CLI command sequences for course and project lifecycles. Improve CLI error messages so that sync failures guide users toward recovery instead of dead-ending. Make `course import` compute and store SLT hashes so the natural import→mint→sync workflow doesn't require manual hash gymnastics.

## Problem Frame

An experienced developer using the CLI cannot figure out the correct sequence of commands to create a course or project with full chain + DB consistency. The two state systems (on-chain Cardano, off-chain DB) have an implicit sync model that is undocumented. Getting the ordering wrong produces silent failures: transactions confirm on-chain but DB updates fail, leaving orphaned state. Issue #52 documents 10 failed transactions across 4 different workflow attempts before the user pieced together the correct flow.

The root cause is a circular dependency: the DB needs a module record with a matching SLT hash before the on-chain TX confirms, but there's no documented (or easy) way to set that hash on a DRAFT module before minting.

## Requirements Trace

- R1. Document the TX lifecycle (BUILD→SIGN→SUBMIT→REGISTER→CONFIRM→UPDATE) with terminal states and recovery paths
- R2. Document the correct course lifecycle: from course creation through module publishing with exact CLI commands
- R3. Document the correct project lifecycle: from project creation through task management
- R4. Document recovery procedures when `tx run` returns "transaction confirmed but DB update failed"
- R5. Make `course import` compute SLT hashes via `ComputeSltHash()` and include them in the update payload
- R6. Improve `tx run` error messages for the `"failed"` state to include tx_hash, tx_type, and recovery hints
- R7. All documentation must be in `docs/` within the CLI repo (developer-facing reference)

## Scope Boundaries

- **Not in scope**: Server-side API changes (e.g., making `register-module` work as upsert — suggestion #3 from the issue). Those belong in `andamio-api` issues.
- **Not in scope**: `--verify-db` pre-flight flag for `tx run` (suggestion #6) — larger feature, separate issue.
- **Not in scope**: Changes to the `andamio-api` gateway or `andamio-db-api-go`.

## Context & Research

### Relevant Code and Patterns

- `cmd/andamio/tx_run.go` — `RunResult` struct, `pollTxStatus` with terminal states at lines 235-242, `fail()` closure for partial progress reporting
- `cmd/andamio/tx_lifecycle.go` — Five-step pipeline: build, sign, submit, register, poll
- `cmd/andamio/course_import.go` — SLT locking check at line 245 (`sltsLocked := existing.Status != "DRAFT"`), `updateModuleContent` at line 1116
- `cmd/andamio/course_teacher_ops.go` — `runCourseTeacherRegisterModule` at line 123, `runCourseTeacherPublishModule` at line 161 (checks `source: "merged"`)
- `internal/cardano/slt_hash.go` — `ComputeSltHash(slts []string) string`
- `internal/cardano/task_hash.go` — `ComputeTaskHash(task TaskData) (string, error)`

### Institutional Learnings

- **TX state machine pattern** (`docs/solutions/architecture/cli-tx-state-machine-pattern-and-task-hash-verification.md`): No CLI command should combine `executeTxLifecycle` with a separate off-chain API POST. Off-chain operations are separate commands.
- **Three-layer recovery** (andamio-api docs): (1) TX state machine primary path, (2) state healer forward-corrections on reads, (3) abandoned TX recovery via reconcilers.
- **On-chain-as-source-of-truth** (andamio-api task hash self-healing): Chain is authoritative. The gateway's self-healing pattern adopts on-chain hashes and corrects DB records to match.
- **PBL retro** (`docs/solutions/developer-experience/cli-midnight-pbl-retro-fixes.md`): `register-module` sets APPROVED, but `import` requires DRAFT for SLTs. Users get "slt_index out of range" with no guidance. Need `update-module-status --status DRAFT` as workaround.
- **API payload semantics** (import parity solution doc): Omit = unchanged, empty array = delete all. Critical for import/update operations.
- **Module status flow**: DRAFT (SLTs editable) → APPROVED (after register-module, SLTs locked) → ON_CHAIN (after publish). Lessons/intro/assignment remain editable in all states.

## Key Technical Decisions

- **SLT hash computation in import**: The hash will be computed locally using `ComputeSltHash()` (already available in `internal/cardano/`) and included in the module update payload as `slt_hash`. This is a non-breaking addition — the API already accepts `slt_hash` on the update endpoint. The hash is computed from the same SLT text strings the import already sends, so consistency is guaranteed.

- **Error message improvement scope**: Only the `"failed"` terminal state in `pollTxStatus` needs improvement. The register-failure path already has good recovery guidance (line 175-179 suggests `andamio tx register`). The `"failed"` path should mirror that pattern.

- **Documentation format**: User-facing workflow guides go in `docs/` as standalone Markdown files (not in `docs/solutions/` which is for post-mortem learnings). Two guides: one for the sync model, one for lifecycles with exact commands.

- **Project lifecycle included**: The project lifecycle is structurally simpler (no SLT hash complication) but follows the same TX lifecycle pattern. Documenting it alongside courses prevents the same confusion.

## Open Questions

### Resolved During Planning

- **Should import auto-set module status to APPROVED after hash computation?** No. Setting the hash on the update payload is sufficient for the API to match during sync. Status transitions should remain explicit user actions.
- **Where do the docs live?** In `docs/` within the CLI repo. The cross-repo docs site (`andamio-docs`) already has CLI guides; these are complementary developer references.

### Deferred to Implementation

- **Exact API response when `slt_hash` is included in the update payload**: Need to verify the API accepts it on the `course-module/update` endpoint (vs only on `register`). If not accepted, the hash can be logged/displayed for manual use.
- **Project lifecycle exact TX types**: Need to verify the full set of project TX types from `andamio tx types` output.

## Implementation Units

- [ ] **Unit 1: Document the TX lifecycle and chain/DB sync model**

  **Goal:** Create a reference document explaining how chain/DB synchronization works

  **Requirements:** R1, R4

  **Dependencies:** None

  **Files:**
  - Create: `docs/TX-LIFECYCLE.md`

  **Approach:**
  - Document the 5-step pipeline: build → sign → submit → register → poll
  - Document terminal states: `updated` (success), `failed` (chain OK, DB failed), `expired` (never confirmed)
  - Document the three-layer recovery model: state machine → healer → reconciler
  - Document what `"failed"` means and recovery steps (re-register, check status, manual reconciliation)
  - Include a state diagram showing transitions
  - Reference `andamio tx status`, `andamio tx register`, `andamio tx pending` as diagnostic/recovery tools

  **Patterns to follow:**
  - `andamio-api/docs/TX_LIFECYCLE_REFERENCE.md` for the authoritative server-side reference
  - `docs/solutions/architecture/cli-tx-state-machine-pattern-and-task-hash-verification.md` for CLI-specific patterns

  **Test scenarios:**
  - Document accurately reflects all terminal states in `tx_run.go`
  - Recovery steps are actionable CLI commands
  - No contradiction with existing solution docs

  **Verification:**
  - Document covers all 5 steps, all 3 terminal states, and recovery for each failure mode

- [ ] **Unit 2: Document course lifecycle workflow**

  **Goal:** Step-by-step guide for creating a course with full chain + DB consistency

  **Requirements:** R2

  **Dependencies:** Unit 1 (references TX lifecycle)

  **Files:**
  - Create: `docs/COURSE-LIFECYCLE.md`

  **Approach:**
  - Document the correct sequence: create course on-chain → import content → compute/set SLT hash → mint modules on-chain → register-module → publish-module → enroll students
  - Show exact CLI commands at each step with placeholder values
  - Document module status transitions (DRAFT → APPROVED → ON_CHAIN) and what's editable at each stage
  - Call out the `register-module` → APPROVED status gotcha and the `update-module-status --status DRAFT` workaround
  - Include troubleshooting section for common failures (from issue #52's transaction log)
  - Cross-reference TX-LIFECYCLE.md for recovery

  **Patterns to follow:**
  - Composability pattern: commands shown in pipeable form with `--output json | jq`
  - Existing CLAUDE.md command reference for accurate flags

  **Test scenarios:**
  - Every CLI command in the guide is a real command with correct flags
  - The sequence doesn't hit the circular dependency described in #52
  - Status transitions are accurate per the import code's `sltsLocked` check

  **Verification:**
  - A developer could follow the guide end-to-end without hitting undocumented failures

- [ ] **Unit 3: Document project lifecycle workflow**

  **Goal:** Step-by-step guide for creating a project with full chain + DB consistency

  **Requirements:** R3

  **Dependencies:** Unit 1

  **Files:**
  - Create: `docs/PROJECT-LIFECYCLE.md`

  **Approach:**
  - Document: create project on-chain → create/import tasks → assign contributors → manage commitments → assessments
  - Show exact CLI commands at each step
  - Simpler than courses (no SLT hash complication) but same TX lifecycle
  - Document task hash computation and verification
  - Cross-reference TX-LIFECYCLE.md

  **Patterns to follow:**
  - Same structure as COURSE-LIFECYCLE.md for consistency
  - Project command reference in CLAUDE.md

  **Test scenarios:**
  - Every CLI command in the guide is real and correct
  - Task create → export → import round-trip works with the documented flow

  **Verification:**
  - A developer could follow the guide for project creation without undocumented failures

- [ ] **Unit 4: Add SLT hash computation to `course import`**

  **Goal:** Import computes the SLT hash and includes it in the update payload so the natural import→mint→sync workflow works

  **Requirements:** R5

  **Dependencies:** None (independent of docs units)

  **Files:**
  - Modify: `cmd/andamio/course_import.go`
  - Test: `cmd/andamio/course_import_test.go`

  **Approach:**
  - After parsing SLTs from `outline.md`, call `cardano.ComputeSltHash(sltTexts)` to get the hash
  - Include `slt_hash` in the update payload alongside `slts` (only when `!sltsLocked`, same guard as SLTs)
  - Log the computed hash to stderr: `"Computed SLT hash: %s\n"`
  - Include `slt_hash` in the `ImportResult` struct for `--output json`
  - If the API rejects `slt_hash` on the update endpoint, fall back to displaying it as a hint for manual `register-module` use

  **Patterns to follow:**
  - `course credential compute-hash` already calls `ComputeSltHash` — follow its SLT extraction pattern
  - `internal/cardano/slt_hash.go` for the hash function
  - Import's existing `sltsLocked` guard for conditional inclusion

  **Test scenarios:**
  - Import of a module with 3 SLTs produces the same hash as `course credential compute-hash` for the same SLTs
  - Import with `--output json` includes `slt_hash` in the result
  - Import of a non-DRAFT module does NOT attempt to set `slt_hash`
  - Import with `--dry-run` shows the computed hash without sending it

  **Verification:**
  - `andamio course import <course-id> <dir> --output json | jq .slt_hash` returns a valid hex hash
  - Hash matches `andamio course credential compute-hash --slt "..." --slt "..."`

- [ ] **Unit 5: Improve `tx run` error messages for DB sync failures**

  **Goal:** When `tx run` returns the `"failed"` state, provide actionable recovery guidance

  **Requirements:** R6

  **Dependencies:** None

  **Files:**
  - Modify: `cmd/andamio/tx_run.go`

  **Approach:**
  - In `pollTxStatus`, the `"failed"` case (line 238-239) currently returns a bare error
  - Enhance the error to include: tx_hash, tx_type, and a hint
  - The hint should suggest: `andamio tx status <hash>` to check current state, and `andamio tx register --tx-hash <hash> --tx-type <type>` to retry registration
  - Format: multi-line stderr message before returning the error (gated on `!isJSON`)
  - For JSON mode, include the hint in `RunResult.Error` field
  - Mirror the pattern already used for register-failure recovery at lines 175-179

  **Patterns to follow:**
  - Register-failure recovery message at `tx_lifecycle.go` lines 175-179
  - `SIGINT` handler at `tx_run.go` lines 76-88 which suggests `andamio tx status`

  **Test scenarios:**
  - `"failed"` state error message includes tx_hash and tx_type
  - `"failed"` state in JSON mode includes recovery hint in error field
  - Error message is actionable (contains valid CLI commands)

  **Verification:**
  - `tx run` with `--output json` on a DB-failure produces a `RunResult` with populated `tx_hash`, `state: "failed"`, and `error` containing recovery commands

- [ ] **Unit 6: Update CLAUDE.md and cross-references**

  **Goal:** Link the new docs from CLAUDE.md and ensure discoverability

  **Requirements:** R7

  **Dependencies:** Units 1-3

  **Files:**
  - Modify: `CLAUDE.md`

  **Approach:**
  - Add entries to the "Key Documentation" or a new "Workflow Guides" section in CLAUDE.md
  - Link TX-LIFECYCLE.md, COURSE-LIFECYCLE.md, PROJECT-LIFECYCLE.md
  - Update the "Planned Features" or relevant sections to reference the new import hash behavior

  **Patterns to follow:**
  - Existing CLAUDE.md structure and formatting

  **Test scenarios:**
  - All links are valid relative paths
  - No broken references

  **Verification:**
  - `CLAUDE.md` references all three new docs

## System-Wide Impact

- **Interaction graph:** The SLT hash computation in import (Unit 4) adds a dependency on `internal/cardano/slt_hash.go` from `course_import.go`. This is a clean dependency — the hash package is already used by `course_credential.go`.
- **Error propagation:** The improved error messages (Unit 5) don't change error types or exit codes — they enrich the message content. `RunResult.State` remains `"failed"` with exit code 1.
- **API surface parity:** The `slt_hash` field in `ImportResult` is additive (new JSON field, non-breaking). The `slt_hash` in the update payload is a server-side concern — if the API ignores it, no harm done.
- **Integration coverage:** The SLT hash computation should produce the same hash as `course credential compute-hash` for the same inputs. This is testable with golden values.

## Risks & Dependencies

- **API acceptance of `slt_hash` on update endpoint**: The `course-module/update` endpoint may not accept `slt_hash`. If so, Unit 4 falls back to displaying the hash for manual use via `register-module`. Low risk — the field is likely silently ignored if unsupported.
- **Documentation accuracy**: The lifecycle guides describe the intended workflow, but some steps depend on API behavior that the CLI doesn't control (e.g., batch confirm matching by hash). Mitigated by cross-referencing the gateway's TX_LIFECYCLE_REFERENCE.md.

## Sources & References

- Related issue: #52
- Related PR: #51 (compute-hash commands, recently merged)
- CLI TX state machine pattern: `docs/solutions/architecture/cli-tx-state-machine-pattern-and-task-hash-verification.md`
- PBL retro fixes: `docs/solutions/developer-experience/cli-midnight-pbl-retro-fixes.md`
- API TX lifecycle reference: `andamio-api/docs/TX_LIFECYCLE_REFERENCE.md`
- API task hash self-healing: `andamio-api/docs/solutions/architecture-patterns/task-hash-self-healing-wholesale-reconciliation.md`
- API abandoned TX recovery: `andamio-api/docs/solutions/architecture-patterns/abandoned-tx-recovery-in-reconcilers.md`
