# Project Task Management

**Date:** 2026-03-18
**Status:** Brainstorm
**Author:** James + Claude

## What We're Building

CLI commands for project managers to create, manage, and bulk-draft tasks for their projects. Follows the same patterns established by course import/export: API CRUD commands plus a file-based export/import workflow for drafting tasks as local Markdown files.

### Command Structure

All commands live under `project` as a subcommand group:

```
andamio project tasks list [project_id]     # List tasks (manager view)
andamio project task get <index>             # Get task details
andamio project task create                  # Create a single task
andamio project task update <index>          # Update a task
andamio project task delete <index>          # Delete a draft task
andamio project task export [project_id]     # Export tasks to local files
andamio project task import [project_id]     # Import tasks from local files
```

### Project Identification

- If `project_id` (policy ID) is passed as argument, use it directly
- If omitted, call `POST /v2/project/manager/projects/list` to list projects where user is manager, then prompt for selection
- This mirrors how course export handles course selection

## Why This Approach

1. **Consistency** — follows the established course export/import patterns that James is already testing
2. **Progressive complexity** — CRUD commands are simple GET/POST wrappers; export/import adds the file-based workflow on top
3. **Manager workflow** — managers often need to draft multiple tasks at once; local files make bulk creation natural
4. **Reuses infrastructure** — JWT auth, Tiptap-to-Markdown conversion, structured output, dry-run support all exist

## Key Decisions

### 1. Local file format: Directory of Markdown files with YAML frontmatter

Each task is a separate `.md` file within a project directory:

```
tasks/<project-name>/
  setup-environment.md
  write-documentation.md
  review-pull-requests.md
```

Each file has YAML frontmatter for metadata and Markdown body for content:

```yaml
---
title: "Set up development environment"
lovelace: "5000000"
expiration_time: "2026-04-01T00:00:00Z"
tokens:
  - policy_id: "abc123..."
    asset_name: "ProjectToken"
    quantity: "100"
index: 0          # populated on export, used for updates
status: "DRAFT"   # populated on export, read-only
---

## Task Description

Set up the development environment following the project README...
```

### 2. Auth: JWT required (manager endpoints)

All task management endpoints are under `/v2/project/manager/` and require JWT auth. Commands will use `PreRunE` to check `cfg.HasUserAuth()`, same as course import.

### 3. Content: Markdown <-> Tiptap conversion

Task `content_json` uses Tiptap document format, same as course lessons. Reuse existing goldmark-based Markdown-to-Tiptap and Tiptap-to-Markdown converters from course import/export.

### 4. API Endpoints

| Command | Endpoint | Method |
|---------|----------|--------|
| `project tasks list` | `/v2/project/manager/tasks/list` | POST |
| `project task create` | `/v2/project/manager/task/create` | POST |
| `project task update` | `/v2/project/manager/task/update` | POST |
| `project task delete` | `/v2/project/manager/task/delete` | POST |

Manager project listing for interactive picker:

| Command | Endpoint | Method |
|---------|----------|--------|
| (internal) | `/v2/project/manager/projects/list` | POST |

### 5. Task data model (from API)

**Create request** (required): `project_state_policy_id`, `title`, `lovelace`, `expiration_time`
**Create request** (optional): `content`, `content_json`, `tokens[]`

**Update request** (required): `project_state_policy_id`, `index`
**Update request** (optional): `title`, `content`, `content_json`, `lovelace`, `expiration_time`, `tokens[]`

**Task response fields**: `task_hash`, `arbitrary_hash`, `index`, `title`, `content`, `content_json`, `lovelace`, `expiration_time`, `created_by`, `status`, `pending_tx_hash`, `tokens[]`, `commitments[]`

### 6. Export/import scope

- **Export**: fetches all tasks for a project, writes each as a Markdown file with frontmatter
- **Import**: reads task files from local directory, creates new tasks or updates existing ones (matched by `index`)
- **New tasks** (no `index` in frontmatter) → `task/create`
- **Existing tasks** (has `index`) → `task/update`
- Support `--dry-run` flag to preview API payloads without sending

## Resolved Questions

- **Command nesting**: Under `project`, not top-level — mirrors API path structure
- **File format**: Directory of Markdown files with frontmatter — matches course lesson pattern
- **Naming**: export/import verbs — consistent with course workflow
- **Project selection**: Both explicit policy ID and interactive picker
- **Task file naming**: `{slug}.md` (e.g. `setup-environment.md`) — simpler, index tracked only in frontmatter
- **Expiration time format**: ISO 8601 in frontmatter (e.g. `2026-04-01T00:00:00Z`), converted to Unix ms on import — human-friendly for editing
- **Token rewards**: Include from the start as YAML arrays in frontmatter (`tokens: [{policy_id, asset_name, quantity}]`)
- **Conflict handling on import**: Warn if task was modified since export, then overwrite. Local files are source of truth but user gets visibility into conflicts
