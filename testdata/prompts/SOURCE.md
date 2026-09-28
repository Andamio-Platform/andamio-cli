# Prompts envelope fixtures — source of truth

These fixtures pin `internal/prompts` against the Andamio app that renders
prompts assignments. The app is the authority for every rule; the CLI mirrors
them by hand and this directory is what catches drift.

Only fcb-fan-engagement-app renders prompts today. andamio-app-v2 support is
in progress. When it lands, add its files to the table below and make
`Validate` enforce the **union** of both apps' rules, as `testdata/quiz` does,
so an envelope the CLI accepts renders in both.

## Mirrored from

Repository `Andamio-Platform/fcb-fan-engagement-app`, read 2026-09-28:

| File | Commit | Mirrored as |
|------|--------|-------------|
| `src/lib/prompts/prompts-envelope.ts` | `6bb0efa4d8b7f20bc880ce1bbc4a5b4771f03a8d` | `Recognize`/`Classify` ← `isPromptsContentEnvelope` and `isPromptsEvidenceEnvelope`, `Validate` ← `validatePromptsDefinition` |
| `src/lib/prompts/prompts-envelope.test.ts` | `6bb0efa4d8b7f20bc880ce1bbc4a5b4771f03a8d` | `valid/minimal.json` ← `validPrompts`; `evidence/valid/minimal.json` ← `validEvidence`; every guard and `validatePromptsDefinition` case ← one fixture |
| `src/lib/prompts/prompts-answers.ts` | `6bb0efa4d8b7f20bc880ce1bbc4a5b4771f03a8d` | the evidence shape `buildPromptsEvidence` writes (label and question snapshotted beside each trimmed answer) |

Fetch a file with:

```
gh api repos/Andamio-Platform/fcb-fan-engagement-app/contents/src/lib/prompts/prompts-envelope.ts --jq .content | base64 -d
gh api repos/Andamio-Platform/fcb-fan-engagement-app/contents/src/lib/prompts/prompts-envelope.test.ts --jq .content | base64 -d
```

**A rule change in the app is re-mirrored by hand.** Nothing here fetches the
app at test time. When a validator changes, update `internal/prompts/prompts.go`,
update or add fixtures, and bump the commit in the table above.

## Layout

- `valid/<case>.json` — must recognize as a definition and produce zero issues.
- `invalid/<case>.json` + `invalid/<case>.issues` — must recognize as a
  definition (invalid is not the same as unrecognized) and produce exactly the
  sidecar's codes **in order**. The app checks version, then intro, then an
  empty prompts list (returning early), then each prompt, and the CLI keeps
  that order. Sidecar format: first line `source: app`, then one code per line.
- `evidence/valid/<case>.json` — must recognize as prompts evidence.
- `evidence/invalid/<case>.json` — must not. The app's evidence guard is
  field-strict (every answer needs string `promptId`, `label`, `question` and
  `answer`), version-agnostic, and has no length cap; there is no separate
  evidence validator, so recognition is the validity check.

## Deliberate matches with the app

- `malformed-intro` fires only when `intro` is present, non-null and not an
  object. The app does not require `type: "doc"`, so neither does the CLI
  (`valid/intro-any-object.json`). The quiz validator's stricter intro check is
  a CLI addition that is not carried over here.
- `non-integral-version.json` and `evidence/invalid/snake-case-keys.json` are
  not in the app's test file. They exercise its `version !== 1` rule and its
  camelCase `promptId` field.

`not-prompts` is a guard code for callers that skip `Recognize`; it has no
fixture and is tested directly in `internal/prompts/prompts_test.go`.
