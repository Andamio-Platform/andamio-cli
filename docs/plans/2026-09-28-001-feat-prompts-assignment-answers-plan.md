---
title: Pull down prompts assignment answers - Plan
type: feat
date: 2026-09-28
artifact_contract: ce-unified-plan/v1
artifact_readiness: implementation-ready
product_contract_source: ce-plan-bootstrap
execution: code
---

# Pull down prompts assignment answers - Plan

## Goal Capsule

- **Objective:** A teacher reviewing a prompts assignment (FC Barcelona module `102` on mainnet) can read every student's answers from the CLI, as JSON for agents, as a spreadsheet-ready CSV, or as Markdown, without walking a raw envelope.
- **Means:** A new `internal/prompts` package that recognizes and validates the two prompts envelopes, and a kind-aware `enrichCommitmentEvidence` plus dedicated CSV and Markdown renderers for `teacher assignments list|get` (KTD1–KTD6).
- **Authority:** This plan, then cli#171, then the repo's `CLAUDE.md` Composability Rules and Failure Contract. The Product Contract wins on behavior; a KTD wins on mechanism.
- **Execution profile:** Go CLI, no new dependencies, no API change. `go test ./...` is the contract.
- **Product Contract preservation:** changed: R7, AE3 and new R7a — `list` had no way to narrow to one module, so the settled single-module `--wide` rule could never succeed on a multi-module course such as FCB; a client-side `--module` filter makes it reachable, and legacy Tiptap rows in a prompts module no longer block the pivot. Settled decision 3 is unchanged.
- **Stop conditions:** Stop and report if a change to `teacher assignments` JSON output would alter any existing field (only additions are allowed), or if the upstream contract in fcb-fan-engagement-app turns out to differ from what KTD2 records.
- **Tail ownership:** The caller (LFG) owns simplify, review, commit, PR, and CI.

## Product Contract

### Summary

`teacher assignments list|get` learns the prompts evidence envelope. For a prompts commitment it adds a readable `evidence_text` and a structured `evidence_answers` beside the untouched raw `evidence`. `-o csv` and `-o markdown` get real renderers in place of Go map dumps: CSV is one row per answer by default, with `--wide` pivoting to one row per student for a single prompts module. The round-trip half of cli#171 (export, import, import-assignment) is a separate follow-up.

### Problem Frame

A prompts assignment is a written assignment asked in parts. It is live on mainnet (FCB module `102`) and the fan app has supported it since fcb-fan-engagement-app#325. Its commitment evidence is `{"type":"prompts-evidence","version":1,"answers":[{promptId,label,question,answer},…]}`.

`enrichCommitmentEvidence` in `cmd/andamio/teacher_assignments.go` hands every evidence object to `tiptapToMarkdown`, which matches nothing in a prompts envelope, so `evidence_text` is silently absent. For `-o csv` and `-o markdown`, `runTeacherAssignmentsList` and `runTeacherAssignmentsGet` pass the whole gateway envelope to `output.PrintJSON`, whose generic CSV and Markdown paths print nested maps with `%v`. The FCB review needs the answers in a spreadsheet, and today the only way to get them is `jq` over the raw envelope.

### Key Decisions

