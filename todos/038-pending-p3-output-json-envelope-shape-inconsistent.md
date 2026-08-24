---
status: pending
priority: p3
issue_id: "038"
tags: [json, api-contract, composability]
dependencies: []
---

# `--output json` envelope shape is inconsistent across list commands

## Problem

`CLAUDE.md` states that `--output json` is "the scripting surface" with "stable
JSON schemas". Measured against the live preprod gateway during the #152 sweep,
three different normalisation policies are in play:

| Command | Gateway returns | CLI emits | Policy |
|---|---|---|---|
| `course list` | `{data: [...]}` | bare `array` | unwraps |
| `project list` | `{data: [...]}` | bare `array` | unwraps |
| `token list` | bare `array` | `{data: ...}` | wraps |
| `tx types` | `{types: ...}` | `{types: ...}` | passes through |
| the 11 `getJSON` routes | varies | unchanged | passes through |

Each is individually as-designed — the "List with formatting" pattern extracts
`data`, and `token list` is documented as tolerating both shapes. The problem is
that the designs disagree with each other. A script consuming two commands needs
two different shapes from one flag:

```bash
andamio course list --output json | jq '.[]'        # bare array
andamio token list  --output json | jq '.data[]'    # enveloped
```

There is no documented rule saying which precedent a new list command should
follow, so the next one added will pick one of three by coin flip.

## Why this is not urgent

Fixing it means changing the emitted shape of at least one already-shipped
command, which is itself a breaking change to the scripting surface — the exact
class of change `CHANGELOG.md` says must be called out explicitly. It deserves
its own scoped PR with a deprecation note, not a drive-by normalisation.

## Proposed Solution

1. Decide the house rule (enveloped `{data: ...}` is the better default — it
   leaves room for pagination metadata, which 2.5 introduces).
2. Write it down in `CLAUDE.md` next to the Composability Rules.
3. Migrate the outliers in one release, with a `CHANGELOG` breaking-change entry.

Step 2 is worth doing on its own even if the migration never happens: today a
contributor has no way to know which precedent is intended.

## Found

During the #152 preprod validation sweep (2026-08-24). Recorded in
`docs/validation/2026-08-preprod-v2.5.0-rc5.md` and corroborated by the
learnings researcher, which confirmed no existing `docs/solutions/` entry
covers cross-command envelope consistency.
