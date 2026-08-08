---
title: "Two draft-to-on-chain linking mechanisms: content-hash equality vs pending_tx_hash correlation"
date: 2026-08-08
category: architecture
module: internal/cardano
problem_type: architecture_pattern
component: tooling
severity: high
status: resolved
applies_when:
  - "researching how an off-chain draft record links to its on-chain counterpart"
  - "reviewing or writing docs that describe course-module or task body matching"
  - "diagnosing a state: failed confirm on a modules_manage or tasks_manage transaction"
  - "a docs/solutions search returns pending_tx_hash for a non-instance-creation transaction"
  - "building a command that derives a tx run body from a saved draft"
tags:
  - cardano
  - tx-linking
  - content-hash
  - pending-tx-hash
  - slt-hash
  - task-hash
  - register-module
  - docs-corpus
modules:
  - internal/cardano/slt_hash.go
  - internal/cardano/task_hash.go
  - cmd/andamio/course_teacher_ops.go
  - cmd/andamio/course_owner.go
  - cmd/andamio/project_owner.go
  - cmd/andamio/tx_run.go
pull_requests:
  - "#63: idempotent register-module on slt_hash match (closes #57)"
  - "#54: document chain/DB sync model and lifecycle workflows"
  - "#148: transaction-loop command inventory - the review that surfaced this"
---

# Two draft-to-on-chain linking mechanisms: content-hash equality vs pending_tx_hash correlation

## Context

