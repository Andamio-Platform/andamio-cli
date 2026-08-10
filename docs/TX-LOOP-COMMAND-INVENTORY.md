# Transaction-Loop Command Inventory

**Date:** 2026-08-05, updated 2026-08-07
**Scope:** cross-reference every transaction type defined by the template's transaction-loop against the CLI commands that build them.

## Source of truth

[`andamio-app-template`](https://github.com/Andamio-Platform/andamio-app-template)'s `src/config/transaction-ui.ts` defines the canonical `TransactionType` union — 18 transaction types, each mapped to a fixed gateway endpoint in `TRANSACTION_ENDPOINTS`. Two skills in that repo describe the state machines these transactions move through: `.skills/transactions` (the general `BUILD → SIGN → SUBMIT → REGISTER → WATCH` pipeline, see [TX-LIFECYCLE.md](TX-LIFECYCLE.md)) and `.skills/task-lifecycle` (the contributor `COMMIT → SUBMIT → REVIEW → ASSESS` flow).

`TX-LIFECYCLE.md`'s own transaction table lists 17, not 18 — a different axis, not a discrepancy. `TransactionType` is one entry per tx-*building* endpoint; `tx_type` is the gateway's separate registration/tracking enum (`andamio-api`'s `internal/common/constants/tx_state_machine.go`, the value passed to `tx register --tx-type`). The one endpoint with no matching `tx_type` is `GLOBAL_USER_ACCESS_TOKEN_CLAIM` — its route is registered (`internal/router/v2/claim_access_token_router.go`) but nothing in the tracking enum names it, which is the same row marked a true gap below for having no CLI command either.

`TRANSACTION_ENDPOINTS`'s paths all start with `/api` (e.g. `/api/v2/tx/project/manager/tasks/manage`). `tx build`/`tx run` add `/api` themselves, so pass the path without it — `/v2/tx/project/manager/tasks/manage` — or the request doubles up to `/api/api/v2/tx/...` and fails. Every command example in this doc already uses the stripped form.

Each of the 18 endpoint paths was cross-referenced against `cmd/andamio/*.go`'s command implementations and, where relevant, against `COURSE-LIFECYCLE.md`/`PROJECT-LIFECYCLE.md`. `COURSE_TEACHER_MODULES_MANAGE` and `PROJECT_MANAGER_TASKS_MANAGE` are each a two-step workflow: a dedicated command handles the draft/status half over REST, then a separate, undedicated `tx run` call handles the on-chain half. `PROJECT_MANAGER_TASKS_ASSESS` has no draft step and no dedicated command, but has a fully documented `tx run` example, plus a `tx build`/`sign`/`submit` split for a human-review gate before signing. The 4 true-gap endpoints, and the 6 endpoints behind the `COURSE_STUDENT_*`/`PROJECT_CONTRIBUTOR_*` types, are registered in `andamio-api`'s `main` branch (`internal/router/v2/tx_router.go`, `internal/router/v2/claim_access_token_router.go`) as of this doc's last update. This confirms the routes exist in source, not that preprod or mainnet are currently running that code — an unmatched gateway path returns 401 the same as a matched-but-unauthorized one, so an HTTP probe can't tell the two apart.

**"No mention anywhere" does not mean "impossible via the CLI."** `andamio tx build <endpoint> --body <json>` (`cmd/andamio/tx_build.go`) is a fully generic passthrough with no endpoint allowlist — any of the 18 paths can be hit today by hand-typing the exact URL and constructing the JSON body from scratch, with no flags, validation, or help text to guide it. "True gap" below means *no dedicated, documented, discoverable command* — not that the transaction is unreachable.

## Summary

| Transaction Type | CLI Status |
|---|---|
| `GLOBAL_GENERAL_ACCESS_TOKEN_MINT` | No dedicated command — hand-build the body and call `tx build /v2/tx/global/user/access-token/mint` (then `sign`/`submit`/`register`, or `tx run` for all of it at once). Only appears as an example in `tx build --help`, no worked walkthrough in a lifecycle doc — low priority |
| `GLOBAL_USER_ACCESS_TOKEN_CLAIM` | **True gap** — no mention anywhere |
| `INSTANCE_COURSE_CREATE` | No dedicated command — full worked example using `tx run /v2/tx/instance/owner/course/create` in `COURSE-LIFECYCLE.md` — low priority |
| `INSTANCE_PROJECT_CREATE` | No dedicated command — full worked example using `tx run /v2/tx/instance/owner/project/create` in `PROJECT-LIFECYCLE.md` — low priority |
| `COURSE_OWNER_TEACHERS_MANAGE` | Dedicated (`course owner teachers`, fixed in #140) |
| `COURSE_TEACHER_MODULES_MANAGE` | Draft step dedicated (`course teacher update-module-status`), on-chain mint only reachable via generic `tx run /v2/tx/course/teacher/modules/manage` — body must be hand-retyped to match the draft exactly (e.g. SLT text) or the gateway can't link the two records; no command chains them |
| `COURSE_TEACHER_ASSIGNMENTS_ASSESS` | Dedicated (`teacher assessment build`) |
| `COURSE_STUDENT_ASSIGNMENT_COMMIT` | Out of CLI scope — handled by the [Andamio app](https://app.andamio.io). 1.0 removed the `course student` command surface; the gateway route is unaffected and live |
| `COURSE_STUDENT_ASSIGNMENT_UPDATE` | Out of CLI scope, same as above |
| `COURSE_STUDENT_CREDENTIAL_CLAIM` | Out of CLI scope, same as above |
| `PROJECT_OWNER_MANAGERS_MANAGE` | **True gap** — `project owner` has list/create/update/register, nothing for managers. Endpoint appears in an internal wallet-signing brainstorm doc, not in any user-facing docs or help text |
| `PROJECT_OWNER_BLACKLIST_MANAGE` | **True gap** — no CLI command. Endpoint appears in the same internal brainstorm doc, not in any user-facing docs or help text |
| `PROJECT_MANAGER_TASKS_MANAGE` | Same shape as `COURSE_TEACHER_MODULES_MANAGE` — draft step dedicated (`project task create/update/delete`), mint only via generic `tx run /v2/tx/project/manager/tasks/manage`, same hand-duplication risk, no chaining command |
| `PROJECT_MANAGER_TASKS_ASSESS` | No dedicated command — `project manager` only has read-only `commitments`/`qualified-contributors`. Documented via `tx run /v2/tx/project/manager/tasks/assess`, plus a `tx build`/`sign`/`submit` split to keep a human between the recommendation and the signature |
| `PROJECT_CONTRIBUTOR_TASK_COMMIT` | Out of CLI scope — handled by the [Andamio app](https://app.andamio.io). 1.0 removed the `project contributor` command surface; the gateway route is unaffected and live |
| `PROJECT_CONTRIBUTOR_TASK_ACTION` | Out of CLI scope, same as above |
| `PROJECT_CONTRIBUTOR_CREDENTIAL_CLAIM` | Out of CLI scope — handled by the [Andamio app](https://app.andamio.io). Unlike the other two `project contributor` rows, this one has no retired stub in `retired.go`; the pre-1.0 `project contributor` command group never had a `claim` subcommand, so this was never CLI-shaped to begin with, not removed in 1.0 |
| `PROJECT_USER_TREASURY_ADD_FUNDS` | **True gap** — no CLI command. Endpoint appears in the same internal brainstorm doc, not in any user-facing docs or help text. Also listed by name only as row 17 ("Fund Treasury") in `TX-LIFECYCLE.md`'s transaction table, role `User` — same role tag as `GLOBAL_GENERAL_ACCESS_TOKEN_MINT`, not restricted to the Andamio app |

## Finding

Every Manager-role transaction and every Owner-role project-management transaction is either missing a dedicated command entirely, or reachable only through a generic, hand-typed `tx run`/`tx build` call — despite 1.0's own `CHANGELOG.md` describing the CLI as being "for the people who author work and assess it: course Owners and Teachers, and project Managers." Managers are the stated primary audience, and most of their transaction surface has no dedicated on-chain command (`PROJECT_MANAGER_TASKS_MANAGE`'s mint step, `PROJECT_MANAGER_TASKS_ASSESS`) or none at all (`PROJECT_OWNER_MANAGERS_MANAGE`, `PROJECT_OWNER_BLACKLIST_MANAGE`). Two of these — `COURSE_TEACHER_MODULES_MANAGE` and `PROJECT_MANAGER_TASKS_MANAGE` — carry a specific risk worth calling out on their own: the on-chain body has to be hand-retyped to exactly match data the draft command already saved, with nothing chaining the two calls or validating that they match. The 6 "out of scope" rows above are not part of this problem — those are a deliberate 1.0 decision affecting a different audience (learners/contributors), not a gap in the CLI's own stated audience.

## Command surface, in context

The CLI has **118 commands** in total (excluding the root), per the live `cobra.Command` tree on `main` — counted 2026-08-08. Of those:

- **53 make no network call**: 17 retired stubs (`course student *`, `project contributor *`, including the two group entries themselves), 21 group commands (`andamio course`, `andamio config`, etc. — print help or an unknown-subcommand error), 14 local leaf commands (`config` ×5, `auth login`, `auth status`, `tx sign`, `user logout`, `user status`, `dev logout`, `dev status`, `course credential compute-hash`, `project task compute-hash`), and `andamio exit-codes` — the one command in the tree with no `RunE` at all.
- **65 make a network call**: 64 hit the Andamio API — the overwhelming majority of it ordinary REST CRUD (list/get/create/update/delete on courses, projects, modules, tasks, dev keys, users, auth) — plus `tx submit`, which posts to a configurable Cardano submit endpoint rather than the Andamio API.
- **Only 2 of the 64 Andamio-API-calling commands POST to one of the 18 canonical tx-building endpoints** (`course owner teachers`, `teacher assessment build`). Of the remaining 16: 4 have no dedicated command at all and are reachable only by hand-typing a generic `tx build`/`tx run` call (`GLOBAL_GENERAL_ACCESS_TOKEN_MINT`, `INSTANCE_COURSE_CREATE`, `INSTANCE_PROJECT_CREATE`, `PROJECT_MANAGER_TASKS_ASSESS`); 2 more (`COURSE_TEACHER_MODULES_MANAGE`, `PROJECT_MANAGER_TASKS_MANAGE`) pair a dedicated draft/status command with that same generic call for the on-chain mint step; 4 are true gaps with no example at all; and the remaining 6 are the out-of-scope student/contributor rows.

In other words: the CLI is structurally a REST client with a thin, mostly-generic transaction-building layer bolted on, not a transaction-first tool — which tracks with the coverage gaps above.

**No hot-wallet integration for transaction signing.** `tx sign` (`cmd/andamio/tx_sign.go`, `internal/cardano/sign.go`) requires `--skey <path>`, a local `cardano-cli` JSON key-envelope file — there's no CIP-30 flow for signing a transaction. `user login` and `dev login` do open a browser to a wallet-connect page (`/auth/cli`, `/auth/dev-cli`) — but that's for authentication, a separate flow from transaction signing. A developer still has to generate and hold a raw signing key on disk to drive any transaction through the CLI, which raises the bar for using the tx-building commands that do exist independent of the coverage gaps above.

## Scope

This inventory is the deliverable itself — capturing where the CLI's transaction coverage stands today. Closing the 7 gaps found here (2 commands to chain the existing draft step to its `tx run` mint call, 1 new command for `PROJECT_MANAGER_TASKS_ASSESS` — no draft step to chain, but no dedicated command of its own either, despite being fully documented — and 4 new commands for the true gaps), and the separate no-hot-wallet finding above, are unscoped future work that this inventory surfaces but does not include.
