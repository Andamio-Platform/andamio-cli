---
status: pending
priority: p1
issue_id: "031"
tags: [architecture, api-contract, breaking-changes, type-safety]
dependencies: []
---

# No Compile-Time Coupling to andamio-api's Contract

## Problem Statement

The CLI decodes gateway responses into `map[string]interface{}` at nearly every
call site (`var resp map[string]interface{}`, then string-keyed lookups with
`.(type)` assertions) rather than typed structs generated from or matched
against `andamio-api`'s OpenAPI spec. When a field is renamed or removed on
the API side, most of these lookups don't error — they just return a zero
value (`ok == false` is silently swallowed in several places), so the CLI
produces a *wrong* result instead of failing loudly.

`andamio-api` already has automated breaking-change protection for its public
contract (`cmd/verify-public-swagger-contract`, wired into CI via
`.github/workflows/verify-public-swagger.yml`, diffs `docs/swagger.public.json`
against a baseline ref on every PR). That tool has no visibility into the CLI
repo, so a rename that's intentional and safe on the API side (caught and
approved there) can still silently break the CLI's own output with nothing
in either repo's CI catching it.

## Findings

- **Typed pass-through** (`cmd/andamio/dev_keys.go`, `devKeyListItem`) —
  comment states it "mirrors andamio-api's keys_viewmodels.KeyListItem
  one-for-one." This is the safe pattern: a rename would fail `go build`
  once someone (correctly) updates the CLI struct to match — but only if
  the CLI author remembers there's a coupling to maintain by hand.
- **Loose reshape with silent degrade** (`cmd/andamio/course_credential.go`,
  `runCredentialVerifyHash`, ~line 116) — reads `m["slt_hash"]` and
  `m["on_chain_slts"]` out of an untyped map, republishes under a
  differently-named `verifyResult` struct mixed with locally-computed
  fields (`match`, `computed_hash`). If `on_chain_slts` were renamed
  upstream, this command would report `"no SLT texts found"` for every
  module instead of erroring.
- **Pure local construction, API response discarded** (`cmd/andamio/course_create_module.go`,
  `CreateModuleResult`) — built entirely from user input flags; the API's
  response body (`createResp map[string]interface{}`) isn't read at all.
- Pattern is inconsistent across all ~18 files in `cmd/andamio/*.go` that
  define JSON-tagged result structs — no shared convention for how (or
  whether) a command's printed JSON relates to what the gateway actually
  returned.
- Related, narrower precedent already flagged: `todos/002-pending-p3-unify-export-converter-structure.md`
  (untyped map wrapper structures in `course_export.go`) and
  `todos/003-pending-p3-export-json-include-titles.md` (breaking JSON
  output shape called out as a risk for a single proposed field change).

## Proposed Solutions

### Option A: Generate a typed client from andamio-api's OpenAPI spec
Use `oapi-codegen` (or similar) against `andamio-api`'s existing swagger to
generate request/response types, replace `map[string]interface{}` decode
sites incrementally, command by command.

`andamio-api` already depends on `github.com/oapi-codegen/runtime` (v1.2.0)
alongside `gofiber/swagger` and the `go-openapi/*` spec libraries — the
codegen tooling and the swagger contract it would run against both already
exist in the org. This isn't a new adoption decision, just pointing an
existing dependency at the CLI.

- **Pros**: Root-cause fix. A field rename fails `go build` immediately, for
  every typed pass-through field, with zero ongoing CI tooling needed.
- **Cons**: Touches all ~18 files; some commands (like `verify-hash`) still
  need hand-written reshaping on top of the generated types since they mix
  in local computation.
- **Effort**: Large
- **Risk**: Low (mechanical, compiler-verified as you go)

### Option B: Replace ad-hoc map lookups with typed intermediate structs (no full codegen)
For commands that reshape data (not just typed pass-through), decode into a
small locally-defined struct via `json.Unmarshal` instead of
`map[string]interface{}` + type assertions, so a missing/renamed field
errors instead of degrading silently.

- **Pros**: Much smaller than Option A, fixes the *silent* part of the
  failure mode command-by-command without waiting for a repo-wide effort.
- **Cons**: Doesn't give compile-time coupling to the API's actual contract
  — a rename still isn't caught until the command is run and fails to
  unmarshal a required field.
- **Effort**: Small–Medium, incremental per command
- **Risk**: Low

### Option C: Keep it out of scope, rely on CI-level snapshot diffing instead
Leave the code as-is; build a CI check that diffs the CLI's own declared
output structs (or generated JSON) between commits (the original
breaking-change-detection plan). Doesn't fix the underlying silent-degrade
risk, just adds a tripwire for unintentional changes to the CLI's own
output surface.

- **Pros**: No refactor of existing command code required.
- **Cons**: Treats the symptom, not the cause; still won't catch an
  API-side rename degrading a reshaped field silently (only catches the
  CLI's *own* struct changing, not API drift flowing through untyped maps).
- **Effort**: Medium (per the existing DEVNOTES plan)
- **Risk**: Low, but limited coverage

## Recommended Action

## Technical Details

**Affected files:** all `cmd/andamio/*.go` files with `map[string]interface{}`
response decoding — notably `course_credential.go`, `course_create_module.go`,
`project_task.go`, `project_manager_ops.go`; contrast with `dev_keys.go` as
the one example already using a typed, API-shape-mirroring struct.

## Acceptance Criteria

- [ ] Decide scope: full OpenAPI-generated client (A), targeted typed
      reshaping (B), or CI snapshot diffing only (C) — or some combination
- [ ] If A or B: at least one call site converted as a proof of concept
- [ ] Document the chosen convention so future commands follow it

## Work Log

| Date | Action | Learnings |
|------|--------|-----------|
| 2026-07-28 | Identified while scoping the breaking-change-detection CI plan | Raised as a separate architecture question — the original plan (CI snapshot/diff) treats a symptom; this TODO names the cause. Out of scope for the current maintainer task. |

## Resources

- `andamio-api`'s `cmd/verify-public-swagger-contract` (existing baseline-ref
  swagger diff, for comparison)
- Related: `todos/002-pending-p3-unify-export-converter-structure.md`,
  `todos/003-pending-p3-export-json-include-titles.md`
