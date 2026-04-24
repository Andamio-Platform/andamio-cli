---
status: ready
priority: p2
issue_id: "026"
tags: [release, changelog, preflight, pr-69-followup]
dependencies: []
---

# `release.sh` preflight misses the "Unreleased has content but not promoted" case

## Problem Statement

`scripts/release.sh`'s CHANGELOG preflight (added in PR #69) grep-checks for `## [$VERSION]` heading. If the heading is missing, it prompts `Continue without a CHANGELOG entry for $VERSION? [y/N]`.

**The scenario the check misses:** Maintainer accumulates entries under `## [Unreleased]` during the release cycle, then forgets to rename the heading to `## [0.12.0] - 2026-04-22` at release time. The preflight fires the "no entry" warning — maintainer, seeing full `Unreleased` bullets, thinks "I have entries, they're just under Unreleased, good enough" and presses `y`.

**Consequences:**
1. Tagged release ships with `## [Unreleased]` still containing the release's changes.
2. Next release cycle adds more bullets to the *same* `Unreleased` block, silently conflating two releases.
3. Users/agents reading the tagged CHANGELOG cannot tell which release shipped which change.
4. **Breaking-change callouts get attributed to the wrong version.** In the current Unreleased block, there's an explicit breaking-change note for `register-module --output json` — if this ships unpromoted, consumers won't know which version to pin against.

This is exactly the failure mode the plan (`docs/plans/2026-04-22-002-feat-release-hygiene-changelog-version-json-plan.md`) tried to avoid, but the heading-match check alone doesn't catch it.

Found during ce:review autofix of PR #69 (adversarial finding, confidence 0.85).

## Affected Files

- `scripts/release.sh:68-87` — the CHANGELOG preflight block

## Proposed Fix

Augment the preflight: when `## [$VERSION]` is missing, inspect `## [Unreleased]` for non-empty content. If Unreleased has body content (bullets), fail with a non-bypassable message; if Unreleased is empty, fall through to the current interactive prompt.

Sketch (not prescriptive):

```bash
# After the existing heading check fails:
UNRELEASED_BODY=$(awk '/^## \[Unreleased\]/{flag=1; next} /^## \[/{flag=0} flag' CHANGELOG.md)
if echo "$UNRELEASED_BODY" | grep -qE '^\s*-'; then
  echo "  ✗ '## [Unreleased]' has entries but no '## [$VERSION]' heading"
  echo "    Promote Unreleased content to a new versioned heading before tagging:"
  echo "      ## [$VERSION] - $(date +%Y-%m-%d)"
  exit 1
fi
# Unreleased is empty — allow the maintainer to proceed with the existing prompt
read -p "    Continue without a CHANGELOG entry for $VERSION? [y/N] " ...
```

Key design choices:
- **Hard block when Unreleased has content**, not a soft warning. The "I have entries under Unreleased" false-positive is precisely what this check exists to catch; a bypass prompt here defeats the purpose.
- **Soft prompt only when Unreleased is genuinely empty.** A patch release with no user-facing changes has nothing to promote — the existing warn+confirm UX is correct.
- **Reuse the `## [$VERSION]` check** (already in place) — if the maintainer DID promote correctly, both checks pass and the script proceeds silently.

## Acceptance

- [ ] Preflight detects non-empty `## [Unreleased]` when no versioned heading matches.
- [ ] Hard-block path exits 1 with a clear "promote Unreleased first" message.
- [ ] Soft-prompt path (empty Unreleased) continues to work as today.
- [ ] Manual test: (a) CHANGELOG with proper `[0.12.0]` heading → passes silently; (b) CHANGELOG with populated Unreleased + no versioned heading → hard-exits with 1; (c) CHANGELOG with empty Unreleased + no versioned heading → prompts as today.
- [ ] Update CHANGELOG entry for this fix under the appropriate section when it lands.

## Context

- **ce:review run artifact:** `.context/compound-engineering/ce-review/2026-04-22-pr69-release-hygiene/findings.md`
- **Origin PR:** https://github.com/Andamio-Platform/andamio-cli/pull/69
- **Related plan:** `docs/plans/2026-04-22-002-feat-release-hygiene-changelog-version-json-plan.md` — deferred question about Unreleased-body inspection was resolved in favor of heading-match simplicity; this todo revisits that decision based on the real failure mode.
