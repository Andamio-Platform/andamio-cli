# Quiz envelope fixtures — source of truth

These fixtures pin `internal/quiz` against the Andamio app's reference
validator. The app is the authority for every rule the two share; the CLI
mirrors it by hand and this directory is what catches drift.

## Mirrored from

Repository `Andamio-Platform/fcb-fan-engagement-app`, read 2026-09-05:

| File | Commit | Mirrored as |
|------|--------|-------------|
| `src/lib/quiz/quiz-envelope.ts` | `3842a31f9b7a83bc8d7b4273dcb9dfa6b551ed8c` | `Recognize` ← `isQuizContentEnvelope`, `Validate` ← `validateQuizDefinition` |
| `src/lib/quiz/quiz-envelope.test.ts` | `77aa83366ef6b2df2e9c4624d567b890945c87f3` | `valid/minimal.json` ← `validQuiz`; every `validateQuizDefinition` case ← one `invalid/*.json` |

Fetch either file with:

```
gh api repos/Andamio-Platform/fcb-fan-engagement-app/contents/src/lib/quiz/quiz-envelope.ts --jq .content | base64 -d
gh api repos/Andamio-Platform/fcb-fan-engagement-app/contents/src/lib/quiz/quiz-envelope.test.ts --jq .content | base64 -d
```

**A rule change in the app is re-mirrored by hand.** Nothing here fetches the
app at test time. When the app's validator changes, update `internal/quiz/quiz.go`,
update or add fixtures, and bump the commits in the table above.

## Layout

- `valid/<case>.json` — must recognize as a quiz and produce zero issues.
- `invalid/<case>.json` + `invalid/<case>.issues` — must recognize as a quiz
  (invalid is not the same as unrecognized) and produce exactly the sidecar's
  code set. Sidecar format: first line `source: app` or
  `source: cli-additional`, then one expected issue code per line; order is
  irrelevant.

## Source labels

- `source: app` — the app's `validateQuizDefinition` emits the same code set
  for this input. Most of these are the app's own test cases verbatim
  (`null-question` keeps the app's `passThreshold: 2`, so the app also
  reports `threshold-exceeds-questions` for it); `non-integral-version` and
  `threshold-non-integral` are not in the app's test file but exercise its
  `version !== 1` and `Number.isInteger` rules.
- `source: cli-additional` — rules the app does not enforce. Codes:
  `malformed-prompt` (prompt missing, not a string, or empty),
  `malformed-help` (help present, non-null, not a string),
  `malformed-intro` (intro present, non-null, not an object with `type: "doc"`).
  A `source: app` fixture must never yield one of these codes; the test
  enforces that.

`not-a-quiz` is a guard code for callers that skip `Recognize`; it has no
fixture because every fixture here is quiz-shaped by construction, and it is
tested directly in `internal/quiz/quiz_test.go`.
