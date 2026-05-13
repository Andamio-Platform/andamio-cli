---
title: "feat: surface assignment commitment status in `teacher assignments list`"
type: feat
status: completed
date: 2026-05-13
---

# Surface assignment commitment status in `teacher assignments list`

## Overview

Add a `Status` column to `andamio teacher assignments list` (text mode) so operators reading CLI output can see whether each assignment is `AWAITING_SUBMISSION`, `SUBMITTED`, `ACCEPTED`, `REFUSED`, `CREDENTIAL_CLAIMED`, `LEFT`, or a transient `PENDING_TX_*`. The CLI displays the API enum verbatim — no aliasing or relabeling. JSON output already passes the API envelope through unchanged, so this work is text-mode only plus a small render extraction so the column gets unit coverage.

Closes [#93](https://github.com/Andamio-Platform/andamio-cli/issues/93).

## Problem Frame

Surfaced by the cross-surface status vocabulary audit (orch: `Status vocabulary audit — API, CLI, app.md`). Tasks (`cmd/andamio/project_task.go:311`) and course modules already expose status in their listings; assignment commitments are the last commitment surface without it. Without a status column, an operator cannot tell at a glance which submissions are awaiting review, already accepted, refused (retriable), or finalized via credential claim.

The endpoint already returns the data:

- Path: `POST /api/v2/course/teacher/assignment-commitments/list`
- With `--course`: full merged history (on-chain + DB). `data[].content.commitment_status` carries the DB-authoritative status (the enum from the issue's canon).
- Without `--course`: lightweight on-chain-only summary. `content.commitment_status` is typically absent; only top-level `on_chain_status` is populated. Per resolved planning question below, we render those rows as empty/dash and let operators re-run with `--course <id>` for full status.

## Requirements Trace

- **R1.** Text mode of `andamio teacher assignments list` shows a `Status` column with `content.commitment_status` from the API response, displayed verbatim. (Issue acceptance bullet 1.)
- **R2.** JSON output mode includes the status field unchanged. The existing handler already pass-throughs the gateway envelope; verify this remains true and add a regression test. (Issue acceptance bullet 2.)
- **R3.** No aliasing or relabeling — display the raw enum string. Mapping to user-facing language stays in the app layer. (Issue acceptance bullet 3.)

## Scope Boundaries

- Only `cmd/andamio/teacher_assignments.go` (the `teacher assignments list` command) is in scope.
- No changes to the API. The CLI consumes whatever the gateway emits.
- No aliasing layer, no localization, no color codes, no enum validation.

### Deferred to Separate Tasks

- **`course teacher commitments`** (`cmd/andamio/course_teacher_ops.go:682`): hits the same endpoint but renders via the generic `printListPost` → `output.PrintList` (title + id only). Adding a status column there requires either extending `PrintList`'s signature or building a custom renderer like this plan does. Leave for a follow-up if/when the audit calls it out — the issue scopes specifically to `teacher_assignments.go:70-71`.
- **`teacher assignments get`**: JSON-only output via `output.PrintJSON(m)` already carries the nested `content.commitment_status` field. No change needed.

## Context & Research

### Relevant Code and Patterns

- `cmd/andamio/teacher_assignments.go:87-104` — the existing custom printf table for `teacher assignments list`. Columns today: `STUDENT`, `MODULE`, `SOURCE`, `COURSE ID`. The fix extends this loop.
- `cmd/andamio/project_task.go:311-342` — precedent for a custom table with a `Status` column. Reads `task_status` (top-level) with a fallback to `source`. The pattern (read field, fallback if empty, render fixed-width column) is the model to follow here.
- `cmd/andamio/project_manager_ops.go:139-155` (`renderQualifiedContributorsText`) — precedent for extracting a render function that takes `stdout`/`stderr` writers so it can be unit-tested without an httptest server. This plan applies the same shape.
- `internal/output/output.go:62` — `PrintList` only supports two columns (title + id), so a custom printf table is the right tool for a 5-column listing. No change to the `output` package is needed.

### Schema (from `openapi.json`)

`TeacherAssignmentCommitmentItem`:

| Field | Source | Notes |
|---|---|---|
| `course_id` | top-level | identifier |
| `course_module_code` | top-level | human-readable module code |
| `student_alias` | top-level | participant |
| `source` | top-level | `"merged"` / `"db_only"` / `"chain_only"` |
| `on_chain_status` | top-level | on-chain student status (separate enum) |
| `content.commitment_status` | nested | **the status this plan surfaces.** Issue canon: `AWAITING_SUBMISSION`, `SUBMITTED`, `ACCEPTED`, `REFUSED`, `CREDENTIAL_CLAIMED`, `LEFT`, `PENDING_TX_*`. |

The CLI does not validate or enumerate values; it displays whatever the gateway returns. The issue's enum list (above) is the human-readable canon; `andamio-api/internal/internal_api/andamio_db_client/client.go:162-174` is the executable source the issue points to; the OpenAPI description string `"DRAFT, SUBMITTED, APPROVED, etc."` is stale and ignored.

### Institutional Learnings

- `docs/solutions/feature-implementations/cli-teacher-assignment-commitments-commands.md` — the original feature solution doc. Key reminders: non-text format dispatch happens before the empty-data check; the JSON path is already verbatim pass-through.
- `docs/solutions/architecture/cli-composability-audit-and-fix.md` — stderr/stdout discipline. Status column lives in stdout (data); any hint about empty status (none planned here) would belong on stderr.

## Key Technical Decisions

- **Render only `content.commitment_status`, no cross-enum fallback.** When `--course` is omitted, `content.commitment_status` is typically absent; render empty (rationale: the field is genuinely absent in the API response — empty is the "raw API value"). Mixing `on_chain_status` into the same column would be aliasing across two different enums and violates the issue's "no aliasing" rule.
- **Use `"—"` (em-dash) as the empty-value placeholder.** Cleaner in fixed-width tables than a blank cell and matches what humans typically read as "no value." A pure empty string would render as trailing whitespace, which is harder to scan. This is presentation, not aliasing — no enum value is invented.
- **Status column uses dynamic width, minimum 20.** The longest known enum string is `CREDENTIAL_CLAIMED` (18); transient `PENDING_TX_*` variants fall within the same range. The column is sized to fit the widest value present in the current result set without truncation, with a 20-character floor so short result sets still align cleanly. Truncating would silently corrupt the enum — that violates "verbatim" — so if a longer value appears the column expands and the row stretches rather than slicing.
- **Extract the row-render loop into a `renderTeacherAssignmentsListText(data []interface{}, stdout io.Writer) error` function.** Mirrors the `renderQualifiedContributorsText` pattern (`project_manager_ops.go:144`) so the new column is unit-testable end-to-end without spinning up an httptest server. The fetch path stays in `runTeacherAssignmentsList`; the renderer is pure.

## Open Questions

### Resolved During Planning

- **What does the Status column show when `--course` is omitted?** Resolved with the user: render empty (em-dash placeholder). No cross-enum fallback. Users get full status by re-running with `--course <id>`.
- **Do we need to update the JSON path?** No. `output.PrintJSON(resp)` already pass-throughs the gateway envelope verbatim. The status field is already present in JSON output today — this plan adds a regression test, not new behavior.
- **Do we modify `course teacher commitments` or the `output.PrintList` helper?** No. Scoped out per Scope Boundaries.

### Deferred to Implementation

- None. The change is small and fully specified.

## Implementation Units

- [x] **Unit 1: Add Status column to the `teacher assignments list` text renderer**

**Goal:** Surface `content.commitment_status` as a new `Status` column in the text-mode table for `andamio teacher assignments list`, displayed verbatim. JSON path stays unchanged (already correct).

**Requirements:** R1, R2, R3

**Dependencies:** None

**Files:**
- Modify: `cmd/andamio/teacher_assignments.go`
- Test: `cmd/andamio/teacher_assignments_test.go` (new)

**Approach:**
- Extract the existing text-mode rendering loop (`teacher_assignments.go:86-104`) into a new package-level function `renderTeacherAssignmentsListText(data []interface{}, stdout io.Writer) error`. The fetch and format-dispatch logic stays in `runTeacherAssignmentsList`; only the loop moves.
- Insert a new `Status` column between `SOURCE` and `COURSE ID`. New header line: `STUDENT`, `MODULE`, `SOURCE`, `STATUS`, `COURSE ID`. Column widths: 20 / 12 / 15 / dynamic-min-20 / remainder. The Status column expands when a row's value exceeds 20 chars so the enum is never truncated.
- Read status with nested access: `m["content"].(map[string]interface{})["commitment_status"].(string)`. Use the safe two-step type assertion (check `content` exists and is a map before reading the field). If the nested value is missing or empty, render `"—"` (em-dash). Do not fall back to `on_chain_status` or any other field.
- Keep all existing fields (`student_alias`, `course_module_code`, `source`, `course_id`) and their widths unchanged.
- Leave `runTeacherAssignmentsGet` and the JSON pass-through path (`output.PrintJSON(resp)`) untouched.

**Patterns to follow:**
- `cmd/andamio/project_task.go:311-342` — custom printf table with a `Status` column.
- `cmd/andamio/project_manager_ops.go:139-155` — extracted `renderXText(data, writer)` for unit testability.
- `cmd/andamio/teacher_assignments.go:75-78` — non-text format dispatch must remain ahead of any data-shape work.

**Test scenarios:**
- Happy path — `renderTeacherAssignmentsListText` writes a `STATUS` column header and a row with `content.commitment_status = "AWAITING_SUBMISSION"` renders the value verbatim in the Status column.
- Happy path — row with `content.commitment_status = "CREDENTIAL_CLAIMED"` renders verbatim (validates that longer enum values fit within the 20-wide column).
- Happy path — row with a `PENDING_TX_*` value in `content.commitment_status` renders verbatim. Use whatever transient enum the API currently emits (look it up against `andamio-api` or a recent preprod fixture before writing the test). Validates that the "no filtering, no aliasing" principle holds for transient statuses.
- Edge case — row missing the `content` key entirely renders `"—"` in the Status column (the `--course` omitted case where the on-chain summary has no nested content).
- Edge case — row where `content` is a map but `commitment_status` is missing/empty renders `"—"`.
- Edge case — multiple rows with different statuses (mix of `SUBMITTED`, empty, `REFUSED`) all render correctly in one table, preserving column alignment.
- Integration — full handler `runTeacherAssignmentsList` with `--output json` against an httptest server returns the gateway envelope verbatim, including `data[].content.commitment_status` and `data[].on_chain_status`. This guards the issue's R2 acceptance bullet against future regressions.
- Integration — full handler with default text output against an httptest server returns a table containing `STATUS` header and the expected verbatim status values. Covers the end-to-end fetch → render path.

**Verification:**
- `go build -o andamio ./cmd/andamio && ./andamio teacher assignments list --help` shows the unchanged help text (no flag changes).
- Against a preprod environment with a real teacher JWT, `./andamio teacher assignments list --course <id>` renders the new `STATUS` column populated with the API enum verbatim.
- `./andamio teacher assignments list --course <id> --output json | jq '.data[0].content.commitment_status'` returns the same enum string the text mode printed.
- `./andamio teacher assignments list` (no `--course`) renders the new `STATUS` column with `"—"` placeholders for on-chain-only rows.
- `go test ./cmd/andamio/...` passes; new tests cover the scenarios above.

## System-Wide Impact

- **Interaction graph:** None. `runTeacherAssignmentsList` is a leaf command; no callbacks, middleware, or observers depend on its output shape.
- **API surface parity:** A parallel command `course teacher commitments` (`course_teacher_ops.go:682`) hits the same endpoint but uses the generic `printListPost`/`PrintList` helper that only supports title + id columns. It is intentionally out of scope (see Scope Boundaries). If the audit later requires the same status surface there, expect a follow-up that either replaces the generic helper call with a custom renderer or extends `output.PrintList` to support N columns.
- **Integration coverage:** Adding an httptest-backed test for the full `runTeacherAssignmentsList` text + JSON paths is the integration scenario unit tests of the renderer alone cannot prove (verifies that fetch + format dispatch + render compose correctly, and that JSON pass-through preserves nested status fields). Cover this in Unit 1's test scenarios.
- **Unchanged invariants:**
  - JSON output remains a verbatim pass-through of the gateway envelope. Adding a Status column to text mode does not transform, alias, or filter the JSON path.
  - `runTeacherAssignmentsGet` behavior is unchanged. It already returns the full item via `output.PrintJSON(m)`, and that envelope already includes `content.commitment_status`.
  - Existing text column ordering (`STUDENT` first, `COURSE ID` last) is preserved; the new column is inserted in the middle so left-anchored greps on student alias continue to work.

## Risks & Dependencies

| Risk | Mitigation |
|------|------------|
| Future gateway response shape change (e.g., `commitment_status` moves out of `content`) silently produces a column of em-dashes and operators don't notice. | Add the integration test that asserts the live-shape happy path against an httptest server fixture. If the gateway moves the field, the test fails immediately. |
| `PENDING_TX_*` enum values are wider than expected and break column alignment. | Column width is 20, which fits known values plus one column of breathing room. If a longer value appears we let the row stretch rather than truncate — truncation would silently corrupt the enum and violate "verbatim." |
| The em-dash placeholder is read as a literal enum value by a downstream tool. | The placeholder only appears in text mode. JSON output preserves the actual API shape (missing field or empty string), so any script consuming the command uses `--output json`. |

## Documentation / Operational Notes

- Update `CHANGELOG.md` under `## [Unreleased]` with a short entry: "feat(teacher): show assignment commitment status in `teacher assignments list` text output". The release script will promote it to the next versioned heading.
- No update needed to `CLAUDE.md`'s command reference table — the row for `course teacher commitments` already exists and the command name plus auth column do not change. (The Status column is a text-output detail, not a command-surface change.)
- No `docs/COURSE-LIFECYCLE.md` or `docs/TX-LIFECYCLE.md` updates required — those describe state lifecycles, not CLI table columns.

## Sources & References

- GitHub issue: [#93 — Expose assignment commitment status in teacher_assignments listings](https://github.com/Andamio-Platform/andamio-cli/issues/93)
- Origin audit (private): `orch:Status vocabulary audit — API, CLI, app.md`
- Canon enum source: `andamio-api/internal/internal_api/andamio_db_client/client.go:162-174` (referenced from the issue)
- Related CLI command: `cmd/andamio/teacher_assignments.go:55-107` (target)
- Pattern precedents: `cmd/andamio/project_task.go:311-342`, `cmd/andamio/project_manager_ops.go:139-155`
- OpenAPI schema: `openapi.json` — `TeacherAssignmentCommitmentItem`, `AssignmentCommitmentContent`
- Prior solution: `docs/solutions/feature-implementations/cli-teacher-assignment-commitments-commands.md`
