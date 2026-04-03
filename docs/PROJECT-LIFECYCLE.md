# Project Lifecycle

Step-by-step guide for creating and managing Andamio projects through the CLI. Projects follow the same transaction pipeline as courses (see [TX-LIFECYCLE.md](TX-LIFECYCLE.md)) but are simpler -- tasks replace modules, contributors replace students, and there is no SLT hash complexity.

## Prerequisites

Before starting, you need:

1. **API key** -- stored via `andamio auth login --api-key <key>`
2. **User JWT** -- obtained via `andamio user login` (browser wallet signing) or `andamio user login --skey <path> --alias <name>` (headless)
3. **Submit URL** -- configured via `andamio config set-submit-url <url>` (e.g., Blockfrost submit endpoint)
4. **Submit headers** -- if required by your submit provider: `andamio config set-submit-header project_id <blockfrost-id>`
5. **Signing key** -- a Cardano `.skey` file for transaction signing

Verify your setup:

```bash
andamio user status
andamio config show
```

## Project Creation Workflow

### Step 1: Create project on-chain

Build, sign, submit, and register the project creation transaction:

```bash
andamio tx run /v2/tx/instance/owner/project/create \
  --body '{"alias":"<your-alias>"}' \
  --skey ./payment.skey \
  --tx-type project_create \
  --instance-id <project-id>
```

Wait for the command to report `updated` status. This means the transaction is confirmed on-chain and the database has synced. See [TX-LIFECYCLE.md](TX-LIFECYCLE.md) for details on transaction states and recovery.

### Step 2: Register project metadata

Link the on-chain project to off-chain metadata:

```bash
andamio project owner register --project-id <project-id> --title "My Project"
```

### Step 3: Update project metadata (optional)

Add description, images, and visibility settings:

```bash
andamio project owner update --project-id <project-id> \
  --title "My Project" \
  --description "A detailed description of the project" \
  --image-url "https://example.com/image.png" \
  --video-url "https://example.com/video.mp4" \
  --category "development" \
  --public
```

Only flags you include are sent -- omitted fields remain unchanged.

### Verify creation

```bash
andamio project get <project-id>
andamio project owner list
```

## Task Management

Tasks are the work units within a project. They are created off-chain first, then minted on-chain when ready.

### Create tasks

```bash
andamio project task create <project-id> \
  --title "Fix login bug" \
  --lovelace 5000000 \
  --expiration 2026-06-01

andamio project task create <project-id> \
  --title "Write API documentation" \
  --lovelace 3000000 \
  --expiration 2026-07-01
```

### List tasks

```bash
# Manager view (includes draft tasks)
andamio project task list <project-id>

# Public view
andamio project tasks <project-id>
```

### Export tasks to local files

Export all tasks as Markdown files for local editing:

```bash
andamio project task export <project-id>
```

This creates a `tasks/<project-slug>/` directory with one Markdown file per task.

### Edit and reimport

After editing the exported Markdown files locally:

```bash
# Preview changes without applying
andamio project task import <project-id> --dry-run

# Apply changes
andamio project task import <project-id>
```

### Mint tasks on-chain

Once tasks are finalized, mint them on-chain:

```bash
andamio tx run /v2/tx/project/manager/tasks/manage \
  --body '{"alias":"<your-alias>","project_id":"<project-id>","tasks":[...]}' \
  --skey ./payment.skey \
  --tx-type tasks_manage
```

### Update and delete draft tasks

```bash
# Update a task by index
andamio project task update <task-index> --project-id <project-id> \
  --title "Updated title" \
  --lovelace 8000000

# Delete a draft task
andamio project task delete <task-index> --project-id <project-id>
```

## Contributor Workflow

Contributors discover projects, commit to tasks, submit evidence, and claim credentials.

### Discover and commit

