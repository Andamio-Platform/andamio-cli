---
status: pending
priority: p3
issue_id: "010"
tags: [code-review, validation, task-commands]
dependencies: []
---

# `validateLovelace` Has No Upper Bound Check

## Problem Statement

`validateLovelace` validates that the lovelace input is a non-negative integer, but places no upper bound. A user could accidentally specify `9223372036854775807` (max int64, ~9.2 quadrillion ADA — far beyond the ~45 billion ADA total supply). The validated result is also discarded; the raw string is passed directly to the API.

This is a defence-in-depth gap — the API provides real protection — but could prevent accidental on-chain mistakes.

## Findings

- `project_task.go:340–348`: `validateLovelace` parses but doesn't bound-check
- `project_task.go:497`: raw `lovelace` string forwarded to API payload, parsed `int64` discarded
- Total ADA supply in lovelace: ~45,000,000,000,000,000 (45 quadrillion)

## Proposed Solution

Add an upper-bound check in `validateLovelace`:
```go
const maxLovelace = int64(45_000_000_000_000_000) // ~total ADA supply
if val > maxLovelace {
    return fmt.Errorf("--lovelace value %d exceeds total ADA supply (%d); did you mean a smaller amount?", val, maxLovelace)
}
```

This catches obvious mistakes (e.g., confusing ADA with lovelace: 5 ADA → 5000000 lovelace, not 5).

## Acceptance Criteria

- [ ] `validateLovelace("9999999999999999999")` returns an error about the upper bound
- [ ] `validateLovelace("5000000")` continues to pass
- [ ] Error message is clear about the ADA/lovelace distinction

## Work Log

- 2026-03-18: Identified via security review of feat/project-task-commands branch
