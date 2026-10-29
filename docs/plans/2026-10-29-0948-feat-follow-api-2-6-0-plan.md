---
title: Follow Andamio API 2.6.0 - Plan
type: feat
date: 2026-10-29
artifact_contract: ce-unified-plan/v1
product_contract_source: ce-plan-bootstrap
execution: code
---

# Follow Andamio API 2.6.0 - Plan

## Goal Capsule

- **Objective:** An operator following the CLI's help and docs against Andamio API 2.6.0 never hits a step 2.6.0 refuses, and anyone using `dev` or `apikey` learns that key management moved to the developer portal before those commands are removed in 2.0.0.
- **Means:** Documentation and help corrections, one dead branch removed, `--tx-hash` stops being sent, and a deprecation notice from one registry (KTD1, KTD3).
- **Authority:** cli#194 > this plan > existing code comments. The Failure Contract in `CLAUDE.md` and `cmd/andamio/exitcode_test.go` is not changed by this work.
- **Stop conditions:** Stop and ask if any change would alter an exit code, a `kind` string or a `--output json` shape, except the `register-module` `PENDING_TX` case KTD3 approves. Stop if a preprod check shows 2.6.0 still accepting `PENDING_TX` on update-status.
- **Execution profile:** Standard. Go code plus docs, one PR, a minor release (1.3.0).
- **Finishes and ships:** `ce-work` implements and opens the PR. James reviews and merges.

---

## Product Contract

### Summary

The CLI's course-module help and `docs/COURSE-LIFECYCLE.md` stop describing `PENDING_TX`, `ARCHIVED` and `DEPRECATED`. The lifecycle doc loses its "set `PENDING_TX`" step, because 2.6.0 reads `ON_CHAIN` from chain. `course owner register --tx-hash` stays accepted, stops being sent, and says it is ignored. Every `dev` and `apikey` command prints, on stderr, that keys are managed at https://app.andamio.io/api-setup and that the command is removed in 2.0.0.

### Problem Frame

Andamio API 2.6.0 went live on mainnet on 2026-10-29. In 2.6.0 a module is `ON_CHAIN` when andamioscan lists it with a matching SLT hash, and the DB status no longer records it (`internal/orchestration/module_derivation.go` in andamio-api at `v2.6.0`). Update-status accepts only `DRAFT` or `APPROVED`, and publish changes nothing. `COURSE-LIFECYCLE.md` still tells operators to run `update-module-status --status PENDING_TX`, which 2.6.0 refuses with a 400. The course register request dropped `tx_hash`, so the CLI's "stops being taken when andamio-api#884 ships" text is now overdue. `/v2/keys` left the public contract (andamio-api `contract/api_scope.yaml`, dev-portal internal), and the app's developer portal is where keys are managed.

### Requirements

**Module status**

- R1. No CLI help text describes a course-module status other than `DRAFT`, `APPROVED` or `ON_CHAIN`.
- R2. `docs/COURSE-LIFECYCLE.md` describes the 2.6.0 lifecycle, and every command it tells an operator to run is one 2.6.0 accepts.
- R3. Task and commitment statuses (`PENDING_TX` on tasks, `PENDING_TX_*` on commitments) stay as documented.

**Course register**

- R4. `course owner register --tx-hash` is still accepted, is not sent to the API, and its deprecation text says it is ignored and removed in 2.0.0.

**Developer portal**

- R5. Every run of `dev login`, `dev logout`, `dev refresh`, `dev status`, `dev keys list`, `dev keys create`, `dev keys delete`, `apikey usage` and `apikey profile` prints a deprecation notice on stderr. The notice names https://app.andamio.io/api-setup and says the command is removed in andamio CLI 2.0.0.
- R6. The notice changes nothing else. Stdout, exit codes and `--output json` shapes stay byte-identical, and the commands keep working.
- R7. The CHANGELOG, `CLAUDE.md`, README and onboarding docs record the deprecation and the 2.0.0 removal.

### Key Decisions

