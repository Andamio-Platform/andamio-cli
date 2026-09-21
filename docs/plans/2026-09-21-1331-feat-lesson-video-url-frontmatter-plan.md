---
title: Lesson video_url frontmatter - Plan
type: feat
date: 2026-09-21
artifact_contract: ce-unified-plan/v1
artifact_readiness: implementation-ready
product_contract_source: ce-plan-bootstrap
execution: code
---

# Lesson video_url frontmatter - Plan

## Goal Capsule

- **Objective:** A teacher authenticated in the CLI can give each lesson of a course module a video link, change it, and remove it, and the link survives an export followed by an import.
- **Means:** Optional YAML frontmatter on `lesson-N.md`, read by `course import` and written by `course export` (KTD1, KTD2).
- **Authority:** This plan, then the repo's `CLAUDE.md` Export/Import Pattern and Composability Rules. The Product Contract wins on behavior; a KTD wins on mechanism.
- **Execution profile:** Go CLI, existing import/export code paths, no new dependencies. `go test ./...` is the contract.
- **Stop conditions:** Stop and report if `github.com/adrg/frontmatter` cannot distinguish "no frontmatter" from "frontmatter present" in a way that keeps R5 true, or if the gateway's lesson object turns out not to carry `video_url` on the teacher list response.
- **Tail ownership:** The caller (LFG) owns simplify, review, commit, PR, and CI.

## Product Contract

### Summary

`course import` reads an optional `video_url` from YAML frontmatter at the top of each `lesson-N.md` and sends it as that lesson's `video_url`. `course export` writes the same frontmatter for any lesson that has a video, so the round trip is a server-side no-op. Lesson files without frontmatter behave exactly as they do today.

### Problem Frame

The gateway's `course-module/update` accepts `lessons[].video_url` (`AggregateLessonInput` in the OpenAPI spec), and the teacher list returns it on each lesson (`LessonV2`). The CLI has no input for it. `updateModuleContent` in `cmd/andamio/course_import.go` copies `video_url` from the existing lesson and nothing else, and lesson files carry only an H1 title and a Markdown body. A teacher who wants a video on a lesson has to set it in the app. `course export` also drops the value, so a teacher cannot see from disk which lessons have one.

`docs/COURSE-LIFECYCLE.md` adds to the confusion: its `outline.md` example shows `description`, `image_url` and `video_url` frontmatter keys, but `OutlineFrontmatter` parses only `title` and `code`, so those keys are silently ignored.

### Key Decisions

- **Lesson frontmatter carries `video_url` only.** (session-settled: user-directed — chosen over also adding `image_url` and `description` in the same change: the need is a video link per lesson, and the change stays minimal.) Governs R1, R6.
- **The input surface is frontmatter on `lesson-N.md`, consumed by `course import`.** (session-settled: user-directed — chosen over a new per-lesson flag or command such as `course teacher update-lesson --video-url`: lessons are authored as files, import is the atomic update path, and frontmatter round-trips with export.) Governs R1, R7.

### Requirements

**Import**

- R1. A `lesson-N.md` may begin with a YAML frontmatter block containing `video_url`. `course import` sends a non-empty value as that lesson's `video_url`, replacing whatever the gateway holds.
- R2. When the `video_url` key is absent, or the file has no frontmatter, the lesson's existing `video_url` is preserved, as today.
- R3. When the key is present with an empty or null value, the lesson's video is cleared: the lesson payload carries no `video_url`.
- R4. A non-empty `video_url` must be an absolute `http` or `https` URL with a host. A value that is not fails the import before any request is sent, and the error names the file.
- R5. A lesson file without frontmatter produces the same title and `content_json` as before this change, including a file that opens with a `---` thematic break and contains a later one. With frontmatter, the H1 title is extracted from the text after the block.
- R6. A frontmatter key other than `video_url` fails the import before any request is sent. The error names the file, the key, and the supported key.

**Export**

- R7. `course export` writes a `video_url` frontmatter block at the top of `lesson-N.md` for a lesson whose `video_url` is non-empty. A lesson without one is written byte-for-byte as today.
- R8. Exporting a module and importing the result sends, for every lesson, the `video_url` the gateway already holds.

**Visibility and docs**

- R9. In text modes, import reports on stderr each lesson whose frontmatter changes the stored video: a set line when the value differs from the existing `video_url` or none exists, and a clear line when an existing value is removed. An unchanged value prints nothing. JSON mode prints nothing extra; the dry-run payload already shows the value.
- R11. In every output mode, import warns on stderr, naming the file, when a `video_url` it sends is not a YouTube video URL the Andamio app can embed. The value is still sent.
- R10. The docs state that the Andamio app embeds only YouTube links. `README.md`, `docs/COURSE-LIFECYCLE.md`, `CLAUDE.md`, the `course import` and `course export` help text, and `CHANGELOG.md` describe the lesson frontmatter. The `outline.md` example in `docs/COURSE-LIFECYCLE.md` lists only the keys `OutlineFrontmatter` parses.

