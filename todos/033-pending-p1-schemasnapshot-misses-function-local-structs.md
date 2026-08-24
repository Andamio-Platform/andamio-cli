---
status: pending
priority: p1
issue_id: "033"
tags: [ci, breaking-change-gate, schemasnapshot, json]
dependencies: []
---

# schemasnapshot misses function-local JSON struct literals

## Problem

`internal/schemasnapshot/schemasnapshot.go`'s `parseFiles` only walks
`parsedFile.Decls` — top-level declarations. A `type X struct{...}` declared
*inside* a function body is never visited, so it's absent from
`testdata/golden/schema.golden` and its json tags can be renamed with zero
detection from the surface gate (PR #141).

Confirmed today (12 Aug 2026) still uncovered: five real `--output json`
envelopes defined as function-local structs —

- `auth status` (`cmd/andamio/auth.go:51`)
- `config show` (`cmd/andamio/config.go:161`)
- `verify-hash` result, twice (`cmd/andamio/course_credential.go:93`,
  `cmd/andamio/project_task.go:714`)
- the documented `--version` payload (`cmd/andamio/main.go:51`)

This is the largest coverage hole found in the 10 Aug 2026 deep review of
PR #141 (7-pass multi-agent review + validation + adversarial refutation;
this finding survived all of it).

## Proposed Solution

Walk with `ast.Inspect` instead of iterating `Decls` directly, so type
declarations at any nesting depth are collected. This also incidentally
helps with nested anonymous-struct field visibility (a separate, lower
priority finding — see if it's covered by the same change).

Alternative considered and rejected as a standalone fix: hoist the five
structs above to package level. Doesn't scale — the next function-local
struct someone writes reintroduces the same hole silently.

## Notes

- Regenerate `testdata/golden/schema.golden` after the fix
  (`go test ./cmd/andamio -run TestSchemaSurfaceGolden -update`) and check
  the diff by hand once — this is exactly the kind of change worth eyeballing
  rather than blindly trusting.
- Pairs naturally with [[todos/035]] (schemasnapshot has no unit tests of its
  own) — a function-local-struct fixture is one of the table-driven cases
  that todo asks for, and would have caught this at authoring time.