- **Removal is scheduled for 2.0.0.** CHANGELOG policy allows breaking changes only in a major, and 2.0 is where `course owner create` and the `--tx-hash` flag already retire. Governs R4, R5. (session-settled: user-approved, chosen over an earlier or unnamed release: semver.)
- **"Developer login" means the whole `dev` group, and `apikey` is included.** `apikey usage` and `apikey profile` need the dev JWT, so they lose their login path when `dev login` goes. Governs R5. (session-settled: user-approved, chosen over deprecating only `dev keys` and `dev login`: the rest of the group has no use without them.)

### Scope Boundaries

- `docs/TX-LOOP-TEST-RESULTS.md` keeps its `PENDING_TX` mentions. It is a dated record of a past run, not guidance.
- `course teacher publish-module` stays, even though 2.6.0 makes publish a no-op. The lifecycle doc says what it does now (R2).
- Considered and not built: a `warnings` field in the JSON envelope for the deprecation. The `dev keys create` one-time warning already goes to stderr in JSON mode, and a field would change the JSON shape (R6). Evidence that would change this: an agent consumer that reads only stdout and needs to detect the deprecation.
- Considered and not built: an env var to silence the notice. Nobody asked for one, and the notice is one stderr line.

#### Deferred to Follow-Up Work

- The CLI docs in andamio-docs (`content/docs/apps-tooling/cli/index.mdx`), tracked in `Andamio-Platform/andamio-docs#64`.
- Deprecating `course teacher publish-module` now that 2.6.0 makes it a no-op. Separate issue.
- Removing `dev`, `apikey`, `course owner create` and `--tx-hash` in 2.0.0, through `cmd/andamio/retired.go`.

---

## Planning Contract

### Key Technical Decisions

- KTD1. **The deprecation notice is printed by a wrapper, not by cobra's `Deprecated` field.** Cobra hides a `Deprecated` command from help. Marking every leaf would make `andamio dev --help` list no subcommands. Marking the groups would hide `dev` and `apikey` from `andamio --help` while they still work. One table in a new file lists the nine commands. At init, each command's `RunE` is wrapped so the notice prints before the original runs. Help output never shows the notice, because `RunE` does not run for `--help`. The wrapper does not touch `PersistentPreRunE`. A child `PersistentPreRunE` would replace the root's, which parses `--output` (the trap `retired.go` documents). This differs from the `course owner create` precedent on purpose, because that was one leaf with a visible sibling replacement.
- KTD2. **The notice prints in every output mode, JSON included, and goes to stderr before any request.** This follows the `dev keys create` one-time-key WARNING, which already writes to stderr under `--output json`. Printing first means a run that then fails on auth still shows where to go. `dev keys create` text mode keeps the raw key alone on stdout, so `| pbcopy` still captures only the key.
- KTD3. **The `PENDING_TX` branch in `register-module` is removed, with no compatibility shim.** A 2.6.0 gateway maps a stored `PENDING_TX` to `APPROVED` before answering, so the CLI can no longer see it. Against a pre-2.6 gateway, a module in `PENDING_TX` now reaches the existing "unexpected status" error, which tells the operator what happened. (session-settled: user-approved, chosen over keeping the case for 2.5 gateways: it can no longer be reached on mainnet or preprod.)
- KTD4. **`--tx-hash` keeps its flag registration until 2.0.0, and only the payload line goes.** Deleting the flag would make scripts fail with "unknown flag" in a minor release. `MarkDeprecated` already hides it from help. This mirrors `--pending-tx-hash` on `course owner create`. (session-settled: user-approved, chosen over deleting the flag now: a minor release must not break scripts.)

### Assumptions

- Preprod runs 2.6.0 as well as mainnet. U5 checks this before the README is reworded.

---

## Implementation Units

### U1. Module status in help and code

- **Goal:** Help text names only `DRAFT`, `APPROVED` and `ON_CHAIN`, and the dead branch goes.
- **Requirements:** R1, R3. KTD3.
- **Dependencies:** none.
- **Files:**
  - `cmd/andamio/course_teacher_ops.go` (register-module `Long`, the `case "APPROVED", "PENDING_TX", "ON_CHAIN"` branch)
  - `cmd/andamio/course_import_assignment.go` (`Long`)
  - `cmd/andamio/course_teacher_ops_test.go` or the existing register-module test file (find with `grep -ln "already_registered" cmd/andamio/*_test.go`)
