# Task Commands — Manual Test Plan

**Branch:** `feat/project-task-commands`
**Date:** 2026-03-18
**Features covered:** `project task` CRUD, export, import, composability refactor

Work through each section in order. Each test has a command to run, and the exact output or behaviour to look for. Use `✅` / `❌` to track results.

---

## 0. Setup

### 0.1 Build the binary

```bash
go build -o /tmp/andamio ./cmd/andamio
alias andamio=/tmp/andamio
andamio --version
```

**Expect:** version string printed (commit hash + date).

### 0.2 Confirm authentication

```bash
andamio user status
```

**Expect:** shows a JWT is present and session time remaining. If not, run `andamio user login` and complete the browser flow.

### 0.3 Find your project ID

```bash
andamio project list --output json | jq -r '.data[] | "\(.project_id)  \(.content.title)"'
```

**Expect:** one or more lines like `abc123def  My Project`. Pick a project where you are a manager.

```bash
# Set this for the rest of the test session
export PROJECT_ID="<your-project-id>"
echo "Using: $PROJECT_ID"
```

---

## 1. Help Text & Usage Strings

Verifies the `[project-id]` optional syntax is gone and `<project-id>` required syntax is shown.

### 1.1 `project task --help`

```bash
andamio project task --help
```

**Expect:** lists all subcommands: `create delete export get import list update`. No mention of "interactive" or "picker".

### 1.2 `project task list --help`

```bash
andamio project task list --help
```

**Expect:**
```
Usage:
  andamio project task list <project-id> [flags]
```
The usage line shows `<project-id>` (angle brackets = required), NOT `[project-id]`.

Also expect: `Find your project IDs with: andamio project list --output json` in the description.

### 1.3 `project task create --help`

```bash
andamio project task create --help
```

**Expect:**
```
Usage:
  andamio project task create <project-id> [flags]
```
Flags listed: `--title` (required), `--lovelace` (required), `--expiration` (required), `--content`, `--github-issue`.

### 1.4 `project task export --help`

```bash
andamio project task export --help
```

**Expect:** `Usage: andamio project task export <project-id> [flags]`

### 1.5 `project task import --help`

```bash
andamio project task import --help
```

**Expect:** `Usage: andamio project task import <project-id> [flags]`. Flags: `--dry-run`.

---

## 2. Composability — No Interactive Picker

These tests confirm commands fail fast and cleanly when project-id is omitted, instead of hanging on stdin.

### 2.1 `list` with no args → immediate error

```bash
andamio project task list
```

**Expect:** exits immediately (does NOT hang) with:
```
Error: accepts 1 arg(s), received 0
Usage:
  andamio project task list <project-id> [flags]
```
Exit code non-zero: `echo $?` → `1`.

### 2.2 `create` with no args → immediate error

```bash
andamio project task create --title "Test" --lovelace 1000000 --expiration 2026-12-01
```

**Expect:** exits immediately with `Error: accepts 1 arg(s), received 0`. Does NOT hang.

### 2.3 `export` with no args → immediate error

```bash
andamio project task export
```

**Expect:** exits immediately with `Error: accepts 1 arg(s), received 0`.

### 2.4 `import` with no args → immediate error

```bash
andamio project task import
```

**Expect:** exits immediately with `Error: accepts 1 arg(s), received 0`.

### 2.5 Commands work without a TTY (piped stdin)

```bash
echo "" | andamio project task list "$PROJECT_ID" --output json
```

**Expect:** returns JSON (or an auth/API error if the project has no tasks) — does NOT hang. Stdin being closed/empty does not affect execution.

### 2.6 Bad project ID → clear error with discovery hint

```bash
andamio project task list "does-not-exist-abc123"
```

**Expect:** error message contains both the bad ID and guidance:
```
Error: project does-not-exist-abc123 not found in your managed projects

List your projects with:
  andamio project list --output json
```

---

## 3. Stdout/Stderr Separation

These tests verify that progress messages go to stderr and data goes to stdout, enabling clean pipes.

### 3.1 `list` in text mode — stdout is only table data

```bash
andamio project task list "$PROJECT_ID" > /tmp/list-stdout.txt 2> /tmp/list-stderr.txt
echo "--- STDOUT ---"
cat /tmp/list-stdout.txt
echo "--- STDERR ---"
cat /tmp/list-stderr.txt
```

