---
run_id: 2026-04-22-pr69-release-hygiene
mode: autofix
pr: 69
base: 4d2d4aa410f0f1c691af4181c4b614005b48d649
plan: docs/plans/2026-04-22-002-feat-release-hygiene-changelog-version-json-plan.md
reviewers: correctness, testing, maintainability, project-standards, agent-native, learnings-researcher, api-contract, adversarial
---

# ce:review autofix run — PR #69 (release hygiene)

## Verdict

**Ready with follow-ups.** 5 `safe_auto` fixes applied; 4 `gated_auto` residuals filed as todos. No P0 findings. Reviews converged with strong cross-persona signal — the most serious finding was a factual error in the CHANGELOG backfill that would have shipped a misattribution.

## Applied safe_auto fixes

| # | File | Reviewer | Fix |
|---|---|---|---|
| 1 | `CHANGELOG.md` | adversarial | Corrected factual error: compute-hash feature landed in v0.10.2 (commit `5a2fccd`, `git tag --contains` verified), not v0.11.0. Moved bullets to new `## [0.10.2]` section; updated v0.11.0 to list only what actually shipped (tables, draft-before-mint, task_hash fix); updated "Earlier releases" summary to remove double-count. |
| 2 | `CHANGELOG.md` Unreleased | api-contract + correctness | Documented the `commit` 7-char truncation contract and the `built` timestamp format explicitly. Also corrected the misleading "Plain-text `--version` output is unchanged" claim — goreleaser builds previously showed the full 40-char SHA; post-PR they show 7 chars. |
| 3 | `scripts/release.sh:74` | correctness + adversarial (cross-reviewer, merged 1.00) | Fixed unescaped dots in grep pattern. Was `grep -q "^## \[${VERSION}\]"` — dots in `${VERSION}` (e.g., `0.11.0`) are regex wildcards, matching `0X11X0` or similar headings. Now `grep -qF "## [${VERSION}]"` (fixed-string match). |
| 4 | `cmd/andamio/main_test.go` | testing (2 findings merged) | Integration tests now assert exact ldflag-injected values (`version="test"`, `commit="1234567"`, `built="test-date"`) not just JSON validity. Extracted `assertVersionJSONEnvelope` helper used by both `TestVersionFlag_JSONMode_Integration` and `TestVersionFlag_JSONMode_ShortFlagOrder`. Closes the false-confidence gap where ldflag→JSON wiring could break silently. |
| 5 | `cmd/andamio/main.go` `rootCmd.Long` | agent-native | Added JSON envelope schema note (`{"version":"<x>","commit":"<sha7>","built":"<timestamp>"}`) so agents running `andamio --help` can discover the capability from the binary without reading CHANGELOG.md. |

`go test ./...` green. `go vet ./...` clean. `go build ./...` clean.

## Residual actionable (gated_auto → downstream-resolver)

### [P2] `--output` value validation silently bypassed on `--version` path (cross-reviewer consensus, merged 1.00)

**Reviewers:** correctness (0.95) + testing (0.82) + agent-native + api-contract (0.85)

**File:** `cmd/andamio/main.go:43-58`

Case-sensitive `output.Format(outputFormat) == output.FormatJSON` diverges from `output.SetFormat`'s `strings.ToLower` normalization. Every other command in the CLI rejects `--output BOGUS` with exit 1; `--version` silently falls through to text output with exit 0. Specific symptoms:

- `andamio --version --output JSON` (uppercase) → text output (wrong)
- `andamio --version --output xml` → text output, exit 0 (wrong — should error like other commands)
- `andamio --version --output csv/markdown` → text output, exit 0 (undocumented fallback)

Filed as todo #024.

### [P3] Text `--version` regressed from full-SHA → 7-char on goreleaser builds (cross-reviewer consensus)

**Reviewers:** correctness (0.85) + api-contract

**File:** `cmd/andamio/main.go:57` + `CHANGELOG.md`

Pre-PR text output used the full 40-char commit SHA via `fmt.Sprintf("commit: %s", commit)`; post-PR uses `shortCommit(commit)` which truncates to 7. The CHANGELOG's (now-corrected) original claim "Plain-text `--version` output is unchanged" was wrong for goreleaser builds. The autofix corrected the CHANGELOG; the open question is whether to restore full-SHA in text mode or accept the regression.

Filed as todo #025.

### [P2] `release.sh` preflight greenlights release when `Unreleased` has content but no versioned heading

**Reviewer:** adversarial (0.85)