```bash
# List projects you can contribute to
andamio project contributor list

# View available tasks
andamio project tasks <project-id>

# Commit to a task (on-chain)
andamio tx run /v2/tx/project/contributor/commit \
  --body '{"alias":"<your-alias>","project_id":"<project-id>","task_index":1}' \
  --skey ./payment.skey \
  --tx-type project_join
```

### Submit evidence

```bash
# Submit from a Markdown file
andamio project contributor update \
  --project-id <project-id> \
  --task-index 1 \
  --evidence-file work.md

# Or submit inline
andamio project contributor update \
  --project-id <project-id> \
  --task-index 1 \
  --evidence "Completed the fix. PR: https://github.com/org/repo/pull/42"
```

### Check commitment status

```bash
# List all your commitments
andamio project contributor commitments

# Get a specific commitment
andamio project contributor commitment \
  --project-id <project-id> \
  --task-index 1
```

### Manager assessment

The project manager reviews submitted work and assesses tasks on-chain:

```bash
# List pending assessments
andamio project manager commitments --project-id <project-id>

# Assess tasks (on-chain)
andamio tx run /v2/tx/project/manager/tasks/assess \
  --body '{"alias":"<your-alias>","project_id":"<project-id>","assessment_decisions":[...]}' \
  --skey ./payment.skey \
  --tx-type task_assess
```

### Claim credential

After a task is assessed and approved, the contributor claims their credential on-chain:

```bash
andamio tx run /v2/tx/project/contributor/credential/claim \
  --body '{"alias":"<your-alias>","project_id":"<project-id>"}' \
  --skey ./payment.skey \
  --tx-type project_credential_claim
```

## Verifying Task Hashes

Task hashes provide on-chain integrity verification. Use these commands to compute and verify hashes locally.

### Compute a hash from task content

```bash
andamio project task compute-hash \
  --content "Fix login bug" \
  --lovelace 5000000 \
  --expiration 2026-06-01
```

Or compute from a task file:

```bash
andamio project task compute-hash --file tasks/my-project/001-fix-login-bug.md
```

### Verify hashes against on-chain data

```bash
andamio project task verify-hash <project-id>
```

This compares the locally computed hashes against the on-chain values and reports any mismatches.

## Troubleshooting

### TX confirmed but database not updated

The transaction reached `confirmed` state but never moved to `updated`. The gateway has automatic recovery mechanisms (state machine retries, state healer, abandoned TX reconciler). See the Recovery Procedures section in [TX-LIFECYCLE.md](TX-LIFECYCLE.md).

```bash
andamio tx status <tx-hash>
```

### Task hash mismatch

If `verify-hash` reports mismatches, the gateway self-heals by adopting on-chain hashes as the source of truth. No manual intervention is needed -- the mismatch will resolve on the next API read.

### Evidence submission fails

Check that your JWT session is still valid:

```bash
andamio user status
```

If the session has expired, re-authenticate:

```bash
andamio user login
```

### Task import fails

Ensure you are authenticated with a JWT (not just an API key) and that you have manager permissions for the project:

```bash
andamio user status
andamio project owner list
```

## Quick Reference

Recommended project lifecycle sequence from creation to contributor credential:

| Step | Role | Command | On-chain? |
|------|------|---------|-----------|
| 1 | Owner | `tx run .../project/create` | Yes |
| 2 | Owner | `project owner register` | No |
| 3 | Owner | `project owner update` | No |
| 4 | Manager | `project task create` | No |
| 5 | Manager | `project task export` / edit / `project task import` | No |
| 6 | Manager | `tx run .../tasks/manage` | Yes |
| 7 | Contributor | `tx run .../contributor/commit` | Yes |
| 8 | Contributor | `project contributor update --evidence-file` | No |
| 9 | Manager | `tx run .../tasks/assess` | Yes |
| 10 | Contributor | `tx run .../contributor/credential/claim` | Yes |

Steps 1, 6, 7, 9, and 10 are on-chain transactions that follow the 5-step pipeline described in [TX-LIFECYCLE.md](TX-LIFECYCLE.md). All other steps are off-chain API calls or local operations.
