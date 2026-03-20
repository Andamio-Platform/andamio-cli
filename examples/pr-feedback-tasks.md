# PR Feedback Tasks

Creates Andamio tasks from recent pull requests that need feedback. Reads PRs from a GitHub repo, identifies what changed, and creates tasks asking for specific feedback on documentation, UX, or implementation.

## Usage

```
/pr-feedback-tasks                           # Use default repo from config
/pr-feedback-tasks Andamio-Platform/andamio-cli
```

## Process

### 1. Read recent PRs

```bash
gh pr list --repo <repo> --state merged --limit 5 --json number,title,body,files
```

For each PR, identify:
- What files changed (docs? UI? API? CLI?)
- What the PR claims to do (title + body)
- What kind of feedback would be useful

### 2. Classify feedback needed

| Change type | Feedback ask |
|------------|-------------|
| Docs changed | "Read the updated docs and report anything unclear" |
| UI changed | "Try the new flow and report friction points" |
| API changed | "Test the new endpoints and report unexpected behavior" |
| CLI changed | "Run the new commands and report any issues" |

### 3. Create tasks via CLI

For each PR that needs feedback, create an Andamio task:

```bash
export PROJECT_ID=$(andamio manager projects --output json | jq -r '.data[0].project_id')

andamio project task create "$PROJECT_ID" \
  --title "Give feedback on PR #<number>: <title>" \
  --github-issue "<org>/<repo>#<number>" \
  --lovelace 2000000 \
  --expiration <2 weeks from now>
```

### 4. Report what was created

Output a summary:
- Which PRs were processed
- What tasks were created
- What feedback was requested for each

## Rules

- Only process merged PRs from the last 7 days
- Skip PRs that already have a feedback task (check existing tasks for the issue ref)
- Keep task titles short and specific: "Give feedback on CLI import-all command (PR #15)"
- Set lovelace to 2000000 (2 ADA) per feedback task
- Set expiration to 2 weeks from creation

## Reads

- GitHub PRs via `gh pr list`
- Existing tasks via `andamio project task list`

## Writes

- New tasks via `andamio project task create`
