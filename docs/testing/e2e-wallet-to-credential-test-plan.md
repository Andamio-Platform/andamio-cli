# Wallet Create → Course/Project Completion — E2E Test Plan

**Branch:** `feat/wallet-create` (depends on `todos/041` being available — see prerequisite in §1)
**Date:** 2026-09-09
**Status:** Draft, not yet executed
**Features covered:** `wallet create`, full course lifecycle (owner/teacher/student), full
project lifecycle (owner/manager/contributor), `tx run` against the learner/contributor
routes the retired `course student *` / `project contributor *` commands used to wrap

Written against MRP Month 2 deliverable 1 ("Claim-assignment flow validated end-to-end across
template, CLI, and the andamio-dev plugin" — see `../MONTH2-DEVNOTES.md` in the parent
`andamio` folder for the pivot context this sits under) and by reading current source across
this repo, `andamio-dev-kit-internal`, and `andamio-app-template`.

Purpose: figure out exactly what "end-to-end, across all three surfaces" has to mean given
what the codebase actually supports today, before writing or running anything. Two things
found while reading make this non-trivial and are the reason this plan is structured the way
it is — read those first.

Not part of the shipped CLI/kit — internal test planning only.

---

## 0. Two findings that shape this whole plan

### 0.1 The CLI's dedicated student/contributor *commands* are gone — the underlying routes are not