During review of [andamio-cli#148](https://github.com/Andamio-Platform/andamio-cli/pull/148) — a docs-only transaction-loop command inventory, still open at the time of writing — a learnings-research agent was asked to check the PR's claim that `COURSE_TEACHER_MODULES_MANAGE` and `PROJECT_MANAGER_TASKS_MANAGE` each require the on-chain `tx run` body to be "hand-retyped to match the draft exactly (e.g. SLT text) or the gateway can't link the two records."

The agent searched this repo's `docs/solutions/` corpus, found the places where an off-chain record is linked to an on-chain transaction, and concluded that the linkage mechanism is `pending_tx_hash` — the off-chain POST carries the transaction hash and the gateway correlates on that. From that premise it followed, correctly, that retyping content to match "exactly" is not how anything gets linked, and therefore that the PR's warning was overstated. The conclusion was confident, specific, and supported by real citations from real files.

It was also wrong. A targeted re-check against source reversed it: `andamio-cli` has **two** distinct linkage mechanisms, and `pending_tx_hash` is not the one that governs `modules_manage` or `tasks_manage`. Those two transaction families are linked by **content-hash equality** — the on-chain transaction body's content *is* the hash preimage, so "the retyped body must match the draft exactly" and "the hashes must match" are the same statement approached from opposite ends. The PR's warning was accurate.

The reason the search failed is worth more than the fact it corrected. Every `pending_tx_hash` citation the agent found was genuine. `docs/solutions/integration-issues/cli-api-payload-mismatches.md:36` records that `course owner create` and `project owner create` require `course_id` + `pending_tx_hash`. `cli-onchain-commitment-commands-and-address-derivation.md` — since deleted, see below — explained that the on-chain transaction carries only an evidence hash while "the full Tiptap evidence document is sent to the REST API separately with `pending_tx_hash` to link the two." `docs/solutions/architecture/cli-tx-state-machine-pattern-and-task-hash-verification.md:44` discusses a `pending_tx_hash` POST that bypassed the state machine. Three independent documents, all accurate, all describing correlation-by-transaction-hash.

What the corpus did *not* contain, before this entry, was anything describing content-hash linkage for `modules_manage` or `tasks_manage`. At the time of the search, `grep -rln "slt_hash\|SLT hash" docs/solutions/` matched five files; every one used `slt_hash` as a request-payload identifier or an output-envelope field, not as the correlation key that binds a DB module to an on-chain module. The content-hash mechanism is documented — but in `docs/COURSE-LIFECYCLE.md` and `docs/PROJECT-LIFECYCLE.md`, which are workflow guides, not in the solutions corpus that a learnings search reads.

That is the hazard, and it is an arrangement problem rather than a missing-entry problem. The corpus does not merely stay silent on mechanism (1); its coverage of mechanism (2) is dense and well-cited enough that a reasonable semantic search terminates there with high confidence. A gap that produces "I don't know" is cheap. A gap that produces a confident wrong answer, sourced from correct citations, is expensive — and this one already cost a review cycle.

## Guidance

### Mechanism 1 — content-hash equality

**Governs:** `modules_manage` (`COURSE_TEACHER_MODULES_MANAGE`, built at `/v2/tx/course/teacher/modules/manage`) and `tasks_manage` (`PROJECT_MANAGER_TASKS_MANAGE`, `/v2/tx/project/manager/tasks/manage`).

There is no correlation identifier on the wire for these. The link key is a Blake2b-256 hash computed independently on both sides from the same content, and the on-chain transaction body carries that content directly. If the body you hand-type into `tx run` differs from the draft you saved earlier — by one character of SLT text, by a changed expiration, by an added token — the two sides compute different hashes and do not link.

For courses, the hash function is `ComputeSltHash` at `internal/cardano/slt_hash.go:18`. Its entire input is the ordered list of SLT strings: an indefinite-length CBOR array (`0x9f` … `0xff`) of Plutus builtin byte strings, one per SLT, chunked at 64 bytes to match `stringToBuiltinByteString` (`internal/cardano/slt_hash.go:39-58`). Nothing else contributes — not module code, not course ID, not title. The SLT texts *are* the identity.

The CLI computes this hash at import time and sends it: `cmd/andamio/course_import.go:1321-1329` computes `cardano.ComputeSltHash(data.SLTs)` and sets `payload["slt_hash"]`, but only when the SLTs are unlocked (module is `DRAFT`). Later, the `tx run` body carries the same SLT strings to the chain, and the two hashes must agree.

For projects, the function is `ComputeTaskHash` at `internal/cardano/task_hash.go:34`. Its input is four fields encoded as a Plutus Constructor 0 (`internal/cardano/task_hash.go:76-96`): NFC-normalized `project_content`, `expiration_time`, `lovelace_amount`, and the `native_assets` list. Note that native assets are inside the preimage — that specific omission is what caused the hash mismatch recorded in `docs/solutions/architecture/cli-tx-state-machine-pattern-and-task-hash-verification.md`.

**How to tell this mechanism applies:** the transaction body contains the *substance* of the off-chain record (SLT texts, task content/reward/deadline) rather than a reference to it. If the on-chain datum encodes the content, the content is the link.

**How failure surfaces — loudly.** For courses, the confirm produces `state: failed` rather than `state: updated` (`docs/COURSE-LIFECYCLE.md:167-169`), and the documented cause is exactly this ("the SLTs in the TX body do not match the imported SLTs", `docs/COURSE-LIFECYCLE.md:389`). The recovery path is `course teacher register-module --slt-hash <hash>`, whose `--slt-hash` flag is required (`cmd/andamio/course_teacher_ops.go:150-151`) and whose value is trimmed and rejected if empty, because an empty hash could silently match a NULL DB field (`cmd/andamio/course_teacher_ops.go:404-411`). The command is idempotent on match and errors on mismatch: `strings.EqualFold(existing.SltHash, sltHash)` at `cmd/andamio/course_teacher_ops.go:287-288` gates the outcome, with a `DRAFT` module advancing to `APPROVED` and an already-`APPROVED`/`PENDING_TX`/`ON_CHAIN` module returning a no-op. A mismatch returns `mismatchError` (`cmd/andamio/course_teacher_ops.go:417-423`), which prints both hashes on separate lines and names the destructive remedy:

```
module 101 already exists with slt_hash mismatch:
  stored:   <hash>
  supplied: <hash>
  gateway:  <wrapped error>
To replace, run:
  andamio course teacher delete-module --course-id <id> --module-code 101
```

The failure is detectable even before you attempt recovery: `course credential verify-hash` recomputes from the API's SLTs (`cmd/andamio/course_credential.go:135`), and `project task verify-hash` recomputes task hashes and reports mismatches (`cmd/andamio/project_task.go:807-812`). One caveat on the project side: `verify-hash` skips any item whose `task_hash` is empty — `continue // skip non-on-chain tasks` at `cmd/andamio/project_task.go:744-746` — so a draft that never made it on-chain is invisible to that check rather than reported as a mismatch.

### Mechanism 2 — `pending_tx_hash` correlation

**Governs:** instance creation — `INSTANCE_COURSE_CREATE` (`/v2/tx/instance/owner/course/create`) and `INSTANCE_PROJECT_CREATE` (`/v2/tx/instance/owner/project/create`).

Here the off-chain record is POSTed as a separate call that carries the transaction hash as an explicit link key. `cmd/andamio/course_owner.go:172-175` builds the create payload as `{"course_id": courseID, "pending_tx_hash": pendingTxHash}`, and both flags are marked required (`cmd/andamio/course_owner.go:111` and `:119`). `cmd/andamio/project_owner.go:127` does the same for projects. There is no content hashing anywhere in this path; the correlation is an opaque identifier that both calls agree on because the caller pastes the same string into both.

**How to tell this mechanism applies:** the off-chain payload contains a field literally named `pending_tx_hash`, and the on-chain body carries no copy of the off-chain content.

**How failure surfaces:** as a plain payload validation error from the gateway — a missing or wrong `pending_tx_hash` is a bad request, not a silent divergence. This is the class documented in `docs/solutions/integration-issues/cli-api-payload-mismatches.md`.

### The gateway side (verified, with caveats)

> Every `andamio-api/...` path in this section is in the **sibling gateway repo**, not in `andamio-cli`. Automated citation checks that resolve paths against this repo will flag them; that is expected, not a broken reference.

One question was open when this was written: whether the gateway matches `tasks_manage` purely by hash, given the `docs/TX-LOOP-TEST-RESULTS.md:59` note that a `tasks_manage` confirm "used index fallback." That is now traced, with the caveats stated below.

Reading `andamio-api` at the local `main` checkout (commit `55231641`), `tasks_manage` is **hash-first with two fallback layers**, not purely hash-matched:

1. The handler at `andamio-api/internal/service/tx_type_registry.go:804` builds a batch-status request in which each task carries *both* `TaskHash` and `Index`. The inline comment at `andamio-api/internal/service/tx_type_registry.go:868` is explicit: "Send both index and task_hash so the DB API can fall back to index-based matching when task_hash is NULL (see andamio-db-api-go#129)." That is the "index fallback" the TX-loop test observed.
2. If that call fails — HTTP ≥ 400, or HTTP 200 with `success: false` in the body (`andamio-api/internal/service/tx_type_registry.go:940-950`) — the handler falls back to `NewWholesaleTaskSync` (`andamio-api/internal/service/tx_type_registry.go:963`). Wholesale sync matches on-chain tasks to DB tasks by `(lovelace, expiration_posix)` (`andamio-api/internal/service/wholesale_task_sync.go:40-43`, matching loop at lines 91-135) and, on a hash difference, adopts the on-chain hash as truth (mismatch logged at `andamio-api/internal/service/wholesale_task_sync.go:123`, adopted at `:127`). This is the self-healing behavior `docs/PROJECT-LIFECYCLE.md:288` describes.

By contrast, `modules_manage` at `andamio-api/internal/service/tx_type_registry.go:224` has **no fallback**. It collects `slt_hash` values from the andamioscan event's create/delete arrays (`andamio-api/internal/service/tx_type_registry.go:245-277`), sends them to the batch-confirm endpoint (`andamio-api/internal/service/tx_type_registry.go:285`), and treats a partial failure as a hard error (`andamio-api/internal/service/tx_type_registry.go:299`). This matches `docs/TX-LOOP-TEST-RESULTS.md:101` ("Task confirm fallback to `(project_state_id, task_index)` works but project confirm has no equivalent fallback for `task_hash`") and matches `docs/COURSE-LIFECYCLE.md:471`: "The gateway matches them by SLT hash. If the DB module does not exist or has a different hash when the on-chain TX confirms, the automatic sync fails and manual recovery is needed."

**Three caveats on the gateway findings, which should not be dropped when this is quoted:**

- These line references are to a **local checkout of `andamio-api` `main`**, not to deployed code. This CLI's own `CLAUDE.md` records that mainnet has not cut over to API 2.5 (`andamio-ops#189`), so deployed behavior may differ from what `main` shows.
- The index fallback itself lives in **`andamio-db-api-go`**, not in the gateway. The gateway only *supplies* the index; how the DB API consumes it was not read here. The gateway's comment cites `andamio-db-api-go#129`, which is merged and titled "fix(gateway): resolve NULL taskHash matching in batch-status" — consistent with the gateway's account, but its implementation was not inspected.
- The `(lovelace, expiration_posix)` match key is not unique by construction. Two tasks with the same reward and deadline collide; the code handles this with a `usedDB` set and a first-unused-candidate rule (`andamio-api/internal/service/wholesale_task_sync.go:104-135`), which is order-dependent. The consequences of that were not analyzed here.

The practical takeaway is unchanged by the fallbacks: hash equality is the **primary** key for both families, the fallbacks are recovery paths that fire only after the hash path has already failed, and `modules_manage` has none at all. Writing a body that matches the draft remains the thing that makes the happy path work.

### The chaining gap is real

Nothing in the CLI derives a `tx run` body from a saved draft. `tx run` accepts only `--body` (inline JSON) or `--body-file` (a path), exactly one of the two (`cmd/andamio/tx_run.go:62-63`, validated at lines 94-99), and the body is parsed straight from that flag or file with no reference to any prior command's state (`cmd/andamio/tx_run.go:106-120`). Confirming the negative: `grep -rn "modules/manage\|tasks/manage" --include="*.go" cmd/ internal/` returns nothing — no CLI source file constructs either endpoint path, let alone its body. The only mentions are in `docs/COURSE-LIFECYCLE.md:159` and `docs/PROJECT-LIFECYCLE.md:143`, as worked examples the user copies and edits by hand.

So PR #148's gap is genuine. The correct framing is that it is a **hand-duplication risk with loud detection**, not a silent-corruption risk.

## Why This Matters

Believing the wrong mechanism produces two concrete failure modes, and the first has already happened once.

**A reviewer "corrects" accurate documentation.** This is the observed case. Armed with the `pending_tx_hash` model, the review conclusion was that the PR's warning about exact body matching was overstated — which, had it been acted on, would have edited a true and load-bearing warning out of a durable inventory document. The same reasoning would justify softening `docs/COURSE-LIFECYCLE.md:156` ("The SLT texts in the body must exactly match what was imported in Step 4") and `docs/COURSE-LIFECYCLE.md:471` ("The gateway matches them by SLT hash"). Both are correct. Both look like overcaution if you think a transaction hash is doing the linking. Removing them would strand the next operator with a `state: failed` confirm and no documented cause.

**Someone builds the chaining command against the wrong model.** The chaining gap is real and someone will eventually close it. Built on the content-hash model, the command reads the saved draft, emits SLT texts (or task content/expiration/lovelace/assets) byte-identically into the `tx run` body, and its correctness test is that `ComputeSltHash` over the emitted body equals the hash the draft already stored. Built on the `pending_tx_hash` model, the command instead threads a transaction hash through, treats the body's content as free-form, and ships something that builds and submits cleanly, then fails at confirm — after the transaction is already on chain and has already cost fees. For `modules_manage` there is no gateway fallback to catch it; recovery is a manual `register-module --slt-hash`, or, if the hashes genuinely diverge, the destructive `delete-module` path that `mismatchError` names.

There is a third, quieter cost: a wrong mechanism model makes the failure look *silent*, which mis-prioritizes the gap. If you think the two records are linked by an identifier, a content mismatch sounds like data drift nobody notices. Knowing it is a hash means the mismatch is loud — `state: failed`, a two-hash error message, a `verify-hash` diagnostic — which is why this belongs on the "ergonomics gap" list rather than the "correctness hazard" list.

## When to Apply

Reach for this when any of the following is true:

- You are reviewing or writing documentation that describes how an off-chain Andamio record links to its on-chain counterpart. Establish *which* family you are describing before you assert a mechanism.
- You are working on `modules_manage` or `tasks_manage` in any capacity — building the chaining command, changing what `course import` sends, altering `ComputeSltHash` or `ComputeTaskHash`, or touching `register-module`'s recovery logic.
- A search of `docs/solutions/` returns `pending_tx_hash` in response to a question about linkage, and the transaction under discussion is *not* instance creation. That is the exact trap; treat the result as a false positive and go to the source.
- You are diagnosing a `state: failed` confirm on a modules or tasks transaction. The first hypothesis should be hash inequality from a hand-typed body, not a missing correlation field.
- More generally: any time a corpus search returns a confident answer built from citations that are individually correct but were written about a *different* subsystem than the one you asked about. The tell is that every citation checks out and none of them is about your case.

## Examples

**Worked trace — the hash path (`modules_manage`).**

Step 1, the draft. `andamio course import --create` reads `outline.md`, computes `ComputeSltHash(["Describe the protocol architecture", "Build a basic transaction", "Verify credential hashes"])` at `cmd/andamio/course_import.go:1323`, and POSTs it as `payload["slt_hash"]` alongside the SLT texts. The DB now holds a `DRAFT` module whose `slt_hash` is a Blake2b-256 digest of those three strings and nothing else.

Step 2, `PENDING_TX`. `andamio course teacher update-module-status --status PENDING_TX` — required before the gateway will confirm to `ON_CHAIN` (`docs/COURSE-LIFECYCLE.md:148`).

Step 3, the chain. The operator hand-types (`docs/COURSE-LIFECYCLE.md:159`):

```bash
andamio tx run /v2/tx/course/teacher/modules/manage \
  --body '{"alias":"<alias>","course_id":"<id>","modules":[{"module_code":"101","slts":["Describe the protocol architecture","Build a basic transaction","Verify credential hashes"]}]}' \
  --skey ./payment.skey --tx-type modules_manage
```

Note what is *not* in that body: no `slt_hash`, no `pending_tx_hash`, no draft record ID. The three strings under `slts` are the only thing tying this transaction to the DB record.

Step 4, the confirm. The gateway pulls `slt_hash` from the andamioscan event (`andamio-api/internal/service/tx_type_registry.go:254`) and batch-confirms against the DB by that hash (`andamio-api/internal/service/tx_type_registry.go:285`). Match → `PENDING_TX` becomes `ON_CHAIN`, response `state: updated`. No match → `state: failed`, no fallback.

**Before / after — the failure and the recovery.** Suppose the operator typed `"Verify credential hashes."` with a trailing period. The transaction builds, signs, and submits without complaint; the trailing period changes one byte of the CBOR preimage and therefore the whole digest. The confirm returns `state: failed`. Recovery, per `docs/COURSE-LIFECYCLE.md:414-418`:

```bash
andamio course teacher register-module --course-id <id> --module-code 101 --slt-hash <on-chain-hash>
```

Because the DB row's stored hash is the no-period version, `strings.EqualFold` at `cmd/andamio/course_teacher_ops.go:287` fails and the command exits non-zero printing both hashes. The operator's choice is then to fix the local outline and re-mint, or `delete-module` and re-import. Either way they were *told*.

Had the SLT texts matched, the same command would have compared equal, advanced the `DRAFT` to `APPROVED`, and exited 0 — the idempotent-on-match branch at `cmd/andamio/course_teacher_ops.go:291-330`.

**Contrast — the `pending_tx_hash` path (`INSTANCE_COURSE_CREATE`).**

```bash
# 1. Mint on-chain. Body carries no off-chain content.
andamio tx run /v2/tx/instance/owner/course/create --body '{...}' --skey ./payment.skey --tx-type course_create
# → returns tx hash abc123...

# 2. Create the off-chain record, carrying the tx hash as the explicit link key.
andamio course owner create --course-id <id> --pending-tx-hash abc123... --title "My Course"
```

The payload built at `cmd/andamio/course_owner.go:172-175` is `{"course_id": ..., "pending_tx_hash": "abc123..."}` plus optional metadata. Nothing here is hashed, nothing needs to match byte-for-byte, and the title can be edited freely afterward via `course owner update` without disturbing the link. This is the mechanism the corpus documents — and it is genuinely how instance creation works. It is simply not how modules and tasks work.

## Related

**Contributed to the wrong conclusion** (each is accurate about its own subject; the hazard is that a linkage query lands here and stops):

- `docs/solutions/integration-issues/cli-api-payload-mismatches.md` — lines 36 and 40 establish `pending_tx_hash` as a required payload field on owner-create and on the (since-retired) student leave/claim commands. The single strongest attractor for a linkage query.
- `cli-onchain-commitment-commands-and-address-derivation.md` — **deleted 2026-08-08**, in the refresh that followed this doc. Its "Two-Phase Commit" section described the commitment/evidence flow, where an on-chain evidence hash and an off-chain document were joined by `pending_tx_hash`, and framed that as a general pattern rather than one scoped to those commands — which made it the strongest single attractor for the wrong answer. Every mechanism it documented had since been removed from the CLI (the `commit-tx` commands, the `course student` / `project contributor` surface, and `internal/cardano/address.go`), so it was deleted rather than rescoped. Recover with `git log --diff-filter=D -- docs/solutions/` if the history is ever needed.
- `docs/solutions/architecture/cli-tx-state-machine-pattern-and-task-hash-verification.md:44` — the `commit-tx` removal, which turns on a `pending_tx_hash` POST that bypassed the state machine.

**Closest sibling, and the one that should have carried mechanism (1):**

- `docs/solutions/architecture/cli-tx-state-machine-pattern-and-task-hash-verification.md` — already covers `ComputeTaskHash` (`internal/cardano/task_hash.go`) and `project task verify-hash`, and already documents a real hash-mismatch bug (native assets omitted from the datum). It is the single closest entry in the corpus, and it still does not say that hash equality is the *linkage key* for `tasks_manage`. It frames hashing as verification, not as correlation. That framing is a large part of why the search missed.

**Also relevant:**

- `docs/solutions/architecture/typed-output-envelope-with-gateway-state-fallbacks.md` — covers `register-module`'s `RegisterModuleEnvelope` and its gateway-state fallbacks, i.e. the recovery command's output contract.
- `docs/solutions/integration-issues/evidence-submission-payload-format-and-field-alignment.md` — another `slt_hash`/`task_hash` user, in the request-identifier sense rather than the linkage sense. Useful as a reminder that these field names do double duty.

**Non-solutions sources that hold the ground truth** (worth citing directly rather than relying on the corpus):

- `docs/COURSE-LIFECYCLE.md:156, 165, 389-391, 414-418, 471` — the authoritative statement of the SLT-hash linkage and its recovery path.
- `docs/PROJECT-LIFECYCLE.md:288, 324` — the task-side parallel. Note line 3 claims projects have "no SLT hash complexity," which is true only in the narrow sense that tasks have no SLTs; task hashing is its own complexity.
- `docs/TX-LOOP-TEST-RESULTS.md:59, 101` — the empirical record of the index fallback, and the observation that module confirm has no equivalent.
