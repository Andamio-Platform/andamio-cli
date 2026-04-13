# Course Lifecycle

Step-by-step guide for creating and managing courses with the Andamio CLI, ensuring on-chain and off-chain (database) state stay in sync throughout.

For the underlying transaction pipeline and recovery procedures, see [TX-LIFECYCLE.md](TX-LIFECYCLE.md).

## Prerequisites

Before starting, configure the CLI with all required credentials:

```bash
# Store your API key (read access)
andamio auth login --api-key <your-api-key>

# Authenticate with your wallet (edit access)
andamio user login

# Set the Cardano submit endpoint
andamio config set-submit-url https://cardano-preprod.blockfrost.io/api/v0/tx/submit

# If using Blockfrost, set the project ID header
andamio config set-submit-header project_id <your-blockfrost-project-id>
```

You also need a Cardano `.skey` file for signing transactions.

Verify your setup:

```bash
andamio user status
andamio config show
```

## Module Status Lifecycle

Modules move through four states. Understanding these states is critical for knowing which operations are allowed at each stage.

```
DRAFT --> APPROVED --> PENDING_TX --> ON_CHAIN
```

| Status | SLTs editable? | Content editable? | How it transitions |
|--------|---------------|-------------------|-------------------|
| **DRAFT** | Yes | Yes | Created by `course import --create`. Full import works. |
| **APPROVED** | No (locked) | Yes | Set by `register-module` or automatic gateway sync after `modules_manage` TX. |
| **PENDING_TX** | No (locked) | Yes | Set by `update-module-status --status PENDING_TX` before submitting the `modules_manage` TX. **Required** — the gateway will not advance a module to ON_CHAIN unless it is in PENDING_TX when the TX confirms. |
| **ON_CHAIN** | No (locked) | Yes | Set by gateway batch confirm when the `modules_manage` TX reaches `updated`. Module has on-chain counterpart and merged DB record. |

Once SLTs are locked, `course import` skips sending SLT data to avoid `SLT_LOCKED` errors. Lessons, introduction, and assignment content remain editable at all statuses.

## Course Creation Workflow

### Step 1: Create course on-chain

Mint the course instance on Cardano. The `tx run` command handles the full build-sign-submit-register-poll pipeline.

```bash
andamio tx run /v2/tx/instance/owner/course/create \
  --body '{"alias":"<your-alias>"}' \
  --skey ./payment.skey \
  --tx-type course_create \
  --instance-id <course-id>
```

The build response includes the `course_id`. Save it -- every subsequent command references it.

**Verify**: Wait for `state: updated` in the poll output, confirming both chain and DB are in sync.

### Step 2: Register course in the database

Create the off-chain course record with metadata:

```bash
andamio course owner register --course-id <course-id> --title "My Course"
```

Optional flags: `--description`, `--image-url`, `--video-url`, `--category`, `--public`.

**Verify**: Confirm the course exists:

```bash
andamio course get <course-id>
```

### Step 3: Prepare module content locally

Create a compiled module directory with the following structure:

```
compiled/my-course/101/
  outline.md            # Required: frontmatter + SLT list
  lesson-1.md           # One per SLT
  lesson-2.md
  introduction.md       # Optional
  assignment.md         # Optional
  assets/               # Optional: images
    diagram.png
```

**outline.md** format:

```markdown
---
title: "Module 101: Foundations"
code: "101"
description: "Introduction to core concepts"
image_url: ""
video_url: ""
---

## SLTs

1. Describe the protocol architecture
2. Build a basic transaction
3. Verify credential hashes
```

Each `lesson-N.md` corresponds to the Nth SLT. The first `# Heading` in each file becomes the lesson title; the remaining content becomes the body.

### Step 4: Import content to create a DRAFT module

Import the compiled directory. The `--create` flag creates the module if it does not already exist in the database:

```bash
andamio course import ./compiled/my-course/101 --course-id <course-id> --create
```

This command:
1. Parses `outline.md` to extract the module code, SLTs, and metadata.
2. Creates the module in **DRAFT** status.
3. Computes and stores the SLT hash (Blake2b-256 of the Plutus-encoded SLT list).
4. Uploads lessons, introduction, and assignment content.
5. Uploads any new images from `assets/` to the CDN.

Note the SLT hash from the output. You can also compute it independently:

```bash
andamio course credential compute-hash --file ./compiled/my-course/101/outline.md
```

**Verify**: Check that the module appears:

```bash
andamio course modules <course-id>
```

### Step 5: Set module to PENDING_TX and mint on-chain

Before submitting the on-chain transaction, advance the module to `PENDING_TX`. The gateway requires this status before it can confirm the module to `ON_CHAIN`.

```bash
andamio course teacher update-module-status \
  --course-id <course-id> --module-code 101 --status PENDING_TX
```

Then submit the on-chain transaction. The SLT texts in the body must exactly match what was imported in Step 4.