**Expect:**
- STDOUT contains only the task table (or "No tasks found." on stderr if empty)
- STDERR is empty (list has no progress messages — the table itself is the output)

### 3.2 `create` in text mode — progress on stderr, nothing on stdout

```bash
andamio project task create "$PROJECT_ID" \
  --title "Test Task Alpha" \
  --lovelace 1000000 \
  --expiration 2026-12-01 \
  > /tmp/create-stdout.txt \
  2> /tmp/create-stderr.txt
echo "exit: $?"
echo "--- STDOUT (should be empty) ---"
cat /tmp/create-stdout.txt
echo "--- STDERR (should have progress) ---"
cat /tmp/create-stderr.txt
```

**Expect:**
- Exit 0
- STDOUT: empty
- STDERR: `Creating task: Test Task Alpha` and `Task created successfully.`

### 3.3 `create --output json` — JSON on stdout, nothing on stderr

```bash
andamio project task create "$PROJECT_ID" \
  --title "Test Task Beta" \
  --lovelace 2000000 \
  --expiration 2026-12-01 \
  --output json \
  > /tmp/create-json-stdout.txt \
  2> /tmp/create-json-stderr.txt
echo "--- STDOUT (should be JSON) ---"
cat /tmp/create-json-stdout.txt | jq .
echo "--- STDERR (should be empty) ---"
cat /tmp/create-json-stderr.txt
```

**Expect:**
- STDOUT: valid JSON response from API
- STDERR: empty (progress suppressed by `--output json`)

### 3.4 `export` in text mode — file list on stderr, stdout empty

```bash
andamio project task export "$PROJECT_ID" \
  > /tmp/export-stdout.txt \
  2> /tmp/export-stderr.txt
echo "--- STDOUT (should be empty) ---"
cat /tmp/export-stdout.txt
echo "--- STDERR (should show progress) ---"
cat /tmp/export-stderr.txt
```

**Expect:**
- STDOUT: empty
- STDERR: `Exporting N tasks to tasks/<slug>/` and per-file lines like `  001-test-task-alpha.md`

### 3.5 Pipe stdout to `jq` without `--output json` — no corruption

```bash
andamio project task list "$PROJECT_ID" --output json | jq '.data | length'
```

**Expect:** prints a number (count of tasks). No `jq` parse errors. If there were progress strings on stdout, `jq` would fail.

---

## 4. `project task list`

### 4.1 List in text mode

```bash
andamio project task list "$PROJECT_ID"
```

**Expect:** formatted table with columns: `Index  Title  Status  Lovelace  Expiration`. If no tasks: "No tasks found." on stderr.

### 4.2 List in JSON mode

```bash
andamio project task list "$PROJECT_ID" --output json | jq 'keys'
```

**Expect:** JSON object with at least a `data` key. Tasks array accessible at `.data`.

### 4.3 Extract task indices via jq

```bash
andamio project task list "$PROJECT_ID" --output json \
  | jq -r '.data[] | "\(.task_index)  \(.content.title)"'
```

**Expect:** one line per task: `0  Test Task Alpha`, etc. Note the task index you'll use in later tests.

```bash
# Save the first task index for later tests
export TASK_INDEX=$(andamio project task list "$PROJECT_ID" --output json \
  | jq -r '.data[0].task_index')
echo "Using task index: $TASK_INDEX"
```

---

## 5. `project task create`

### 5.1 Create with all required flags

```bash
andamio project task create "$PROJECT_ID" \
  --title "Test Task: Basic Create" \
  --lovelace 3000000 \
  --expiration 2026-12-01
```

**Expect:** stderr shows `Creating task: Test Task: Basic Create` and `Task created successfully.`. Exit 0.

### 5.2 Create with date-only expiration format

```bash
andamio project task create "$PROJECT_ID" \
  --title "Test Task: Date-only expiry" \
  --lovelace 1000000 \
  --expiration 2026-09-15
```

**Expect:** succeeds. The date-only format `2026-09-15` should be accepted (parsed as midnight UTC).

### 5.3 Create with `--github-issue` — title gets prefixed

```bash
andamio project task create "$PROJECT_ID" \
  --title "Fix the login flow" \
  --lovelace 5000000 \
  --expiration 2026-12-01 \
  --github-issue "andamio/andamio-app#42"
```

**Expect:** stderr shows `Creating task: [andamio/andamio-app#42] Fix the login flow`. The title stored in the API should have the `[org/repo#N]` prefix.

