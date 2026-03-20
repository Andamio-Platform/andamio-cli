---
name: pr-feedback-tasks
description: Read recent merged PRs from a GitHub repo, understand what changed, and create Andamio feedback tasks via CLI.
argument-hint: "<org/repo> [--project <id>] [--days <n>] [--limit <n>]"
---

# PR Feedback Tasks

**Purpose:** Turn recent PR activity into actionable feedback tasks on Andamio. Claude reads each PR's diff, understands what changed, and creates a task with a specific feedback ask — not just "review this PR" but "test the new billing webhook flow and confirm idempotency works."

## Usage

```
/pr-feedback-tasks Andamio-Platform/andamio-cli
/pr-feedback-tasks Andamio-Platform/andamio-api --days 14
/pr-feedback-tasks Andamio-Platform/andamio-app-v2 --project abc123 --limit 3
```

**Defaults:** last 7 days, up to 10 PRs, 2 ADA per task, expiration 2 weeks out.

---

## Execution Flow

### Phase 1: Resolve Project

If `--project <id>` is provided, use it. Otherwise, fetch managed projects and let the user pick:

```bash
andamio manager projects --output json | jq '.[].project_id'
```

Present a numbered list with project titles. User picks by number.

### Phase 2: Fetch Recent Merged PRs

```bash
gh pr list --repo <org/repo> --state merged --limit <n> --json number,title,body,mergedAt,files,labels
```

Filter to PRs merged within `--days` window. If none found, report and exit.

### Phase 3: Deduplicate

Fetch existing tasks:

```bash
andamio project task list <project-id> --output json
```

For each PR, check if a task already exists containing `<org/repo>#<number>` in the title. Skip if so.

### Phase 4: Read and Classify Each PR

For each new PR, read the actual diff:

```bash
gh pr diff <number> --repo <org/repo>
```

Understand what changed and classify the feedback needed. This is where Claude adds value over a script — read the diff, not just the file paths.

**Classification guide:**

| What changed | Feedback ask |
|-------------|-------------|
| Documentation or README | "Read the updated [section] and flag anything unclear or missing" |
| UI components or pages | "Try [specific flow] and report friction — does it feel right?" |
| API endpoints or handlers | "Hit [endpoint] with [example request] and confirm the response" |
| CLI commands | "Run [command] with [args] and report any issues" |
| Infrastructure or config | "Verify [service] is running correctly after this change" |
| Bug fix | "Confirm [bug] is fixed — try [reproduction steps]" |
| Mixed/large PR | Pick the 1-2 most important areas for feedback |

**Write a specific ask, not a generic one.** Bad: "Give feedback on PR #15." Good: "Test `andamio course import --create` with a new module — PR #15 added create-on-import support. Does it handle duplicate codes correctly?"

### Phase 5: Create Tasks

For each PR that needs feedback:

For each PR, write a temporary Markdown file with the feedback ask, then create the task using `--content-file`:

```bash
# Write the feedback content as Markdown
cat > /tmp/feedback-pr-<number>.md << 'FEEDBACK'
## What Changed

PR #19 refactored API key auth to isolate wallet and API key headers.

## How to Test

1. Login with `andamio user login`, confirm `user me` still works
2. Run `andamio course list` with API key auth — confirm no header conflicts
3. Try `andamio course get` with a bad course ID — should show a helpful 404 message, not a crash

## What to Look For

- Auth headers shouldn't conflict between wallet and API key flows
- 404 errors should show a human-readable message with a hint
FEEDBACK

# Create the task with rich Markdown content
andamio project task create <project-id> \
  --title "<feedback-specific title>" \
  --content-file /tmp/feedback-pr-<number>.md \
  --github-issue "<org/repo>#<number>" \
  --lovelace <amount> \
  --expiration <date>

# Clean up
rm /tmp/feedback-pr-<number>.md
```

The Markdown is converted to Tiptap JSON by the CLI — so headings, code blocks, and lists render properly in the Andamio app.

### Phase 5b: Comment on GitHub PR

After creating each task, comment on the original PR linking back to the Andamio task. Derive the app URL from the CLI's configured base URL:

- `https://preprod.api.andamio.io` → `https://preprod.andamio.io`
- `https://api.andamio.io` → `https://andamio.io`

```bash
# Get the task index from the create output (or from task list after creation)
TASK_INDEX=$(andamio project task list <project-id> --output json | \
  jq -r --arg ref "<org/repo>#<number>" '[.data[] | select(.content.title | contains($ref))] | last | .task_index')

# Derive app URL from API base URL
API_URL=$(andamio config show 2>&1 | grep "Base URL" | awk '{print $NF}')
APP_URL=$(echo "$API_URL" | sed 's|\.api\.|.|; s|/api||')

# For draft tasks (not yet on-chain):
TASK_URL="${APP_URL}/studio/project/<project-id>/draft-tasks/${TASK_INDEX}"

# Comment on the PR
gh pr comment <number> --repo <org/repo> --body "$(cat <<EOF
🎯 **Feedback task created on Andamio**

A task has been created for community feedback on this PR:
**[View task on Andamio](${TASK_URL})**

Reward: $(echo "scale=0; <lovelace> / 1000000" | bc) ADA · Expires: <expiration>
EOF
)"
```

This closes the loop: PR activity on GitHub → feedback task on Andamio → link back to GitHub. Contributors can find the task from either direction.

**Title:** Start with the action verb. Keep under 120 characters.
- "Test CLI import-all with 7 modules (PR #19)"
- "Review updated transaction signing guide (PR #22)"
- "Confirm billing webhook handles duplicate events (PR #244)"

**Content file:** This is where Claude's diff reading pays off. Write a structured Markdown feedback prompt with:
- **What Changed** — summary of the PR and why it matters
- **How to Test** — concrete steps: commands to run, pages to visit, endpoints to hit
- **What to Look For** — expected behavior, edge cases, things that might break

Use Markdown formatting — headings, code blocks, numbered lists. The CLI converts it to rich content in the app.

### Phase 6: Report

Output a summary table:

```
## PR Feedback Tasks Created

| PR | What Changed | Task | Feedback Ask | GH Comment |
|----|-------------|------|-------------|------------|
| #19 | CLI auth isolation | [Task #4](url) | Try login after... | ✅ |
| #22 | Docs update | [Task #5](url) | Read the new... | ✅ |

Created: 3  Skipped: 1 (already had task)  GH comments: 3
```

---

## Rules

- Only process **merged** PRs — open PRs are still in flight.
- **Read the diff.** File path heuristics are a fallback, not the primary classification. The diff tells you what actually matters.
- **One task per PR.** Even for large PRs, pick the single most valuable feedback ask. Multiple tasks per PR = nobody does any of them.
- **Be specific.** Include the command to run, the page to visit, or the endpoint to hit. Generic asks get ignored.
- **Skip trivial PRs.** Version bumps, dependency updates, CI config changes — these don't need feedback tasks.
- **Default lovelace: 2000000** (2 ADA). Use `--lovelace` to override for higher-value feedback.
- **Default expiration: 14 days** from now.

---

## Integration Points

**Reads:**
- GitHub PRs via `gh pr list` and `gh pr diff`
- Existing Andamio tasks via `andamio project task list`
- Managed projects via `andamio manager projects`

**Writes:**
- New tasks via `andamio project task create`
- Comments on GitHub PRs via `gh pr comment` linking back to the Andamio task