```bash
andamio tx run /v2/tx/course/teacher/modules/manage \
  --body '{"alias":"<your-alias>","course_id":"<course-id>","modules":[{"module_code":"101","slts":["Describe the protocol architecture","Build a basic transaction","Verify credential hashes"]}]}' \
  --skey ./payment.skey \
  --tx-type modules_manage
```

When the transaction confirms, the gateway matches the on-chain module to the DB record by SLT hash and advances it from `PENDING_TX` to `ON_CHAIN`.

**Verify**: A `state: updated` response means the gateway successfully matched and synced. Proceed to Step 7.

If the response shows `state: failed`, the gateway could not match the module. Proceed to Step 6.

**If the TX fails to build or submit**: Reset the module back to `APPROVED` so it is not stuck in `PENDING_TX`:

```bash
andamio course teacher update-module-status \
  --course-id <course-id> --module-code 101 --status APPROVED
```

### Step 6: Register module (recovery only)

This step is only needed if Step 5 returned `state: failed`, meaning the gateway could not automatically link the on-chain module to the DB record.

Manually link them by providing the SLT hash:

```bash
andamio course teacher register-module \
  --course-id <course-id> \
  --module-code 101 \
  --slt-hash <hash-from-step-4>
```

This sets the module status to **APPROVED**, which locks SLTs.

### Step 7: Publish module

Publish the module to finalize its on-chain/off-chain link:

```bash
andamio course teacher publish-module --course-id <course-id> --module-code 101
```

Look for a "merged" source signal in the response, which confirms the on-chain and DB records are fully synchronized.

### Step 8: Update content (ongoing)

After modules are published, you can continue updating lessons, introduction, and assignment content. SLTs are locked and will be skipped automatically.

```bash
andamio course import ./compiled/my-course/101 --course-id <course-id>
```

To import all modules in a course at once:

```bash
andamio course import-all ./compiled/my-course --course-id <course-id>
```

## Assignment Commitment Lifecycle

Assignment commitments track a student's progress through a module. They use different status names and transitions from project task commitments.

```
AWAITING_SUBMISSION → PENDING_TX_COMMIT → SUBMITTED
                                              → PENDING_TX_ASSESS → ACCEPTED / REFUSED
                                              → PENDING_TX_LEAVE  → LEFT
ACCEPTED → PENDING_TX_CLAIM → CREDENTIAL_CLAIMED
REFUSED  → PENDING_TX_COMMIT → SUBMITTED  (resubmit via new commit TX)
```

**Key difference from project tasks:** Assignments have **no `PENDING_TX_SUBMIT` status**. On-chain, commit and submit use the same redeemer. Evidence resubmission requires a full new commit TX through `PENDING_TX_COMMIT`, not a separate update TX.

Terminal states (no further transitions):
- **CREDENTIAL_CLAIMED** — credential NFT claimed by student
- **LEFT** — student left the module voluntarily

## Student Enrollment and Assignments

### Enroll in a course module (on-chain)

```bash
andamio tx run /v2/tx/course/student/enroll \
  --body '{"alias":"<student-alias>","course_id":"<course-id>","course_module_code":"101"}' \
  --skey ./payment.skey \
  --tx-type course_enroll
```

### Submit assignment evidence

```bash
andamio course student submit \
  --course-id <course-id> \
  --module-code 101 \
  --evidence-file ./evidence.md
```

Or inline:

```bash
andamio course student submit \
  --course-id <course-id> \
  --module-code 101 \
  --evidence "Completed all exercises. See repo: https://github.com/..."
```

### Teacher reviews a submission

```bash
andamio course teacher review \
  --course-id <course-id> \
  --module-code 101 \
  --participant-alias <student-alias> \
  --decision accept
```

List pending reviews:

```bash
andamio course teacher commitments --course-id <course-id>
```

### Claim credential (on-chain)

After the teacher accepts the submission:

```bash
andamio tx run /v2/tx/course/student/credential/claim \
  --body '{"alias":"<student-alias>","course_id":"<course-id>","course_module_code":"101"}' \
  --skey ./payment.skey \
  --tx-type credential_claim
```

## Verifying Hashes

SLT hashes are the link between on-chain modules and their database records. Use these commands to verify integrity.

### Compute hash from local content

From individual SLT flags:

```bash
andamio course credential compute-hash \
  --slt "Describe the protocol architecture" \
  --slt "Build a basic transaction" \
  --slt "Verify credential hashes"
```

From an outline file:

```bash
andamio course credential compute-hash --file ./compiled/my-course/101/outline.md
```

Add `--output json` for structured output including the hash, SLT count, and SLT texts.

### Verify against on-chain data

Compare locally computed hashes against the API-stored values for all modules in a course:

```bash
andamio course credential verify-hash <course-id>
```

This fetches each module's SLTs from the API, re-computes the hash, and reports mismatches.

## Troubleshooting

