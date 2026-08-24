---
title: "chore: 1.0 preprod validation sweep against the v2.5.0-rc5 gateway"
type: chore
status: active
created: 2026-08-24
issue: 152
depth: deep
---

# chore: 1.0 preprod validation sweep against the v2.5.0-rc5 gateway

**Origin:** GitHub issue #152 (supersedes #133). Follow-on is #128 (tag 1.0) — explicitly **out of scope**.

---

## Summary

CLI 1.0 is merged to `main` but untagged, and it declares a hard requirement on Andamio API 2.5. The rc5 gateway set has soaked on preprod since 2026-08-18 with zero 5xx, but all that soak evidence is **passive and server-side** — no one has actively driven the CLI, one of the two real API consumers, against the exact bits promoting to prod. The soak window closes 2026-08-25 and no rc6 will be cut.

This plan executes the client-side sweep that makes CLI 1.0's "verified against 2.5" claim true against the promoting build, and ends at a **validation report**. It does not tag, release, or touch mainnet.

## Problem Frame

Three things make this more than "run the commands and see":

1. **The issue's own step-3 precondition is factually wrong** (see Correction below). An implementer following it literally gets a green-looking run that silently exercises nothing.
2. **There is no rc6.** A gateway-side defect found here cannot be fixed before cutover, so the report's job is *classification* (launch-blocker vs known-issue-at-launch), not repair. Misfiling a gateway defect as a CLI bug wastes the only remaining window.
3. **`main` has moved ~60 commits since #130 merged**, so the 1.0 that gets tagged is not the 1.0 that was originally verified.

---

## Correction Carried Into This Plan

Issue #152 names `tester_0001` as the owner/teacher/manager identity **and** requires "a course where `tester_0001` teaches with ≥1 commitment in `SUBMITTED`". Verified against preprod on 2026-08-24: **those two requirements are incompatible.**

Wallet `andamio-preprod-001` holds **three** access tokens under policy `aa1cbea2524d369768283d7c8300755880fd071194a347cf0a4e274f`. All three are signable with the same `payment.skey`, and headless login succeeds for any of them — **the wrong alias produces no error**, just an empty result set:

| Alias | Teacher courses | Commitments awaiting review |
|---|---|---|
| `qa-1778157478` | `beebcdee…`, `9d1682d2…` (broken DB state — do not use) | **1 — `SUBMITTED`** |
| `tester_0001` | `b9baa6ba…`, `9f437601…`, `4ef42f85…` | **0** |
| `andamio-preprod-001` | `f2298842…` "Acceptance Test Course v2.2.0-rc1" | not surveyed |

**Decision: the assessment leg (U4) runs under `qa-1778157478`.** The report must state which alias each step ran under, and must record the issue's wrong precondition as a finding needing a follow-up correction to #152.

`tester_0001` is separately the **developer-portal** identity (`dev_alias`, tier admin). Its dev JWT expired 2026-08-19, so any `dev keys` / `apikey` step needs `dev refresh` or `dev login` first.

---

## Requirements Trace