- **Approach:**
  1. Drop the `PENDING_TX` row from the register-module state table.
  2. Make the `import-assignment` help say "DRAFT, APPROVED and ON_CHAIN modules alike".
  3. Remove `PENDING_TX` from the `case`, leaving `APPROVED` and `ON_CHAIN` as no-ops.
  4. Check the `update-module-status` help and `--status` flag text (around `course_teacher_ops.go:112` and `:172`), and make them name only `DRAFT` and `APPROVED` as settable.
  - Leave the task-side `PENDING_TX` in `project_task_import.go` and the commitment `PENDING_TX_*` text in `teacher_assignments.go` and `project_manager_ops.go` alone (R3).
- **Test scenarios:**
  - An existing module with status `APPROVED` and a matching hash returns `action: already_registered`, unchanged.
  - An existing module with status `ON_CHAIN` and a matching hash returns `already_registered`, unchanged.
  - An existing module with status `PENDING_TX` and a matching hash returns the "unexpected status" error and exits non-zero.
  - A guard walks the `course` command tree and fails if any `Short` or `Long` contains `PENDING_TX`, `ARCHIVED` or `DEPRECATED`. Task and commitment commands sit outside `course`, so the guard does not catch them.
- **Verification:** `andamio course teacher register-module --help` and `andamio course import-assignment --help` mention no removed status.

### U2. Course lifecycle and concepts docs