Verify by listing:
```bash
andamio project task list "$PROJECT_ID" | grep "42"
```
**Expect:** a row containing `[andamio/andamio-app#42]`.

### 5.4 Create with `--content`

```bash
andamio project task create "$PROJECT_ID" \
  --title "Task With Description" \
  --lovelace 2000000 \
  --expiration 2026-12-01 \
  --content "This task involves building the authentication module."
```

**Expect:** succeeds. Content stored but not shown in list table.

### 5.5 Create with `--output json` — API response returned

```bash
andamio project task create "$PROJECT_ID" \
  --title "Task JSON Response" \
  --lovelace 1500000 \
  --expiration 2026-12-01 \
  --output json
```

**Expect:** JSON printed to stdout. No progress on stderr. Exit 0.

### 5.6 Create — missing required flag

```bash
andamio project task create "$PROJECT_ID" --title "Incomplete"
```

**Expect:** error about missing required flags: `required flag(s) "expiration", "lovelace" not set`. Exit non-zero.

### 5.7 Create — invalid lovelace (non-numeric)

```bash
andamio project task create "$PROJECT_ID" \
  --title "Bad lovelace" \
  --lovelace "five-ada" \
  --expiration 2026-12-01
```

**Expect:** error: `--lovelace must be a non-negative integer, got "five-ada"`. Exit non-zero.

### 5.8 Create — negative lovelace

```bash
andamio project task create "$PROJECT_ID" \
  --title "Negative lovelace" \
  --lovelace "-100" \
  --expiration 2026-12-01
```

**Expect:** error: `--lovelace must be non-negative`. Exit non-zero.

### 5.9 Create — invalid expiration format

```bash
andamio project task create "$PROJECT_ID" \
  --title "Bad expiration" \
  --lovelace 1000000 \
  --expiration "next tuesday"
```

**Expect:** error about invalid expiration format, showing valid examples. Exit non-zero.

---

## 6. `project task get`

### 6.1 Get a task by index

```bash
andamio project task get "$TASK_INDEX" --project-id "$PROJECT_ID"
```

**Expect:** JSON printed with the full task object including `task_index`, `content.title`, `lovelace_amount`, `task_status`, `expiration`.

### 6.2 Get in JSON mode (explicit)

```bash
andamio project task get "$TASK_INDEX" --project-id "$PROJECT_ID" --output json | jq '.content.title'
```

**Expect:** prints the task title as a JSON string.

### 6.3 Get — missing `--project-id` flag

```bash
andamio project task get "$TASK_INDEX"
```

**Expect:** error: `required flag(s) "project-id" not set`. Exit non-zero.

### 6.4 Get — non-existent index

```bash
andamio project task get 9999 --project-id "$PROJECT_ID"
```

**Expect:** error: `task with index 9999 not found`. Exit non-zero.

---

## 7. `project task update`

Get the index of the task you want to update first:
```bash
andamio project task list "$PROJECT_ID" --output json \
  | jq -r '.data[] | select(.task_status == "DRAFT") | "\(.task_index)  \(.content.title)"'
```

Pick a DRAFT task index:
```bash
export UPDATE_INDEX=<draft-task-index>
```

### 7.1 Update title only

```bash
andamio project task update "$UPDATE_INDEX" \
  --project-id "$PROJECT_ID" \
  --title "Updated Title $(date +%s)"
```

**Expect:** stderr shows `Updating task N...` and `Task N updated successfully.`. Verify with `get`:
```bash
andamio project task get "$UPDATE_INDEX" --project-id "$PROJECT_ID" | jq '.content.title'
```

### 7.2 Update lovelace only

```bash
andamio project task update "$UPDATE_INDEX" \
  --project-id "$PROJECT_ID" \
  --lovelace 9000000
```

**Expect:** succeeds. Verify:
```bash
andamio project task get "$UPDATE_INDEX" --project-id "$PROJECT_ID" | jq '.lovelace_amount'
```
**Expect:** `9000000`.

### 7.3 Update expiration only

```bash
andamio project task update "$UPDATE_INDEX" \
  --project-id "$PROJECT_ID" \
  --expiration 2027-01-01
```

**Expect:** succeeds.

### 7.4 Update multiple fields at once

```bash
andamio project task update "$UPDATE_INDEX" \
  --project-id "$PROJECT_ID" \
  --title "Fully Updated Task" \
  --lovelace 7000000 \
  --expiration 2026-11-30 \
  --content "Updated description text"
```

