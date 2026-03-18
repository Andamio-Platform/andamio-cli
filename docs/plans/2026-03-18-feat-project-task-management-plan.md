---
title: "feat: Project Task Management Commands"
type: feat
status: active
date: 2026-03-18
origin:
  - docs/brainstorms/2026-03-18-project-task-management-brainstorm.md
  - ~/projects/02-areas/andamio/docs/brainstorms/2026-03-18-github-integration-cli-brainstorm.md
---

# feat: Project Task Management Commands

## Overview

Add CLI commands for project managers to create, list, update, delete, export, and import tasks for their projects. Follows the established course export/import patterns: API CRUD commands plus a file-based export/import workflow for drafting tasks as local Markdown files with YAML frontmatter.

Includes a `--github-issue` flag on `task create` for convention-based GitHub issue linking — no GitHub SDK, composable with `gh` via scripts.

**Demo target:** CF Dev Office Hours, Friday Mar 21 — show GitHub issue → Andamio task → learning path + on-chain reward pipeline.

## Problem Statement / Motivation

Project managers currently must use the web app to create and manage tasks. The CLI already supports course content management via export/import, but has no task management capabilities. Managers who need to create many tasks (e.g., onboarding a new cohort) have no way to draft them locally, review in bulk, or script the process.

The demo story: a GitHub issue connects to (a) what someone needs to learn (Andamio course/module) and (b) where they earn reputation and rewards. The CLI makes this composable.

(See brainstorm: `docs/brainstorms/2026-03-18-project-task-management-brainstorm.md`)
(See brainstorm: `~/projects/02-areas/andamio/docs/brainstorms/2026-03-18-github-integration-cli-brainstorm.md`)

## Proposed Solution

### Command Structure

All commands nest under `project` (see brainstorm: resolved question "Command nesting"):

```
andamio project tasks list [project_id]       # List tasks (manager view)
andamio project task get <index>              # Get single task (filter from list)
andamio project task create [project_id]      # Create a task (flags or interactive)
andamio project task update <index>           # Update a task
andamio project task delete <index>           # Delete a draft task
andamio project task export [project_id]      # Export tasks to local Markdown files
andamio project task import [project_id]      # Import tasks from local Markdown files
```

### Project Identification

Two modes (see brainstorm: resolved question "Project selection"):
1. **Explicit**: Pass `project_id` as argument — used directly
2. **Interactive**: Omit `project_id` — CLI calls `POST /v2/project/manager/projects/list`, prints numbered list, reads selection from stdin (no TUI library needed)

### Key Identifier: `project_state_policy_id`

Critical discovery from API research: Task CRUD endpoints require `project_state_policy_id` (not `project_id`). This maps to `contributor_state_id` on the project objects.

**Resolution flow:**
1. `POST /v2/project/manager/projects/list` returns `ManagerProjectListItem` with both `project_id` and `contributor_state_id`
2. CLI caches this mapping when listing projects
3. For CRUD/export/import: resolve `project_id` → `project_state_policy_id` via the projects list response

### GitHub Issue Linking

