---
title: Typed output envelopes with gateway-state fallbacks
date: 2026-04-23
problem_type: best_practice
tags: [typed-envelope, output-json, contract-stability, defensive-lookup, gateway-state, whitespace, go, encoding-json]
applies_when:
  - Designing or migrating a CLI `--output json` contract where the wire shape must stay stable across versions
  - Gateway response field names vary across environments or aren't captured in test fixtures
  - A CLI command has multiple success branches that each construct a similar envelope
  - The real preprod gateway response shape is unknown but the contract-level refactor is ready to ship
related_docs:
  - docs/solutions/architecture/go-retry-classifier-and-backoff-patterns.md
  - docs/solutions/architecture/cli-composability-audit-and-fix.md
  - docs/solutions/architecture/cmd-package-helper-placement-and-output-consistency.md
  - docs/solutions/feature-implementations/cli-course-module-management-commands.md
---

# Typed output envelopes with gateway-state fallbacks

## Context

The `course teacher register-module` command emits a `--output json` envelope with five keys: `action`, `status`, `slt_hash`, `advanced_from`, `response`. It was originally constructed three times — once per success branch (`registered`, `advanced`, `already_registered`) — via separate `map[string]interface{}` literals. The envelope shape was documented in three places (cobra Long help, handler docstring, CLAUDE.md command table) that drifted apart across PRs. Worse: `status` was hardcoded to `"APPROVED"` on two branches regardless of gateway truth, and `slt_hash` always echoed the user's supplied input rather than the canonical stored value — so a caller registering with `"ABC123"` against a stored `"abc123"` saw a byte-level mismatch against `course modules --output json`.

Issue #66 fixed this. The plan was marked `status: blocked` on preprod fixture capture (todo #021), but the plan's own Key Technical Decisions accepted that partial gateway-state population is acceptable today provided the lookup pipe is in place for when fixtures land. User authorized the unblock; PR #72 shipped the typed struct + defensive lookup + honest asymmetry documentation.

Four learnings surfaced during implementation and two review rounds that apply to any future `--output json` contract in this repo or similar CLIs.

## Guidance

### 1. Envelope = typed struct with JSON tags, not `map[string]interface{}`

**Do this:**

```go
// RegisterModuleEnvelope is the stable --output json contract for
// `course teacher register-module`. Scripts should branch on Action.
type RegisterModuleEnvelope struct {
    Action       string         `json:"action"`
    Status       string         `json:"status"`
    SltHash      string         `json:"slt_hash"`
    AdvancedFrom *string        `json:"advanced_from"`
    Response     map[string]any `json:"response"`
}

func registerOrRecoverModule(...) (*RegisterModuleEnvelope, string, error) { ... }
```

**Not this:**

```go
// Three branches, three near-identical literals, contract drifts against docstring:
envelope := map[string]interface{}{
    "action":        "registered",
    "status":        "APPROVED",
    "slt_hash":      sltHash,
    "advanced_from": nil,
    "response":      resp,
}
```

Benefits the typed struct delivers:

- **Single source of truth.** The struct's doc comment is the canonical schema. Cobra Long help and handler docstrings shrink to one-line pointers at the struct name. No more three-place drift.
- **Go-level contract.** `output.PrintJSON(envelope)` still accepts `any` and marshals correctly, but downstream tests read `envelope.Status` instead of `envelope["status"]` — a typo becomes a compile error, not a silent `nil` interface.
- **`*string` for nullable fields.** `advanced_from` is `null` on two branches and `"DRAFT"` on the third. `*string` preserves this: nil marshals to JSON `null`, non-nil marshals to the string. Using `string` + `omitempty` would drop the key on non-advance branches — a breaking shape change for consumers that check `has("advanced_from")`.
- **`map[string]any` (not plain `any`) for wrapped responses.** Preserves map-typing for Go-level consumers; `nil` map still marshals as JSON `null`.

### 2. Populate from gateway state via defensive `lookupStringField`, not direct key access

