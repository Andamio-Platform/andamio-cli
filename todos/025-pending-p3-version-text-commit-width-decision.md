---
status: pending
priority: p3
issue_id: "025"
tags: [code-review, cli, version, text-format, pr-69-followup]
dependencies: []
---

# `--version` text format regressed from full SHA to 7-char short SHA

## Problem Statement

Pre-PR-#69, `andamio --version` on a goreleaser-built binary printed the **full 40-character** commit SHA:

```
andamio 0.11.2 (commit: 340b677e7ef2107bd0d581fccf699ba74cad9cf7, built: 2026-04-08T13:06:36Z)
```

Post-PR-#69, the same goreleaser build now prints the **7-character short** SHA:

```
andamio 0.11.2 (commit: 340b677, built: 2026-04-08T13:06:36Z)
```

**Root cause** (`cmd/andamio/main.go:57`): the refactored `buildVersionOutput` wraps `commit` with `shortCommit(commit)` in the text path. Pre-PR, `SetVersionTemplate(fmt.Sprintf("andamio %s (commit: %s, built: %s)\n", version, commit, date))` interpolated the full `commit` value. The `shortCommit` helper was introduced to guard against `commit[:7]` panicking on the `"none"` default — but it was applied to both the text and JSON paths, changing the text format for release binaries.

The integration test (`TestVersionFlag_TextMode_Integration`) uses regex `^andamio \S+ \(commit: \S+, built: \S+\)\n$` which permits both widths — so the regression shipped undetected.

The CHANGELOG's original claim "Plain-text `--version` output is unchanged" was factually wrong for goreleaser builds. The ce:review autofix round corrected the CHANGELOG to reflect the actual behavior; this todo tracks the **design decision** the team should make.

Found during ce:review autofix of PR #69 — cross-reviewer consensus (correctness 0.85 + api-contract).

## Affected Files

- `cmd/andamio/main.go:57` — the text-mode `fmt.Sprintf` call using `shortCommit(commit)`
- `cmd/andamio/main.go:21-28` — `shortCommit` helper
- `cmd/andamio/main_test.go:119-124` — the too-permissive regex in `TestVersionFlag_TextMode_Integration`
- `CHANGELOG.md` — already updated (autofix) to describe the new behavior accurately

## Options

### Option A (accept the regression, tighten the test)

Keep `shortCommit` in the text path. Update the integration regex to `^andamio \S+ \(commit: [0-9a-f]{7}, built: \S+\)\n$` to lock the 7-char format.

**Rationale:** The plain-text output becomes internally consistent with the JSON path (both show 7 chars). Local dev builds and goreleaser builds now print identically. Short SHAs are sufficient for `git show`. The CHANGELOG callout is already in place.

### Option B (restore full-SHA text, use shortCommit only for JSON)

Revert the `shortCommit(commit)` wrap in the text path:

```go
return fmt.Sprintf("andamio %s (commit: %s, built: %s)\n", version, commit, date)
```

Keep `shortCommit` for the JSON envelope only. Update CHANGELOG to drop the "Plain-text `--version` output now uses the same 7-character short commit" note.

**Rationale:** Preserves existing goreleaser-build output byte-for-byte. Users who grep the text output for the full SHA (e.g., in support tickets, issue reports) continue to get it. The JSON consumer gets a short SHA optimized for `git show <short>`; the text consumer gets the forensic full SHA.

### Option C (emit both in text)

```
andamio 0.11.2 (commit: 340b677 (340b677e...cd9cf7), built: ...)
```

Too verbose; not recommended.

**Preferred:** Option A. Internal consistency is more valuable than byte-for-byte back-compat for a pre-1.0 CLI that's actively evolving, and the CHANGELOG now correctly advertises the change. Consumers that want the full SHA can run `git rev-parse <short>` against a checkout.

## Acceptance

- [ ] Decision on A/B documented (commit message is fine).
- [ ] If Option A: tighten `TestVersionFlag_TextMode_Integration` regex to `[0-9a-f]{7}` (exactly 7 hex chars).
- [ ] If Option B: revert `shortCommit` in the text path; keep in JSON. Tighten the regex to `[0-9a-f]{40}` for release builds (test binary injects `1234567` so it would need a separate test or a more permissive regex).
- [ ] Update CHANGELOG.md accordingly if Option B is chosen (retract the autofix-applied note about 7-char short commit in text mode).

## Context

- **ce:review run artifact:** `.context/compound-engineering/ce-review/2026-04-22-pr69-release-hygiene/findings.md`
- **Origin PR:** https://github.com/Andamio-Platform/andamio-cli/pull/69