Convention-based. No GitHub SDK, no `gh` dependency (see orch brainstorm: key decision #1).

- `--github-issue org/repo#123` flag on `task create`
- Stores the reference as a prefix in the task title: `[org/repo#123] Original Title`
- The CLI doesn't validate the issue exists — composability with `gh` handles that
- JSON output mode (`-o json`) is the composability surface

**Demo script:**
```bash
#!/bin/bash
REPO="Andamio-Platform/andamio-api"
PROJECT_ID="..."

gh issue list --repo $REPO --label "good-first-issue" --json number,title | \
  jq -c '.[]' | while read issue; do
    NUMBER=$(echo $issue | jq -r '.number')
    TITLE=$(echo $issue | jq -r '.title')
    andamio project task create $PROJECT_ID \
      --title "$TITLE" \
      --github-issue "$REPO#$NUMBER" \
      --expiration "2026-04-18T00:00:00Z" \
      --lovelace "2000000"
  done
```

## Technical Considerations

### API Endpoints

| Command | Endpoint | Method | Body |
|---------|----------|--------|------|
| List manager projects | `/v2/project/manager/projects/list` | POST | (none) |
| List tasks | `/v2/project/manager/tasks/list` | POST | `{project_id, status?}` |
| Create task | `/v2/project/manager/task/create` | POST | `{project_state_policy_id, title, lovelace, expiration_time, ...}` |
| Update task | `/v2/project/manager/task/update` | POST | `{project_state_policy_id, index, ...}` |
| Delete task | `/v2/project/manager/task/delete` | POST | `{project_state_policy_id, index}` |

### Task Data Model

**`MergedTaskListItem`** (from `andamio-api/internal/orchestration/types.go:409`):
```
task_hash, project_id, created_by, contributor_state_id,
on_chain_content, expiration (ISO), expiration_posix (int64),
lovelace_amount (int64), assets[], task_index (int),
task_status (DRAFT/PENDING_TX/ON_CHAIN), source (merged/db_only/chain_only),
content: { title, description, content_json (Tiptap) }
```

**`CreateTaskRequest`** (required): `project_state_policy_id`, `title`, `lovelace` (string), `expiration_time` (string, Unix ms)
**`CreateTaskRequest`** (optional): `content` (plain text), `content_json` (Tiptap), `tokens[]`

**`UpdateTaskRequest`** (required): `project_state_policy_id`, `index` (int32)
**`UpdateTaskRequest`** (optional): all content fields, partial updates supported

**`ProjectManagerTasksListPostRequest`**: `project_id` (required), `status` (optional filter)

### Auth

All manager endpoints require JWT. Commands use `PreRunE` to check `cfg.HasUserAuth()`.

**Known issue** (from `docs/solutions/integration-issues/cli-api-auth-middleware-mismatch.md`): v1 middleware rejects requests with both API key and JWT headers simultaneously. For task commands (all manager/JWT), send only `Authorization: Bearer` header.

### Content Conversion

Reuse existing Tiptap↔Markdown converters from course export/import:
- `tiptapToMarkdown()` (course_export.go:549) — for export
- `markdownToTiptap()` (course_import.go:641) — for import

These are in the same `main` package under `cmd/andamio/`, so they're directly callable.

### File Format (see brainstorm: key decision #1)

Directory structure:
```
tasks/<project-slug>/
  setup-environment.md
  write-documentation.md
  review-pull-requests.md
```

Each file:
```yaml
---
title: "Set up development environment"
lovelace: "5000000"
expiration_time: "2026-04-01T00:00:00Z"
tokens:
  - policy_id: "abc123..."
    asset_name: "ProjectToken"
    quantity: "100"
index: 0                          # populated on export, omit for new tasks
status: "DRAFT"                   # populated on export, read-only
project_id: "abc123"              # populated on export, used on import
project_state_policy_id: "xyz789" # populated on export, used on import
---

Set up the development environment following the project README...
```

### Status-Based Mutability

Following the SLT locking pattern from course import:
- **DRAFT** tasks: fully mutable (create/update/delete)
- **Non-DRAFT** tasks (ON_CHAIN, PENDING_TX): export but skip on import with warning

### Conflict Handling (see brainstorm: resolved question)

Warn-then-overwrite: on import, if a task with matching `index` exists, print a warning showing the task title and status, then proceed with the update.

### Image Handling

Defer for initial implementation. Task content is typically simpler than course lessons. If `content_json` contains images, they'll render as CDN URLs in the exported Markdown. Full image download/upload/manifest support can be added later if needed.

## Acceptance Criteria

### Phase 1: CRUD Commands (target: demo-ready by Friday Mar 21)

- [x] `project tasks list [project_id]` — lists tasks with title, index, status, lovelace. Supports `--output json/text/csv/markdown`. Interactive project picker when ID omitted.
- [x] `project task get <index>` — shows full task detail (filters from list response, since no single-get endpoint exists). Requires `--project-id` flag.
- [x] `project task create [project_id]` — creates a task. Required flags: `--title`, `--lovelace`, `--expiration`. Optional: `--content`, `--github-issue`. Requires JWT.
- [x] `project task create --github-issue org/repo#123` includes the GitHub reference as a title prefix `[org/repo#123]`
- [x] `project task update <index>` — updates a task by index. Requires `--project-id` flag. Optional flags for each updatable field. Requires JWT.
- [x] `project task delete <index>` — deletes a draft task. Requires `--project-id` flag. Requires JWT.
- [x] All write commands check `HasUserAuth()` in `PreRunE`
- [x] All commands support `--output json` for structured output
- [ ] Demo script works: `gh issue list | ... | andamio project task create` pipeline (needs live API testing)

### Phase 2: Export/Import

- [x] `project task export [project_id]` — exports all tasks to `tasks/<project-slug>/` as Markdown files with YAML frontmatter. Converts `content_json` to Markdown body via `tiptapToMarkdown()`. Creates directory if needed. Requires JWT.
- [x] `project task import [project_id]` — reads task files from `tasks/<project-slug>/`, creates new tasks (no `index` in frontmatter) or updates existing (has `index`). Converts Markdown body to Tiptap via `markdownToTiptap()`. Requires JWT.
- [x] Import supports `--dry-run` flag to preview payloads without sending
- [x] Import skips non-DRAFT tasks with a warning
- [x] Import converts ISO 8601 `expiration_time` to Unix ms string for API
- [x] Import handles token arrays from YAML frontmatter
- [x] Export/import produce structured `ExportResult`/`ImportResult` for `--output json`
- [x] Progress messages suppressed in JSON output mode

### Quality Gates

- [x] URL-encode all user-supplied path parameters (per `docs/solutions/security-issues/cli-security-hardening-input-validation.md`)
- [x] Empty arrays omitted from API payloads (not sent as `[]`) to avoid accidental deletion
- [x] `go build` passes cleanly
- [ ] Manual testing with preprod API (needs JWT auth session)

## Implementation Plan

### Phase 1: CRUD Commands (~1 session, target Friday demo)

#### Step 1.1: Project task list and project picker

New file: `cmd/andamio/project_task.go`

- Define `projectTaskCmd` parent command (`project task`)
- Define `projectTasksListCmd` (`project tasks list [project_id]`)
- Implement `resolveProjectID()`: if arg provided, use it; if omitted, call `POST /v2/project/manager/projects/list`, print numbered list with `content.title` and `project_id`, read selection from stdin via `bufio.Scanner`
- Implement `resolveProjectStatePolicyID(projectID)`: call manager projects list, find matching project, return `contributor_state_id`
- Implement task listing: call `POST /v2/project/manager/tasks/list` with `{project_id}`
- Use `output.PrintList()` with `titleKey: "content.title"`, `idKey: "task_index"`
- Register commands in `init()`

#### Step 1.2: Task create with GitHub linking

- `projectTaskCreateCmd` (`project task create [project_id]`)
- `PreRunE`: check `HasUserAuth()`
- Required flags: `--title`, `--lovelace`, `--expiration` (ISO 8601)
- Optional flags: `--content`, `--github-issue`
- Resolve `project_state_policy_id` via `resolveProjectStatePolicyID()`
- Convert expiration from ISO 8601 to Unix ms string: `time.Parse(time.RFC3339, exp)` → `.UnixMilli()` → `strconv.FormatInt()`
- If `--github-issue` provided: prepend `[org/repo#123]` to title
- `client.Post("/api/v2/project/manager/task/create", payload, &resp)`
- Dual output: text mode prints success message, JSON mode prints `output.PrintJSON(resp)`

#### Step 1.3: Single task get

- `projectTaskGetCmd` (`project task get <index> --project-id <id>`)
- Fetch all tasks via list endpoint, filter by `task_index`
- Print via `output.PrintJSON()`

#### Step 1.4: Task update and delete

- `projectTaskUpdateCmd` (`project task update <index> --project-id <id>`)
  - Optional flags for each updatable field (`--title`, `--lovelace`, `--expiration`, `--content`)
  - Build partial update payload — only include flags that were explicitly set
- `projectTaskDeleteCmd` (`project task delete <index> --project-id <id>`)
  - Simple payload: `{project_state_policy_id, index}`
  - Confirm deletion in text mode, silent in JSON mode

### Phase 2: Export/Import (~1 session, post-demo)

#### Step 2.1: Task export

New file: `cmd/andamio/project_task_export.go`

- `projectTaskExportCmd` (`project task export [project_id]`)
- Fetch project metadata (for slug via `content.title`), fetch all tasks
- Derive project slug: lowercase, replace spaces with hyphens, strip special chars
- For each task:
  - Convert `content_json` to Markdown via `tiptapToMarkdown()`
  - Derive filename slug from task title
  - Write as `{slug}.md` with YAML frontmatter (title, lovelace, expiration as ISO 8601, tokens, index, status, project_id, project_state_policy_id)
- Create `tasks/<project-slug>/` directory via `os.MkdirAll()`
- Use `writeFileAtomic()` for safe writes (existing pattern in course_export.go)
- Return `TaskExportResult` struct with task count, directory path

#### Step 2.2: Task import

New file: `cmd/andamio/project_task_import.go`

- `projectTaskImportCmd` (`project task import [project_id]`)
- Scan `tasks/<project-slug>/` for `.md` files
- Define `TaskFrontmatter` struct:
  ```go
  type TaskFrontmatter struct {
      Title                string         `yaml:"title"`
      Lovelace             string         `yaml:"lovelace"`
      ExpirationTime       string         `yaml:"expiration_time"`
      Tokens               []TaskToken    `yaml:"tokens"`
      Index                *int           `yaml:"index"`      // nil = new task
      Status               string         `yaml:"status"`     // read-only
      ProjectID            string         `yaml:"project_id"`
      ProjectStatePolicyID string         `yaml:"project_state_policy_id"`
  }
  ```
- Parse YAML frontmatter via `adrg/frontmatter`
- Convert Markdown body to Tiptap via `markdownToTiptap()`
- Convert ISO 8601 expiration to Unix ms: `time.Parse(time.RFC3339, exp)` → `.UnixMilli()` → `strconv.FormatInt()`
- For each file:
  - No `index` (nil) → `POST /v2/project/manager/task/create`
  - Has `index` → fetch existing tasks, check status, warn if non-DRAFT, `POST /v2/project/manager/task/update`
- `--dry-run` flag: marshal payload to JSON, print, skip API call
- Return `TaskImportResult` struct with created/updated/skipped counts

## Demo Flow (CF Dev Office Hours, Friday Mar 21)

```bash
# 1. Authenticate
andamio user login

# 2. List projects I manage
andamio project tasks list
# → Interactive picker shows managed projects

# 3. Show a GitHub issue
gh issue view 42 --repo Andamio-Platform/andamio-api

# 4. Create an Andamio task linked to it
andamio project task create <project_id> \
  --title "Implement rate limiting middleware" \
  --github-issue "Andamio-Platform/andamio-api#42" \
  --expiration "2026-04-18T00:00:00Z" \
  --lovelace "5000000"

# 5. List tasks — see the link
andamio project tasks list <project_id>

# 6. JSON output for scripting
andamio project tasks list <project_id> -o json | jq '.[] | select(.content.title | contains("#42"))'
```

The audience sees: a real GitHub issue becomes an Andamio task. The developer who picks it up has a learning path and earns on-chain reputation.

## Dependencies & Risks

| Risk | Mitigation |
|------|------------|
| API schema uncertainty — OpenAPI spec lacks detailed schemas | Validate against live preprod API. Field names confirmed from generated Go models in `andamio-api`. |
| `project_state_policy_id` resolution adds latency | One extra API call per operation. Acceptable for CLI; cache in session if needed. |
| No single-task GET endpoint | `task get` filters from list response. Fine for typical project sizes. |
| Auth header conflict (both API key + JWT) | Send only JWT for manager endpoints (documented in learnings). |
| `expiration_time` format confusion | CLI accepts ISO 8601, converts internally. Clear in `--help` text. |
| Demo deadline (Friday) | Phase 1 CRUD is self-contained. Export/import deferred to Phase 2. |

## Backlog (Post-Demo)

- **Flexible JSON content field**: Explore making `content_json` accept structured metadata alongside Tiptap content. Enables rich GitHub links, external references, custom attributes.
- **App rendering of metadata**: Once JSON field is extensible, render linked GitHub issues in the task UI.
- **`task batch-status`**: Wire up batch status update endpoint for scripted workflows.
- **Bidirectional sync**: `andamio project task sync` polls GitHub issue status via `gh`.
- **GitHub Actions**: Action that calls `andamio` CLI on issue events.
- **Script library**: `andamio-scripts/` repo with composable workflow patterns.

## Sources & References

### Origin

- **Brainstorm (CLI):** [docs/brainstorms/2026-03-18-project-task-management-brainstorm.md](docs/brainstorms/2026-03-18-project-task-management-brainstorm.md) — Key decisions: commands under `project`, directory of Markdown files, ISO 8601 dates, include tokens, warn-then-overwrite conflicts.
- **Brainstorm (GitHub):** `~/projects/02-areas/andamio/docs/brainstorms/2026-03-18-github-integration-cli-brainstorm.md` — Key decisions: CLI stays Andamio-only, convention-based linking, composability over built-in GitHub features.
- **Plan (GitHub):** `~/projects/02-areas/andamio/docs/plans/2026-03-18-feat-cli-task-commands-github-integration-plan.md` — Demo flow, script examples, `--github-issue` flag design.

### Internal References

- Command pattern: `cmd/andamio/project.go` (simple commands)
- Export pattern: `cmd/andamio/course_export.go` (Tiptap→Markdown, directory writing, `writeFileAtomic`)
- Import pattern: `cmd/andamio/course_import.go` (frontmatter, Markdown→Tiptap, `ImportParams`)
- Create pattern: `cmd/andamio/course_create_module.go` (PreRunE, flags, dual output)
- Auth check: `cmd/andamio/teacher.go` (PersistentPreRunE JWT chain)
- Client POST: `internal/client/client.go:78` (Post method)
- Output formatting: `internal/output/output.go` (PrintList, PrintJSON, GetFormat)
- Task data model: `andamio-api/internal/orchestration/types.go:409` (MergedTaskListItem)
- Create request: `andamio-api/openapi/generated/go/andamio_db_client/model_create_task_request.go`
- Manager projects: `andamio-api/internal/orchestration/types.go:485` (ManagerProjectListItem)
- Task list request: `andamio-api/openapi/generated/go/andamio_db_client/model__project_manager_tasks_list_post_request.go`

### Institutional Learnings Applied

- Empty arrays delete data — omit instead of sending `[]` (from `docs/solutions/logic-errors/export-import-round-trip-title-preservation.md`)
- Auth header conflict — don't send both API key and JWT simultaneously (from `docs/solutions/integration-issues/cli-api-auth-middleware-mismatch.md`)
- URL-encode path params (from `docs/solutions/security-issues/cli-security-hardening-input-validation.md`)
- Use GFM extension for goldmark (from `docs/solutions/feature-implementations/cli-course-import-markdown-to-tiptap-conversion.md`)
- Extract shared functions to prevent divergence bugs (from `docs/solutions/feature-implementations/cli-course-module-management-commands.md`)
