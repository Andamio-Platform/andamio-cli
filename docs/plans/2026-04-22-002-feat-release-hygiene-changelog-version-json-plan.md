---
title: "feat: CHANGELOG.md + andamio --version --output json"
type: feat
status: completed
date: 2026-04-22
deepened: 2026-04-22
origin: https://github.com/Andamio-Platform/andamio-cli/issues/67
---

# feat: CHANGELOG.md + andamio --version --output json

## Overview

Add two release-hygiene artifacts that together give scripts and agents a machine-readable way to detect which CLI they're talking to: a `CHANGELOG.md` at the repo root following Keep a Changelog conventions (backfilled from existing git tags), and a JSON variant of `--version` emitting `{version, commit, built}` when `--output json` is passed. Also add a CHANGELOG-entry check to `scripts/release.sh` so the next breaking change does not ship without a discoverable note.

## Problem Frame

PR #63 shipped a breaking `--output json` envelope change to `course teacher register-module`. The release note sat in the PR body only. There was no `CHANGELOG.md` to scan and no way for an agent to ask `andamio --version` for a structured answer. PR #68 (typed ConflictError) extended the `--output json` surface area further — a second breaking-adjacent change with no discoverable signal. The next one will have the same problem unless we fix the surface once. See issue #67 for the full framing and PR #63 review thread for the original P1 finding.

## Requirements Trace

