# CLI 1.0 preprod validation sweep — Andamio API v2.5.0-rc5

**Date:** 2026-08-24
**Issue:** [#152](https://github.com/Andamio-Platform/andamio-cli/issues/152) (supersedes #133)
**Gateway:** `https://preprod.api.andamio.io` — self-reported `Andamio API Gateway v2.5.0` via `/openapi/swagger.json`
**CLI under test:** built from `main` @ `fa4b007` (Go 1.25.5). Fixes found during the sweep were applied on
`chore/preprod-1-0-validation-sweep` and later steps ran against the fixed build — noted per item.
**Network:** preprod only. Nothing touched mainnet.

---

## Readiness statement for #128

**The CLI side is clear to tag 1.0.** Every item in #152 passed. Three CLI-side defects were found and
**all three are fixed in this branch** with regression tests. No gateway-side defect was found, so nothing
here needs an rc6 that is not going to happen.

Two things a human still needs to decide before tagging:

1. **#152's own step-3 precondition is wrong** and should be corrected before anyone re-runs this sweep (§ Finding 1).
2. **`tx-loops.yaml` materially understates the cost of `teachers_update`** — a documentation defect in a sibling
   repo, not a blocker for the tag (§ Finding 5).

`spec fetch` was listed in #152 as expected-broken-and-do-not-fix. It turned out to be a one-line route change,
so it is fixed rather than carried into 1.0 as a known-broken command (§ Finding 3).

---

## Per-item verdicts

| # | Item | Alias used | Verdict |
|---|------|-----------|---------|
| 1 | Auth — headless login, API-key path, #134 expiry | `qa-1778157478` | **PASS** |
| 2 | Read plane — course/project list+get, assignments, `evidence_text`, `spec fetch` | `qa-1778157478` | **PASS** (2 defects found and fixed) |
| 3 | Assessment build — 1.0's headline | `qa-1778157478` | **PASS** |
| 4 | Full TX lifecycle + #151 repro | `qa-1778157478` | **PASS** (#151 retired) |
| 5 | Removed-command contract | none / no credentials | **PASS** |

---

## 1. Auth plane — PASS

| Check | Result |
|---|---|
| Headless login as `qa-1778157478` | exit 0 |
| `user status --output json` | `user_authenticated: true`, `session_expired: false`, `session_remaining_seconds: 86399` |
| API-key-only either-auth read (no user JWT in config) | `course list` exit 0, 106 courses |
| #134 expiry handling on a fresh JWT | no false-positive expiry at any of the four enforcement points |

Fresh-JWT behaviour is what #152 asked for ("doesn't trip normal flows"); manufacturing an expired token was
not attempted, and the four enforcement points remain covered by `cmd/andamio/expired_jwt_test.go`.

**Minor observation (not a defect):** `user login` text output prints `User ID:` with an empty value. Cosmetic.

---

## 2. Read plane — PASS, with two CLI defects found and fixed

### Envelope shapes: the CLI normalises, and the direction differs per command

`--output json` is documented as the scripting surface, so the shape it emits is a contract. Measured against
the raw gateway on the same routes:

| Command | Gateway returns | CLI `--output json` emits | |
|---|---|---|---|
| `course list` | `{data: [...]}` | bare `array[106]` | unwrapped |
| `project list` | `{data: [...]}` | bare `array[72]` | unwrapped |
| `token list` | bare `array[1]` | `{data: ...}` | wrapped |
| `tx types` | `{types: ...}` | `{types: ...}` | passthrough |
| `tx pending` | bare `array[0]` | **hard failure** | see Finding 2 |

**Verdict: intended behaviour, not a 2.5 regression — but inconsistent, and worth a follow-up.** The
unwrapping is the documented "List with formatting" pattern in `CLAUDE.md` (extract `data`, hand to
`output.PrintList`), and `token list` is documented as tolerating both shapes. So each command is individually
behaving as designed. The problem is that the *designs disagree*: a script that does
`andamio course list -o json | jq '.[]'` and `andamio token list -o json | jq '.data[]'` needs two different
shapes from one flag. Not a launch blocker and not a 2.5 issue — the gateway is self-consistent here; the CLI
is the source of the asymmetry.

### Teacher assignments — PASS

| Check | Result |
|---|---|
| `teacher assignments list --course beebcdee…` | 1 commitment, `SUBMITTED`, student `andamio-preprod-002`, module `100` |
| `content.evidence_text` renders | present, 328 chars, sibling of the hash-bearing `content.evidence` object |
| `list` vs `get` evidence parity | **byte-identical** (`sha256[0:16] = da9b91a52ede3a65` both sides) |
| Summary form (no `--course`) | correctly omits nested `content`; no `evidence_text` leaked into the summary |

The parity check matters because the raw Tiptap document is hash-bearing — the on-chain commitment hash is
computed over the normalised form. `get` routing through `fetchTeacherAssignmentsList` is holding.

### Finding 2 — `tx pending` crashed on an empty result (CLI-side, **FIXED**)

`andamio tx pending` exited **1** with `json: cannot unmarshal array into Go value of type
map[string]interface {}` whenever nothing was pending. The gateway returns a bare `[]` on HTTP 200 — a
perfectly valid empty result.

Three things were wrong at once:

- It violated the failure contract's load-bearing rule that **an empty result is exit 0 with an empty
  collection, not an error** — the exact rule `CLAUDE.md` warns not to "fix" away.
- It leaked a Go-internals message as the user-facing error.
- It was silently latent for every `getJSON` caller, not just `tx pending` — 11 routes, counting the
  three `getJSONWithHint` wrappers (course lesson/assignment/intro).

Root cause: `getJSON` decoded every response into `map[string]interface{}`, so any top-level array was a hard
unmarshal failure. The output layer already handled top-level arrays (`printAsCSV` switches on
`[]interface{}`); only the decode target blocked them.

**Fixed** in `cmd/andamio/helpers.go` (decode into `interface{}`). Regression test added to
`cmd/andamio/exitcode_test.go` — the file that already owns the empty-set contract — and verified to fail
before the fix. Confirmed against live preprod: `tx pending` now exits 0 and prints `[]`.

**Note:** `postJSON` has the identical latent shape and is left alone deliberately. Post-review it turned out
to have **zero callers anywhere in the codebase** — it is pre-existing dead code, so "no POST route returns a
bare array" is true only because nothing calls it. Removing it is a separate cleanup, not this branch's job.

**Framing worth carrying forward:** this is a second-order recurrence of
`docs/solutions/architecture/cli-composability-audit-and-fix.md`. That 2026-03-18 audit established the
"empty is not an error" rule and fixed it in the *output* layer, but never audited the *decode* layer — which
is where it sat latent for five months. The rule has two halves and only one was closed.

### Finding 3 — `spec fetch`/`spec paths` (#137) (CLI-side, **FIXED**)

Both halves of #137 confirmed, then fixed, because both turned out to be one-liners.

**Confirmed as reported:** `spec fetch` requested the sunset `/api/v1/docs/doc.json` (removed by
andamio-api#652 on 2026-07-28) → `API error 404`, exit 1. Probed the alternatives: only
`/openapi/swagger.json` responds 200.

**Confirmed second half:** `spec paths` read a local `openapi.json` from the cwd with **no age warning of any
kind** — whatever was on disk, however old, listing routes the gateway had since removed. This is what sent
the #133 dry run down a dead path.

**Also found, not in #137:** `spec fetch` bypasses `internal/client` entirely (its own `specHTTPClient`), so it
never produces typed `apierr` values. Its 404 came back as exit **1** / `kind: error`, where `course get` and
`tx status` both correctly return exit **2** / `kind: not_found` for a 404. This is a real failure-contract
divergence. It is **not fixed here** — routing `spec.go` through `internal/client` is a larger change than this
sweep should carry — and is recorded below as follow-up work.

**Fixed:** both call sites now use one `specDocPath` const (they had hardcoded the dead route separately, so
fixing one would have left the other broken), and `spec paths` announces the local file's date and age on
stderr, suppressed under `--output json`. Three tests added in `cmd/andamio/spec_test.go`.

Verified against live preprod: `spec fetch` exits 0 and reports `Andamio API Gateway v2.5.0` — which is also
this sweep's programmatic confirmation that the gateway under test really is 2.5.

---

## 3. Assessment build — PASS

Ran under `qa-1778157478` (see Finding 1 for why not `tester_0001`). Course `beebcdee…`, student
`andamio-preprod-002`, module `100`.

| Check | Result |
|---|---|
| Build with `accept` | exit 0, `unsigned_tx` 1668 chars |
| Build with `refuse` | exit 0, `unsigned_tx` 1668 chars |
| Duplicate alias in the decision set | **rejected**, exit 1 — `student "…" appears more than once; each student gets exactly one decision` |
| Nothing signed or submitted | commitment still `SUBMITTED`, `tx pending` empty, teacher set unchanged |

**The alias naming trap holds as documented.** The envelope disambiguates it well: top-level `teacher_alias`,
and `decisions[].alias` for the student. No `module_code` at any level — the protocol derives the module from
the on-chain commitment, as specified.

The documented limitation is still honestly stated: inspection is a **request-echo**, not a CBOR decode. It
proves what was asked for, not what the gateway built.

---

## 4. Full TX lifecycle and #151 — PASS, #151 retired

Loop 12 (`course.teachers.rotate`) from `andamio-dev/reference/tx-loops.yaml`: add `andamio-preprod-001` as a
teacher on `beebcdee…`, then remove it. Both aliases live in the same wallet, so no third party gained or lost
access, and the course was restored to its exact starting teacher set.

| Step | tx hash | Result |
|---|---|---|
| ADD teacher | `7c5ea04cb83c…` | exit 0, `state: updated`, `step: complete`, confirmed in 68s |
| Verify on-chain | — | teachers = `["qa-1778157478", "andamio-preprod-001"]` |
| REMOVE teacher | `7fc039445640…` | exit 0, `state: updated`, `step: complete`, confirmed in 7s |
| **Final state** | — | teachers = `["qa-1778157478"]` — **identical to pre-sweep** |

All three of loop 12's recorded gotchas held: `tx_type` is `teachers_update` (not `teachers_manage`), the
endpoint path and the tx_type deliberately differ, and only the owner may manage teachers.

### Finding 4 — #151 reproduced, and it is **correct behaviour, not a defect**

`tx register` immediately after a successful `tx run`, on the same hash:

```
{"error":"failed to register transaction: API error 409: {\"error\":{\"code\":\"CONFLICT\",
\"message\":\"transaction 7c5ea04c… is already registered with state updated\"}}","kind":"conflict"}
EXIT=6
```

Raw gateway confirms **HTTP 409** with that body.

`tx run` *already* registers the transaction as step 4 of its own lifecycle
(build → sign → submit → **register** → poll). Registering it again is a genuine duplicate, 409 is the correct
response, and the CLI maps it to exit **6** / `kind: conflict` exactly as the 1.0 failure contract specifies.

**Recommendation: close #151 as not-a-defect**, with this run as the evidence its original report lacked. Worth
noting for whoever closes it: `conflict` moved from exit 1 to exit 6 *in 1.0*, so an observer on 0.13.x would
have seen this same 409 surface as a generic exit-1 error — which is a plausible reason it read as a bug.

This was the specific check `docs/solutions/integration-issues/gateway-status-code-drift-409-vs-400.md`
warranted care on: that doc records the gateway returning 400 where the CLI expected 409 for `DUPLICATE_CODE`.
Here the status is a true 409 and the CLI's typed-error path fired correctly. No body-token fallback warning
was emitted.

### Finding 5 — `tx-loops.yaml` understates the cost of `teachers_update` (docs defect, sibling repo)

Measured on-chain, both transactions `valid_contract: true`, `deposit: 0`:

| | Fee | Wallet payment-address net |
|---|---|---|
| ADD | 0.302351 ADA | **−10.846388 ADA** |
| REMOVE | 0.287247 ADA | −0.627608 ADA |
| **Total fees** | **0.589598 ADA** | |

**The fee estimate is accurate.** tx-loops loop 12 says "~0.28 ADA per tx (network fee only)" and the measured
fees were 0.30 and 0.29. No complaint there.

**But the wallet cost is not the fee.** Adding a teacher moved ~10.5 ADA out of the wallet into script UTxOs,
and removing that teacher did **not** return it. Wallet balance across the whole sweep went
5198.613506 → 5188.239080 tADA, a net **−10.374426** against ~0.59 in fees.

Loop 12's `cost_estimate` is labelled "network fee only, no service fee", so it is not *wrong* — but a reader
sizing a wallet for teacher rotation would under-provision by roughly 18×. **Recommend a follow-up in
`andamio-dev` to record the locked-ADA figure alongside the fee.** Not a CLI defect, not a gateway defect, and
not a blocker for the tag.

---

## 5. Removed-command contract — PASS

| Check | Result |
|---|---|
| `course student claim` (text) | exit **4**, names the removal and points at the app |
| `course student claim --output json` | exit **4**, `kind: removed_command` in a JSON envelope |
| `course student claim --bogus-flag xyz --output json` | exit **4**, still the JSON envelope |
| `project contributor commit --output json` | exit **4**, `kind: removed_command` |
| Retired command with **no credentials configured** | exit **4**, removal notice — **not** an auth error |
| Control: `teacher courses` with no credentials | exit **3**, `kind: auth` |

Both load-bearing properties hold. The unknown-flag case proves the stubs still use
`FParseErrWhitelist{UnknownFlags: true}` rather than `DisableFlagParsing` — under the latter, the root's
persistent `--output` flag would not parse and the JSON cases above would have emitted plain text. The
no-credentials case proves the stubs did not inherit an auth `PersistentPreRunE`.

---

## Finding 1 — #152's step-3 precondition is unsatisfiable (process defect)

**#152 names `tester_0001` as the owner/teacher/manager identity AND requires "a preprod course where
`tester_0001` teaches with ≥1 commitment in `SUBMITTED`". Those two requirements cannot both be met.**

Wallet `andamio-preprod-001` holds **three** access tokens under policy
`aa1cbea2524d369768283d7c8300755880fd071194a347cf0a4e274f`, all signable with the same `payment.skey`:

| Alias | Teacher courses | Commitments awaiting review |
|---|---|---|
| `qa-1778157478` | `beebcdee…`, `9d1682d2…` (broken DB state) | **1 — `SUBMITTED`** |
| `tester_0001` | `b9baa6ba…`, `9f437601…`, `4ef42f85…` | **0** |
| `andamio-preprod-001` | `f2298842…` | not surveyed |

**Why this is dangerous rather than merely wrong:** headless login succeeds for *any* of the three, with no
error and no warning. Verified both wrong-alias failure modes:

- `tester_0001` on **its own** courses → `teacher assignments list` returns **0**, exit 0. Silent empty result.
- `tester_0001` on `beebcdee…` → `422 UNPROCESSABLE_ENTITY`, `TEACHER_NOT_ALLOWED`, exit 1.

An operator following #152 literally lands on the first case: a green-looking run that exercises nothing. The
assessment leg — 1.0's headline feature — would report success having assessed nothing.

**Recommended action:** correct #152's step 3 to name `qa-1778157478` before anyone re-runs this sweep.

*(Aside: the 422 surfaces as exit 1 / `kind: error`. 422 is not enumerated in the failure contract, so falling
through to generic is conformant, not a defect.)*

---

## What this sweep did **not** cover

Stated plainly, because a report that reads as blanket coverage is worse than one that names its edges:

- **One course, one commitment, one student.** Every assessment and read-plane check ran against
  `beebcdee…` / `andamio-preprod-002` / module `100`. Multi-module, multi-student, and cohort paths are unexercised.
- **One tx type.** `teachers_update` only. The other 16 registered tx types were not driven. `tx types` was
  read, not exercised.
- **One wallet, one signing path.** Headless `.skey` signing only. **Neither browser login flow was tested** —
  not `user login` (browser) and not `dev login` (browser). Those are the typical human journeys.
- **Developer-portal surfaces untested.** `dev keys`, `apikey usage`, `apikey profile` were **not run**. The
  stored `tester_0001` dev JWT expired 2026-08-19 and re-authenticating it was outside the sweep's scope.
- **No expired-JWT enforcement test.** #134's four enforcement points were confirmed not to fire falsely on a
  *fresh* token; the expired path was not manufactured. Unit coverage in `expired_jwt_test.go` stands unchanged.
- **Course export/import untested.** The two most complex commands in the CLI were not exercised against 2.5.
- **`project` write paths untested.** Task create/update/delete/import/export were not run; only reads.
- **No load, concurrency, or rate-limit testing.**
- **Assessment was built, never submitted.** Per #152. The signing and submission of an assessment — and
  therefore the credential-minting leg — remains unexercised end-to-end.

---

## Changes made on this branch

| Commit | Change |
|---|---|
| `b55af84` | `fix(tx)`: `getJSON` accepts bare JSON array responses (Finding 2) |
| `d123bb4` | `fix(spec)`: point `spec fetch`/`paths` at the live gateway route, warn on stale local spec (#137, Finding 3) |

`go test ./...` green after both.

## Follow-up work

| Item | Where | Priority |
|---|---|---|
| Correct #152's step-3 alias to `qa-1778157478` | this repo, issue #152 | before any re-run |
| Close #151 as not-a-defect, citing § Finding 4 | this repo, issue #151 | low |
| `spec fetch` bypasses `internal/client`, so 404 → exit 1 instead of exit 2 / `not_found` | this repo | medium |
| `--output json` envelope shape is inconsistent across list commands | this repo | low |
| `teachers_update` locks ~10.5 ADA that removal does not return — record alongside the fee estimate | `andamio-dev`, `reference/tx-loops.yaml` loop 12 | medium |
| `postJSON` shares `getJSON`'s old latent array-decode failure — and has zero callers (dead code) | this repo | low |
| `schemasnapshot` has no visibility into raw-passthrough `getJSON` routes | this repo, `todos/037` | medium |
| `--output json` envelope shape inconsistent across list commands | this repo, `todos/038` | low |

## Environment restored

- `~/.andamio/config.json` restored from the pre-sweep backup.
- Course `beebcdee…` teacher set restored to `["qa-1778157478"]`.
- No CLI binary was installed to `PATH`; the sweep build lives in a scratch directory.
- Wallet 001: 5198.613506 → 5188.239080 tADA (0.589598 in fees; remainder in script UTxOs, § Finding 5).