| ID | Requirement (from #152) | Unit |
|---|---|---|
| R1 | Auth: headless login, API-key path, #134 expiry behavior doesn't trip normal flows | U2 |
| R2 | Read plane: course/project list+get, `teacher assignments list`/`get` with `evidence_text`, `spec fetch` (expected broken per #137) | U3 |
| R3 | Assessment: `teacher assessment build`, build-only, nothing signed or submitted | U4 |
| R4 | Full TX lifecycle: one complete `tx run`, then the #151 `register` repro | U5 |
| R5 | Removed-command contract: retired learner command exits 4 / `kind: removed_command` | U6 |
| R6 | Validation report with per-item verdict, evidence, readiness statement for #128 | U7 |
| R7 | Small CLI-side fixes in-PR; bigger CLI defects filed as issues; gateway defects classified only | U8 |

---

## Scope Boundaries

**In scope:** the five sweep items, the report, small CLI-side fixes, filing issues for larger CLI defects.

**Non-goals:**
- Tagging or releasing 1.0 (#128)
- Gateway changes of any kind
- Anything touching mainnet
- Flipping README's "stay on 0.13.x for mainnet" statement (happens at cutover, not here)
- Fixing #137 (`spec fetch`) beyond a trivial one-liner — confirm and record

### Deferred to Follow-Up Work
- Correcting #152's `tester_0001` precondition (file as a comment or follow-up issue; do not silently edit the issue)
- Closing #133 once this completes (issue says to; that is a maintainer action)
- Any gateway-side defect found — classified in the report, routed by a human

---

## Key Technical Decisions

**D1. Assessment runs under `qa-1778157478`, not `tester_0001`.** Rationale above. Alternative — staging a fresh student commitment under `tester_0001` — is rejected: 1.0 removed the learner commands, so staging would mean driving the student leg by raw `curl` or the released 0.13.x binary under an isolated `$HOME`. That is a lot of moving parts to reproduce a fixture that already exists.

**D2. The TX lifecycle (U5) uses `teachers_update` on course `beebcdee…`.** Cheapest validated loop at ~0.28 ADA (tx-loops.yaml loop 12, `course.teachers.rotate`), owner-only, and **reversible**: add `andamio-preprod-001` as a teacher, then remove it. Both aliases live in the same wallet, so no third party gains or loses access. Restores the course to its exact starting teacher set.

**D3. Config is treated as hostile shared state.** `~/.andamio/config.json` is global and machine-wide with no per-invocation base-URL override. Already backed up at `scratchpad/config.json.bak` and restored to pre-check state. Re-verify at start, restore at end, and never assume a prior step's alias is still the active one — re-assert identity before each role-sensitive step.

**D4. Every step records the exact command and the raw response.** The report's value is evidence a reader can re-run, not a verdict they have to trust. Evidence files land under a scratchpad run directory and the salient excerpts get inlined in the report.

---

## Prior Art That Shapes This Sweep

`docs/solutions/integration-issues/gateway-status-code-drift-409-vs-400.md` is directly load-bearing for U5. It documents that the gateway returns **400** where the CLI's typed-error gate expected **409** for `DUPLICATE_CODE`, and that the CLI now carries a body-token fallback that emits a stderr drift warning. #151 reports a **409** from `register` after a `tx run`. So U5 must record the *actual status code and body*, not just "it failed" — the same class of drift is already known to exist in this surface, and the CLI's own fallback warning is a signal worth capturing if it fires.

That doc's central lesson also applies to this whole sweep: *a flagged risk is an open task, not a closed one.* This sweep exists because #130's "verified against 2.5" was an assumption.

---

## Implementation Units

### U1. Preflight and evidence harness

**Goal:** Establish a known-good starting state and a place to put evidence, so no later step has to guess what the environment was.

**Requirements:** prerequisite for R1–R7

**Dependencies:** none

**Files:** none in-repo (scratchpad only)

**Approach:** Confirm `config show` reports preprod for **both** base URL and submit URL (a mainnet-base/preprod-submit split builds a mainnet tx and submits it to preprod — it fails on network mismatch, but confirm rather than discover). Rebuild the 1.0 binary if `main` has moved since the existing build. Record `--version`, `go version`, the resolved `main` SHA, and both wallet tADA balances. Create a run directory for per-step evidence files.

**Verification:** A recorded preflight block naming binary SHA, gateway base URL, submit URL, and both balances — enough for a reader to know exactly what was under test.

---

### U2. Auth plane (issue step 1)

**Goal:** Prove headless login, the API-key path, and #134 expiry handling all work against rc5.

**Requirements:** R1

**Dependencies:** U1

**Files:** none expected; `cmd/andamio/user.go`, `internal/config/jwt.go` if a defect surfaces

**Approach:** Headless login as `qa-1778157478` (per D1). Confirm `user status` reports authenticated with a sane `session_expired` / remaining-seconds. Exercise the API-key-only path by confirming an either-auth read works with no user JWT present. For #134: confirm a *fresh* JWT does not trip any of the four enforcement points — this is a "doesn't break normal flows" check, not an attempt to manufacture an expired token.

**Patterns to follow:** `docs/solutions/logic-errors/fix-three-cli-issues-hex-encoding-lesson-merge-headless-login.md` for headless-login pitfalls.

**Test scenarios (evidence to capture, not unit tests):**
- Headless login as `qa-1778157478` → exit 0, `user status` shows `user_authenticated: true`
- Either-auth read with API key and no user JWT → exit 0, non-empty result
- `user status --output json` → `session_expired` present and `false`; remaining-seconds present
- Known trap: **do not pipe `user login` into `head`** — SIGPIPE can kill the CLI before it persists config, so login prints success while nothing saves

**Verification:** Each of the four recorded with command + response.

---

### U3. Read plane and the 2.5 contract surface (issue step 2)

**Goal:** Exercise the surfaces 1.0 was rebuilt for, and judge the envelope asymmetry already observed.

**Requirements:** R2

**Dependencies:** U2

**Files:** none expected; `cmd/andamio/course.go`, `cmd/andamio/teacher_assignments.go` if a defect surfaces

**Approach:** Run course and project `list` + `get`; `teacher assignments list` and `get` on the `beebcdee…` fixture confirming `content.evidence_text` renders as a sibling of the hash-bearing raw `content.evidence`; and `spec fetch`.

**The envelope asymmetry needs a deliberate judgment, not a note.** Already observed: `course list` returns a **bare JSON array**, while `course get` returns `{"data": {...}}`. Since step 2 is specifically about the 2.5 contract and 1.0 was rebuilt around the 2.5 pagination convention, decide and record: is this intended (list vs single-resource shapes differ by design), a gateway regression, or a CLI passthrough that should normalize? Check `internal/client/testdata/` for a recorded contract and the gateway's own handler if the sibling checkout is available. `token list` already "tolerates both `{data: [...]}` and a bare array" per CLAUDE.md — establish whether that tolerance is the intended house style or a workaround.

`spec fetch` is **expected broken** per #137 (points at the sunset `/api/v1/docs/doc.json`; gateway moved to `/openapi/swagger.json`). Confirm and record; do not fix unless genuinely trivial. Note #137 also flags that `spec paths` silently falls back to a stale local `openapi.json` — check whether that silent fallback is still present, since it actively misled the #133 session.

**Test scenarios:**
- `course list` / `project list` → exit 0, envelope shape recorded verbatim
- `course get` on a known id → exit 0, `{"data": {...}}` recorded
- `teacher assignments list --course beebcdee…` → the SUBMITTED commitment, `content.evidence_text` present and non-empty
- `teacher assignments get` for the same commitment → identical evidence to `list` (they route through the same fetch and must not diverge)
- Absent-evidence case: confirm `evidence_text` is *absent*, not empty-string, where there is no evidence
- `spec fetch` → confirm the #137 failure mode and exact exit code
- Empty-but-valid result set → **exit 0**, not an error (the failure contract's load-bearing rule)

**Verification:** Envelope asymmetry has a written verdict with reasoning, not just an observation.

---

### U4. Assessment build (issue step 3 — 1.0's headline)

**Goal:** Build an assessment transaction against rc5. **Build only — nothing signed, nothing submitted.**

**Requirements:** R3

**Dependencies:** U2, U3

**Files:** none expected; `cmd/andamio/teacher_assessment.go` if a defect surfaces

**Approach:** Authenticate as `qa-1778157478` (D1). Target course `beebcdee…`, student `andamio-preprod-002`, module `100`. Run `teacher assessment build` with a single `--decision`. Inspect the `AssessmentBuildEnvelope`.

**The payload naming is a known trap and the sweep should prove it holds:** top-level `alias` is the **teacher**; `assignment_decisions[].alias` is the **student**. Two fields, same name, two different people. There is no `module_code` at any level — the protocol derives the module from the on-chain commitment. Confirm rc5 still honors this shape.

Also confirm the documented limitation is still honestly stated: inspection is a **request-echo**, not a CBOR decode. It proves what was asked for, not what the gateway built.

**Test scenarios:**
- Build with one `accept` decision → exit 0, envelope carries both the unsigned tx and the echoed decision set
- Build with one `refuse` decision → exit 0 (do not submit either)
- Duplicate alias in the decision set → **rejected**, not last-wins
- Wrong-alias control: run the same build under `tester_0001` and confirm it finds nothing — this is the evidence that substantiates the correction finding
- Confirm no transaction was submitted (no new tx hash on the course, commitment still `SUBMITTED`)

**Verification:** A built-but-unsubmitted tx, plus recorded proof the wrong alias yields nothing.

---

### U5. Full TX lifecycle and the #151 repro (issue step 4)

**Goal:** Drive one complete `tx run` (build → sign → submit → register → poll) end-to-end on preprod, then attempt the #151 repro.

**Requirements:** R4

**Dependencies:** U1, U2

**Files:** none expected; `cmd/andamio/tx.go`, `internal/submit/`, `internal/cardano/` if a defect surfaces

**Approach:** Per D2, use `teachers_update` on `beebcdee…` (owner `qa-1778157478`): **add** `andamio-preprod-001` as teacher, confirm on-chain, then **remove** it to restore the original teacher set. Consult `andamio-dev/reference/tx-loops.yaml` loop 12 before the first submit.

Loop 12's recorded gotchas, which the sweep should confirm still hold:
- `tx_type` is `teachers_update`, **not** `teachers_manage` — registration fails with invalid tx_type if wrong
- Endpoint path is `/v2/tx/course/owner/teachers/manage` while the registration tx_type is `teachers_update` — the mismatch is intentional
- Only the course **owner** can manage teachers, not existing teachers
- At least one teacher must remain

**Immediately after the successful `tx run`, attempt the #151 repro:** run an explicit `tx register --tx-hash <hash> --tx-type teachers_update` for the hash `tx run` already registered, and record the result either way. #151 has no surviving detail beyond "returns 409", so this run either confirms or retires it.

**Record the exact status code and body, not just pass/fail.** `docs/solutions/integration-issues/gateway-status-code-drift-409-vs-400.md` documents that this gateway family returns 400 where the CLI expected 409 for a duplicate, and that the CLI now carries a body-token fallback emitting a stderr drift warning. If that warning fires, capture it. A "409" in a stale report may well have been a 400 — or a correct idempotent-conflict signal that needs no fix at all.

**Execution note:** This is the only step that spends real tADA and mutates on-chain state. Verify the teacher set before and after; do not leave the course with an altered teacher list.

**Test scenarios:**
- `tx run` add-teacher → exit 0, tx hash returned, poll reaches a terminal confirmed state
- `course get` after confirmation → teacher list includes the added alias
- `tx register` re-registration of the same hash → **record status, `kind`, exit code, and body verbatim**; classify as confirms-#151 / retires-#151 / different-failure
- `tx status <hash>` → terminal state consistent with the poll result
- `tx run` remove-teacher → teacher set restored to exactly its pre-sweep contents
- Failure path: if any submit fails, capture the submit-API response body before retrying — a network-mismatch error here means D3's config check was wrong

**Verification:** Course teacher set identical to its starting state; #151 has a definite verdict backed by a raw response body.

---

### U6. Removed-command contract (issue step 5)

**Goal:** Spot-check that a retired learner command still exits 4 with `kind: removed_command`.

**Requirements:** R5

**Dependencies:** U1

**Files:** none expected; `cmd/andamio/retired.go`, `cmd/andamio/retired_test.go` if a defect surfaces

**Approach:** Run a retired command (e.g. a `course student` path) in both text and `--output json` mode. The JSON mode check is the load-bearing one: retired stubs use `FParseErrWhitelist{UnknownFlags: true}` rather than `DisableFlagParsing` **specifically** so the root's persistent `--output` flag still parses. If someone swapped that, `--output json` silently emits plain text instead of a JSON error envelope — a silent failure nothing else in the build notices.

Also confirm the stub does not greet an unauthenticated caller with "not authenticated" instead of the removal notice (the second load-bearing property).

**Test scenarios:**
- Retired command, text mode → exit 4, message names the removal and the replacement
- Retired command, `--output json` → exit 4 **and** a JSON envelope with `kind: removed_command`
- Retired command with **no credentials configured** → still the removal notice, not an auth error
- Retired command with an unknown flag appended → still the removal notice, not a flag-parse error

**Verification:** All four recorded. Any failure here is a CLI-side defect and in scope for U8.

---

### U7. Validation report

**Goal:** Produce `docs/validation/2026-08-preprod-v2.5.0-rc5.md` — the deliverable.

**Requirements:** R6

**Dependencies:** U2–U6

**Files:** `docs/validation/2026-08-preprod-v2.5.0-rc5.md` (new; directory does not yet exist)

**Approach:** Per-item verdict — **pass / fail / blocked** — with the exact command and response evidence for each, and the alias each step ran under. Then a final readiness statement addressed to #128.

Required contents beyond the per-item table:
- **The alias-precondition finding**, with the evidence from U4's wrong-alias control run, and a recommendation to correct #152
- **A classification section** for every failure: CLI-side (fixed here / filed as issue) vs gateway-side (**launch-blocker** vs **known-issue-at-launch**). There is no rc6 — a gateway-side item stops at classification and a human routes it
- **The envelope-asymmetry verdict** from U3
- **The #151 verdict** from U5, with the raw status and body
- **What this sweep did not cover**, stated plainly — the sweep touches one course, one commitment, one tx type, and one wallet. A report that reads as blanket coverage would be worse than one that names its own edges

**Execution note:** Write verdicts from recorded evidence only. If a step was skipped or blocked, say so — a "pass" that was not actually run is the one outcome that makes this whole exercise worse than not doing it.

**Verification:** Every item in R1–R5 has a verdict and evidence; the readiness statement is unambiguous about whether #128 is clear to proceed.

---

### U8. CLI-side fixes found along the way

**Goal:** Fix what is cheap to fix; file what is not.

**Requirements:** R7

**Dependencies:** U2–U6

**Files:** determined by findings; test files alongside per the repo's contract-guard convention

**Approach:** Rule of thumb from the issue — fix in-branch if writing the issue would cost more than the fix. Anything larger gets a filed issue (no boards).

**Any behavioral fix must land with a test in the guard file that owns that invariant**, per CLAUDE.md's contract table: exit-code/`kind` mapping → `exitcode_test.go`; retired commands → `retired_test.go`; JWT expiry → `expired_jwt_test.go`. Do not build a parallel mechanism for a rule that already has a guard.

**Test scenarios:** per fix; each fix adds a case to the guard file that owns its invariant.

**Verification:** `go test ./...` green; every finding is either fixed, filed, or explicitly classified in U7.

---

## Risks

| Risk | Mitigation |
|---|---|
| Wrong alias silently produces an empty green run | D1; U4 runs an explicit wrong-alias control to make the failure mode visible |
| Global config clobbered mid-sweep | D3; backup exists, restore at end, re-assert identity before role-sensitive steps |
| On-chain state left mutated | U5 adds then removes the same teacher; teacher set verified before and after |
| A gateway defect gets "fixed" CLI-side to make the sweep pass | Classification discipline is a named requirement (R7) and a required report section (U7) |
| Sweep reads as broader coverage than it is | U7 requires an explicit "what this did not cover" section |
| Stale local `openapi.json` misleads the run (burned #133) | U3 checks whether `spec paths`' silent fallback is still present |
| #151 verdict rests on a status code that has drifted before | U5 records raw status + body, not pass/fail; prior-art doc read first |

## Deferred to Implementation

- Which retired command U6 uses (any is valid; pick from `retired.go`'s registry)
- Whether the envelope asymmetry (U3) warrants a CLI fix, an issue, or nothing — depends on the verdict
- The exact set of U8 fixes
- Whether `spec fetch` (#137) is trivial enough to fix here — likely a one-line path change, but #137's second half (stale-fallback warning) is not

## Out of Scope for This Branch

**No `git push`, no PR.** Work commits locally on a branch and stops. This overrides the normal pipeline tail (push / open PR / watch CI).