### Acceptance Examples

- AE1. Covers R1, R5. Given `lesson-2.md` with a frontmatter block `video_url: "https://youtu.be/dQw4w9WgXcQ"` followed by `# Intro` and a paragraph, and an existing lesson 2 with `video_url` `https://old.example/v`, the payload's lesson 2 has title `Intro`, a body with one paragraph and no frontmatter text, and `video_url` `https://youtu.be/dQw4w9WgXcQ`.
- AE2. Covers R2. Given the same existing lesson and a `lesson-2.md` with no frontmatter, the payload's lesson 2 has `video_url` `https://old.example/v`.
- AE3. Covers R3. Given the same existing lesson and frontmatter `video_url: ""`, the payload's lesson 2 has no `video_url` key, and its existing `description` and `image_url` are still preserved.
- AE4. Covers R6. Given frontmatter `video-url: https://youtu.be/dQw4w9WgXcQ`, import fails naming `lesson-2.md`, `video-url`, and `video_url`, and sends nothing.

### Scope Boundaries

- No `image_url` or `description` in lesson frontmatter.
- No frontmatter on `introduction.md`, `assignment.md`, or new keys on `outline.md`.
- No check that the URL resolves, and no rejection of non-YouTube hosts (R11 warns instead).
- `course import-assignment` is unchanged.

#### Deferred to Follow-Up Work

- `andamio-docs`: `content/docs/apps-tooling/cli/import-format.mdx` should document the lesson frontmatter (separate repo; fold into `Andamio-Platform/andamio-docs#64` or a new issue).
- `andamio-lesson-coach-v2`: its compile step could emit `video_url` frontmatter.

## Planning Contract

### Key Technical Decisions

- KTD1. **`video_url` is the only recognised lesson frontmatter key, and any other key is an error rather than ignored.** (session-settled: user-directed — chosen over also adding `image_url` and `description` in the same change: the need is a video link per lesson, and the change stays minimal.) Implements R1, R6. A silent ignore would turn a typo such as `video-url` into an import that succeeds and does nothing. No working lesson file has frontmatter today, because the converter would render it as a thematic break plus body text.
- KTD2. **Parse lesson frontmatter in `readCompiledModule` with `github.com/adrg/frontmatter`, the library `outline.md` already uses.** (session-settled: user-directed — chosen over a new per-lesson flag or command such as `course teacher update-lesson --video-url`: lessons are authored as files, import is the atomic update path, and frontmatter round-trips with export.) Implements R1, R5. Decode into a generic map so that key presence, unknown keys, and null values are all observable, then pass the remaining text to `extractH1Title`.
- KTD3. **`LessonImport` carries the video as three states: not specified, set to a value, cleared.** Implements R2, R3. `updateModuleContent` keeps its existing preservation loop for `description` and `image_url`, and applies the local state to `video_url` afterwards. "Cleared" omits the key from the lesson map. This relies on A1.
- KTD4. **Validate with `net/url` at parse time.** Implements R4. Validation lives in `readCompiledModule`, so `--dry-run`, `course import`, and `course import-all` all fail before any network call, matching how `assignment.quiz.json` is validated.
- KTD5. **Export emits the block only when there is a value, using the `%q` quoting `generateOutline` uses for YAML strings.** Implements R7. `fetchModuleData` must carry `video_url` from `slt.lesson` into `SLTData.Lesson`, which today holds only `content_json` and `title`.

- KTD6. **A leading `---` … `---` block is frontmatter only when it decodes to a YAML mapping.** Implements R5, R6. `adrg/frontmatter` v0.2.0 returns the full text and no error for an unclosed leading `---`, skips blank lines before it, and decodes the text between a closed pair as YAML. A lesson that opens with a thematic break and has a second one would therefore fail to decode. When the block is not a mapping (scalar prose, a sequence, or a YAML error), import treats the file as having no frontmatter and converts the whole text as today. The exception is a block whose text mentions `video_url`: that is an author's broken frontmatter, and it errors naming the file. A block that decodes to a mapping with other keys falls under KTD1.
- KTD7. **The YouTube check mirrors `parseYouTubeVideoId` in andamio-app-v2 `src/lib/youtube-embed.ts` and only warns.** Implements R11. Recognised: hosts `youtube.com`, `www.youtube.com`, `m.youtube.com`, `youtu.be`, `youtube-nocookie.com`, `www.youtube-nocookie.com`, with an 11-character id (`[A-Za-z0-9_-]`) from `watch?v=`, `youtu.be/<id>`, `/shorts/`, `/embed/`, `/live/`, or `/v/`. The gateway stores any string and other clients may render other hosts, so the CLI does not reject. The warning is emitted in JSON mode too, because a silent no-render is the failure it exists to prevent.

