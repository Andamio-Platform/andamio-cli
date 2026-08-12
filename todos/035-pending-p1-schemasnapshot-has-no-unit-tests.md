---
status: pending
priority: p1
issue_id: "035"
tags: [ci, breaking-change-gate, schemasnapshot, testing]
dependencies: []
---

# schemasnapshot has zero unit tests — the gate is only proven against itself

## Problem

`internal/schemasnapshot/schemasnapshot.go` is 170 lines of AST walking, tag
parsing, and sorting. Its only exercise today is `surface_test.go`'s golden
comparison — which compares the scanner's output to a golden that the same
scanner generated. A bug in the scanner doesn't fail that test; it produces
a quietly wrong golden that gets rubber-stamped the first time someone runs
`-update`. That's the exact failure mode the surface gate (PR #141) exists
to prevent.

[[todos/033]] and [[todos/034]] are the existence proof: both are real
scanner blind spots that fixture-level tests would have caught at authoring
time, not three review rounds later.

Also flagged in the same review: the embedded-field branch in
`schemasnapshot.go` has provably never executed — no line in the current
golden has `field name == type`, which is what that branch produces.

## Proposed Solution

Table-driven fixture tests directly against `schemasnapshot.Generate`,
covering:

- a function-local tagged struct
- an embedded field, both exported and unexported
- a nested anonymous struct
- a file that fails to parse
- a missing source directory
- a field tagged `json:"-"`
- two structs with the same name in different files (tests the sort-key
  collision risk noted in PR #141's round-2 review)

## Notes

- Doing the `Generate` API signature change proposed alongside this (return
  `([]byte, error)` instead of writing straight to a temp file) first makes
  these fixtures much easier to write — no filesystem round-trip needed per
  case. Small refactor, do it before writing the table.
- This todo is what makes [[todos/033]] and [[todos/034]]'s fixes actually
  trustworthy going forward, not just one-off patches.
