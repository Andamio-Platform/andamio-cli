# Transaction-Loop Command Inventory

**Date:** 2026-08-05, updated 2026-08-07
**Scope:** cross-reference every transaction type defined by the template's transaction-loop against the CLI commands that build them.

## Source of truth

[`andamio-app-template`](https://github.com/Andamio-Platform/andamio-app-template)'s `src/config/transaction-ui.ts` defines the canonical `TransactionType` union — 18 transaction types, each mapped to a fixed gateway endpoint in `TRANSACTION_ENDPOINTS`. Two skills in that repo describe the state machines these transactions move through: `.skills/transactions` (the general `BUILD → SIGN → SUBMIT → REGISTER → WATCH` pipeline, see [TX-LIFECYCLE.md](TX-LIFECYCLE.md)) and `.skills/task-lifecycle` (the contributor `COMMIT → SUBMIT → REVIEW → ASSESS` flow).

Each of the 18 endpoint paths was grepped for across `cmd/andamio/*.go`, then every apparent gap was checked against the actual command implementation rather than trusting the grep alone — two cases turned out to be miscategorized on a first pass (commands that exist but call the wrong endpoint, not commands that are missing). The 5 "true gap" endpoints, and the 3 endpoints behind the `COURSE_STUDENT_*`/`PROJECT_CONTRIBUTOR_*` types, were separately confirmed as real, live, registered routes against a fresh `andamio-api` pull (`internal/router/v2/tx_router.go`, `internal/router/v2/claim_access_token_router.go`) — none of this is dead or planned-but-unbuilt API surface.

**"No mention anywhere" does not mean "impossible via the CLI."** `andamio tx build <endpoint> --body <json>` (`cmd/andamio/tx_build.go`) is a fully generic passthrough with no endpoint allowlist — any of the 18 paths can be hit today by hand-typing the exact URL and constructing the JSON body from scratch, with no flags, validation, or help text to guide it. "True gap" below means *no dedicated, documented, discoverable command* — not that the transaction is unreachable.

## Summary

| Transaction Type | CLI Status |
|---|---|
| `GLOBAL_GENERAL_ACCESS_TOKEN_MINT` | Generic only (`tx build`), documented in help text — low priority |
| `GLOBAL_USER_ACCESS_TOKEN_CLAIM` | **True gap** — no mention anywhere |
| `INSTANCE_COURSE_CREATE` | Generic only, documented in help text — low priority |
| `INSTANCE_PROJECT_CREATE` | Generic only, documented in help text — low priority |
| `COURSE_OWNER_TEACHERS_MANAGE` | Dedicated (`course owner teachers`, fixed in #140) |
| `COURSE_TEACHER_MODULES_MANAGE` | **Wrong endpoint, not missing** — `register/publish/delete/update-module-status` all exist but POST to `/api/v2/course/teacher/course-module/*` (REST proxy), not the tx path. Same bug class `#140` already fixed for `course owner teachers`. |
| `COURSE_TEACHER_ASSIGNMENTS_ASSESS` | Dedicated (`teacher assessment build`) |
| `COURSE_STUDENT_ASSIGNMENT_COMMIT` | Out of CLI scope, not retired from the product — handled by the [Andamio app](https://app.andamio.io). 1.0 removed the `course student` command surface; the gateway route is unaffected and live |
| `COURSE_STUDENT_ASSIGNMENT_UPDATE` | Out of CLI scope, same as above |
| `COURSE_STUDENT_CREDENTIAL_CLAIM` | Out of CLI scope, same as above |
| `PROJECT_OWNER_MANAGERS_MANAGE` | **True gap** — `project owner` has list/create/update/register, nothing for managers |
| `PROJECT_OWNER_BLACKLIST_MANAGE` | **True gap** — no mention anywhere |
| `PROJECT_MANAGER_TASKS_MANAGE` | **Wrong endpoint, not missing** — `project task create/update/delete` exist but POST to `/api/v2/project/manager/task/*` (REST proxy), same bug class as above |
| `PROJECT_MANAGER_TASKS_ASSESS` | **True gap** — `project manager` only has read-only `commitments`/`qualified-contributors` |
| `PROJECT_CONTRIBUTOR_TASK_COMMIT` | Out of CLI scope, not retired from the product — handled by the [Andamio app](https://app.andamio.io). 1.0 removed the `project contributor` command surface; the gateway route is unaffected and live |
| `PROJECT_CONTRIBUTOR_TASK_ACTION` | Out of CLI scope, same as above |
| `PROJECT_CONTRIBUTOR_CREDENTIAL_CLAIM` | Out of CLI scope, same as above |
| `PROJECT_USER_TREASURY_ADD_FUNDS` | **True gap** — no mention anywhere |

**Tally:** 2 dedicated · 6 out of CLI scope by design (learner/contributor roles served by the app, not retired — intentional 1.0 CLI scope cut, per `CHANGELOG.md`: "a change to the CLI, not the API... scoped out, not ruled out") · 3 generic-but-low-priority · 2 wrong-endpoint (same class as `#140` — existing commands need porting to the tx lifecycle, not new builds) · 5 true gaps (`GLOBAL_USER_ACCESS_TOKEN_CLAIM`, `PROJECT_OWNER_MANAGERS_MANAGE`, `PROJECT_OWNER_BLACKLIST_MANAGE`, `PROJECT_MANAGER_TASKS_ASSESS`, `PROJECT_USER_TREASURY_ADD_FUNDS`)

## Finding

Every Manager-role transaction and every Owner-role project-management transaction has either no dedicated CLI support or hits the wrong (REST-proxy) endpoint — despite 1.0's own `CHANGELOG.md` describing the CLI as being "for the people who author work and assess it: course Owners and Teachers, and project Managers." Managers are the stated primary audience, and most of their transaction surface is either broken in the same way `#140` was, or doesn't exist yet. The 6 "out of scope" rows above are not part of this problem — those are a deliberate 1.0 decision affecting a different audience (learners/contributors), not a gap in the CLI's own stated audience.

## Command surface, in context

The CLI has **98 executable commands** in total (every `cobra.Command` with a `RunE`). Of those:

- **~10 are purely local** — no network call at all (`config *` x5, `tx sign`, the bare `manager`/`teacher` group commands, the root command, the removed-command handler).
- **~88 call the API**, but the overwhelming majority of that is ordinary REST CRUD — list/get/create/update/delete on courses, projects, modules, tasks, dev keys, users, auth.
- **Only 2 of those ~88 POST to one of the 18 canonical tx-building endpoints** (`course owner teachers`, `teacher assessment build`). The rest of the tx-building surface is either the 3 generic-passthrough examples above or entirely absent from dedicated commands.

In other words: the CLI is structurally a REST client with a thin, mostly-generic transaction-building layer bolted on, not a transaction-first tool — which tracks with the coverage gaps above.

**No hot-wallet integration.** `tx sign` (`cmd/andamio/tx_sign.go`, `internal/cardano/sign.go`) requires `--skey <path>`, a local `cardano-cli` JSON key-envelope file. There is no CIP-30 / browser-wallet connect flow anywhere in the CLI — that only exists in the app. A developer has to generate and hold a raw signing key on disk to drive any transaction through the CLI at all, which raises the bar for using the tx-building commands that do exist independent of the coverage gaps above.

## Scope

This inventory is the deliverable itself — capturing where the CLI's transaction coverage stands today. Closing the 7 gaps found here (2 endpoint migrations + 5 new commands), and the separate no-hot-wallet finding above, are unscoped future work that this inventory surfaces but does not include.
