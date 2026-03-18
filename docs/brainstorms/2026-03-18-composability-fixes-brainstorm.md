---
date: 2026-03-18
topic: composability-fixes
---

# CLI Composability Fixes

## What We're Building

A focused pass to close the gaps between the CLI's stated composability goal (fully scriptable, pipe-friendly, usable by agents and CI/CD) and the current implementation. The foundation is solid — no interactive prompts, `--output json` on data commands, `isJSON` gating — but a set of concrete violations undermine trust in the scripting contract.

## Why This Approach

Two bigger alternatives were considered:

- **New scripting features only** — skipped because you can't build on a leaky foundation
- **Full composability audit + new features** — deferred; fixes first, features later

The "fix the bugs first" approach is right because it closes known violations cheaply before adding new surface area.

## Key Decisions

- **JSON on status/config commands**: `auth status`, `user status`, `config show` will gain `--output json` support. These are the entry points for scripts that need to check auth state before doing anything else.
- **Empty-list messages to stderr**: `printList`'s "No X found" message will move from stdout to stderr. Stdout must only carry structured data.
- **stdout leak fixed**: `course_create_module.go:90` `fmt.Printf` → `fmt.Fprintf(os.Stderr)`.
- **`spec paths` gets JSON mode**: Returns an array of endpoint objects so scripts can programmatically inspect available paths.
- **Exit code conventions**: Define a small, stable set:
  - `0` — success
  - `1` — generic error (network, server, unexpected)
  - `2` — not found (resource doesn't exist)
  - `3` — auth required (no API key or JWT)
- **JSON error envelope**: `{"error": "message"}` on stdout when `--output json` is set and a command fails. Scripts check for the `error` key; exit code confirms failure.

## Scripting Contract (post-fix)

```bash
# Check auth before proceeding
if ! andamio user status --output json | jq -e '.logged_in' > /dev/null; then
  echo "Not authenticated" && exit 3
fi

# Safe empty-list handling
TASKS=$(andamio project task list "$PROJECT_ID" --output json)
COUNT=$(echo "$TASKS" | jq '.data | length')
# stdout is clean JSON even when list is empty — no "No tasks found" noise

# Exit code branching
if andamio user exists "$ALIAS" --output json > /dev/null; then
  echo "alias taken"  # exit 0
else
  echo "alias available"  # exit 2
fi
```

## Resolved Questions

- **`spec paths` JSON detail**: path + method + summary only. Full schema lives in `openapi.json`.
- **Exit code rollout**: Change globally. The old 0/1 behavior was never a documented contract.

## Next Steps

→ `/workflows:plan` for implementation details