Real gateway responses have field-name drift across environments (`slt_hash` vs `course_module_slt_hash`) and sometimes nest fields inside a `content` object. A tolerant helper absorbs this without per-endpoint special cases:

```go
// lookupStringField returns the first non-empty string match for any of the given
// field names, checking top-level first then content-nested. Whitespace-only
// values are treated as "not present"; surrounding whitespace on a valid value
// is trimmed.
func lookupStringField(m map[string]interface{}, names ...string) string {
    for _, name := range names {
        if v, ok := m[name].(string); ok {
            if trimmed := strings.TrimSpace(v); trimmed != "" {
                return trimmed
            }
        }
    }
    if content, ok := m["content"].(map[string]interface{}); ok {
        for _, name := range names {
            if v, ok := content[name].(string); ok {
                if trimmed := strings.TrimSpace(v); trimmed != "" {
                    return trimmed
                }
            }
        }
    }
    return ""
}
```

Call sites:

```go
status := lookupStringField(resp, "status", "course_module_status", "module_status")
if status == "" {
    status = "APPROVED"  // hardcoded fallback
}
respHash := lookupStringField(resp, "slt_hash", "course_module_slt_hash")
if respHash == "" {
    respHash = sltHash  // supplied fallback
}
```

Three properties this gives you:

- **Drift tolerance.** When preprod renames `slt_hash` to `course_module_slt_hash` without warning, the helper picks up the new name on the first attempt. No code change needed.
- **Whitespace safety.** A buggy gateway returning `"status": " "` or `"status": "\t\n"` falls through to the hardcoded fallback instead of leaking whitespace into downstream equality checks against canonical statuses (`"APPROVED"`, `"DRAFT"`). A value like `" APPROVED "` returns as `"APPROVED"` — no silent inequality with the canonical form.
- **Null safety.** Go allows map reads on `nil` maps (returns zero values), so `lookupStringField(nil, ...)` returns `""` cleanly — no panic if a caller accidentally passes a nil response.

### 3. Ship typed contracts before fixtures — document asymmetry honestly

The motivating refactor had three branches with unequal ability to produce canonical values:

- `already_registered`: canonical values guaranteed today (source: the teacher modules list, which we already fetched for the status/hash compare).
- `registered` / `advanced`: gateway response may or may not include the fields; today's observed behavior is that it doesn't. Falls back to supplied/hardcoded values.

The temptation is to block: "capture fixtures first, then build the refactor." That's wrong when:

1. The typed struct pattern itself delivers value (clarity, contract stability) regardless of fixture completeness.
2. At least one branch can produce canonical values today (here: `already_registered`).
3. The defensive lookup pipe is additive — when fixtures land later, candidate field names widen in-place with no caller-visible change.

**What to do instead:**

- Ship the struct + `lookupStringField` + fallbacks.
- Document the asymmetry honestly in the struct docstring AND the CHANGELOG. Name which branches deliver semantic upgrades today vs which wait for fixtures.
- Reference the tracking todo/issue for fixture capture so future maintainers understand the deferred work.

```
// SltHash canonicalization is asymmetric by branch (todo #021 unblocks
// full symmetry):
//   - "already_registered": always canonical (existing.SltHash from the teacher
//     modules list). Supplied "ABC123" against stored "abc123" returns "abc123".
//   - "registered" / "advanced": gateway field if present, else the supplied
//     hash (post-trim). Today's gateway typically doesn't populate it on these
//     branches, so consumers see supplied casing until real preprod fixtures land.
```

The opposite failure mode — shipping a refactor with no acknowledgment of the asymmetry — leads to consumer scripts that assume all branches canonicalize equally, then break mysteriously when the `registered` branch returns uppercase.

### 4. Know the JSON key-ordering shift when migrating map → struct

`json.Marshal` orders **map keys alphabetically** but **struct fields by declaration order**. Migrating `map[string]interface{}` → struct changes the emitted key order on the wire.

**Before (map, alphabetical):**