- **Goal:** An operator can follow `docs/COURSE-LIFECYCLE.md` end to end against 2.6.0.
- **Requirements:** R2, R3.
- **Dependencies:** U1, so the docs quote the corrected help.
- **Files:** `docs/COURSE-LIFECYCLE.md`, `CONCEPTS.md`, `CLAUDE.md` (register-module and publish-module rows).
- **Approach:**
  1. The status lifecycle becomes `DRAFT --> APPROVED --> ON_CHAIN`. `ON_CHAIN` is read from chain once andamioscan indexes a minted module whose SLT hash matches. No command sets it.
  2. Step 5 becomes "mint on-chain": the `modules_manage` `tx run` only, with no status step. Drop the "reset to APPROVED if the TX fails" note, since nothing was moved.
  3. Rewrite the troubleshooting entry "TX confirmed but module stuck in APPROVED". On 2.6.0 the likely causes are an andamioscan indexing delay or an SLT hash mismatch between DB and chain. The checks are `course modules` and `course credential verify-hash`.
  4. Rewrite "Module on-chain but stuck at DRAFT" for 2.6.0 behavior: a hash-matched on-chain module reads as `ON_CHAIN` whatever the stored status. Remove the entry if that leaves nothing to do.
  5. Drop `PENDING_TX` from the register-module idempotency list. Make the import-assignment paragraph name the three statuses.
  6. Rewrite the "slt_index out of range" fix: on 2.6.0 a module on chain cannot return to DRAFT (the gateway's draft lock refuses it), so the DRAFT reset applies only to a module not yet minted.
  7. In the Quick Reference, merge rows 5a and 5b. Describe row 7 (`publish-module`) as a check that changes nothing on 2.6.0.
  8. Update the line-16 status definition in `CONCEPTS.md`, and the `register-module` row and the `PENDING_TX` mention in the `publish-module` row in `CLAUDE.md`.
  - Leave the commitment state diagrams (around lines 259-266) unchanged (R3).
- **Test expectation:** none, documentation only. U1's help guard covers the code side.
- **Verification:** `grep -n "PENDING_TX\|ARCHIVED\|DEPRECATED" docs/COURSE-LIFECYCLE.md CONCEPTS.md` returns only commitment `PENDING_TX_*` lines. Every hit of `grep -n "status DRAFT" README.md docs/COURSE-LIFECYCLE.md` applies only to a module that is not on chain.

### U3. Stop sending `--tx-hash`

- **Goal:** `course owner register --tx-hash` is accepted and ignored.
- **Requirements:** R4. KTD4.
- **Dependencies:** none.
- **Files:** `cmd/andamio/course_owner.go`, `cmd/andamio/contract_884_test.go`.
- **Approach:**
  1. Remove the `tx_hash` payload block and its comment in `registerCourse`.
  2. Change the `MarkDeprecated` message to say the API stopped taking it in 2.6.0, it is ignored, and it is removed in 2.0.0.
  3. Change the flag usage string to match.
- **Patterns to follow:** `pending-tx-hash` on `courseOwnerCreateCmd`, and `TestCourseOwnerCreate_RegistersWithoutPendingTxHash`.
- **Test scenarios:**
  - Replace `TestCourseOwnerRegister_StillSendsTxHashWhenGiven`: register with `--tx-hash tx1` sends a body with no `tx_hash` key and succeeds.
  - `courseOwnerRegisterCmd.Flags().Lookup("tx-hash")` is non-nil and its `Deprecated` message names 2.0.0, so scripts passing it do not fail. The `registerTestCmd` helper builds its own flags and cannot prove this.
  - `TestCourseOwnerRegister_StillRequiresTitle` passes unchanged.
- **Verification:** `go test ./cmd/andamio -run 'CourseOwner'` passes.

### U4. Developer-portal deprecation notice

- **Goal:** All nine commands print where keys are managed and when they go, without changing their output.
- **Requirements:** R5, R6. KTD1, KTD2.
- **Dependencies:** none.
- **Files:**
  - `cmd/andamio/dev_portal_deprecation.go` (new: the command table, the notice text and the init-time `RunE` wrapper)
  - `cmd/andamio/dev.go`, `cmd/andamio/dev_keys.go`, `cmd/andamio/apikey.go` (`Short` gets a "Deprecated:" lead, and the group `Long` gets a paragraph naming the portal and 2.0.0)
  - `cmd/andamio/dev_portal_deprecation_test.go` (new)
- **Approach:**
  1. The table holds the nine command vars. Each already sets `RunE` in its struct literal, so wrapping at init is order-independent.
  2. The wrapper writes one line to stderr, then calls the original `RunE` and returns its error unchanged.
  3. Write the notice text once, as a constant. Tests read the constant and do not copy the text.
  4. Add the `Short` and `Long` deprecation text so `andamio --help` and `andamio dev --help` show it while every command stays listed.
- **Execution note:** Write the stdout-isolation test for `dev keys create` first. That pipe-safety guarantee is the easiest one to break.
- **Test scenarios:**
  - Table-driven over all nine paths: running the command through the root command against an `httptest` server prints the notice constant on stderr exactly once.
  - The same nine paths with `--output json`: stderr has the notice, stdout parses as the same JSON the command produced before.
  - `dev keys create --name x --environment preprod` in text mode: stdout is exactly the raw key and a newline, and the notice is on stderr only.
  - A command that fails auth (for example `dev keys list` with no dev JWT) still prints the notice, and the exit code and `kind` are unchanged (3 / `auth`).
  - `andamio dev keys list --help` does not print the notice.
  - `andamio --help` still lists `dev` and `apikey`, and `andamio dev --help` still lists all four subcommands plus `keys`.
  - A guard fails if the table and the `dev` and `apikey` subtrees disagree: every leaf under them is in the table, and every table entry is a leaf there. A future dev-portal command then cannot skip the notice.
- **Verification:** `go test ./cmd/andamio` passes, including `exitcode_test.go`, `dev_test.go` and `dev_keys_test.go` unchanged.

### U5. CHANGELOG, README and onboarding

- **Goal:** Release notes and onboarding say what changed and what retires in 2.0.0.
- **Requirements:** R7, plus the doc side of R4.
- **Dependencies:** U1, U3, U4.
- **Files:** `CHANGELOG.md`, `README.md`, `CLAUDE.md` (dev, apikey and `course owner register` rows, Auth Flow paragraph), `docs/andamio-cli-context.md`, `.claude/skills/getting-started/SKILL.md`.
- **Approach:**
  1. Under `## [Unreleased]`, add a `### Deprecated` entry for `dev` and `apikey` that names the portal URL and 2.0.0.
  2. Add a `### Changed` entry: `--tx-hash` is no longer sent, and module-status help and docs follow 2.6.0.
  3. Remove nothing from earlier versioned sections.
  4. In README, update the supported-API note. Run `curl -s https://preprod.api.andamio.io/` or the health/version route the gateway exposes, and confirm both networks report 2.6.0 before writing it. Keep "1.0 requires 2.5 or later" unless that check shows a reason to change it.
  5. Mark the `dev` and `apikey` rows in `CLAUDE.md` deprecated, with the removal release. Note in the Auth Flow paragraph that the dev slot is deprecated.
  6. Rewrite README sections 7 and 8 ("Set Modules to DRAFT for Content Import", "Re-Import Content into Linked Modules"). Linked modules are on chain, so 2.6.0 refuses the DRAFT step. Re-import into them updates lessons, introduction and assignment without it, and SLTs are skipped.
  7. Make the getting-started skill and `docs/andamio-cli-context.md` point to the portal for keys and stop recommending `dev login`.
- **Test expectation:** none, documentation. `scripts/changelog-section.sh Unreleased` or the release preflight still extracts the section.
- **Verification:** `grep -rn "dev login" README.md docs/andamio-cli-context.md .claude/skills` shows only deprecation-marked mentions.

---

## Verification Contract

| Gate | Command or check | Applies to |
|------|------------------|-----------|
| Build | `go build -o andamio ./cmd/andamio` | all |
| Full suite | `go test ./...` | all |
| Contract guards unchanged | `go test ./cmd/andamio -run 'ExitCode\|Retired\|Unknown\|ExpiredJWT'` passes with no edits to those test files | U1, U3, U4 |
| Help spot check | `./andamio course teacher register-module --help`, `./andamio dev --help`, `./andamio --help` | U1, U4 |
| Pipe safety | `./andamio dev keys create ... 2>/dev/null` prints only the key (preprod, dev JWT required) | U4, optional manual |
| Lifecycle on 2.6.0 | On preprod, `./andamio course teacher update-module-status --status PENDING_TX ...` is refused, and `--status APPROVED` with `--slt-hash` is accepted | U2, manual, James |

## Definition of Done

- Every unit's Verification holds, and `go test ./...` passes.
- cli#194's three checkboxes are each true: no removed module status in help or docs, `--tx-hash` ignored with removal named, and the `dev`/`apikey` notice with 2.0.0 named.
- No exit code, `kind` or JSON shape changed, except the `register-module` `PENDING_TX` case KTD3 approves, and no file in the Failure Contract guard table was edited.
- No leftover code from abandoned approaches stays in the diff, such as a half-applied cobra `Deprecated` attempt.
- The PR body references `cli#194` and `andamio-api` `v2.6.0` and contains no private paths.

---

## Appendix

### Sources

- andamio-api `v2.6.0`, `internal/orchestration/module_derivation.go`: `DeriveModuleStatus` maps on-chain to `ON_CHAIN` and stored `PENDING_TX`/`ON_CHAIN` to `APPROVED`.
- andamio-api `v2.6.0`, `internal/handlers/v2/db_handlers/course_v2_teacher_handlers.go`: update-status accepts `DRAFT` or `APPROVED` only, and `TeacherPublishModule` writes nothing.
- andamio-api `v2.6.0`, `internal/router/v2/merged_router.go`: `course-module/register` is still served, so `register-module` keeps working.
- andamio-api `v2.6.0`, `contract/api_scope.yaml`: `/api/v2/keys` routes are dev-portal internal, and `/api/v2/apikey/developer/*` stays because app-v2 calls it.
- `cmd/andamio/retired.go`: the `PersistentPreRunE` and help-visibility traps that KTD1 avoids.
- `docs/solutions/integration-issues/cli-dev-portal-dual-credential-pattern.md`: why `apikey` depends on the dev JWT.