### Assumptions

- A1. The gateway replaces each lesson entity from the array item it receives, so a lesson sent without `video_url` ends with none. Source: `CLAUDE.md`, "array items (lessons, slts) replace the full entity — must include all fields to preserve them", and `docs/solutions/logic-errors/fix-three-cli-issues-hex-encoding-lesson-merge-headless-login.md`. Not verified against a live write in this plan; U1 verification names the manual check.
- A3. Warning rather than rejecting on a non-YouTube `video_url` (R11) was chosen during plan review without user input. Rejecting would be a one-line change in the same check.
- A2. `course import-all` needs no separate work because it calls `importModule`, which calls `readCompiledModule`.

### Deferred Implementation Notes

- How to obtain the raw block text for KTD6's `video_url` mention check, given that the library returns only the decoded value and the remaining body.
- Exact names of the new `LessonImport` fields and helper functions.

### Risks

- A stored `video_url` that fails R4 (written by another client) makes an exported module fail re-import until the author edits the file. The app's editor saves only canonical YouTube watch URLs, and no such stored value is known.

### Sources

- `cmd/andamio/course_import.go`: `readCompiledModule` (lesson loop), `extractH1Title`, `updateModuleContent` (lesson metadata preservation and the merge with existing lessons), `fetchExistingModule`.
- `cmd/andamio/course_export.go`: `fetchModuleData` (builds `SLTData.Lesson`), `writeCompiledModule`, `convertLessonToMarkdown`, `generateOutline`.
- `cmd/andamio/course_import_quiz_test.go`: `TestUpdateModuleContent_QuizPayloadPreservesMetadata` and `TestExportImportRoundTrip_QuizIsServerNoOp` show the test shape: build a directory, call `readCompiledModule`, call `updateModuleContent` with `dryRun` true and a nil client, inspect `resp["payload"]`.
- `docs/solutions/logic-errors/export-import-round-trip-title-preservation.md`: earlier round-trip losses in the same two files.

## Implementation Units

### U1. Import reads lesson frontmatter and applies `video_url`

- **Goal:** `course import` sets, preserves, or clears a lesson's video according to the lesson file's frontmatter.
- **Requirements:** R1, R2, R3, R4, R5, R6, R9, R11; AE1–AE4.
- **Dependencies:** none.
- **Files:** `cmd/andamio/course_import.go`, `cmd/andamio/course_import_test.go`.
- **Approach:**
  1. In the lesson loop of `readCompiledModule`, split frontmatter from the body before `extractH1Title` (KTD2, KTD6), reject unknown keys (KTD1), validate the value (KTD4), and warn on a non-YouTube value (KTD7).
  2. Record the three-state result on `LessonImport` (KTD3).
  3. In `updateModuleContent`, apply that state to the lesson map after the existing preservation loop, and print the stderr line when the value changes and the mode is not JSON (R9).
  4. Leave the "preserved existing lesson (no local file)" branch untouched; it already copies `video_url`.
  5. Extend the `course import` long help to mention the optional frontmatter.
- **Execution note:** Start with a characterization test for a lesson file that has no frontmatter, and one that opens with a bare `---` line, before changing the parser.
- **Patterns to follow:** `parseQuizFile` for parse-time errors that name the file; the `outline.md` frontmatter parse for library use; `if !isJSON` gating for stderr progress.
- **Test scenarios:**
  - Covers AE1. Frontmatter with a valid URL plus H1 and body: title from the H1, body has no frontmatter text, payload lesson carries the new URL over a different existing one.
  - Covers AE2. No frontmatter, existing lesson has a video: payload keeps the existing URL.
  - Frontmatter block present but without the `video_url` key (empty block): existing URL preserved.
  - Covers AE3. `video_url: ""`: payload lesson has no `video_url` key, while existing `description` and `image_url` remain.
  - `video_url:` with a null value: same as the empty string.
  - New module (`SLTCount` 0, no existing lesson) with a URL: payload lesson carries it.
  - Covers AE4. Unknown key `video-url`: `readCompiledModule` errors naming the file, the key, and `video_url`.
  - Invalid values `not a url`, `ftp://host/v.mp4`, `https://` and `/relative/path`: each errors naming the file.
  - Malformed YAML inside the block: error names the file.
  - File with no frontmatter: title and Tiptap output equal to the pre-change result for the same text.
  - File opening with a bare `---` line and no closing delimiter: imports without error and keeps its content.
  - File opening with `---`, a prose paragraph, a second `---`, then more text: imports with the same Tiptap output as before the change (two horizontal rules and both paragraphs).
  - Block containing `video_url: "https://unclosed` (malformed YAML mentioning the key): errors naming the file.
  - Frontmatter URL equal to the existing URL: no set line. Different URL: one set line. Empty value with an existing URL: one clear line. Empty value with no existing URL: no line.
  - `https://vimeo.com/123` and `https://youtu.be/abc` (id not 11 characters): sent in the payload, one warning naming the file, also in JSON mode. `https://www.youtube.com/watch?v=dQw4w9WgXcQ`, `https://youtu.be/dQw4w9WgXcQ` and `https://www.youtube.com/shorts/dQw4w9WgXcQ`: no warning. `https://youtube.com.evil.com/watch?v=dQw4w9WgXcQ`: warning.
  - JSON output mode: no stderr line for set or clear.