```json
{"action": "registered", "advanced_from": null, "response": {...}, "slt_hash": "abc123", "status": "APPROVED"}
```

**After (struct, declaration order):**

```json
{"action": "registered", "status": "APPROVED", "slt_hash": "abc123", "advanced_from": null, "response": {...}}
```

- Consumers parsing with `jq`, `JSON.parse`, or any JSON library: unaffected (key order is not semantic).
- Consumers doing byte-for-byte output diffs or regex-matching on the raw string: will see a diff.

**What to do:**

- Declare struct fields in the order you want them emitted. Keep the natural reading order (identity → state → nullability → payload) rather than forcing alphabetical.
- Call out the ordering shift explicitly in the CHANGELOG as "cosmetic" so byte-diff consumers can update their golden files.
- Consider running tests with `jq -S` (sorted keys) in CI when wire-format stability across map/struct migrations matters.

## Why This Matters

**Pattern 1 (typed struct)**: Extends the typed-error principle from `docs/solutions/architecture/go-retry-classifier-and-backoff-patterns.md` to success-path contracts. Retry classifiers use typed errors so the retry predicate survives error-message format changes; output envelopes use typed structs so the wire contract survives refactors to the underlying response parsing. Same philosophy, different target.

**Pattern 2 (lookupStringField with trim)**: Two bugs in one fix. (a) Without multiple candidate names, a rename in one environment's response breaks the CLI silently. (b) Without trim-based emptiness check, a whitespace-only gateway value leaks into downstream equality checks — a rare edge case in theory, but the fix is one `strings.TrimSpace` call and eliminates the category.

**Pattern 3 (ship partial with honest docs)**: Blocking on fixtures for a 6-unit refactor that delivers value today on 1-of-3 branches is the wrong call when the other 2 branches are cleanly future-compatible. The plan for #66 encoded this as `Blocked → Active` with explicit justification in the plan's own Key Technical Decisions section. Reader: plans marked `blocked` should be re-read for whether the block is load-bearing or conservative.

**Pattern 4 (key ordering)**: Easy to miss because `go test` on a JSON round-trip still passes — unmarshal-then-assert-key-presence is order-independent. Byte-diff CI (rare but real in some golden-file setups) catches it only after the PR lands. Flag it proactively in the CHANGELOG.

## When to Apply

- **Pattern 1:** Every time you're constructing a `--output json` envelope from multiple branches, or a single-branch envelope with more than ~3 fields. For a one-branch envelope with 2 fields, a map is fine.
- **Pattern 2:** Any time you're reading optional fields from a gateway response that you don't fully control. The helper is ~15 lines; the cost of not having it is silent breakage when field names drift.
- **Pattern 3:** When a planned refactor has multiple implementation units and you discover that only some can deliver their full stated benefit without external data capture. Don't conflate "this unit's full value needs X" with "the refactor needs X" — split and ship what you can.
- **Pattern 4:** Any map → struct migration that affects `--output json` emission. Flag in the CHANGELOG under the `Changed` section.

## Examples

### Testing gateway-state with hand-crafted fixtures

Unit tests can exercise the `lookupStringField` pipe without real preprod fixtures by constructing responses that the code treats as realistic:

```go
// Test: registered branch with gateway-provided status reflects it, not fallback.
registerResp: map[string]interface{}{
    "module_id": "m-101",
    "status":    "PENDING_VERIFY",
    "slt_hash":  "canonical_hash",
},
wantStatus:  "PENDING_VERIFY",  // lookup pipe wins over hardcoded "APPROVED"
wantSltHash: "canonical_hash",  // lookup pipe wins over supplied

// Test: empty-string gateway fields fall through to fallback.
registerResp: map[string]interface{}{
    "module_id": "m-101",
    "status":    "",
    "slt_hash":  "",
},
wantStatus:  "APPROVED",  // fallback
wantSltHash: "abc123",    // supplied

// Test: whitespace-only gateway fields also fall through.
registerResp: map[string]interface{}{
    "module_id": "m-101",
    "status":    "   ",
    "slt_hash":  "\t\n",
},
wantStatus:  "APPROVED",
wantSltHash: "abc123",

// Test: surrounding whitespace on valid value gets trimmed.
registerResp: map[string]interface{}{
    "module_id": "m-101",
    "status":    "  PENDING_VERIFY  ",
},
wantStatus: "PENDING_VERIFY",  // trimmed, not "  PENDING_VERIFY  "
```

