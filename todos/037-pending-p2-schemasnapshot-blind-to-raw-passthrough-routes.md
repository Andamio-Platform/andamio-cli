---
status: pending
priority: p2
issue_id: "037"
tags: [ci, breaking-change-gate, schemasnapshot, json]
dependencies: []
---

# schemasnapshot has no visibility into raw-passthrough getJSON routes

## Problem

`internal/schemasnapshot` guards the `--output json` surface by walking Go
struct declarations and serialising their json tags into
`testdata/golden/schema.golden`. That only works for envelopes the CLI
*constructs*.

The 11 routes served by `getJSON` / `getJSONWithHint` (`cmd/andamio/helpers.go`)
build no struct at all — they decode into `interface{}` and hand the value
straight to `output.PrintJSON`. Their emitted shape is whatever the gateway
returned, so the scanner has never had anything to see:

- `tx pending`, `tx types`, `tx status`
- `course get`, `course modules`, `course slts`
- `course lesson`, `course assignment`, `course intro` (via `getJSONWithHint`)
- `project get`
- `user exists`

A gateway-side rename in any of these lands on users with the surface gate
green. Verified during the #152 sweep: both goldens passed unchanged across a
commit that altered what `tx pending` emits (exit 1 → a bare array).

This is **distinct from `todos/033`–`036`**, which cover function-local structs,
the unscanned `internal/cardano` package, missing scanner unit tests, and flag
semantics. None of them mention routes that declare no struct.

## Why it matters now

CLI 1.0 declares a hard requirement on Andamio API 2.5, and 2.5 carries a
contract naming change (see `todos/031`). The mainnet cutover is the moment this
CLI first meets renamed gateway fields — and these 11 routes are exactly the
ones where a rename passes through silently.

## Proposed Solution

The scanner cannot infer these shapes from source; something has to record them.
Options, roughly in order of cost:

1. **Record response fixtures.** Capture a real response per route under
   `internal/client/testdata/` and diff against it in CI, the way
   `preprod-duplicate-module-response.md` already documents one contract.
2. **Emit through typed structs.** Give each route a declared response type so
   the existing scanner sees it. Highest fidelity, most churn, and it would
   change what the CLI passes through today.
3. **Name the exclusion.** At minimum, record in the gate's own docs that
   raw-passthrough routes are out of scope, so the golden is not read as
   covering the whole `--output json` surface.

Option 3 is the floor and should happen regardless — `todos/034` makes the same
argument for `internal/cardano`: an unnamed exclusion reads as coverage.

## Found

During the #152 preprod validation sweep (2026-08-24), by the api-contract
reviewer. See `docs/validation/2026-08-preprod-v2.5.0-rc5.md`.