**File:** `scripts/release.sh:68-87`

Current preflight only checks for `## [$VERSION]` heading. If maintainer forgets to promote `Unreleased` → new versioned heading, script warns "no entry for $VERSION" and accepts confirmation — but the accumulated breaking-change entries under `Unreleased` silently ship without being attributed to the release. Plan explicitly tried to avoid this failure mode but the implementation doesn't catch it.

Filed as todo #026.

### [P3] `release.sh` `read -p` + `set -e` silences the "Cancelled" message on EOF/non-TTY

**Reviewer:** correctness (0.90)

**File:** `scripts/release.sh:81`

When stdin is closed (piped, CI, non-TTY), `read -p "... [y/N]" -n 1` fails under `set -euo pipefail` and the script exits before printing the "Cancelled" diagnostic. Safe-fail (release not tagged), but operator sees no explanation. Same pattern exists at the final Proceed prompt (line ~102) — pre-existing, not introduced by this PR.

Filed as todo #027.

## Advisory (human)

### [P3] TraverseChildren persistent-flag double-parse caveat
**Reviewer:** adversarial (0.70). Cobra parses persistent flags on root during Traverse, then again on the subcommand's merged FlagSet. Safe today (only `--output` is persistent, `StringVarP` is idempotent) but future side-effectful flags (StringArray, counting Bool) would double-apply. Add a one-line caveat to the existing TraverseChildren comment.

### [P3] Missing CI enforcement for CHANGELOG updates
**Reviewer:** adversarial (0.80). `release.sh` preflight runs at tag time, after PRs merge. Without a PR-time check, CHANGELOG will drift. Consider a GitHub Actions check that fails when CHANGELOG.md isn't modified (with `no-changelog` label override for infra/test/doc-only PRs).

### [P3] `versionString()` / `buildVersionOutput()` divergence
**Reviewers:** correctness (0.70) + maintainability (0.78), merged 0.88. Two functions now produce version text with different semantics for `commit == "none"`. `versionString()` is effectively dead because the template override uses `buildVersionOutput()` — consider deleting `versionString()` and assigning a static sentinel to `rootCmd.Version`.

### [P3] main.go growing a version-rendering mini-module
**Reviewer:** maintainability (0.55). Currently below 0.60 confidence gate but noting for future: consider splitting `shortCommit`/`versionString`/`buildVersionOutput`/version package vars into `cmd/andamio/version.go` + `version_test.go`. Matches the file-per-concern pattern used elsewhere.

### [P3] Envelope keys naming convention divergence
**Reviewer:** api-contract (0.65) + learnings-researcher. `--version` uses flat `{version, commit, built}`; `register-module` (PR #63) uses wrapped `{action, status, slt_hash, advanced_from, response}`. No CLI-wide envelope convention documented. Worth addressing as the envelope pattern proliferates — see issue #66 (typed envelope structs).

## Learnings

- **`docs/solutions/architecture/cli-composability-audit-and-fix.md`** blesses this PR's approach. The `--version` JSON envelope is the natural extension of the Prevention Strategy #3 ("--output json is the scripting surface with stable schemas") to the root-level capability-detection flag. learnings-researcher suggested a new solution doc titled something like "Version JSON surface, CHANGELOG discipline, and Cobra flag traversal" covering the three patterns. Advisory — not required.
- **TraverseChildren = true** is the first documented instance of this Cobra pattern in the repo. Worth a short note in `docs/solutions/architecture/` for the next persistent-flag addition.
- **The PR introduces `{version, commit, built}` as a new agent-detectable capability signal.** Prior solution docs establish the `--output json` stability contract; this PR creates the first example of a capability signal separate from a command result. Pattern-worthy.

## Requirements completeness (plan: explicit)

| # | Requirement | Status |
|---|---|---|
| R1 | CHANGELOG.md at repo root, Keep a Changelog format, 3+ releases backfilled | met (plus one factual correction applied via autofix) |
| R2 | `andamio --version --output json` emits `{version, commit, built}` | met — integration tests now assert exact ldflag values end-to-end |
| R3 | Plain-text `--version` output preserved | partially met — format shape preserved but commit truncation changed on goreleaser builds; documented in updated CHANGELOG. See todo #025 for the design decision. |
| R4 | `scripts/release.sh` CHANGELOG preflight | met — plus grep-escape fix applied via autofix |

All 4 requirements met in implementation. R3 has a documented contract divergence (now corrected in the CHANGELOG) that the human owner should review — filed as todo #025.