- **R1.** `CHANGELOG.md` exists at the repo root following [Keep a Changelog](https://keepachangelog.com/) conventions (`Added` / `Changed` / `Deprecated` / `Removed` / `Fixed` / `Security` sections under each version heading). Backfilled for at least the last 3 releases (stretch: all 11 existing releases `v0.6.0` → `v0.11.2`).
- **R2.** `andamio --version --output json` emits `{"version": "<x>", "commit": "<sha7>", "built": "<iso-8601>"}` on stdout with a trailing newline. All three values come from the existing ldflag-injected package vars (`version`, `commit`, `date` in `cmd/andamio/main.go`).
- **R3.** `andamio --version` (no `--output` or `--output text`) continues to emit the current human-readable text exactly as today. No regression for existing consumers.
- **R4.** `scripts/release.sh` preflight reminds the maintainer to add a CHANGELOG entry before tagging. Bare minimum: a grep check that `CHANGELOG.md` contains the target `$VERSION` string somewhere in the first 50 lines; if absent, warn and require interactive confirmation before proceeding. (Enforcement vs. reminder is a deferred decision — see Open Questions.)

## Scope Boundaries

- **Out of scope:** a full capability manifest (`andamio capabilities --output json` listing command → envelope schema). Issue #67 calls this out explicitly as overkill for current scale. Revisit only when the CLI grows more breaking-change surface.
- **Out of scope:** automated CHANGELOG generation from commits. Manual entries are fine for now; commit style is not disciplined enough to auto-extract from.
- **Out of scope:** backfilling CHANGELOG entries with fine-grained change-by-change detail for every historical release. A one-line summary per tag (derived from the tag message or the top-of-release commit subject) is sufficient for the backfill.
- **Out of scope:** moving the release workflow to semantic-release, standard-version, or any other tool. `release.sh` stays the source of truth.
- **Out of scope:** changing exit codes or adding a `version` subcommand. `--version` stays the only version-emission surface.

## Context & Research

### Relevant Code and Patterns

- `cmd/andamio/main.go:14-26` — the `version`/`commit`/`date` package vars + `versionString()` helper. These are already populated at build time via `-ldflags "-X main.version=..."` in GoReleaser and `scripts/release.sh`'s build-test.
- `cmd/andamio/main.go:41-45` — `rootCmd.SetVersionTemplate(...)` with a `Sprintf`-produced template string. Cobra's template system supports Go `text/template` syntax; `AddTemplateFunc` can register a function callable from the template.
- `cmd/andamio/main.go:35-38` — `PersistentPreRunE` calls `output.SetFormat(outputFormat)`. Flag parsing runs before the version template renders, so `outputFormat` (a package-level `var`) IS populated by the time a version-print happens. This means a template function reading `outputFormat` directly will see the correct value.
- `internal/output/` — the output format registry (`SetFormat`/`GetFormat`/`FormatJSON` constants). The version JSON path doesn't need to go through `output.PrintJSON` (which is for the global data-stream-to-stdout contract) — but using the same `FormatJSON` constant keeps the check consistent.
- `scripts/release.sh:31-45` — preflight check structure. New checks are added as `if`-blocks with `✓` / `✗` output and an `exit 1` on failure. Consistent pattern; CHANGELOG reminder slots in naturally.
- `CLAUDE.md` (project) — documents `--version` under "Global Flags" and release workflow under "Release". Both sections will need one-line updates to reflect the new JSON mode.

### Institutional Learnings

- `docs/solutions/architecture/cli-composability-audit-and-fix.md` — establishes the `--output json` contract ("`--output json` is the scripting surface. All list/get commands must support it with stable JSON schemas"). Extending `--version` to honor `--output json` is the natural completion of that contract.
- PR #63 review (ce:review finding P1 #2) — the direct trigger for issue #67; the review explicitly called out that there is no way for agents to detect which CLI they're talking to. This PR closes that gap.
- No existing `docs/solutions/*` covers Keep a Changelog conventions or release note hygiene; this is net-new institutional practice worth documenting in a short follow-up solution doc if the release cadence picks up.

### External References

- Keep a Changelog 1.1.0 — https://keepachangelog.com/en/1.1.0/ (section structure, versioning order, "Unreleased" header)
- Cobra `SetVersionTemplate` docs — standard Go `text/template` syntax; `cobra.AddTemplateFunc` for custom functions callable from the template

## Key Technical Decisions

- **Use Cobra template functions for the JSON branch, not a separate subcommand.** Register a template function via `cobra.AddTemplateFunc("versionOutput", fn)` where `fn` reads the already-populated `outputFormat` package var and returns either the JSON payload or the current text. Rationale: acceptance criterion R2 explicitly requires `andamio --version --output json` (flag-based, not subcommand-based). Cobra parses flags before rendering the version template, so the package var is populated. This avoids adding a new subcommand surface and matches the issue's stated preference.
- **Back the JSON branch with `encoding/json.Marshal`, not a hand-rolled `fmt.Sprintf`.** Rationale: `cli-composability-audit-and-fix.md` Prevention Strategy #3 explicitly flags hand-rolled JSON literals as a review red flag. `json.Marshal` of a small struct is the canonical path. Handles escaping correctly and survives future field additions.
- **Commit hash in JSON is the truncated 7-char `sha` prefix, not the full 40-char SHA.** Rationale: symmetry with the existing text output, which already shows `commit[:7]`. Consumers who need the full SHA can run `git show <tag>` or query the GitHub release. Adding both (short + long) would bloat the envelope without a clear consumer.
- **`built` timestamp is the verbatim ldflag-injected `date` string.** Rationale: released binaries are built by `.goreleaser.yml` (line 11: `-X main.version={{.Version}} -X main.commit={{.Commit}} -X main.date={{.Date}}`) which populates all three package vars. `scripts/release.sh`'s local `go build -ldflags "-X main.version=$VERSION"` injects only `version` — but that build goes to `/dev/null` as a compile-test, so it doesn't ship. Dev builds (`go build ./cmd/andamio` without ldflags) leave all three at compile-time defaults (`"dev"` / `"none"` / `"unknown"`). The JSON should mirror this: emit whatever `date` holds. Formatting to a strict ISO-8601 at emission time would require parsing-and-reformatting an already-formatted string, which is brittle; trust the builder to inject a sane value.
- **CHANGELOG.md backfills the last 3 releases explicitly (`v0.11.2`, `v0.11.1`, `v0.11.0`) and bundles earlier releases as a single "Earlier releases" entry.** Rationale: acceptance R1 says "at least the last 3 releases." Going deeper runs into commit archaeology with diminishing marginal value. The 3-most-recent-plus-bundle shape is a defensible cutoff and easy to extend later by promoting the bundle into individual entries.
- **`release.sh` CHANGELOG check is a reminder, not a hard block.** Rationale: a hard block adds friction for no-release-note patches (e.g., doc-only fixes). A warning + interactive confirmation matches the existing UX (the script already prompts `Proceed? [y/N]` before tagging). Maintainer can acknowledge and continue if the release genuinely has no user-facing change.
- **CHANGELOG check pattern is `grep -q "^## \[$VERSION\]" CHANGELOG.md` (version-heading match), not a check for a non-empty `Unreleased` section.** Rationale: at tag time, the maintainer's workflow is to move `## [Unreleased]` content under a new `## [x.y.z] - YYYY-MM-DD` heading as part of the release PR, not in the tag commit. The heading-match check fires correctly after that move, and it's unambiguous to implement (one regex, one exit path). The `Unreleased`-body check would require parsing CHANGELOG structure to distinguish "empty" from "populated," which is more fragile. This decision was surfaced as deferred during scoring; committing to the heading-match approach up-front avoids implementer ambiguity.

## Open Questions

### Resolved During Planning

- **Should `--version --output json` also include a `spec_version` or `envelope_version` to signal breaking-change cutoffs?** No — deferred. The `version` field alone gives consumers a reliable ordinal. A separate `spec_version` only becomes useful if the semver-to-CLI-contract mapping decouples (e.g., `0.12.0` ships two envelope changes in one release). Not our shape today. Revisit if issue #66 (typed envelope structs) lands and demands it.
- **Should the JSON include the build platform (`runtime.GOOS`/`GOARCH`)?** No. The CLI binary IS the platform it runs on — that's a release-asset question, not a runtime one. Consumers who need to identify the build artifact check the binary name. Keeps the envelope minimal.
- **Keep a Changelog format with `Unreleased` section at the top?** Yes. Matches the convention and gives maintainers a staging area for each merged PR's note before the next tag. `release.sh` can later be extended to "move Unreleased → new version heading on tag" — out of scope here but worth naming.

### Deferred to Implementation

- **Whether to extract the version-string construction into a standalone helper (`buildVersionOutput() string`) that's callable outside the template.** Only matters if a future test wants to assert the JSON shape without invoking Cobra. Decide when writing Unit 2's test.
- **Exact CHANGELOG wording for historical releases.** The commit messages and tag annotations are the raw material; phrasing should be proportionate (1-2 lines per release, not a full retrospective). Inspect tag messages during Unit 1 and trust editorial judgment.
- **Exact commit-message rationale for the CHANGELOG check.** The decision itself (version-heading grep over Unreleased-body inspection) is resolved above in Key Technical Decisions; the commit message for Unit 3 should reference that decision and the heading-match workflow it assumes. No implementation-time ambiguity here — just a reminder to surface the rationale for future readers.

## Implementation Units

- [x] **Unit 1: Create `CHANGELOG.md` with backfilled entries**

**Goal:** Ship a Keep a Changelog-compatible `CHANGELOG.md` at the repo root with the three most recent releases plus a bundled tail for older tags. No code changes; doc-only.

**Requirements:** R1

**Dependencies:** None.

**Files:**
- Create: `CHANGELOG.md`

**Approach:**
- Keep a Changelog 1.1.0 structure: top-level `## [Unreleased]` section (empty), then `## [0.11.2] - 2026-04-08`, `## [0.11.1] - 2026-04-07`, `## [0.11.0] - 2026-04-06` with individual entries, then `## [0.10.2] - 2026-04-03 and earlier` as a bundled entry pointing to the GitHub Releases page for detail.
- Each release entry: 1-3 bullets under the relevant section heading (`### Changed`, `### Added`, etc.). Derive from `git tag -l --format='%(refname:short) %(contents:subject)'` + the top PRs per tag from `gh release list`.
- Include an explicit reference to the PR #63 breaking `--output json` envelope change under its actual release (the tag that included it — identify during implementation by `git tag --contains <PR-63-merge-sha>`).
- No backlinks, no version-comparison URLs (the `[version]: url` reference-style trailer from Keep a Changelog's example). Rationale: GitHub already provides compare links; duplicating adds churn without value.
- Add a single introductory paragraph under the top `# Changelog` heading clarifying: date format, semver adherence (mostly — we're pre-1.0 so breaking changes can ship in minor versions), and the policy that `--output json` shape changes are called out explicitly.

**Patterns to follow:**
- https://keepachangelog.com/en/1.1.0/ — canonical format.
- No existing `CHANGELOG.md` in the repo; this is the first.

**Test scenarios:**
- Test expectation: none — pure documentation file with no runtime behavior. Human-readable review in the PR is the quality gate.

**Verification:**
- `CHANGELOG.md` exists at repo root.
- `head -30 CHANGELOG.md` shows the `# Changelog` heading, the intro paragraph, and the `## [Unreleased]` stub.
- Each of the three explicitly-backfilled releases has at least one bullet.
- The PR #63 breaking envelope change is named in its release's entry under `### Changed`.

- [x] **Unit 2: Add JSON output to `andamio --version`**

**Goal:** Honor `--output json` on the `--version` flag by registering a Cobra template function that branches on `outputFormat`. Preserve existing text output when `--output` is absent or `text`.

**Requirements:** R2, R3

**Dependencies:** None.

**Files:**
- Modify: `cmd/andamio/main.go`
- Test: `cmd/andamio/main_test.go` (create — first test file in `cmd/andamio/` that exercises main's cobra wiring directly)

**Approach:**
- Replace the `Sprintf`-built template string at `main.go:41-44` with a template that calls a custom function:
  - Register the function via `cobra.AddTemplateFunc("versionOutput", ...)` before `Execute()` runs.
  - Function body: branch on format; on JSON, marshal `{version, commit, built: date}` via `json.Marshal` and return the string with a trailing newline. Otherwise return the current text format (`"andamio <v> (commit: <c>, built: <d>)\n"`).
- **Type detail for the format comparison.** `outputFormat` is a plain `string` bound by `StringVarP`. `output.FormatJSON` is a typed string constant (`type Format string` with `FormatJSON Format = "json"` — see `internal/output/output.go`). Go forbids direct comparison between a plain string variable and a named-type constant without conversion. Use an explicit cast: `output.Format(outputFormat) == output.FormatJSON`. (A raw `outputFormat == "json"` would compile but re-hardcodes the string literal the constant is there to remove; prefer the cast.)
- Use a small anonymous struct or `map[string]string` as the JSON payload. Struct is slightly cleaner (guarantees key ordering via field order).
- **Commit truncation guard.** `commit[:7]` panics when `commit` is shorter than 7 chars — the compile-time default `"none"` is 4. Extract a `shortCommit(commit string) string` helper that returns `commit` unchanged if `len(commit) < 7`, else `commit[:7]`, and call it from both the text and JSON paths. This is the explicit realization of the Risks-table mitigation; do not skip it.
- Preserve `rootCmd.Version` as `versionString()` so Cobra continues to recognize `--version` as a defined flag. The template function is what actually produces the output; `Version` just has to be non-empty.
- Extract the JSON marshaling into a package-level helper (e.g., `buildVersionJSON() string`) to make the test-access path trivial without having to drive Cobra. `shortCommit` lives alongside it.

**Patterns to follow:**
- `cmd/andamio/course.go` — existing `--output json` branches that use `output.GetFormat() == output.FormatJSON`. Same pattern, different surface.
- `internal/output/` — the canonical format constants and `PrintJSON`. For this surface we are NOT going through `output.PrintJSON` (that's for the global data stream); we're emitting directly because `--version` is its own output channel. Using the same `output.FormatJSON` constant keeps the check consistent.

**Test scenarios:**
- Happy path (text mode): call the extracted helper with `outputFormat = "text"`; assert return starts with `"andamio "` and contains the literal `(commit:`. Locks the human-readable format.
- Happy path (JSON mode): call the helper with `outputFormat = "json"`; assert return parses as valid JSON via `json.Unmarshal` into a `map[string]string`, has exactly the three keys `version`, `commit`, `built`, and that `version` equals the expected test value.
- Edge case: all three ldflag vars at their compile-time defaults (`version = "dev"`, `commit = "none"`, `date = "unknown"`). JSON mode still emits a well-formed object; no panic on `commit[:7]` when `commit = "none"` (verify truncation guard — `"none"[:7]` would panic since `"none"` is 4 chars; the existing `versionString()` at `main.go:21-26` has a `commit == "none"` early-return, but that's only the text path; the JSON builder needs its own guard OR a shared helper).
- Edge case: empty `outputFormat` (before `PersistentPreRunE` runs, or if someone calls the helper directly pre-parse). Treat as text mode — the default. Locked by a test case.
- Integration: invoke `./andamio --version --output json` via `exec.Command` in a test, assert stdout is valid JSON and exit code is 0. Proves the Cobra wiring (template function + flag parsing order) works end-to-end.
- Integration: invoke `./andamio --version` (no output flag), assert stdout matches the prior text format byte-for-byte (using a regex that permits the version/commit/date values to vary but fixes the skeleton). Locks R3's no-regression promise.
- Error path: none. `--version` has no error branches; JSON marshaling of a 3-string struct cannot fail in practice.

**Verification:**
- `go build ./... && ./andamio --version` prints the existing text.
- `./andamio --version --output json | jq -r '.version, .commit, .built'` prints three lines.
- `./andamio --version --output json | jq 'keys | length'` prints `3`.
- `go test ./cmd/andamio/...` passes (the new test file adds 4-6 cases; existing tests unchanged).

- [x] **Unit 3: Add CHANGELOG reminder to `scripts/release.sh`**

**Goal:** Add a preflight step that warns if `CHANGELOG.md` does not mention the version about to be released. Warning only — not a hard block — matching the script's existing `[y/N]` confirm UX.

**Requirements:** R4

**Dependencies:** Unit 1 (CHANGELOG.md must exist for the check to grep; until then the check would always warn).

**Files:**
- Modify: `scripts/release.sh`

**Approach:**
- Insert a new preflight check between the "Tag doesn't already exist" check (~line 60) and the "Build test" block (~line 69).
- Check 1: `CHANGELOG.md` exists at repo root. If not: print `✗ CHANGELOG.md missing — run Unit 1 of issue #67 to create it` and `exit 1`. This is a hard block only because the file's absence indicates something has regressed; Unit 1 is supposed to be merged before this preflight has anything to check against.
- Check 2: `grep -q "^## \[$VERSION\]" CHANGELOG.md`. If absent: print a warning with the expected heading format (`## [x.y.z] - YYYY-MM-DD`), then ask `Continue without a CHANGELOG entry for $VERSION? [y/N]`. Proceed only on `y`. Does NOT exit 1 — the maintainer may be doing a patch release with no user-facing change.
- Order matters: the check runs before the build-test and before the final "Proceed?" prompt. Keeps all preflight decisions grouped.

**Patterns to follow:**
- `scripts/release.sh:31-66` — the `echo "  → ..."` / `echo "  ✓ ..."` / `echo "  ✗ ..."` pattern + `exit 1` on hard-block failure.
- The existing interactive `read -p` pattern at `scripts/release.sh:83-87` — for the soft-warning path on the CHANGELOG check.

**Test scenarios:**
- Test expectation: the existing test suite does not cover `release.sh` (it's a shell script). Manual verification only. Run the following scenarios by hand or via a small shell harness:
  - Happy path: CHANGELOG.md contains `## [0.11.3] - 2026-XX-XX`; running `./scripts/release.sh 0.11.3` passes the new check silently (`✓ CHANGELOG entry found for 0.11.3`).
  - Warning path: CHANGELOG.md exists but has no entry for the target version; script prints the warning and prompts for confirmation. Typing `n` exits without tagging. Typing `y` proceeds to build-test.
  - Hard-block path: delete `CHANGELOG.md` temporarily; script exits 1 with the missing-file message. (Do not actually delete committed CHANGELOG in a test — stage the deletion in a dirty tree, which the existing "clean tree" preflight will catch first anyway. The hard-block path is reachable only if someone manually removes the file; acceptable.)

**Verification:**
- `bash -n scripts/release.sh` is syntactically clean (no parse errors).
- Dry-run path with the CHANGELOG entry present proceeds to the existing build-test block unchanged.
- Dry-run path without the CHANGELOG entry prints the warning and waits for confirmation.

## System-Wide Impact

- **Interaction graph:** `main.go` gains one new template function registered on `rootCmd` before `Execute()`. No other Cobra commands are affected. `scripts/release.sh` gains one preflight check; GoReleaser (the CI release mechanism) is unchanged — the script tags and GH Actions does the rest.
- **Error propagation:** The JSON branch has no error surface (a 3-string `json.Marshal` cannot fail in practice). The CHANGELOG check in `release.sh` either exits 1 (missing file), prompts (missing entry), or silently passes. All three are pre-tag, so they cannot corrupt a release in progress.
- **State lifecycle risks:** None. No persistent state.
- **API surface parity:** `andamio --version` is the only version-emission surface. No `version` subcommand exists or is added. The human-readable text format is preserved byte-for-byte (locked by a test); the JSON format is additive.
- **Integration coverage:** The end-to-end `exec.Command` tests in Unit 2 prove that flag parsing + template rendering + output flag detection compose correctly. Without those, unit tests on `buildVersionJSON` would pass but the Cobra wiring could be broken.
- **Unchanged invariants:**
  - `andamio --version` with no `--output` flag continues to print the existing text unchanged.
  - All exit codes (0/1/2/3 per CLAUDE.md) are unchanged.
  - `./scripts/release.sh` continues to auto-bump patch version from the latest tag when no argument is given.
  - `release.sh` preflight still exits 1 on unclean tree / wrong branch / unsynced with remote / existing tag.

## Risks & Dependencies

| Risk | Mitigation |
|------|------------|
| Cobra's `--version` flag bypasses `PersistentPreRunE`, leaving `output.SetFormat()` un-called and `output.GetFormat()` returning the zero value. | The template function reads `outputFormat` (the package-level var bound by `StringVarP`) directly, not `output.GetFormat()`. `StringVarP` populates `outputFormat` during flag parsing, which runs before the version template renders. Verified by the integration test in Unit 2 that calls `./andamio --version --output json` via `exec.Command`. |
| `commit[:7]` panics when `commit` is shorter than 7 chars (e.g., the compile-time default `"none"` is 4). | Extract a shared `shortCommit(commit)` helper that returns `commit` as-is if `len(commit) < 7`, else `commit[:7]`. Use it in both the text and JSON paths. Covered by the edge-case test "all ldflag vars at defaults." |
| CHANGELOG backfill includes incorrect details for older releases because commit archaeology is lossy. | Scope R1 to "at least the last 3 releases" per issue #67. Bundle earlier releases as a single entry pointing to GitHub Releases for authoritative detail. Err on the side of short, factual bullets over long retrospective prose. |
| `release.sh`'s grep-based CHANGELOG check false-positives when the target version appears in commentary rather than a heading (e.g., `"Fixed regression from 0.11.3 in previous release"`). | Anchor the grep pattern to Keep a Changelog's heading format: `grep -q "^## \[$VERSION\]" CHANGELOG.md`. The `^## [` anchor rules out prose mentions. |
| A patch release with genuinely no user-facing change gets blocked by the CHANGELOG check, forcing the maintainer to add a no-op entry. | The check is a warning + interactive confirm, not a hard exit. Maintainer can type `y` to proceed without an entry. Covers the "docs typo" / "rebuild-only" release case. |

## Documentation / Operational Notes

- `CLAUDE.md` (project) — two specific edits:
  - **Global Flags section** — append after the existing `--version` line: `` `--version --output json` emits `{version, commit, built}` as JSON; the default text format is preserved when `--output` is absent or `text`. ``
  - **Release section** — append: `` `CHANGELOG.md` at the repo root is the source of truth for user-facing release notes. `./scripts/release.sh`'s preflight warns if the target version has no heading in `CHANGELOG.md`. ``
- `docs/TX-LIFECYCLE.md`, `docs/COURSE-LIFECYCLE.md`, `docs/PROJECT-LIFECYCLE.md` — no changes.
- No monitoring / alerting changes.
- Release note for the next tag (when this PR merges): should land in the CHANGELOG's `## [Unreleased]` section as part of the same PR, moved to a versioned heading by the next `./scripts/release.sh` run.

## Sources & References

- **Origin issue:** https://github.com/Andamio-Platform/andamio-cli/issues/67
- **Triggering PR:** https://github.com/Andamio-Platform/andamio-cli/pull/63 (review finding P1 #2 that motivated filing #67)
- **Related:** issue #66 (typed envelope structs) — may introduce `spec_version` signals that extend the `--version` JSON envelope; captured as "revisit" in Open Questions.
- **Existing release workflow:** `scripts/release.sh`
- **Existing version surface:** `cmd/andamio/main.go:14-26, 41-45`
- **Keep a Changelog 1.1.0:** https://keepachangelog.com/en/1.1.0/
