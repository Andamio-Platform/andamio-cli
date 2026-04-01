---
title: "fix: Resolve 6 remaining CLI PBL retro issues"
type: fix
status: completed
date: 2026-04-01
origin: ~/projects/02-areas/andamio/000-task-notes/Tasks/cli-pbl-retro-fixes.md
---

# fix: Resolve 6 remaining CLI PBL retro issues

## Overview

Six issues from the Midnight PBL course setup retro remain unresolved. They fall into three categories: silent failures (#3), DX papercuts (#5, #6, #7), and missing documentation (#9, #10). Items #1, #2, #4 are already resolved. Item #8 is API-side (Roberto's).

## Problem Frame

The Midnight PBL walkthrough exposed 10 issues. Four are fixed (--address flag released, --slt-hash added, DB failure surfacing in tx run, slt_hash mapping is API-side). The remaining six cause silent failures, wrong-first-attempt experiences, and undocumented workflow sequences that block every new user.

## Requirements Trace

- R1. `publish-module` must not silently succeed when no on-chain module exists to link (retro #3)
- R2. Submit API headers must be persistable in config to avoid `--submit-header` on every invocation (retro #5)
- R3. `tx build` help must show correct `initiator_data` object format, not a string (retro #6)
- R4. `course owner create` must clearly communicate it creates an off-chain record, not a course (retro #7)
- R5. Full course creation workflow documented: tx run → owner update → tx run modules_manage → register-module → set DRAFT → import (retro #9)
- R6. register-module → DRAFT → import sequence documented (retro #10)

## Scope Boundaries

- No API-side changes (items #4, #8 are out of scope)
- No breaking command renames — improve help text and add aliases instead
- No new dependencies
- Documentation changes are CLI help text and README, not andamio-docs site (that can follow separately)

## Context & Research

### Relevant Code and Patterns

- `cmd/andamio/course_teacher_ops.go:160-193` — `runCourseTeacherModuleAction` is the generic handler for publish-module and delete-module. Currently sends {course_id, course_module_code} and prints "done" regardless of API response content.
- `internal/config/config.go:12-21` — Config struct. No submit header field. Already has `SubmitURL` and env var override pattern (`ANDAMIO_SUBMIT_URL`).
- `cmd/andamio/tx_build.go:28` — Help example shows `"initiator_data":"addr_test1..."` (wrong: string instead of object).
- `cmd/andamio/course_owner.go:31-42` — `courseOwnerCreateCmd` has `Short: "Create a new course"` and Long text that says "Create a new off-chain course record" but the Short is misleading.
- `cmd/andamio/tx_submit.go:62-68` and `cmd/andamio/tx_lifecycle.go:40-44` — Both resolve submit URL from flag > config. Headers are flag-only, not persisted.
- `README.md` — Has import/export docs but no full course creation workflow.

## Key Technical Decisions

- **Submit headers in config as map[string]string, not a single blockfrost field**: The retro specifically mentions Blockfrost, but submit APIs vary (Maestro, self-hosted). A generic `submit_headers` map serves all providers. Matches the existing `--submit-header "Key: Value"` pattern.
- **Improve help text for `course owner create` rather than renaming**: A rename is breaking. The command has a distinct purpose from `register` (it takes `--pending-tx-hash`). Fixing the `Short` description and adding a prominent note to `Long` is sufficient.
- **publish-module: inspect API response for warning signals rather than pre-flight GET**: The API already returns response data. The CLI should inspect the response for `source` field or warning indicators, and warn when the module wasn't actually linked. A separate GET check would be a second round trip for every call.

## Open Questions

### Resolved During Planning

- **Should we add `config set-blockfrost-key` specifically?** No — a generic `config set-submit-header` command serves all submit API providers.
- **Should `course owner create` be renamed?** No — it's a breaking change and `register` already exists with different semantics. Fix the help text instead.

### Deferred to Implementation

- **What does the publish-module API actually return when no on-chain module exists?** Need to test or read API source. The CLI fix should check for response signals (empty `source`, missing `slt_hash`, etc.) and warn accordingly.

## Implementation Units

- [ ] **Unit 1: Add `submit_headers` to config and `config set-submit-header` command**

  **Goal:** Persist submit API headers in config so users don't need `--submit-header` on every tx invocation.

  **Requirements:** R2

  **Dependencies:** None

  **Files:**
  - Modify: `internal/config/config.go`
  - Modify: `cmd/andamio/config.go`
  - Modify: `cmd/andamio/tx_submit.go`
  - Modify: `cmd/andamio/tx_lifecycle.go`
  - Modify: `CLAUDE.md` (command reference table)

  **Approach:**
  - Add `SubmitHeaders map[string]string` to Config struct with `json:"submit_headers,omitempty"`
  - Add env var override `ANDAMIO_SUBMIT_HEADERS` (JSON map) in `Load()`, matching the `ANDAMIO_SUBMIT_URL` pattern
  - Add `config set-submit-header <key> <value>` command — upserts one header into the map
  - Add `config remove-submit-header <key>` command — removes one header
  - Show submit headers in `config show` output (mask values like API key)
  - In `tx_submit.go` and `tx_lifecycle.go`: merge config headers with flag headers (flag headers take precedence for same key)

  **Patterns to follow:**
  - `config set-submit-url` in `config.go` for command structure
  - `ANDAMIO_SUBMIT_URL` env override in `config.go:142-144` for env var pattern
  - `--submit-header` flag parsing in `tx_submit.go:36` for header format

  **Test scenarios:**
  - Happy path: `config set-submit-header project_id preprodABC` persists to config.json, `config show` displays it, `tx submit` sends it without needing the flag
  - Happy path: Flag headers override config headers with the same key
  - Happy path: Multiple config headers are all sent
  - Edge case: `config remove-submit-header` for a key that doesn't exist returns a clear message
  - Happy path: `config show --output json` includes submit_headers in output

  **Verification:**
  - `andamio config set-submit-header project_id test123 && andamio config show` shows the header
  - Existing `--submit-header` flag still works and takes precedence

- [ ] **Unit 2: Fix `tx build` help example for `initiator_data`**

  **Goal:** Show correct object format in help text so users don't fail on first attempt.

  **Requirements:** R3

  **Dependencies:** None

  **Files:**
  - Modify: `cmd/andamio/tx_build.go`

  **Approach:**
  - Replace the inline example at line 28 with the correct object format: `'{"alias":"dev1","initiator_data":{"change_address":"addr_test1...","used_addresses":["addr_test1..."]}}'`
  - Use `--body-file` example as the primary example since the inline JSON is long. Show the inline version as a secondary example.

  **Patterns to follow:**
  - `tx_run.go:44-55` for multi-line example formatting in Long text

  **Test scenarios:**
  - Happy path: `andamio tx build --help` shows correct initiator_data object format

  **Verification:**
  - `andamio tx build --help` output matches the actual API expectation

- [ ] **Unit 3: Fix `course owner create` help text**

  **Goal:** Make clear that `create` creates an off-chain record for an existing on-chain course, not the course itself.

  **Requirements:** R4

  **Dependencies:** None

  **Files:**
  - Modify: `cmd/andamio/course_owner.go`

  **Approach:**
  - Change `Short` from `"Create a new course"` to `"Create off-chain course record (after on-chain creation)"`
  - Update `Long` to lead with "Create the off-chain metadata record for a course that has already been created on-chain." and add a note: "Note: In most cases, tx run with course_create auto-registers the course. Use 'course owner update' to set metadata after that."
  - Keep the command name as `create` for backwards compatibility

  **Patterns to follow:**
  - `courseOwnerRegisterCmd` Long text (lines 57-68) for the "Typical flow" pattern

  **Test scenarios:**
  - Happy path: `andamio course owner create --help` clearly communicates this is for off-chain records

  **Verification:**
  - Help text mentions "off-chain" and points to `course owner update` as the common path

- [ ] **Unit 4: Warn when `publish-module` response indicates no linkage**

  **Goal:** Surface a warning when publish-module silently succeeds without actually linking a module.

  **Requirements:** R1

  **Dependencies:** None

  **Files:**
  - Modify: `cmd/andamio/course_teacher_ops.go`

  **Approach:**
  - In `runCourseTeacherModuleAction`, after a successful POST, inspect the response for signals that the operation was a no-op. Check for: `source` field not being `"merged"`, or `status` still being the same, or a `warning` field in the response.
  - If the response looks like a no-op, print a warning to stderr: "Warning: module may not have been linked to an on-chain module. Ensure the module exists on-chain first (use 'andamio tx run' with modules_manage)."
  - Keep the command functional — don't error, just warn. The API might legitimately return success.
  - This applies only to `publish-module`, not `delete-module`. Split the handlers if needed, or add the verb check inside the closure.

  **Patterns to follow:**
  - JWT expiry warning in `tx_run.go:188-189` for stderr warning pattern

  **Test scenarios:**
  - Happy path: When API returns a response with `source: "merged"`, no warning is shown
  - Edge case: When API returns success but response lacks merge indicators, warning is printed to stderr
  - Happy path: `--output json` still prints the raw API response (warning still goes to stderr)

  **Verification:**
  - Running publish-module when no on-chain module exists shows a warning instead of silent "done"

- [ ] **Unit 5: Document full course creation workflow in README**

  **Goal:** Add the correct happy-path workflow to README so the next person doesn't have to discover it by trial and error.

  **Requirements:** R5, R6

  **Dependencies:** Units 1-4 (so the docs reflect the fixed behavior)

  **Files:**
  - Modify: `README.md`

  **Approach:**
  - Add a "## Course Creation Workflow" section after the existing "## Course Import/Export" section
  - Document the full sequence with numbered steps:
    1. `tx run /v2/tx/instance/owner/course/create` — create course on-chain (auto-registers in DB)
    2. `course owner update --course-id <id> --title "..." --public` — set metadata
    3. Prepare module content (use lesson-coach or manual markdown)
    4. `course import-all ./compiled/... --course-id <id> --create` — import modules to DB as DRAFT
    5. `tx run /v2/tx/course/teacher/modules/manage --body-file ...` — publish modules on-chain
    6. `course teacher register-module --course-id <id> --module-code <code> --slt-hash <hash>` — link on-chain to DB (for each module)
    7. `course teacher update-module-status --course-id <id> --module-code <code> --status DRAFT` — set DRAFT (register sets APPROVED, import requires DRAFT)
    8. `course import-all ./compiled/... --course-id <id>` — re-import content into linked modules
  - Add a "Common Gotchas" subsection covering: register sets APPROVED (need DRAFT for import), module hash ordering is non-deterministic, publish-module is for DB→chain linking (not on-chain publishing)

  **Patterns to follow:**
  - Existing "## Course Import/Export" section in README for formatting

  **Test scenarios:**
  - Happy path: A developer following the documented steps can set up a course without hitting the retro issues

  **Verification:**
  - README contains a complete, ordered workflow from on-chain creation to content import

- [ ] **Unit 6: Update CLAUDE.md command reference**

  **Goal:** Keep CLAUDE.md accurate with the new config commands and corrected descriptions.

  **Requirements:** R2, R4

  **Dependencies:** Units 1, 3

  **Files:**
  - Modify: `CLAUDE.md`

  **Approach:**
  - Add `config set-submit-header` and `config remove-submit-header` to the config command table
  - Update `course owner create` description in the command table
  - Add `SubmitHeaders` to the Config struct description in Architecture section

  **Patterns to follow:**
  - Existing command table format in CLAUDE.md

  **Test scenarios:**
  - N/A (documentation)

  **Verification:**
  - CLAUDE.md command reference matches actual CLI behavior

## System-Wide Impact

- **Config schema**: Adding `submit_headers` to Config is backwards-compatible (omitempty). Existing config files without the field will load fine.
- **Submit flow**: Both `tx submit` and `tx run` (via `tx_lifecycle.go`) need the config→flag merge. Single merge function recommended.
- **API surface parity**: `config show --output json` must include the new field.
- **Unchanged invariants**: All existing `--submit-header` flag behavior is preserved. Flag headers override config headers for the same key.

## Risks & Dependencies

| Risk | Mitigation |
|------|------------|
| publish-module API response doesn't contain enough signal to detect no-ops | Inspect actual response; if truly opaque, add a pre-flight GET to check module source before publishing |
| `submit_headers` in config could leak API keys in `config show` | Mask values in text output (show first 4 chars + "..."), show full in JSON (user is scripting) |
| Workflow docs may become stale as API evolves | Keep to CLI commands only, reference `andamio tx types` for dynamic discovery |

## Sources & References

- **Origin document:** ~/projects/02-areas/andamio/000-task-notes/Tasks/cli-pbl-retro-fixes.md
- Related code: `cmd/andamio/course_teacher_ops.go`, `internal/config/config.go`, `cmd/andamio/tx_build.go`, `cmd/andamio/course_owner.go`
- Config pattern: `cmd/andamio/config.go` (set-submit-url as model)
- Submit header flow: `cmd/andamio/tx_submit.go`, `cmd/andamio/tx_lifecycle.go`
