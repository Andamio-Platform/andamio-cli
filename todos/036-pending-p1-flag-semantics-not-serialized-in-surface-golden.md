---
status: pending
priority: p1
issue_id: "036"
tags: [ci, breaking-change-gate, surface, cobra, flags]
dependencies: []
---

# Flag semantics (required, default, hidden, deprecated) aren't captured by the surface gate

## Problem

`cmd/andamio/surface.go`'s flag line format is `name | shorthand | type`
only. Removing a `MarkFlagRequired` call (35+ call sites in the CLI today)
or changing a flag's default value silently changes the contract for every
script relying on the CLI rejecting a malformed call — and the golden
doesn't move, so PR #141's gate reports no breaking change.

This was independently re-confirmed in the 10 Aug 2026 deep review via the
same mutation test used in PR #141's own round-2 review (drop a
`MarkFlagRequired` call, gate stays green).

Related, same root cause: `surface.go` also never serializes `cmd.Use` or
`Args` validators per command, so e.g. `RangeArgs(1,2)` → `ExactArgs(2)`
(live example: `cmd/andamio/course_export.go:43`) also merges green today.

## Proposed Solution

Widen the flag line to include `default | required | hidden | deprecated`.
Required-ness reads off the `cobra.BashCompOneRequiredFlag` annotation
cobra sets internally — no new API needed, just read the existing
annotation when building the line.

Fold in the `Args`/`Use` gap from the same review pass while touching this
code: serialize `cmd.Use` per command so arg-count/shape changes are
covered too.

## Notes

- One `-update` regen covers both fixes if done together.
- Two more related, lower-priority findings from the same review worth a
  glance when this is picked up (not required for this todo, just adjacent):
  execute-time surface not in the snapshot (`CompletionOptions
  .DisableDefaultCmd = true` would break shell completion for every user
  with zero gate coverage — fix is calling
  `InitDefaultHelpFlag`/`InitDefaultVersionFlag`/`InitDefaultHelpCmd`/
  `InitDefaultCompletionCmd` before `CommandSurface(rootCmd)`), and
  `schemaSrcDirs` being a hand-maintained list with no meta-test to catch a
  new JSON-emitting package silently going unscanned (the same class of bug
  as [[todos/034]]).
