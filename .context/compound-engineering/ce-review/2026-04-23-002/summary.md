---
run_id: 2026-04-23-002
date: 2026-04-23
mode: autofix
base: df3472fb01c656c034fc30abff1c7725e2ce0825
plan: docs/plans/2026-04-22-003-refactor-typed-register-module-envelope-plan.md
pr: feat/typed-register-module-envelope (pre-PR)
verdict: Ready with fixes (applied)
---

# ce:review autofix run — issue #66 (typed RegisterModuleEnvelope)

## Reviewer team

Lean set — focused refactor (4 files, ~300 lines):
- correctness (always) — 0 findings, clean trace
- testing (always) — 2 findings (P2/P3)
- maintainability (always) — 4 low-confidence findings, all self-resolved as "no change required"
- project-standards (always) — 0 findings, clean
- agent-native (always) — IMPROVES parity, no regression
- learnings-researcher (always) — no stale docs; refactor extends typed-error patterns from PR #68/PR #71
- api-contract — envelope shape change warranted dedicated review → 3 findings (P1/P2/P3)

Skipped: adversarial (small scope, plan-faithful), stack-specific Rails/Python/TS (Go CLI), security/performance/data-migrations (no relevance).

## Applied safe_auto fixes

1. **Struct docstring strengthening (api-contract P2 0.78)**: `RegisterModuleEnvelope` docstring now explicitly documents the three-way SltHash population asymmetry (`already_registered` canonical vs `registered`/`advanced` fallback) with a concrete example (`ABC123` → `abc123`). Notes todo #021 as the unblock path for full symmetry. Adds explicit guidance: "scripts that need guaranteed canonical values should treat register-module's envelope as transactional and consult `course modules --output json` as the authoritative hash source."

2. **Empty-string fallback test coverage (testing P2 0.68)**: Added `TestRegisterOrRecoverModule` case "registered with empty-string gateway fields falls through to defaults" — gateway returns `{"status": "", "slt_hash": ""}` explicitly, envelope should fall through to `"APPROVED"` + supplied hash. Catches the case where a future gateway populates the keys but leaves the values blank.

## Not fixed — advisory only

| # | Finding | Severity | Reason |
|---|---------|----------|--------|
| A | `slt_hash` semantic change on `already_registered` branch | P1 (0.85, api-contract) | This is the stated goal of issue #66 (P2 #8). CHANGELOG documents before/after explicitly; further "louder" framing would add noise |
| B | JSON key-ordering shift alphabetical → declaration order | P3 (0.72, api-contract) | Cosmetic; CHANGELOG documents explicitly with before/after |
| C | Gateway-state lookup boilerplate duplication across 2 branches | P3 (0.55, maintainability) | Reviewer self-resolved — duplication is intentional for readability; revisit if pattern spreads to other commands |
| D | `strPtr` naming / `draftFrom` idiom | P3 (0.65-0.70, maintainability) | Language artifact of Go's "cannot take address of literal" rule; current form is idiomatic |
| E | Response field inspection minimal (nil/non-nil only) | P3 (0.62, testing) | Tautological — httptest handler IS the source of the response, so deep inspection would just verify JSON round-trip didn't mangle the response |

## Agent-native impact

IMPROVES parity:
- Typed struct = stable contract (3 loose maps → 1 source of truth)
- Canonical slt_hash on `already_registered` eliminates case-mismatch surprises when diffing against `course modules --output json`
- Long help text now points at the struct by name; agents reading Go source find the full contract in the docstring

No regression: JSON key-ordering shift is cosmetic (jq/JSON.parse are order-independent).

## Residual risks (advisory)

- If preprod gateway evolves to populate `status`/`slt_hash` on `registered`/`advanced` responses, the `lookupStringField` pipe lights up and values shift silently. Consumers caching envelope hashes may see different values over time without code changes. CHANGELOG flags this.
- `SetOnRetry` on `Client` is not goroutine-safe (already flagged in previous PR's review, unrelated to this refactor).

## Plan requirements (all met)

- R1 ✓ Struct defined with JSON tags matching current keys
- R2 ✓ `registerOrRecoverModule` returns typed struct
- R3 ✓ Envelope Status reflects gateway response (test: PENDING_VERIFY, APPROVED_WITH_WARNING, empty-string fallback)
- R4 ✓ SltHash canonical on `already_registered` (test with supplied uppercase vs stored lowercase)
- R5 ✓ Long help + docstring shrunk to struct pointer
- R6 ✓ Existing tests migrated to struct-field access, JSON round-trip retained

## Quality gates

- `go build ./...` clean
- `go vet ./...` clean
- `go test ./...` — all pass; added 4 new test cases (gateway-state on 3 branches + empty-string fallback); ~7.3s total
