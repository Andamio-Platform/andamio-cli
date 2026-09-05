# Residual review findings — feat/course-import-assignment-quiz

Source: `ce-code-review` run `20260905-071204-f33dab2a` on the branch for andamio-cli#165, reviewed against `docs/plans/2026-09-05-001-feat-course-import-assignment-quiz-plan.md`. Nine reviewers plus an independent validator ran; the cross-model adversarial pass did not run (no sanction to send repository code to an external provider), so the in-process adversarial reviewer covered that lens.

Five actionable findings were applied in the `fix(review)` commit on this branch. The items below are what remains. No tracker tickets were filed: opening GitHub issues is outbound activity that was not requested, so this file is the durable record (`no_sink`).

## Residual Review Findings

### Decision gate (human)

- **P1 · `internal/quiz/quiz.go:255` · CLI validator lags andamio-app-v2** — andamio-app-v2's `src/lib/quiz/quiz-envelope.ts` (commit `69c7b57`, 2026-08-26) enforces three rules the CLI does not: `empty-prompt` (prompt trimmed to empty), `empty-option-label`, and `empty-option-value`. The CLI mirrors fcb-fan-engagement-app's validator, which the issue names as the reference and which lacks those rules; the docs now say "FCB Fan Campus app" rather than "the Andamio app". Decision for James: if quizzes published by this CLI must also render in app.andamio.io, add the three rules (with `source: app` fixtures and a second entry in `testdata/quiz/SOURCE.md`); otherwise record that fcb-fan-engagement-app is the sole authority. Related to the plan's flagged Assumption on the three `cli-additional` rules.

### Report-only residual risks

- Read-back cannot detect a concurrent edit made between the pre-fetch and the POST; `verified: true` then certifies the revert (adversarial, reliability).
- The read-back uses `Post` without retry; a transient blip after an accepted write becomes a hard "verification could not run" failure with the underlying kind (reliability, correctness).
- Quiz rules are hand-mirrored from the app; drift is undetectable by CI. `testdata/quiz/SOURCE.md` records the source commits (KTD4, documented).
- Export writes any non-`doc` `content_json` to `assignment.quiz.json`; import accepts only a CLI-valid v1 quiz, so an app-valid stored quiz that fails CLI validation blocks re-import of that directory until edited (KTD6, documented in help and lifecycle doc).
- `--description ""` cannot clear a description, and `image_url` / `video_url` cannot be cleared through `import-assignment`. `--show-payload` is text-mode only under `--output json`, matching `course import`.
- The preserve-existing-metadata rule is implemented in three shapes across `course_import.go` and `course_import_assignment.go` (maintainability).
- The "assignments are editable in any module status" statement rests on gateway and db-api source; the live preprod check on an `ON_CHAIN` module was not run (stated in CHANGELOG and `docs/COURSE-LIFECYCLE.md`).
- No committed fixture pins the teacher `course-modules/list` assignment wire shape, unlike the #90 qualified-contributors fixture (api-contract).
- `course export` reports a quiz only via the filename in `files`; there is no structured `assignment_quiz` field on `ExportResult` (agent-native, advisory).

### Testing gaps

- No test for the two-argument `import-assignment <module-code> <file.json> --course <name>` form.
- No CLI-level test for a syntactically invalid quiz file; the 503 read-back case is exercised in-process only.
- No fixture with HTML-significant or non-ASCII characters in a prompt to pin `encoding/json` escaping against the gateway's re-serialization on read-back.
- No test for `--description ""`.

### Dropped by validation (recorded for transparency)

- "course import doesn't check the degraded-read guard" — the guard was unreachable in the db-api-down case; the root defect was fixed in the shared fetch instead.
- "Empty `assignment.md` beside `assignment.quiz.json` hard-fails import" — implements R11 and the issue text as written; export removes the stale counterpart.
