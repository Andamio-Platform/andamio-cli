---
status: pending
priority: p1
issue_id: "034"
tags: [ci, breaking-change-gate, schemasnapshot, cardano, tx-sign]
dependencies: []
---

# internal/cardano is not in schemaSrcDirs — tx sign's JSON contract is unguarded

## Problem

`cardano.SignResult` (`signed_tx`/`tx_hash`) is printed verbatim by
`tx sign --output json` — the financial signing surface of the CLI — and
`internal/cardano` isn't in `cmd/andamio/surface_test.go`'s `schemaSrcDirs`,
so no line in `testdata/golden/schema.golden` covers it.

Unlike `internal/apierr`/`output`/`client`, this omission is **not** named in
`surface_test.go`'s exclusion comment (which explains why those three are
deliberately skipped). So `tx sign`'s JSON output currently reads as covered
by the surface gate when it isn't. Found independently by two reviewers in
the 10 Aug 2026 deep review of PR #141.

## Proposed Solution

Add `"../../internal/cardano"` to `schemaSrcDirs` in `surface_test.go` and
regenerate the golden. While in there, audit `internal/submit` too — same
class of risk, not yet checked.

Related, lower-priority finding to fold into the same pass: `Assets
[]cardano.NativeAsset` (json tag `assets,omitempty`) carries an untyped
`cardano.NativeAsset` — worth tagging that struct's fields explicitly rather
than leaving it implicit.

## Notes

- This is a one-line `schemaSrcDirs` change plus a golden regen — small fix,
  high value given it's the signing surface specifically.
- After fixing, update the exclusion comment in `surface_test.go` if any
  other package remains deliberately excluded, so the comment stays an
  accurate map of what's covered vs. not (this was the root cause of the gap
  reading as "covered" in the first place).