**Expect:** succeeds. All fields changed.

### 7.5 Update `--output json`

```bash
andamio project task update "$UPDATE_INDEX" \
  --project-id "$PROJECT_ID" \
  --title "JSON Update" \
  --output json
```

**Expect:** JSON response on stdout. No progress on stderr. Exit 0.

### 7.6 Update — missing `--project-id`

```bash
andamio project task update "$UPDATE_INDEX" --title "No project"
```

**Expect:** error: `required flag(s) "project-id" not set`.

---

## 8. `project task delete`

> ⚠️ Only delete a task you created for testing. Deletion is permanent.

```bash
# Create a throwaway task first
andamio project task create "$PROJECT_ID" \
  --title "THROWAWAY — safe to delete" \
  --lovelace 1000000 \
  --expiration 2026-12-01

# Get its index (it will be the last one)
export DELETE_INDEX=$(andamio project task list "$PROJECT_ID" --output json \
  | jq -r '.data[-1].task_index')
echo "Will delete index: $DELETE_INDEX"

# Confirm the title
andamio project task get "$DELETE_INDEX" --project-id "$PROJECT_ID" | jq '.content.title'
```

### 8.1 Delete the throwaway task

```bash
andamio project task delete "$DELETE_INDEX" --project-id "$PROJECT_ID"
```

**Expect:** stderr shows `Deleting task N...` and `Task N deleted.`. Exit 0.

Verify it's gone:
```bash
andamio project task get "$DELETE_INDEX" --project-id "$PROJECT_ID"
```
**Expect:** error: `task with index N not found`.

### 8.2 Delete — missing `--project-id`

```bash
andamio project task delete 0
```

**Expect:** error: `required flag(s) "project-id" not set`.

### 8.3 Delete `--output json`

```bash
# Create another throwaway
andamio project task create "$PROJECT_ID" \
  --title "THROWAWAY 2 — safe to delete" \
  --lovelace 1000000 \
  --expiration 2026-12-01

export DELETE_INDEX2=$(andamio project task list "$PROJECT_ID" --output json \
  | jq -r '.data[-1].task_index')

andamio project task delete "$DELETE_INDEX2" \
  --project-id "$PROJECT_ID" \
  --output json
```

**Expect:** JSON response on stdout. No progress on stderr.

---

## 9. `project task export`

Make sure you have some tasks first (from section 5 above).

### 9.1 Basic export in text mode

```bash
cd /tmp && mkdir -p andamio-test && cd andamio-test
andamio project task export "$PROJECT_ID"
```

**Expect:** stderr shows:
```
Exporting N tasks to tasks/<project-slug>/
  001-test-task-alpha.md
  002-test-task-beta.md
  ...
Exported N tasks to tasks/<project-slug>/
```
STDOUT is empty.

### 9.2 Verify directory and files created

```bash
ls tasks/
ls tasks/*/
```

**Expect:** `tasks/<project-slug>/` directory with one `.md` file per task, named `<index>-<title-slug>.md`.

### 9.3 Inspect a task file — frontmatter

```bash
cat tasks/*/*.md | head -20
```

**Expect:** YAML frontmatter between `---` delimiters with fields:
```yaml
---
title: "Test Task Alpha"
lovelace: "1000000"
expiration_time: "2026-12-01T00:00:00Z"
index: 0
status: "DRAFT"
project_id: "abc123..."
project_state_policy_id: "..."
---
```

### 9.4 Export `--output json`

```bash
andamio project task export "$PROJECT_ID" --output json | jq .
```

**Expect:** JSON object with:
```json
{
  "project_id": "...",
  "directory": "tasks/<slug>",
  "tasks_exported": N,
  "files": ["001-test-task-alpha.md", ...]
}
```
No progress text on stderr.

### 9.5 Export overwrites existing files cleanly (re-export)

```bash
andamio project task export "$PROJECT_ID"
andamio project task export "$PROJECT_ID"
```

**Expect:** second export completes without error. Files updated in place. No duplicates.

---

## 10. `project task import`

### 10.1 `--dry-run` previews without sending

First, edit a task file:
```bash
TASK_FILE=$(ls tasks/*/*.md | head -1)
echo "Editing: $TASK_FILE"

# Change the title in frontmatter
sed -i '' 's/^title: .*/title: "Dry Run Updated Title"/' "$TASK_FILE"
```