### `modules_manage` TX confirmed but module stuck in APPROVED

**Cause**: The module was not advanced to `PENDING_TX` before the on-chain TX was submitted. The gateway's batch confirm requires `PENDING_TX` status — it will not transition a module from `APPROVED` directly to `ON_CHAIN`.

**Fix**: Set the module to `PENDING_TX`, then the gateway's retry/healer mechanism should pick it up:

```bash
andamio course teacher update-module-status \
  --course-id <course-id> --module-code 101 --status PENDING_TX
```

If the TX has already reached terminal `failed` state, you may need to re-register manually (Step 6).

### "Module not found" after `modules_manage` TX

**Cause**: The gateway could not find a DB module with a matching SLT hash when the on-chain transaction confirmed. This happens when the module was never imported (no DB record), or the SLTs in the TX body do not match the imported SLTs.

**Fix**: Always import content first (Step 4), which computes and stores the hash, before minting on-chain (Step 5). Ensure the SLT texts in the `tx run` body exactly match the outline.

If the TX already confirmed, use `register-module` (Step 6) to manually link the records.

### "slt_index out of range"

**Cause**: The module was registered (status is APPROVED) but has no SLT records in the database. This can happen if `register-module` was called before content was imported.

**Fix**: Reset the module to DRAFT, import content, then re-register:

```bash
andamio course teacher update-module-status \
  --course-id <course-id> \
  --module-code 101 \
  --status DRAFT

andamio course import ./compiled/my-course/101 --course-id <course-id>
```

### "course_module_code already exists" on register-module

**Cause**: A module with that code already exists in the database, created by a prior `course import --create` or a partial earlier run.

**Fix**: `register-module` is now idempotent on hash match. Re-running it is safe:

- If the existing module is in **DRAFT** with a matching `slt_hash`, `register-module` advances it to APPROVED and exits 0.
- If the existing module is already **APPROVED / PENDING_TX / ON_CHAIN** with a matching `slt_hash`, `register-module` exits 0 as a no-op.
- If the existing module's `slt_hash` does **not** match what you supplied, `register-module` exits non-zero and names both hashes. The escape hatch is destructive — delete and re-import with correct SLTs:

```bash
andamio course teacher delete-module --course-id <course-id> --module-code 101
andamio course import ./compiled/my-course/101 --course-id <course-id> --create
```

### Module on-chain (source: merged) but stuck at DRAFT

**Cause**: The `modules_manage` TX confirmed and the gateway merged the on-chain record, but the DB module was never advanced past DRAFT — typically because the `update-module-status --status PENDING_TX` step (Step 5) was skipped, or because modules were re-minted after a delete cycle without rerunning the lifecycle.

**Fix**: Run `register-module` with the on-chain `slt_hash`. With a matching hash on a DRAFT module, it advances to APPROVED. Then publish:

```bash
andamio course teacher register-module \
  --course-id <course-id> --module-code 101 --slt-hash <hash>
andamio course teacher publish-module --course-id <course-id> --module-code 101
```

Find the on-chain `slt_hash` via `andamio course modules <course-id> --output json` or compute it from the outline: `andamio course credential compute-hash --file ./compiled/my-course/101/outline.md`.

### TX confirmed but DB update failed

The transaction is on-chain but the database did not sync. This is the `failed` terminal state in the TX pipeline.

The gateway has automatic recovery mechanisms (retries, state healer, abandoned TX reconciler). Check the current status:

```bash
andamio tx status <tx-hash>
```

See [TX-LIFECYCLE.md](TX-LIFECYCLE.md) for detailed recovery procedures.

### SLTs differ between local content and on-chain

If `verify-hash` reports mismatches, the local SLTs were modified after minting. On-chain state is the source of truth. Update local content to match, or use `verify-hash --output json` to inspect the exact differences.

## Quick Reference

Recommended sequence for creating a course with modules:

| Step | Command | Result |
|------|---------|--------|
| 1 | `tx run .../course/create` | Course minted on-chain, DB record created |
| 2 | `course owner register` | Course metadata stored in DB |
| 3 | Prepare `outline.md`, `lesson-N.md` files | Local content ready |
| 4 | `course import --create` | DRAFT module in DB with SLT hash |
| 5a | `course teacher update-module-status --status PENDING_TX` | Module ready for chain confirmation |
| 5b | `tx run .../modules/manage` | Modules minted on-chain, gateway syncs by hash |
| 6 | `course teacher register-module` | (Recovery only) Manual hash link if Step 5b failed |
| 7 | `course teacher publish-module` | Module fully published, on-chain + DB merged |
| 8 | `course import` (no `--create`) | Update content anytime (SLTs locked, content open) |

Key principle: **always create the DB record before the on-chain record**. The gateway matches them by SLT hash. If the DB module does not exist or has a different hash when the on-chain TX confirms, the automatic sync fails and manual recovery is needed.
