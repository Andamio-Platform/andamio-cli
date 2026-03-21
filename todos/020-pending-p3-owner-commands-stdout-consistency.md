---
status: complete
priority: p3
issue_id: "020"
tags: [code-review, agent-native, composability]
dependencies: []
---

# Make `update` and `register` Emit project_id to stdout in Text Mode

## Problem Statement

`project owner create` prints `project_id: <id>` to stdout on success (line 158), making it pipeable. But `project owner update` and `project owner register` emit nothing to stdout in text mode — only stderr messages. This is inconsistent across the three mutating commands.

For `--output json` mode this doesn't matter (full response is returned), but text-mode consistency improves quick shell scripting without requiring `jq`.

## Findings

- `project_owner.go:157-159` — `create` prints project_id to stdout
- `project_owner.go:217` — `update` only prints to stderr
- `project_owner.go:270` — `register` only prints to stderr
- The composable pattern in CLAUDE.md recommends `--output json | jq` for scripting, so this is a UX enhancement, not a blocker

## Proposed Solutions

### Option A: Print project_id to stdout for all three commands
After success, print `project_id: <id>` to stdout if the API response contains it.
- **Pros**: Consistent pattern, enables simple shell piping
- **Cons**: Depends on API response shape
- **Effort**: Small
- **Risk**: None

## Acceptance Criteria

- [ ] `update` and `register` print project_id to stdout in text mode (if available in response)
- [ ] `--output json` behavior unchanged

## Work Log

| Date | Action | Learnings |
|------|--------|-----------|
| 2026-03-21 | Created from PR #33 agent-native + architecture review | Mutating commands should emit key identifiers to stdout for composability |

## Resources

- PR #33: https://github.com/Andamio-Platform/andamio-cli/pull/33