- **Verification:** `go test ./cmd/andamio` passes with the new cases. `andamio course import <dir> --course-id <id> --dry-run --show-payload` against a directory with one frontmatter lesson shows `video_url` on that lesson only. A1 is confirmed by one manual preprod import that sets and then clears a video on a DRAFT module, read back with `andamio course export`.

### U2. Export writes lesson frontmatter

- **Goal:** `course export` surfaces each lesson's video on disk in the form U1 reads.
- **Requirements:** R7, R8.
- **Dependencies:** U1.
- **Files:** `cmd/andamio/course_export.go`, `cmd/andamio/course_export_test.go`.
- **Approach:**
  1. Carry `video_url` from `slt.lesson` into `SLTData.Lesson` in `fetchModuleData` (KTD5).
  2. In `convertLessonToMarkdown`, prepend the frontmatter block ahead of the H1 when the value is non-empty. Build the block before the existing `contentJSON == nil` early return, so a lesson with a video and no body still writes it.
  3. Extend the `course export` long help to mention the block.
- **Patterns to follow:** `generateOutline` for YAML string quoting; `sanitizeTitle` for stripping newlines from a gateway-supplied string before embedding it.
- **Test scenarios:**
  - Lesson with `video_url`, title and content: file starts with the frontmatter block, then a blank line, then the H1.
  - Lesson without `video_url`, and with an empty-string `video_url`: output identical to the current output for the same lesson.
  - A `video_url` containing a quote, `#`, or `: `: the written block parses back to the identical string.
  - Lesson with `video_url` but no `content_json`: the file is written and re-imports without error.
  - Covers R8. Round trip: `writeCompiledModule`, then `readCompiledModule`, then `updateModuleContent` in dry-run with an existing lesson holding the same URL; payload `video_url` equals the existing value, and title and body match.
- **Verification:** `go test ./cmd/andamio` passes. Exporting a preprod module whose lesson has a video produces a `lesson-N.md` with the block, and `course import --dry-run` of that directory reports no set or clear lines that change the value.

### U3. Documentation and changelog

- **Goal:** The file format docs match what import and export do.
- **Requirements:** R10.
- **Dependencies:** U1, U2.
- **Files:** `README.md`, `docs/COURSE-LIFECYCLE.md`, `CLAUDE.md`, `CHANGELOG.md`.
- **Approach:**
  1. `README.md`: add the optional frontmatter to the `lesson-N.md` format section, with the preserve, set, and clear rules, the YouTube-only note, and the KTD6 rule for files that open with `---`.
  2. `docs/COURSE-LIFECYCLE.md`: document the lesson frontmatter and cut the `outline.md` example down to `title` and `code`.
  3. `CLAUDE.md`: add an item to the Export/Import Pattern list covering the three states and the unknown-key error.
  4. `CHANGELOG.md`: add an `Added` entry under `## [Unreleased]`.
- **Test scenarios:** Test expectation: none -- documentation only.
- **Verification:** Every place that describes the lesson file format mentions the frontmatter, and no doc shows an `outline.md` key the parser ignores.

## Verification Contract

| Check | Command | Applies to |
|---|---|---|
| Build | `go build -o andamio ./cmd/andamio` | U1, U2 |
| Command-layer tests | `go test ./cmd/andamio` | U1, U2 |
| Full suite | `go test ./...` | all |
| Vet | `go vet ./...` | U1, U2 |
| Dry-run smoke | `./andamio course import <dir> --course-id <id> --dry-run --show-payload` | U1 |

## Definition of Done

- R1–R11 hold, and each acceptance example has a passing test.
- `go test ./...` and `go vet ./...` pass with no existing test weakened or removed.
- Lesson files without frontmatter import and export exactly as before.
- No new dependency in `go.mod`.
- Nothing is read from stdin, progress goes to stderr, and JSON mode stdout is unchanged.
- Docs listed in U3 are updated, and the two follow-ups under Scope Boundaries are left as follow-ups, not started.
- No abandoned experimental code remains in the diff.