Run dry-run:
```bash
andamio project task import "$PROJECT_ID" --dry-run
```

**Expect:** stderr shows `Dry-run importing N task files from tasks/<slug>/`. For each file with an `index`:
```
  001-test-task.md: UPDATE task 0 (dry-run)
  {
    "project_state_policy_id": "...",
    "index": 0,
    "title": "Dry Run Updated Title",
    ...
  }
```
No API calls made. Verify: re-list tasks and confirm title hasn't changed:
```bash
andamio project task get 0 --project-id "$PROJECT_ID" | jq '.content.title'
```
**Expect:** original title (not "Dry Run Updated Title").

### 10.2 Real import — update an existing task

Restore or set the title in the file to something testable:
```bash
sed -i '' 's/^title: .*/title: "Import Updated Title"/' "$TASK_FILE"
```

Run real import:
```bash
andamio project task import "$PROJECT_ID"
```

**Expect:** stderr shows:
```
Importing N task files from tasks/<slug>/
  001-test-task.md: WARNING: updating existing task 0 "Import Updated Title"
  001-test-task.md: UPDATED task 0 "Import Updated Title"

Import complete: 0 created, 1 updated, 0 skipped, 0 errors
```

Verify the update took effect:
```bash
andamio project task get 0 --project-id "$PROJECT_ID" | jq '.content.title'
```
**Expect:** `"Import Updated Title"`.

### 10.3 Create new task via import (no `index` in frontmatter)

Create a new .md file without an `index` field:
```bash
cat > tasks/*/999-new-via-import.md << 'EOF'
---
title: "Created via Import"
lovelace: "4000000"
expiration_time: "2026-12-01T00:00:00Z"
project_id: "REPLACE_WITH_YOUR_PROJECT_ID"
project_state_policy_id: "REPLACE_WITH_POLICY_ID"
---

This task was created by importing a Markdown file with no index.
EOF
```

> Note: Copy `project_id` and `project_state_policy_id` from one of the existing exported files.

```bash
andamio project task import "$PROJECT_ID"
```

**Expect:** stderr includes:
```
  999-new-via-import.md: CREATED "Created via Import"
Import complete: 1 created, N updated, 0 skipped, 0 errors
```

Verify via list:
```bash
andamio project task list "$PROJECT_ID" | grep "Import"
```
**Expect:** new task visible.

### 10.4 Import `--output json`

```bash
andamio project task import "$PROJECT_ID" --output json | jq .
```

**Expect:**
```json
{
  "project_id": "...",
  "directory": "tasks/<slug>",
  "tasks_created": 0,
  "tasks_updated": N,
  "tasks_skipped": 0,
  "errors": 0
}
```
No progress on stderr.

### 10.5 Import `--dry-run --output json`

```bash
andamio project task import "$PROJECT_ID" --dry-run --output json | jq .
```

**Expect:** same JSON struct, with `"dry_run": true`.

### 10.6 Import skips non-DRAFT tasks

If you have an on-chain (non-DRAFT) task, its file will have a non-DRAFT `status` in frontmatter. Import should skip it:
```bash
andamio project task import "$PROJECT_ID" 2>&1 | grep "SKIPPED"
```
**Expect (if applicable):** `SKIPPED (task N is ON_CHAIN — task is on-chain and immutable)`.

### 10.7 Import with missing required frontmatter field

Create a broken file:
```bash
cat > tasks/*/broken.md << 'EOF'
---
title: "Missing lovelace and expiration"
---
EOF

andamio project task import "$PROJECT_ID"
```

**Expect:** stderr shows `broken.md: ERROR: missing required field: lovelace`. Import continues with other files. Final summary shows 1 error.

---

## 11. Bash Script / Pipeline Integration

This is the full composable workflow test. Run it end-to-end.

### 11.1 Two-step discovery + list pipeline

```bash
PROJECT_ID=$(andamio project list --output json | jq -r '.data[0].project_id')
andamio project task list "$PROJECT_ID" --output json | jq -r '.data[] | "\(.task_index)  \(.content.title)"'
```

**Expect:** runs to completion without hanging. Task titles printed to terminal.

### 11.2 Count tasks (tests clean stdout)

```bash
COUNT=$(andamio project task list "$PROJECT_ID" --output json | jq '.data | length')
echo "Task count: $COUNT"
```

**Expect:** `Task count: N` where N is a number. No jq parse errors.

