---
title: "feat: add compute-hash commands for course modules and tasks"
type: feat
status: completed
date: 2026-04-02
---

# feat: add compute-hash commands for course modules and tasks

## Overview

Add `andamio course credential compute-hash` and `andamio project task compute-hash` commands that expose existing internal hash functions as standalone CLI commands. This enables computing hashes from raw inputs before data is on-chain, unblocking the correct course/project setup flow.

## Problem Frame

The CLI has internal hash computation functions (`ComputeSltHash`, `ComputeTaskHash`) but only exposes them through `verify-hash` commands that compare against existing on-chain data. There is no way to compute a hash from raw inputs before the data is on-chain.

This creates a chicken-and-egg problem: the DB API requires an `slt_hash` when approving a module, but the only way to get that hash today is to mint the module on-chain first — leading to DB sync failures during the module creation flow.

## Requirements Trace

- R1. `andamio course credential compute-hash` computes SLT hash from `--slt` flags or `--file` (outline.md)
- R2. `andamio project task compute-hash` computes task hash from `--content`, `--lovelace`, `--expiration`, and optional `--token` flags
- R3. Both commands work offline — no API key, JWT, or network required
- R4. Both commands support `--output json` for scripting
- R5. JSON output uses stable schemas: `{"slt_hash": "...", "slt_count": N}` and `{"task_hash": "...", "fields": {...}}`
- R6. Task compute-hash supports `--file` flag to read task data from a markdown file with frontmatter (same format as task export)

## Scope Boundaries

- No commitment hash command in this iteration (issue mentions "if applicable" — defer until needed)
- No changes to existing `verify-hash` commands
- No changes to hash computation logic in `internal/cardano/`
- No integration with `course import` flow (future work)

## Context & Research

### Relevant Code and Patterns

- `internal/cardano/slt_hash.go` — `ComputeSltHash(slts []string) string`, no error return
- `internal/cardano/task_hash.go` — `ComputeTaskHash(task TaskData) (string, error)`, validates inputs
- `cmd/andamio/course_credential.go` — `courseCredentialCmd` parent (no auth gate), `courseCredentialVerifyHashCmd` as pattern
- `cmd/andamio/project_task.go` — `projectTaskCmd` parent has `PersistentPreRunE: jwtAuthPreRunE`, `projectTaskVerifyHashCmd` as pattern
- `cmd/andamio/course_import.go:608` — `parseSLTsFromOutline()` extracts SLTs from outline.md
- `cmd/andamio/project_task.go:310` — `parseExpiration()` parses ISO 8601 dates to Unix ms
- `cmd/andamio/project_task.go:337` — `parseTokenFlags()` parses `--token` flag values
- `cmd/andamio/project_task_import.go:42` — `TaskFrontmatter` struct with all needed fields

### Institutional Learnings

- Manual CBOR encoding is required (not fxamacker/cbor) for exact indefinite/definite array control matching on-chain validators (`docs/solutions/architecture/cli-tx-state-machine-pattern-and-task-hash-verification.md`)
- All progress messages to stderr, data to stdout only (`docs/solutions/architecture/cli-composability-audit-and-fix.md`)
- No stdin reading in command handlers (`docs/solutions/architecture/non-interactive-cli-stdin-picker-removal.md`)
- Use `cobra.ExactArgs(N)` for required positional args (`docs/solutions/architecture/command-structure-refactoring.md`)

## Key Technical Decisions

- **Auth bypass for project task compute-hash**: `projectTaskCmd` has `PersistentPreRunE: jwtAuthPreRunE` which enforces JWT auth for all subcommands. Since compute-hash is purely local computation, override `PersistentPreRunE` on the compute-hash command itself to only run `rootCmd`'s `PersistentPreRunE` (which sets output format). This is the simplest approach — avoids restructuring the command tree.

- **No positional args**: Both commands take all input via flags. This avoids the awkwardness of passing multiple SLT strings as positional args and aligns with the issue's proposed interface.

- **`--file` support for both commands**: SLT hash can read from outline.md (reusing `parseSLTsFromOutline`). Task hash can read from a task markdown file with frontmatter (reusing `TaskFrontmatter` parsing from task import). File flags and individual field flags are mutually exclusive.

- **Echo back input fields in JSON output**: Both commands include the parsed input in JSON output so users can verify what was hashed. SLT: `{"slt_hash": "...", "slt_count": 2, "slts": [...]}`. Task: `{"task_hash": "...", "fields": {"content": "...", "lovelace": ..., "expiration": ..., "tokens": [...]}}`.

## Open Questions

### Resolved During Planning

- **Where to place commands?**: In existing files — `course_credential.go` and `project_task.go` — alongside the verify-hash commands they complement.
- **How to handle auth on project task?**: Override `PersistentPreRunE` on the specific command to skip JWT check (confirmed this is a standard Cobra pattern).

### Deferred to Implementation

- **Should `--debug` flag expose raw CBOR bytes?**: `DebugTaskBytes()` exists for tasks. Could add a `--debug` flag to show pre-hash bytes. Decide during implementation based on command complexity.

## Implementation Units