Four test cases cover: happy path gateway population, empty-string fallback, whitespace-only fallback, surrounding-whitespace trim. All four fail if `lookupStringField`'s trim behavior or candidate-list ordering breaks.

### File-local `strPtr` helper for `*string` test fixtures

Go forbids `&"DRAFT"` (can't take address of unnamed string constant). Test fixtures need a helper:

```go
// strPtr is a file-local helper for building *string literals in tests — Go
// forbids taking the address of an unnamed string constant, so &"DRAFT" is a
// compile error.
func strPtr(s string) *string { return &s }

// Usage in table:
wantAdvancedFrom: strPtr("DRAFT"),
wantAdvancedFrom: nil,
```

### Pointer-value comparison in test assertions

`*string` identity is wrong for comparisons — two `strPtr("DRAFT")` calls return different pointers to different allocations. Use a four-case switch:

```go
switch {
case tt.wantAdvancedFrom == nil && envelope.AdvancedFrom != nil:
    t.Errorf("AdvancedFrom = %q, want nil", *envelope.AdvancedFrom)
case tt.wantAdvancedFrom != nil && envelope.AdvancedFrom == nil:
    t.Errorf("AdvancedFrom = nil, want %q", *tt.wantAdvancedFrom)
case tt.wantAdvancedFrom != nil && envelope.AdvancedFrom != nil && *envelope.AdvancedFrom != *tt.wantAdvancedFrom:
    t.Errorf("AdvancedFrom = %q, want %q", *envelope.AdvancedFrom, *tt.wantAdvancedFrom)
}
```

Each branch guarantees both operands of any dereference are non-nil. `reflect.DeepEqual(got, want)` is an alternative but obscures the specific failure mode.

## Prevention

For any new `--output json` command in this repo:

1. **Start with a struct, not a map.** If you catch yourself writing a multi-line `map[string]interface{}` literal with `json:"..."` tags in comments, stop and extract a typed struct.
2. **Put the struct doc comment above the struct definition.** That doc becomes the single source of truth for the JSON contract. Cobra help text and handler docstrings point at the struct by name rather than re-describing the schema.
3. **Use `lookupStringField` (or an equivalent helper) for reading optional gateway fields.** Never do bare `m["foo"].(string)` against a response you don't fully control.
4. **Name the asymmetry.** If different branches canonicalize fields unequally under current gateway behavior, say so in the struct doc AND the CHANGELOG. Reference the tracking issue for fixture capture so future maintainers know the deferred work.
5. **Flag JSON key ordering in the CHANGELOG when migrating map → struct.** Explicit before/after ordering is enough; consumers decide if they care.
6. **Test table-drive the fallback matrix.** Four cases minimum for each branch with a lookup pipe: gateway-provided, empty-string, whitespace-only, surrounding-whitespace.

## Sources

- Issue: [#66](https://github.com/Andamio-Platform/andamio-cli/issues/66)
- PR: [#72](https://github.com/Andamio-Platform/andamio-cli/pull/72)
- Plan: `docs/plans/2026-04-22-003-refactor-typed-register-module-envelope-plan.md` (status: completed)
- Related pattern: `docs/solutions/architecture/go-retry-classifier-and-backoff-patterns.md` — typed errors over string matching for retry classifiers; this doc extends the same principle to success-path envelope contracts.
- Related pattern: `docs/solutions/architecture/cli-composability-audit-and-fix.md` — `--output json` as the stable scripting surface.
- Review rounds surfacing these patterns: two `/ce:review` passes (autofix + interactive), 8 personas total, 5 safe_auto fixes applied across rounds.