CLI 1.0 retired `course student *` and `project contributor *` entirely
(`cmd/andamio/retired.go`, issue #129). The commands still exist as hidden stubs so old
scripts get a clear error, but every one of them — `submit`, `commit`, `claim`, `update`,
`leave`, `delete` — now fails with:

> Learner and contributor work now happens in the Andamio app, which signs and submits it in
> one flow: https://app.andamio.io

That guidance describes the *product's* intended UX going forward, not a technical
restriction. Nothing about it is enforced server-side:

- `jwtAuthPreRunE` (which gates `tx run` and everything else requiring login) only checks for
  a valid JWT — `requireUserAuth()`, no role check. A `learner` alias satisfies it exactly like
  `owner`/`teacher`/`manager` do; `andamio user login --skey ... --alias learner` works today.
- The gateway routes themselves — `course/student/assignment/commit`,
  `course/student/credential/claim`, `project/contributor/task/commit`,
  `project/contributor/credential/claim` — are all still live and marked 🟦 Verified in the
  devkit's tx-flow index. The devkit's own test helpers
  (`tests/integration/gateway-endpoints/course-commit-assignment.ts`,
  `project-commit-task.ts`, `course-claim-credential.ts`, `project-claim-credential.ts`) drive
  every one of them with a plain signing key and a dev API key — no browser, no CIP-30.
- `tx run` even has a flag built for exactly this: `--metadata key=value`, which
  `project/contributor/credential/claim`'s confirm step needs (`task_hash` has to travel as
  registration metadata, per that helper's own comment) — CLI plumbing that only makes sense
  if hitting these routes generically was an anticipated use.

So the retirement removed the *convenience wrapper subcommands*, not the capability. `wallet
create` (`todos/041`) gives the `learner` persona a `.skey` the same way it does for the other
three; `tx run <endpoint>` is endpoint-agnostic. That means the full commit→assess→claim chain
— including the learner/contributor legs — is CLI-automatable end to end, using the retired
commands' old endpoints directly through the generic `tx run` primitive. See Phases 3 and 4
below, revised accordingly.

The one thing this approach genuinely does not cover: `andamio-app-template`'s own code
(wallet-connect UX, CIP-30 signing, request construction). Driving the gateway routes directly
proves the gateway/Atlas/chain leg of the learner path, not the template's implementation of
it. That's carved out as a separate, smaller manual check — §8.

### 0.2 The commit→assess→claim chain is already proven — but one layer down from where the MRP wants it

`andamio-dev-kit-internal/tests/integration/traces/full-trace.test.ts` already exercises the
complete chain for both course and project, twice per credential (commit → assess → claim),
plus the `/get-qualified` read path, and passes today. But it does this by calling Atlas and
the gateway **directly** from TypeScript test helpers (`loadWallet`, `commitAssignment`,
`assessAssignments`, ...) — it never shells out to the `andamio` binary and never touches
`andamio-app-template`.

That's exactly the gap MONTH2-DEVNOTES.md flagged and left unresolved: *"Haven't yet checked
how this flow is exercised (or not) from `andamio-cli` or `andamio-app-template`
specifically — the milestone wants it proven across all three surfaces, not just the
API/devkit level."* This plan is the answer to that open question for the CLI surface — it is
net-new coverage, not a re-run of something that already exists.

`andamio-app-template` has no browser/E2E harness at all today (no Playwright/Cypress config
found, only Vitest unit tests on isolated `lib/` logic). Per §0.1, this plan closes the
CLI-coverage gap fully, including the learner/contributor legs — but it does that by talking
to the gateway directly, the same way the devkit trace does, just through `andamio` instead of
raw HTTP. The `andamio-app-template` leg specifically stays uncovered by automation either
way; §8 below is a deliberately small manual check for that, not a substitute for a real
harness.

---

## 1. Scope

**In scope** — driven via real `andamio` CLI invocations, using `todos/041`'s
`wallet create` for key material:

1. Wallet creation (one per persona, including `learner`)
2. Alias mint + headless login (one per persona)
3. Course creation, module authoring, teacher assignment
4. Learner commits to both course assignments (`tx run` against the routes the retired
   `course student submit` used to wrap — see §0.1)
5. Teacher-side assessment (review + on-chain assess)
6. Learner claims the course credential (`tx run`, same rationale)
7. Project creation, task authoring
8. Learner commits to the project task (`tx run` against the retired
   `project contributor commit`'s old route)
9. Manager-side assessment (on-chain assess)
10. Learner claims the project credential (`tx run`, `--metadata task_hash=...`)
11. Read-path checks: `qualified-contributors`, listings, role-boundary rejection

**In scope, but manual (browser) — the one thing CLI automation genuinely can't reach:**

12. A single spot-check pass through `andamio-app-template` itself (§8) — wallet connect,
    one commit, one claim — to catch template-code regressions the gateway-level automation
    above has no way to see.

**Out of scope:**

- Sponsorship / Path B for any endpoint. Every course-side sponsored path is mid-migration to
  `/issuer/v1` (api#558) and marked 🟥 in the devkit's tx-flow index; project-side sponsorship
  is `⬜ Planned`, not implemented at all. This plan tests Path A (developer/dev-plane) only,
  which is the only path the CLI itself uses anyway.
- API key acquisition/rotation — supplied externally per Andrew.
- Network/gateway config correctness (`--network` vs `config.BaseURL`) — already covered by
  the manual verification done on 2026-09-08 for `wallet create`; not re-tested here.
- Load, performance, chain-reorg, or failure-injection testing.

**Hard prerequisite:** `feat/wallet-create` (`todos/041`) must be merged, or this plan runs
against that branch specifically — `wallet create` is how every persona below gets its
`.skey` without the old manual `docker run inputoutput/cardano-addresses` workaround.

---

## 2. Personas / wallets needed

Mirrors the devkit's own fixture shape (`tests/integration/fixtures/course.ts`,
`fixtures/project.ts`), which separates roles deliberately so role-boundary checks (e.g. "a
non-manager gets 403") are actually meaningful rather than accidentally true because one
wallet holds every role.

| Persona | Roles held | Needed for |
|---|---|---|
| `owner` | Course owner, Project owner | create course, create project, manage teacher list |
| `teacher` | Course teacher | author/register/publish modules, review + assess assignments |
| `manager` | Project manager | author tasks, assess task commitments |
| `learner` | Course student, Project contributor | commit assignment, commit task, claim both credentials — via `tx run`, see §0.1 |

`owner`/`teacher`/`manager` could technically collapse to fewer wallets, but keeping them
distinct is what makes the negative-path checks in §7 real instead of trivially true.

Each persona needs, in this order:

```bash
andamio wallet create --name <persona> --network preprod
# writes ~/.andamio/wallet/<persona>/{payment,stake}.{skey,vkey}, mnemonic.txt, prints address
```

Then funding — preprod ADA from a faucet or from Andrew's own preprod wallet, "enough for
~15+ txs with collateral" per the devkit trace's own comment (`fundWallet(..., [100_000_000,
100_000_000, 100_000_000])` — roughly 300 ADA split across 3 outputs for collateral/spending/
change headroom). Four personas × that budget is the rough sizing input for however funding
gets sourced — not fixing a number here since "don't worry about API keys, I'll supply them"
suggests funding sourcing is also something Andrew handles rather than something this plan
should prescribe.

---

## 3. Phase 0 — Wallet, alias, auth (all 4 personas)

Per persona:

```bash
# 1. Generate keys (todos/041)
andamio wallet create --name <persona> --network preprod

# 2. Fund the resulting address (external — faucet or Andrew's wallet)

# 3. Mint the on-chain alias (access token) — no dedicated CLI wrapper exists for this;
#    it's a generic tx run against the global mint endpoint.
andamio auth login --api-key <key>          # read-only API key, once per machine is enough
andamio tx run /v2/tx/global/user/access-token/mint \
  --body '{"alias":"<persona>","initiator_data":"<address from step 1>"}' \
  --tx-type <see: andamio tx types>          # confirm exact string at run time, don't hardcode

# 4. Headless login as that persona
andamio user login --skey ~/.andamio/wallet/<persona>/payment.skey \
  --alias <persona> --address <address>
```

**Assertions:**
- `wallet create --output json` returns a valid address for the configured network (not
  `addr_test1...` on a mainnet-configured gateway or vice versa).
- Access-token mint fails cleanly with a clear alias-collision error on the second attempt at
  the same alias (Atlas checks availability per `access-token-mint.md` step 5) — run this once
  intentionally to confirm the error is legible, not just "it works the first time."
- `andamio user status` shows the correct alias per persona after login.
- `andamio auth login --api-key` + expired user JWT: the CLI sends both the API key and the
  user JWT on every request, so an expired JWT still 401s a read-only endpoint the API key
  alone would satisfy. Confirm this reproduces, and that `andamio user logout` clears it —
  cheap regression check for a documented gotcha, worth keeping in the suite.

---

## 4. Phase 1 — Course creation & authoring (owner + teacher personas)

```bash
# Owner: create on-chain, auto-registers off-chain (per course_owner.go's own documented flow)
andamio tx run /v2/tx/instance/owner/course/create \
  --body '{"alias":"owner", ...}' --skey .../owner/payment.skey --tx-type course_create

andamio course owner update --course-id <id> --title "E2E Test Course" --description "..." --public

# Owner: add teacher (full lifecycle wrapper — build/sign/submit/register/confirm in one call)
andamio course owner teachers --course-id <id> --alias owner \
  --skey .../owner/payment.skey --add teacher

# Teacher: author two modules off-chain (no on-chain tx yet — pure DB draft + local hash)
andamio course create-module --course-id <id> --code 101 --title "Module 1" \
  --slt "Understand X" --approve
andamio course create-module --course-id <id> --code 102 --title "Module 2" \
  --slt "Understand Y" --approve

# Put modules on-chain (generic tx run — no dedicated wrapper for modules/manage)
andamio tx run /v2/tx/course/teacher/modules/manage \
  --body '...' --skey .../teacher/payment.skey --tx-type <see: andamio tx types>

# Link the on-chain hash back to the DB record, then publish
andamio course teacher register-module --course-id <id> --module-code 101 --slt-hash <hash>
andamio course teacher publish-module --course-id <id> --module-code 101
# repeat register-module/publish-module for 102
```

**Assertions:**
- `andamio course modules <id>` lists both modules as `ON_CHAIN` after publish.
- Re-running `register-module` with the same hash is a no-op (`action: already_registered`) —
  this idempotency path is explicitly documented in `course_teacher_ops.go` and worth
  confirming directly rather than trusting the doc comment.
- Re-running with a *different* hash against an existing module errors with the mismatch
  message naming `delete-module` as the remediation, rather than silently overwriting.

---

## 5. Phase 2 — Project creation & task authoring (owner + manager personas)

```bash
andamio tx run /v2/tx/instance/owner/project/create \
  --body '{"alias":"owner", ...}' --skey .../owner/payment.skey --tx-type project_create

andamio project owner update --project-id <id> --title "E2E Test Project" --public

# Adding a manager: no CLI wrapper exists (deferred endpoint per devkit INDEX.md —
# POST /api/v2/tx/project/owner/managers/manage has no build-flow file or CLI command yet).
# Falls back to raw tx run; confirm exact body shape against the live gateway/OpenAPI spec
# with `andamio spec paths --filter managers` before scripting this step.
andamio tx run /v2/tx/project/owner/managers/manage \
  --body '...' --skey .../owner/payment.skey --tx-type <see: andamio tx types>

# Manager: author a task (off-chain draft)
andamio project task create <project-id> --title "E2E Task" --lovelace 5000000 \
  --expiration 2026-12-01T00:00:00Z --content "Do the thing"

# Put the task on-chain — same pattern as course modules: generic tx run, no wrapper
andamio tx run /v2/tx/project/manager/tasks/manage \
  --body '...' --skey .../manager/payment.skey --tx-type <see: andamio tx types>
```

**Assertions:**
- `andamio project task list <project-id>` shows the task as on-chain/linked, matching the
  `verify-hash` command's expectation.
- `andamio project task verify-hash <project-id>` passes (task hash matches computed hash).

---

## 6. Phase 3 — Course completion loop: commit → assess → claim (CLI, all roles)

Mirrors the devkit trace's own ordering exactly (commit, then assess, per module — not both
commits up front) — that ordering matters because a teacher assessing a commitment that
doesn't exist yet is one of the failure modes worth being able to provoke deliberately, not
just avoid.

```bash
# Learner commits to module 1 — the route the retired `course student submit` used to wrap.
andamio tx run /v2/tx/course/student/assignment/commit \
  --body '{"alias":"learner","course_id":"<id>","slt_hash":"<hash-101>","assignment_info":"module 1 submission"}' \
  --skey ~/.andamio/wallet/learner/payment.skey --tx-type assignment_submit --instance-id <course-id>

# Teacher: off-chain review record (distinct from the on-chain assess tx below)
andamio course teacher review --course-id <id> --module-code 101 \
  --participant-alias learner --decision accept

# Teacher: the actual on-chain assessment tx. Build-only first, so the unsigned tx + the
# echoed decision set can be inspected before anything gets signed (this envelope pairing is
# the whole point of `assessment build` over `tx run` directly — see teacher_assessment.go's
# own doc comment on why this exists post-1.0).
andamio teacher assessment build --course-id <id> --alias teacher \
  --decision learner=accept --output json > assess-101.json
andamio tx sign --tx "$(jq -r .unsigned_tx assess-101.json)" --skey .../teacher/payment.skey
andamio tx submit --tx <signed-tx>

# Repeat commit + review + assess for module 102 (slt_hash-102)

# Learner claims the course credential — the route `course student claim` used to wrap.
andamio tx run /v2/tx/course/student/credential/claim \
  --body '{"alias":"learner","course_id":"<id>"}' \
  --skey ~/.andamio/wallet/learner/payment.skey --tx-type credential_claim --instance-id <course-id>
```

**Assertions:**
- `andamio course teacher commitments --course-id <id>` no longer lists the assessed
  commitments as pending.
- Re-running the learner's commit with the same `slt_hash` after assessment surfaces a clear
  error rather than silently re-committing — worth provoking once, mirrors the
  `register-module` idempotency check in Phase 1.
- The claim only succeeds after *both* modules are accepted — try claiming after only module
  101 is assessed and confirm it's rejected, not silently allowed.
- `andamio tx status <hash>` (or Andamioscan) confirms each tx on-chain, independent of the
  CLI's own "confirmed" reporting — belt-and-suspenders check that `tx run`'s polling isn't
  just trusting its own optimistic state.

---

## 7. Phase 4 — Project completion loop: commit → assess → claim (CLI, all roles)

```bash
# Learner commits to the task — the route `project contributor commit` used to wrap.
andamio tx run /v2/tx/project/contributor/task/commit \
  --body '{"alias":"learner","project_id":"<id>","contributor_state_id":"<id>","task_hash":"<hash>","task_info":"task submission"}' \
  --skey ~/.andamio/wallet/learner/payment.skey --tx-type task_submit --instance-id <project-id>
# contributor_state_id: confirm exact source (project get / contributor-state lookup) at
# execution time — not yet verified against a live gateway response in this research pass.

# Manager: no `assessment build` equivalent exists for project tasks — generic tx run only.
andamio tx run /v2/tx/project/manager/tasks/assess \
  --body '...' --skey .../manager/payment.skey --tx-type <see: andamio tx types>

# Learner claims the project credential — the route `project contributor claim` used to wrap.
# task_hash travels as registration metadata (not the request body) — the confirm handler
# reads it back out of metadata to mark the right task commitment REWARDED; omit it and the
# claim fails with "required metadata 'task_hash' not found".
andamio tx run /v2/tx/project/contributor/credential/claim \
  --body '{"alias":"learner","project_id":"<id>","contributor_state_id":"<id>"}' \
  --skey ~/.andamio/wallet/learner/payment.skey --tx-type project_credential_claim \
  --instance-id <project-id> --metadata task_hash=<hash>
```

**Assertions:**
- `andamio project manager commitments --project-id <id>` shows the task as assessed with
  the recorded outcome and evidence.
- **Role boundary check** (mirrors the devkit trace's own test at `full-trace.test.ts:161`):
  log in as `learner` and confirm `project manager qualified-contributors` (or any
  manager-only read) returns 403, not the data.
- `andamio project manager qualified-contributors --project-id <id>` includes `learner`'s
  alias once both course modules are accepted (this is exactly what `full-trace.test.ts`
  calls the "populated" case for devkit#116 — same assertion, now via the CLI's own read
  command instead of a raw gateway call).
- Omit `--metadata task_hash=...` once on purpose and confirm the claim fails with the
  documented error rather than something more confusing — cheap check that a real gotcha the
  devkit code comments out for is still surfaced clearly through the CLI's own error path.

---

## 8. Appendix — Manual `andamio-app-template` spot check (secondary, do separately)

Everything above proves the gateway/Atlas/chain leg of every role, including learner and
contributor, entirely through the CLI. What it does *not* touch is `andamio-app-template`'s
own code — wallet-connect UX, CIP-30 signing, how it builds its requests. That's a real gap
(§0.2: zero E2E harness there today) but a narrower one than "the whole learner journey is
untested," so it doesn't need to gate the rest of this plan and doesn't need full duplication
of Phases 3–4. One pass is enough to catch template-level regressions specifically:

1. Connect a browser wallet loaded with the `learner` mnemonic (from `wallet create`'s
   `mnemonic.txt`) to `andamio-app-template` pointed at preprod.
2. Confirm the app resolves the `learner` alias correctly.
3. Commit to one already-authored module via the UI (reuse a course from a prior CLI run, or
   author a throwaway one).
4. Once assessed (via CLI, as in Phase 3), claim the credential via the UI.
5. Confirm both txs land and the credential appears in the learner's dashboard.

Run this after Phases 1–4 confirm the underlying flow works, so a failure here points at the
template's own code rather than at the protocol/gateway layer underneath it.

---

## 9. Known gaps this plan surfaces (don't silently work around these — report them)

- **No CLI-level assess wrapper for project tasks.** `andamio teacher assessment build` has no
  project-side counterpart; assessing a task commitment is raw `tx run` today. Worth a todo if
  this plan's Phase 4 turns out to be as awkward in practice as it looks on paper — same shape
  as `todos/041` (a manual-workaround gap the CLI could close).
- **`POST /project/owner/managers/manage` has no CLI command and no devkit build-flow doc** —
  it's listed only as deferred in the tx-flows INDEX. This plan's Phase 2 manager-add step is
  the least-verified command in the whole plan; expect to need `andamio spec paths` or the
  OpenAPI spec directly to get the body shape right on first execution.
- **`andamio-app-template` has zero E2E automation**, and nothing in this plan changes that —
  driving the learner/contributor routes via `tx run` (Phases 3–4) proves the gateway leg, not
  the template's own code. §8's manual pass is a stopgap, not equivalent coverage. If the MRP
  deliverable's "across template, CLI, and the andamio-dev plugin" wording is read literally —
  the *template* itself exercised, not just the routes it happens to call — that's a separate,
  larger piece of work (Playwright + a CIP-30 test-wallet harness). Worth surfacing at the
  check-in as a scoping question rather than assuming either reading.
- **Alias/index-token availability is a shared, non-resettable resource** (per the trace file's
  own comment: *"Genesis accumulated 9 alias tokens... the alias-governance script now aborts
  on new mints from genesis"*). Running this plan repeatedly needs fresh alias names per run,
  or a dedicated funded wallet reused across runs (the devkit's own `trace`/`genesis` pattern) —
  don't mint a fresh `owner`/`teacher`/`manager`/`learner` alias on every execution without a
  plan for cleanup, or preprod state accumulates the same way genesis apparently did.

---

## 10. Suggested execution shape (for later, not now)

Given the user's "not acting on it yet": when this does get executed, the natural shape is a
shell script per phase (this CLI is explicitly designed for `--output json` scripting — every
command in this plan has a stable JSON envelope), checked into `andamio-cli` or a new `e2e/`
folder in the devkit, rather than a from-scratch rewrite of `full-trace.test.ts`'s TypeScript
approach. Because §0.1 means Phases 3–4 are now fully scriptable too, the whole plan (Phases
0–4) is a single candidate CI job — shell out to the built `andamio` binary the same way a
human would, four personas, no browser. That keeps the devkit's existing (and already green)
bare-Atlas trace as the fast check on the underlying tx flows, and adds this as the thing that
actually answers deliverable 1 for the CLI surface specifically. §8 stays manual and separate
until `andamio-app-template` gets its own harness — it's a smaller, independent piece of work,
not a blocker for shipping the rest as CI.