- [x] **Unit 1: `course credential compute-hash` command**

  **Goal:** Add a command that computes SLT hash from flag inputs or an outline.md file.

  **Requirements:** R1, R3, R4, R5

  **Dependencies:** None

  **Files:**
  - Modify: `cmd/andamio/course_credential.go`
  - Test: `cmd/andamio/course_credential_test.go`

  **Approach:**
  - Add `courseCredentialComputeHashCmd` as a subcommand of `courseCredentialCmd`
  - Two input modes: `--slt` (repeatable string slice flag) or `--file` (path to outline.md)
  - Validate mutual exclusivity: error if both `--slt` and `--file` provided, or if neither provided
  - When `--file` is used, read the file and call `parseSLTsFromOutline()` to extract SLTs
  - Call `cardano.ComputeSltHash(slts)` to compute the hash
  - Text output: print hash and SLT count to stdout
  - JSON output: `{"slt_hash": "...", "slt_count": N, "slts": [...]}`
  - No auth required — `courseCredentialCmd` has no auth gate

  **Patterns to follow:**
  - `courseCredentialVerifyHashCmd` in `course_credential.go` for command structure
  - `parseSLTsFromOutline()` in `course_import.go` for file parsing
  - `isJSON := output.GetFormat() == output.FormatJSON` pattern for output gating

  **Test scenarios:**
  - Compute hash from `--slt` flags, verify deterministic output
  - Compute hash from `--file` pointing to a valid outline.md
  - Error when both `--slt` and `--file` provided
  - Error when neither `--slt` nor `--file` provided
  - JSON output includes all expected fields with correct types
  - File with no SLTs section returns error

  **Verification:**
  - `andamio course credential compute-hash --slt "text1" --slt "text2"` returns consistent hash
  - `andamio course credential compute-hash --file outline.md --output json | jq .slt_hash` works in a pipe
  - Command works with zero config (no `~/.andamio/config.json` needed)

- [x] **Unit 2: `project task compute-hash` command**

  **Goal:** Add a command that computes task hash from flag inputs or a task markdown file.

  **Requirements:** R2, R3, R4, R5, R6

  **Dependencies:** None (can be implemented in parallel with Unit 1)

  **Files:**
  - Modify: `cmd/andamio/project_task.go`
  - Test: `cmd/andamio/project_task_test.go`

  **Approach:**
  - Add `projectTaskComputeHashCmd` as a subcommand of `projectTaskCmd`
  - Override `PersistentPreRunE` on this command to skip JWT auth: set it to a function that only calls `output.SetFormat()` from the `--output` flag value
  - Two input modes: individual flags (`--content`, `--lovelace`, `--expiration`, optional `--token`) or `--file` (path to task markdown with frontmatter)
  - When `--file` is used, parse frontmatter using `adrg/frontmatter` into `TaskFrontmatter`, extract content, lovelace, expiration, and tokens
  - Convert inputs to `cardano.TaskData` struct: parse expiration via `parseExpiration()` to get Unix ms, parse lovelace to uint64, parse tokens via `parseTokenFlags()` or from frontmatter
  - Call `cardano.ComputeTaskHash(taskData)` to compute the hash
  - Text output: print hash and input summary to stdout
  - JSON output: `{"task_hash": "...", "fields": {"content": "...", "lovelace": N, "expiration_ms": N, "tokens": [...]}}`
  - No auth required — auth bypass via PersistentPreRunE override

  **Patterns to follow:**
  - `projectTaskVerifyHashCmd` in `project_task.go` for command structure
  - `parseExpiration()`, `parseTokenFlags()`, `validateLovelace()` in `project_task.go` for input parsing
  - `TaskFrontmatter` in `project_task_import.go` for file-based input

  **Test scenarios:**
  - Compute hash from individual flags, verify deterministic output
  - Compute hash from `--file` pointing to a task markdown file
  - Error when both `--file` and individual flags provided
  - Error when required flags missing (no `--file` and missing `--content`, `--lovelace`, or `--expiration`)
  - Error on invalid inputs (bad expiration format, negative lovelace, invalid token policy ID)
  - JSON output includes all expected fields with correct types
  - Command works without JWT auth (no config needed)
  - Hash matches known test vectors from `internal/cardano/task_hash_test.go`

  **Verification:**
  - `andamio project task compute-hash --content "Build API" --lovelace 5000000 --expiration 2026-12-31` returns consistent hash
  - `andamio project task compute-hash --file task.md --output json | jq .task_hash` works in a pipe
  - Command works with zero config — no JWT prompt, no API key check

## System-Wide Impact

- **Interaction graph:** No callbacks, middleware, or observers affected. These are pure-computation leaf commands.
- **Error propagation:** Errors from `cardano.ComputeTaskHash()` (validation failures) propagate directly to user via Cobra's `RunE` error handling.
- **API surface parity:** These commands complement existing `verify-hash` commands. No other interface needs the same change.
- **CLI help tree:** Two new commands appear under `andamio course credential` and `andamio project task`. Help text should reference the companion `verify-hash` command.

## Risks & Dependencies

- **Low risk: PersistentPreRunE override**: Overriding `PersistentPreRunE` on a specific subcommand is standard Cobra. The override must still call the root command's output format setup. Verify with `--output json` flag to confirm it still works.
- **Low risk: parseSLTsFromOutline reuse**: Function is in `course_import.go` but accessible within the same package. No extraction needed.

## Sources & References

- Related issue: Andamio-Platform/andamio-cli#51
- Existing hash functions: `internal/cardano/slt_hash.go`, `internal/cardano/task_hash.go`
- Hash verification pattern: `docs/solutions/architecture/cli-tx-state-machine-pattern-and-task-hash-verification.md`
- Composability rules: `docs/solutions/architecture/cli-composability-audit-and-fix.md`