### 11.3 Suppress all output — exit code check only

```bash
andamio project task list "$PROJECT_ID" --output json > /dev/null 2>&1
echo "Exit: $?"
```

**Expect:** `Exit: 0`.

### 11.4 Full GitHub-issue-to-task pipeline (dry run)

```bash
# Simulate what gh would return
ISSUES='[{"number":1,"title":"Fix login bug"},{"number":2,"title":"Add dark mode"}]'

echo "$ISSUES" | jq -c '.[]' | while IFS= read -r issue; do
  NUMBER=$(echo "$issue" | jq -r '.number')
  TITLE=$(echo "$issue" | jq -r '.title')
  echo "Would create: [$NUMBER] $TITLE" >&2
  andamio project task create "$PROJECT_ID" \
    --title "$TITLE" \
    --github-issue "org/repo#$NUMBER" \
    --lovelace 5000000 \
    --expiration 2026-12-31 \
    2>&1
done
```

**Expect:** two tasks created. Stderr shows progress for each. Stdout is empty (text mode). No hangs.

Verify:
```bash
andamio project task list "$PROJECT_ID" | grep "org/repo"
```
**Expect:** two rows with `[org/repo#1]` and `[org/repo#2]` prefixes.

### 11.5 Export → count files via subshell

```bash
cd /tmp/andamio-test
FILE_COUNT=$(andamio project task export "$PROJECT_ID" --output json | jq '.tasks_exported')
echo "Exported: $FILE_COUNT tasks"
ls tasks/*/ | wc -l
```

**Expect:** both counts match. The subshell captures JSON cleanly without progress text.

---

## 12. Auth Guard

These confirm that task commands require JWT (not just API key).

### 12.1 Logout and try a task command

```bash
andamio user logout
andamio project task list "$PROJECT_ID"
```

**Expect:** error: `not authenticated. Run 'andamio user login' first`. Exit non-zero.

### 12.2 Re-login

```bash
andamio user login
andamio user status
```

**Expect:** JWT present, session time shown.

---

## 13. Output Format Consistency

### 13.1 CSV output

```bash
andamio project task list "$PROJECT_ID" --output csv
```

**Expect:** comma-separated output. No crash.

### 13.2 Markdown output

```bash
andamio project task list "$PROJECT_ID" --output markdown
```

**Expect:** markdown-formatted table or JSON. No crash.

---

## Summary Checklist

| # | Test | Result |
|---|------|--------|
| 1.2 | `list --help` shows `<project-id>` required | |
| 1.3 | `create --help` shows `<project-id>` required | |
| 2.1 | `list` no args → immediate error, no hang | |
| 2.2 | `create` no args → immediate error, no hang | |
| 2.3 | `export` no args → immediate error, no hang | |
| 2.4 | `import` no args → immediate error, no hang | |
| 2.5 | Commands work with piped stdin (no TTY needed) | |
| 2.6 | Bad project ID → helpful error with discovery hint | |
| 3.2 | `create` text mode: stdout empty, progress on stderr | |
| 3.3 | `create --output json`: JSON on stdout, stderr silent | |
| 3.5 | `list --output json \| jq` works without parse errors | |
| 4.1 | `list` renders table | |
| 4.2 | `list --output json` returns parseable JSON | |
| 5.1 | `create` with all required flags succeeds | |
| 5.3 | `--github-issue` prefixes title correctly | |
| 5.6 | `create` missing flags → clear error | |
| 5.7 | `create` non-numeric lovelace → clear error | |
| 6.1 | `get <index>` returns full task JSON | |
| 6.4 | `get` non-existent index → clear error | |
| 7.1 | `update` changes title | |
| 7.6 | `update` missing `--project-id` → error | |
| 8.1 | `delete` removes task | |
| 9.1 | `export` creates files in `tasks/<slug>/` | |
| 9.3 | Exported files have correct YAML frontmatter | |
| 9.4 | `export --output json` returns structured result | |
| 10.1 | `import --dry-run` previews without sending | |
| 10.2 | `import` updates existing task | |
| 10.3 | `import` creates new task when no `index` in frontmatter | |
| 10.4 | `import --output json` returns structured result | |
| 10.7 | `import` with bad file → error per file, continues | |
| 11.2 | `list --output json \| jq length` works | |
| 11.4 | GitHub-issue pipeline creates tasks end-to-end | |
| 12.1 | Task commands require JWT (fail after logout) | |