- **One package per assignment type: `internal/prompts`, separate from `internal/quiz`.** (session-settled: user-directed — chosen over adding `Prompts` and `PromptsEvidence` to `quiz.Kind`: the team is building a family of assignment types on the same opaque API fields, one package per type.) Governs R1, R2.
- **`evidence_answers` uses snake_case keys `prompt_id`, `label`, `question`, `answer`.** (session-settled: user-approved — chosen over verbatim upstream camelCase `promptId`: CLI JSON is snake_case throughout, cli#90 was a camelCase decode bug, and this key becomes public API.) Governs R4.
- **CSV defaults to one row per answer; `--wide` pivots to one row per commitment for one prompts module only.** (session-settled: user-approved — chosen over wide as the default: `list` without `--course` mixes modules, so wide columns would be mostly empty or undefined.) Governs R6, R7.
- **The text-mode table does not change.** (session-settled: user-directed — chosen over an answers-count column or a truncated first-answer column: the table is a triage list, and reading happens through `get`, `-o markdown`, or JSON.) Governs R9.
- **fcb-fan-engagement-app is the only upstream authority for now.** (session-settled: user-directed — chosen over a union with andamio-app-v2: app-v2 has no prompts support yet; it is being added in a separate effort.) Governs R1.

### Requirements

**Envelope package**

- R1. `internal/prompts` recognizes a prompts definition and prompts evidence exactly as the fcb app's `isPromptsContentEnvelope` and `isPromptsEvidenceEnvelope` do, and validates a definition with the fcb app's `validatePromptsDefinition` rules and issue codes, reporting every violated rule rather than the first.
- R2. The package is pinned by fixtures under `testdata/prompts/{valid,invalid}` and a `SOURCE.md` naming the upstream files and commit, with room for an andamio-app-v2 column later.

**Pulling answers (JSON)**

- R3. For a row whose `content.evidence` is recognized prompts evidence, `content.evidence_text` is one block per answer, `**<label>.** <question>` on one line and the answer on the next, blocks separated by a blank line.
- R4. The same row gets `content.evidence_answers`: an array of `{prompt_id, label, question, answer}` in evidence order.
- R5. `content.evidence` is never modified. `evidence_text` and `evidence_answers` are absent, not empty, when there is nothing to show. Tiptap evidence keeps today's `evidence_text` and gets no `evidence_answers`.

**Pulling answers (CSV and Markdown)**

- R6. `-o csv` on `list` and `get` writes the header `student_alias,course_module_code,status,prompt_id,label,question,answer` and one row per prompts answer. A commitment with Tiptap evidence is one row with blank prompt columns and its Markdown in `answer`. A commitment with no evidence, or unrecognized evidence, is one row with blank prompt and answer columns.
- R7. `list --wide` (CSV only) writes one row per commitment with columns `student_alias,course_module_code,status` followed by one column per prompt id. It succeeds only when every row belongs to the same course and module and at least one row carries prompts evidence. A row in that module with Tiptap evidence (written before the module switched to prompts) keeps blank prompt cells, and a trailing `evidence_text` column carrying its Markdown is added only when such a row exists. Otherwise it fails before writing anything, with a message telling the user to pass `--course <id> --module <code>`. `--wide` with any other output format is an error.
- R7a. `list --module <code>` (requires `--course`) keeps only rows whose `course_module_code` matches, in every output format. It is a client-side filter over the same request.
- R8. `-o markdown` on `list` and `get` writes one section per commitment, headed by the student alias and module, with the status and the `evidence_text` rendering (or a no-submission line).
- R9. Text mode output is unchanged.
- R10. No CSV or Markdown output from these commands contains a Go `map[` or `[` slice dump. An empty result is exit 0: the CSV header alone, or no Markdown sections.

**Docs**

- R11. `CONCEPTS.md`, `docs/COURSE-LIFECYCLE.md`, `CHANGELOG.md` (Unreleased), the `teacher assignments list|get` help text (with a `jq` example on `evidence_answers`), the `assess-assignment` skill, and the golden snapshots describe prompts evidence.

### Acceptance Examples

- AE1. `teacher assignments list --course <id> -o json` with a prompts commitment shows `content.evidence_answers[0].prompt_id == "c102-cause"` and an `evidence_text` containing `**The cause.**`, and `content.evidence` equals the gateway bytes.
- AE2. The same call with `-o csv` prints three rows for a student with three answers, and a written-assignment student in the same result prints one row with the Markdown in `answer`.
- AE3. `list --course <id> --module 102 -o csv --wide` over a course with modules `101` and `102` prints one row per module-102 student with columns `c102-cause,c102-value,c102-ask`. The same call without `--module` fails with the `--course <id> --module <code>` hint and prints nothing on stdout.
- AE4. A prompts definition with a duplicate id and an empty label reports both `duplicate-prompt-ids` and `malformed-prompt`.

### Scope Boundaries

- Out: `course export`, `course import`, `course import-assignment` (the round-trip half of cli#171, a follow-up).
- Out: `course teacher commitments` output.
- Out: any API change, grading, or quiz evidence rendering.
- Out: a union with andamio-app-v2 validator rules until app-v2 ships prompts.

## Planning Contract

### Key Technical Decisions

- KTD1. **`internal/prompts` mirrors `internal/quiz`'s shape but owns its own `Kind`.** (session-settled: user-directed — chosen over adding `Prompts` and `PromptsEvidence` to `quiz.Kind`: the team is building a family of assignment types on the same opaque API fields, one package per type.) Implements R1, R2. Exports: `Kind` (`NotObject`, `Definition`, `Evidence`, `Other`), `Recognize(raw)`, `Classify(map)`, `Parse(raw)`, `Validate(env)`, `Summarize(env)`, `Answers(env)`, `Issue` with `json:"prompt_id,omitempty"`, `Summary` (`prompt_count`, `prompt_ids`), `Answer` (`prompt_id`, `label`, `question`, `answer`). No dependency on `cmd` or on `internal/quiz`.
- KTD2. **Mirror the fcb contract exactly, including where it is looser than the quiz.** Implements R1. Recognition: definition is an object with `type: "prompts"`, numeric `version`, array `prompts`; evidence is `type: "prompts-evidence"`, numeric `version`, array `answers` whose every element is an object with string `promptId`, `label`, `question`, `answer` (field-strict, version-agnostic, no length cap). Validation codes, in upstream check order: `unsupported-version`, then `malformed-intro` (present, non-null, not an object; upstream does not require `type: "doc"`, so neither does the CLI), then `empty-prompts` (returns early), then per prompt `malformed-prompt` (non-object entry, missing or blank id, blank label, blank question) and `duplicate-prompt-ids`. Plus a `not-prompts` guard code, as quiz has `not-a-quiz`. Evidence has no separate validator upstream: recognition is its validity check, so the package exposes `Answers(env) ([]Answer, bool)` instead of an evidence `Validate`.
- KTD3. **Leave `quiz.Classify` unchanged in this PR.** Implements none directly; see the conflict call-out. `quiz.Classify` has one caller, `course export`, which is out of scope here. The follow-up routes export through `prompts.Classify` before `quiz.Classify`, which satisfies cli#171's "stop reporting prompts as Other" without making `internal/quiz` aware of prompts.
  - **Conflict call-out:** the brief directed that `quiz.Classify` stop reporting a prompts envelope as `Other`. Doing it inside `internal/quiz` needs either new `quiz.Kind` values (the alternative rejected in settled decision 1) or a quiz → prompts import (breaking quiz's no-dependency rule). Deferring it to the export change keeps both decisions intact; the directive is still met there.
- KTD4. **`enrichCommitmentEvidence` dispatches on `prompts.Classify(evidence)`.** Implements R3, R4, R5. `Evidence` → set `evidence_answers` from `Answers` (skipped when the array is empty) and `evidence_text` from a `renderPromptsEvidence` helper (skipped when blank). Anything whose `type` is `doc` or absent keeps the existing `tiptapToMarkdown` path. `Other` (quiz evidence, unrecognized envelopes) gets neither field, as today.
- KTD5. **CSV and Markdown get command-local renderers, fed by one row-flattening helper.** Implements R6–R8, R10. A helper turns an enriched commitment row into `(student_alias, course_module_code, status, answers []prompts.Answer, text string)`. `renderTeacherAssignmentsCSV(rows, wide, w)` and `renderTeacherAssignmentsMarkdown(rows, w)` use it. Both `list` and `get` route non-JSON, non-text output through them (`get` passes a one-row slice). JSON output keeps `output.PrintJSON` pass-through unchanged. Status is `content.commitment_status`, blank when absent (the CSV does not use the text table's `—` placeholder). Wide column order is the first-seen prompt order across rows.
- KTD6. **Degraded reads warn on stderr in CSV and Markdown.** Implements R10. Those paths no longer pass the envelope through, so they call `warnMetaWarning(resp)`, matching `CLAUDE.md`'s "every mode except JSON" rule for 206.
- KTD7. **Add `../../internal/prompts` to `schemaSrcDirs`.** Implements R11. `Answer` rides in `--output json` via `evidence_answers`, so its json tags are public contract, the same reason `internal/quiz` is listed.

### Assumptions

- A1. The gateway returns prompts evidence under `content.evidence` exactly as the fcb app stored it, with no key rewriting. Source: cli#171 ("rides the existing opaque JSON fields"). Not verified against a live mainnet response in this plan; U2 verification names the manual check.
- A2. Upstream commit `6bb0efa4d8b7f20bc880ce1bbc4a5b4771f03a8d` (read 2026-09-28 from the local clone of fcb-fan-engagement-app) is current. `SOURCE.md` records it and the `gh api` fetch command so a later reader can check.
- A3. The scoping confirmation was skipped: the user settled scope and every open fork in conversation and asked for a hands-off run.

### Deferred Implementation Notes

- Exact helper names in `cmd/andamio`.
- Whether the Markdown heading uses `##` per student or `###` under a course heading; pick whichever reads well for a single-course result.

### Risks

- A student's answer can hold newlines and commas. `encoding/csv` quotes them; the test must include one.
- An `evidence_text` for Tiptap evidence can be long. CSV cells tolerate it, and it is what R6 asks for.

### Sources

- `cmd/andamio/teacher_assignments.go`: `runTeacherAssignmentsList`, `runTeacherAssignmentsGet`, `fetchTeacherAssignmentsList`, `enrichCommitmentRows`, `enrichCommitmentEvidence`.
- `cmd/andamio/teacher_evidence_test.go`, `cmd/andamio/teacher_assignments_test.go`: test shapes, including the httptest stub for the list endpoint.
- `internal/quiz/quiz.go`, `internal/quiz/quiz_test.go`, `testdata/quiz/SOURCE.md`: the package and fixture pattern to mirror.
- `internal/output/output.go`: `printAsCSV` / `printAsMarkdown`, the generic paths that produce the map dumps.
- `cmd/andamio/surface_test.go`: `schemaSrcDirs`, `TestSchemaSurfaceGolden`, `TestCommandSurfaceGolden`.
- fcb-fan-engagement-app `src/lib/prompts/prompts-envelope.ts`, `prompts-envelope.test.ts`, `prompts-answers.ts`, `prompts-answers.test.ts` at `6bb0efa`.

## Implementation Units

### U1. `internal/prompts` package and fixtures

- **Goal:** One place that knows the prompts envelopes, pinned to the fcb contract.
- **Requirements:** R1, R2; AE4.
- **Dependencies:** none.
- **Files:** `internal/prompts/prompts.go`, `internal/prompts/prompts_test.go`, `testdata/prompts/SOURCE.md`, `testdata/prompts/valid/*.json`, `testdata/prompts/invalid/*.json` + `*.issues`, `testdata/prompts/evidence/{valid,invalid}/*.json`.
- **Approach:**
  1. Write the package per KTD1 and KTD2, following `internal/quiz` for doc comments, `Issue.String`, and never panicking on malformed elements.
  2. Port every upstream test case to a fixture. Sidecar format as in `testdata/quiz` (`source: app` line, then codes). Evidence fixtures are recognize-or-not only.
  3. `SOURCE.md`: the upstream table (file, commit, mirrored as), the fetch commands, the layout, and a note that andamio-app-v2 prompts support is in progress, and that when it lands the rules become the union as for quiz.
- **Patterns to follow:** `internal/quiz/quiz.go`; `internal/quiz/quiz_test.go` fixture walker.
- **Test scenarios:**
  - Valid three-prompt definition, with an object intro, with `intro: null`: zero issues.
  - Version `99` is still recognized as a definition; version `2` yields `unsupported-version`.
  - Empty `prompts` yields only `empty-prompts`.
  - Prompt missing `question`: one `malformed-prompt` with `prompt_id` set. Whitespace-only label: `malformed-prompt`.
  - Entry with no id plus a string entry: two `malformed-prompt`.
  - Two prompts sharing an id: `duplicate-prompt-ids`.
  - `intro: "Read this"`: `malformed-intro`. `intro: {"type":"paragraph"}`: no issue (upstream accepts any object).
  - Covers AE4. Duplicate id plus blank label in one definition: both codes.
  - Recognition: a Tiptap doc, a quiz, quiz evidence, and prompts evidence are not definitions; null, array, string, number are `NotObject`.
  - Evidence recognition: valid evidence; an 800-character answer; rejects `answer: 42`, a record missing `question`, a string element, a null element, `answers: {}`, `type: "prompts"`, `version: "1"`.
  - `Validate` on a non-definition yields one `not-prompts` issue.
  - `Summarize` is tolerant and returns `prompt_ids: []`, never null.
  - `Answers` returns snake_case-tagged records in order and `false` for non-evidence.
- **Verification:** `go test ./internal/prompts` passes; every `.issues` sidecar matches exactly.

### U2. Enrich prompts evidence in JSON

- **Goal:** JSON output of `list` and `get` carries readable and structured answers.
- **Requirements:** R3, R4, R5; AE1.
- **Dependencies:** U1.
- **Files:** `cmd/andamio/teacher_assignments.go`, `cmd/andamio/teacher_evidence_test.go`, `cmd/andamio/surface_test.go`, `cmd/andamio/testdata/golden/schema.golden`.
- **Approach:** Per KTD4. Name the new key with a constant beside `evidenceTextField`. Update the doc comment's output contract. Add `internal/prompts` to `schemaSrcDirs` and regenerate `schema.golden` with `-update` (KTD7).
- **Test scenarios:**
  - Covers AE1. Prompts evidence row: `evidence_text` equals the expected three-block string exactly; `evidence_answers` has three snake_case records; `evidence` deep-equals a copy taken before enrichment.
  - Prompts evidence with an empty `answers` array: neither field present.
  - Prompts evidence with a malformed answer (not recognized): neither field present, `evidence` untouched.
  - Quiz evidence (`type: "quiz-evidence"`): neither field, as today.
  - Tiptap evidence: `evidence_text` as today, no `evidence_answers`.
  - Over the wire through `fetchTeacherAssignmentsList` with an httptest stub holding one prompts row and one Tiptap row.
- **Verification:** `go test ./cmd/andamio -run 'Evidence|Schema'` passes. A1 is confirmed by one manual `teacher assignments list --course <FCB course> -o json` against mainnet showing `evidence_answers` on module `102` rows.

### U3. CSV and Markdown renderers, `--wide`

- **Goal:** Spreadsheet-ready and readable output with no map dumps.
- **Requirements:** R6, R7, R7a, R8, R9, R10; AE2, AE3.
- **Dependencies:** U2.
- **Files:** `cmd/andamio/teacher_assignments.go`, `cmd/andamio/teacher_assignments_test.go`.
- **Approach:**
  1. Add `--wide` and `--module` to `list` only. Reject `--wide` before the request when the format is not CSV, and `--module` without `--course`. Apply the `--module` filter to `data` right after fetch, before any renderer, including text and JSON (JSON keeps the envelope, with `data` filtered).
  2. In `runTeacherAssignmentsList` and `runTeacherAssignmentsGet`, branch: JSON keeps `PrintJSON`; text keeps the table (unchanged); CSV and Markdown call the new renderers (KTD5) after `warnMetaWarning` (KTD6).
  3. Wide validation runs over all rows before any write, so a failure leaves stdout empty; the error is an ordinary usage error (exit 1), with the `--course` hint.
- **Patterns to follow:** `renderTeacherAssignmentsListText` split from the handler so tests call it with a buffer; `encoding/csv` writer as in `internal/output`.
- **Test scenarios:**
  - Covers AE2. Long CSV over one prompts row (three answers) and one Tiptap row: header plus four rows; Tiptap row has blank prompt columns and Markdown in `answer`.
  - Row with no evidence and summary-shape row (no `content`): one row each, blank prompt, answer and status columns.
  - Answer containing a comma, a quote, and a newline round-trips through `csv.NewReader`.
  - Covers AE3. `--wide` over two prompts rows in one module: header `student_alias,course_module_code,status,c102-cause,c102-value,c102-ask`, two rows. A row in the same module with no evidence gets blank prompt cells.
  - `--module 102` over a two-module fixture: only module-102 rows in text, JSON (`data` filtered, other envelope keys intact), CSV and Markdown. `--module` without `--course`: error before any request.
  - `--wide` with two modules, or with no prompts rows: error naming `--course <id> --module <code>`, nothing written.
  - `--wide` over one module with two prompts rows and one Tiptap row: three rows, the Tiptap row with blank prompt cells and its Markdown in a trailing `evidence_text` column; with no Tiptap row, that column is absent.
  - `--wide` with `-o json`: error before any request.
  - Markdown over the same rows: one section per commitment with status and the rendering; no `map[`.
  - Empty data: CSV header only; Markdown writes nothing; both exit 0.
  - `get` with `-o csv` emits the long form for that one commitment.
  - The existing text-table tests pass unmodified.
- **Verification:** `go test ./cmd/andamio` passes. `./andamio teacher assignments list --course <id> -o csv` against a course with prompts evidence opens cleanly in a spreadsheet.

### U4. Docs, help text, changelog, goldens

- **Goal:** Every surface that describes evidence mentions prompts.
- **Requirements:** R11.
- **Dependencies:** U2, U3.
- **Files:** `CONCEPTS.md`, `docs/COURSE-LIFECYCLE.md`, `CHANGELOG.md`, `CLAUDE.md`, `cmd/andamio/teacher_assignments.go` (help), `.claude/skills/assess-assignment/SKILL.md`, `cmd/andamio/testdata/golden/commands.golden` (regenerate with `-update`; it records flags, so `--wide` and `--module` appear under `teacher assignments list`).
- **Approach:**
  1. `CONCEPTS.md`: a "Prompts Envelope" entry beside "Quiz Envelope", covering both shapes and the fcb authority.
  2. `docs/COURSE-LIFECYCLE.md`: replace "Evidence is stored as a Tiptap document" with the two shapes, and show `evidence_answers` and the CSV export.
  3. Help: extend the list output contract with `evidence_answers`, the CSV columns, `--wide`, `--module`, and a `jq` example (`.data[].content.evidence_answers[] | [.prompt_id, .answer]`). Update `get` help to match.
  4. `assess-assignment` skill: read `evidence_answers` for prompts submissions.
  5. `CLAUDE.md`: the `teacher assignments` rows and the evidence-decoding paragraph.
  6. `CHANGELOG.md`: an `Added` entry under `## [Unreleased]` naming cli#171.
- **Test scenarios:** Test expectation: none -- documentation only; goldens are checked by `go test ./cmd/andamio`.
- **Verification:** No doc still says evidence is always Tiptap. `go test ./cmd/andamio -run Golden` passes.

## Verification Contract

| Check | Command | Applies to |
|---|---|---|
| Build | `go build -o andamio ./cmd/andamio` | all |
| Package tests | `go test ./internal/prompts` | U1 |
| Command-layer tests | `go test ./cmd/andamio` | U2, U3, U4 |
| Full suite | `go test ./...` | all |
| Vet | `go vet ./...` | all |

## Definition of Done

- R1–R11 hold and each acceptance example has a passing test.
- `go test ./...` and `go vet ./...` pass with no existing test weakened or removed.
- JSON output of `teacher assignments` only gains fields; text output is byte-identical.
- No new dependency in `go.mod`.
- `course export`, `course import` and `course import-assignment` are untouched.
