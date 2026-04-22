---
status: pending
priority: p3
issue_id: "027"
tags: [release, shell-scripting, error-handling, pr-69-followup]
dependencies: []
---

# `release.sh` `read -p` + `set -e` silences the "Cancelled" message on EOF/non-TTY

## Problem Statement

With `set -euo pipefail` at the top of `release.sh`, `read -p "... [y/N]" -n 1 -r` returns non-zero when stdin is closed (piped, redirected from `/dev/null`, CI without TTY). The non-zero return triggers `set -e` and the script exits with code 1 **before** printing the diagnostic "Cancelled." message.

Reproduction:
```bash
bash -c 'set -euo pipefail; read -p "q: " -n 1 -r </dev/null; echo after; echo "Cancelled"'
# prints nothing, exits 1
```

Net effect is safe-fail (release doesn't tag), but the operator piping the script sees no diagnostic explaining why it exited.

**This pattern is pre-existing** — the final `Proceed? [y/N]` prompt at `scripts/release.sh:81-87` has the same issue. This PR's new CHANGELOG-missing prompt inherits the pattern. Not a regression introduced by PR #69 per se, but worth fixing both prompts together.

Found during ce:review autofix of PR #69 (correctness finding, confidence 0.90).

## Affected Files

- `scripts/release.sh:81` — new CHANGELOG-confirm prompt added in PR #69
- `scripts/release.sh:~102` — pre-existing `Proceed? [y/N]` prompt (same pattern)

## Proposed Fix

Three options, all mechanical:

### Option A (guard with `|| true`)

```bash
read -p "    Continue without a CHANGELOG entry for $VERSION? [y/N] " -n 1 -r || true
echo ""
if [[ ! "${REPLY:-}" =~ ^[Yy]$ ]]; then
  echo "    Cancelled."
  exit 1
fi
```

`|| true` swallows the non-zero from `read`, letting execution continue to the `echo "Cancelled"` line. Requires `${REPLY:-}` (bash parameter expansion) to handle the empty case under `set -u`.

### Option B (explicit TTY check upfront)

```bash
if ! [ -t 0 ]; then
  echo "    ✗ No TTY available for interactive confirmation. Run interactively to bypass."
  exit 1
fi
read -p "..." -n 1 -r
...
```

Fails fast with a clear message when called non-interactively. Better UX for CI wrappers.

### Option C (`if ! read ...; then fail ...; fi`)

```bash
if ! read -p "    Continue ...? [y/N] " -n 1 -r; then
  echo ""
  echo "    Cancelled (stdin closed)."
  exit 1
fi
```

Handles the EOF-as-"no" case explicitly without masking other `read` failures.

**Preferred:** Option B — explicit TTY check upfront. Matches the script's "preflight" style, fails fast, and gives operators a clear reason. Apply to both the new CHANGELOG prompt AND the pre-existing `Proceed?` prompt so the script has a single consistent UX.

## Acceptance

- [ ] Both `read -p` prompts in `release.sh` handle EOF/non-TTY without silently dropping diagnostics.
- [ ] Manual test: `./scripts/release.sh 0.12.0 </dev/null` produces a clear error message and exits 1.
- [ ] Manual test: `./scripts/release.sh 0.12.0 < <(echo y)` (piped `y`) behaves as expected (either proceeds or fails with a clear message, depending on chosen option).
- [ ] `bash -n scripts/release.sh` remains syntactically clean.

## Context

- **ce:review run artifact:** `.context/compound-engineering/ce-review/2026-04-22-pr69-release-hygiene/findings.md`
- **Origin PR:** https://github.com/Andamio-Platform/andamio-cli/pull/69
- **Note:** Pre-existing pattern — not strictly introduced by this PR, but surfaced during its review.
