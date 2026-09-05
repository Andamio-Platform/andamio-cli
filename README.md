# Andamio CLI

A developer tool for the people who author work on the Andamio Protocol and assess it: course **Owners** and **Teachers**, and project **Managers**.

| Role | What you do here |
|------|------------------|
| **Own** | Create courses and projects, register them on-chain, manage teachers and managers |
| **Teach** | Write and publish module content, review submissions, build assessment transactions |
| **Manage** | Create and mint project tasks, review contributions, assess work |

Learners and contributors use the [Andamio app](https://app.andamio.io), which signs and submits their work in one flow.

The CLI is built to be driven by programs as well as people. Every list and get command takes `--output json`; progress goes to stderr and data to stdout; nothing reads stdin or prompts. See [Scripting](#scripting) below.

## Which version you need

> **1.0 requires Andamio API 2.5 or later.** It was built and verified against 2.5 and has never been tested against the 2.4 line.
>
> Both **preprod and mainnet run 2.5** (mainnet cut over on 2026-08-25, `andamio-ops#189`), so 1.0 is the release to use on either. If you are on a self-hosted or pinned gateway older than 2.5, stay on 0.13.x. See the [CHANGELOG](CHANGELOG.md#requires-andamio-api-25-or-later) for what the requirement covers.

## Installation

### Homebrew (macOS)

```bash
brew install Andamio-Platform/tap/andamio-cli
```

### Download a release

Prebuilt binaries for macOS, Linux, and Windows are available on the [Releases page](https://github.com/Andamio-Platform/andamio-cli/releases/latest).

Download the archive for your platform, extract it, and move the binary to your PATH:

```bash
# Example: macOS Apple Silicon — replace VERSION with the latest release
VERSION=1.0.0
curl -sLO "https://github.com/Andamio-Platform/andamio-cli/releases/download/v${VERSION}/andamio_${VERSION}_darwin_arm64.tar.gz"
curl -sLO "https://github.com/Andamio-Platform/andamio-cli/releases/download/v${VERSION}/checksums.txt"
shasum -a 256 --check --ignore-missing checksums.txt
tar xzf "andamio_${VERSION}_darwin_arm64.tar.gz"
sudo mv andamio /usr/local/bin/
```

Available platforms: `darwin_arm64`, `darwin_amd64`, `linux_amd64`, `linux_arm64`, `windows_amd64`, `windows_arm64`.

### Build from source

Requires Go 1.21+.

```bash
go install github.com/Andamio-Platform/andamio-cli/cmd/andamio@latest
```

### Verify

```bash
andamio --version
andamio --help
```

## Quick Start

```bash
# 1. Install the CLI
go install github.com/Andamio-Platform/andamio-cli/cmd/andamio@latest

# 2. Configure your API key (get one from your Andamio dashboard)
andamio auth login --api-key <your-api-key>

# 3. Authenticate with your wallet (for editing courses/projects)
andamio user login

# Or authenticate headlessly with a .skey file (for CI/CD and scripting)
andamio user login --skey ./payment.skey --alias myalias

# 4. Verify everything works
andamio user status
andamio course list
```

## Authentication

The CLI supports two authentication methods:

| Method | Use Case | How to Set Up |
|--------|----------|---------------|
| **API Key** | Read-only access to public endpoints | `andamio auth login --api-key <key>` |
| **User JWT** | Edit courses/projects you own | `andamio user login` |

### Getting a User JWT (Wallet Authentication)

To edit courses or projects, authenticate with your Cardano wallet:

```bash
andamio user login
```

This will:
1. Open your browser to the Andamio app
2. Prompt you to connect your wallet (Nami, Eternl, Lace, etc.)
3. Sign a message to prove ownership of your Access Token
4. Automatically store the JWT for future CLI commands

Check your auth status:
```bash
andamio user status
```

Log out when done:
```bash
andamio user logout
```

## Output Formats

All commands support multiple output formats via the `-o` flag:

```bash
andamio course list                # Default text
andamio course list -o json        # JSON for scripting
andamio course list -o csv         # CSV for spreadsheets
andamio course list -o markdown    # Markdown tables
```

## Scripting

`--output json` is the stable surface. Data goes to stdout, progress and errors
to stderr, and no command reads stdin or prompts — everything works without a
TTY.

Discover ids first, then use them:

```bash
COURSE_ID=$(andamio course list --output json | jq -r '.data[0].course_id')
andamio course modules "$COURSE_ID" --output json | jq -r '.data[].course_module_code'
```

### Exit codes and error kinds

Every failure carries an exit code and, under `--output json`, a `kind` field.
Both come from the same classification, so they never disagree.

| Code | `kind` | When |
|------|--------|------|
| 0 | — | Success, **including an empty but valid result set** |
| 1 | `error` / `server` / `backpressure` / `canceled` | Unexpected, 5xx, retry-later, or interrupted |
| 1 | `verify` | The update was accepted, but the read-back did not confirm the stored value (`course import-assignment`) — inspect, don't retry blindly |
| 2 | `not_found` | Resource doesn't exist |
| 3 | `auth` | No credentials, or 401 / 403 |
| 4 | `removed_command` | Command was retired in 1.0 |
| 5 | `unreachable` | The request never reached the service |
| 6 | `conflict` | Conflicts with existing state |
| 7 | `tier_limit` | Your plan does not permit this action — revoke, upgrade or subscribe; not retryable |

**An empty result is not an error.** A list command that finds nothing emits an
empty collection and exits 0. That is what keeps three different situations
apart — a person infers the difference from context, a program cannot:

```bash
out=$(andamio teacher assignments list --course "$COURSE_ID" --output json)
case $? in
  0) jq '.data | length' <<<"$out" ;;          # 0 means nothing to review
  3) echo "not permitted" >&2 ;;
  5) echo "service unreachable — retry" >&2 ;;
  *) jq -r '.kind + ": " + .error' <<<"$out" >&2 ;;
esac
```

Run `andamio help exit-codes` for the full table.

## Commands

### `andamio config`

- `config show` — Show current configuration
- `config set-url <url>` — Set the API base URL (preprod or mainnet)

### `andamio auth`

- `auth login --api-key <key>` — Store your API key
- `auth status` — Check API key authentication status

### `andamio spec`

- `spec fetch` — Fetch OpenAPI spec from the API and save to `openapi.json`
- `spec paths [--filter <pattern>]` — List available API paths

### `andamio course`

Read:

- `course list` — List available courses
- `course get <course-id>` — Get course details
- `course modules <course-id>` — List modules for a course
- `course slts <course-id> <module-code>` — List SLTs for a module
- `course lesson <course-id> <module-code> <slt-index>` — Get lesson content
- `course assignment <course-id> <module-code>` — Get assignment
- `course intro <course-id> <module-code>` — Get module introduction

Author:

- `course export <course-id> <module-code>` — Export module to local directory
- `course import <path> --course-id <id>` — Import module from local directory
- `course import-assignment <course-id> <module-code> <file.json>` — Publish a quiz assignment (JSON envelope) and verify it by read-back
- `course owner create|update|register` — Create and register a course
- `course owner teachers --course-id <id> --alias <you> --skey <path> --add <alias>` — Manage teachers (on-chain transaction)
- `course teacher register-module|publish-module|update-module-status` — Module lifecycle
- `course credential verify-hash <course-id>` — Verify credential hashes
- `course credential compute-hash` — Compute an SLT hash locally (no auth needed)

### `andamio teacher`

- `teacher courses` — List courses where you are a teacher
- `teacher assignments list [--course <id>]` — List commitments awaiting review
- `teacher assignments get <course-id> <module-code> <student-alias>` — Read one submission
- `teacher assessment build --course-id <id> --alias <you> --decision <student>=accept` — Build an assessment transaction **without signing or submitting it**

`teacher assignments` returns each submission as Markdown in
`content.evidence_text`, alongside the raw Tiptap document in
`content.evidence` — read the first to get the prose, the second to verify a
hash.

`teacher assessment build` stops at the unsigned transaction so a person can
review the decisions before approving them. Sign and submit are separate steps
(`tx sign`, `tx submit`); `tx run` still does the whole lifecycle in one if you
want that.

### `andamio manager`

- `manager projects` — List projects you manage
- `project manager commitments --project-id <id>` — List task commitments, pending and assessed
- `project manager qualified-contributors --project-id <id>` — Who holds every prerequisite SLT

### `andamio project`

- `project list` — List available projects
- `project get <project-id>` — Get project details
- `project task list <project-id>` — List tasks (manager only)
- `project task get <index> --project-id <id>` — Get a task by index
- `project task create <project-id>` — Create a task (`--title`, `--lovelace`, `--expiration` required; `--github-issue` optional)
- `project task update <index> --project-id <id>` — Update task fields
- `project task delete <index> --project-id <id>` — Delete a DRAFT task
- `project task export <project-id>` — Export tasks to `tasks/<slug>/` as Markdown files
- `project task import <project-id>` — Import tasks from Markdown files (`--dry-run` supported)

### `andamio user`

- `user login` — Authenticate via browser wallet signing (get JWT)
- `user logout` — Clear stored user authentication
- `user status` — Show authentication status (API key + JWT)
- `user me` — Get current user info
- `user usage` — Get user usage stats
- `user exists <alias>` — Check if user exists

### `andamio tx`

- `tx pending` — List pending transactions
- `tx types` — List transaction types
- `tx status <tx-hash>` — Get transaction status

### `andamio apikey`

- `apikey usage` — Get API key usage stats
- `apikey profile` — Get API key profile

## Course Import/Export

Export and import course modules for local editing. The format is compatible with [andamio-lesson-coach](https://github.com/Andamio-Platform/andamio-lesson-coach-v2).

### Export

```bash
# Export a module to ./compiled/<course-slug>/<module-code>/
andamio course export <course-id> <module-code>

# Export to a custom directory
andamio course export <course-id> <module-code> --output-dir ./my-courses

# Force overwrite existing export
andamio course export <course-id> <module-code> --force

# JSON output (for scripting)
andamio course export <course-id> <module-code> --output json
```

Export works for modules in any status (DRAFT, APPROVED, ON_CHAIN).

### Import

```bash
# Import a locally-edited module back to the platform
andamio course import ./compiled/my-course/101 --course-id <course-id>

# JSON output
andamio course import ./compiled/my-course/101 --course-id <id> --output json
```

Import automatically:
- Extracts `# H1` headings as titles for lessons, introduction, and assignment
- Uploads new images to the CDN (PNG, JPG, GIF, WebP — max 5MB each)
- Preserves existing CDN image URLs via the image manifest
- Preserves existing metadata (description, image_url, video_url) not present in markdown
- Skips SLT updates for approved/published modules (SLTs are locked after approval)

### Directory Structure

Both commands use this structure (compatible with lesson-coach `/compile` skill):

```
compiled/<course-slug>/<module-code>/
├── outline.md          # YAML frontmatter (title, code) + SLT list
├── introduction.md     # Module introduction (optional)
├── lesson-1.md         # Lesson for SLT 1
├── lesson-2.md         # Lesson for SLT 2
├── ...
├── assignment.md       # Module assignment (optional)
└── assets/             # Images referenced in content
    ├── *.png
    └── .image-manifest.json  # Maps filenames to CDN URLs
```

### File Format

**outline.md** — YAML frontmatter with `title` and `code`, plus numbered SLT list:
```markdown
---
title: "Introduction to Cardano"
code: "101"
---

# Introduction to Cardano

## SLTs

1. Understand blockchain fundamentals
2. Set up a Cardano wallet
```

**lesson-N.md** — First `# H1` becomes the lesson title, rest is content:
```markdown
# Understanding Blockchain

A blockchain is a distributed ledger...

## Key Concepts

- Decentralization
- Immutability
```

**introduction.md** / **assignment.md** — Same format as lessons (H1 = title).

**assignment.quiz.json** — A quiz assignment instead of `assignment.md`. The file is the quiz envelope (`{"type": "quiz", "version": 1, "passThreshold": N, "questions": [...]}`) exactly as the Andamio app stores and grades it; format contract in the app's `docs/quiz-content-format.md`. Export writes this file for a quiz module and no `assignment.md`; import validates it and sends it back verbatim, keeping the module's existing assignment title. A directory holding both `assignment.md` and `assignment.quiz.json` is refused.

### Quiz assignments

Publish a quiz envelope directly, without a module directory:

```bash
# Validate, preview the summary, send nothing
andamio course import-assignment <course-id> 101 quiz.json --dry-run

# Publish, then read back and verify. Title/description come from the existing
# assignment unless overridden; a module with no assignment yet needs --title.
andamio course import-assignment <course-id> 101 quiz.json --title "Module Quiz"

# Scripting
andamio course import-assignment <course-id> 101 quiz.json --output json
# {"course_id":"…","module_code":"101","module_status":"ON_CHAIN",
#  "assignment":{"title":"Module Quiz","title_source":"flag","question_count":5,
#                "pass_threshold":4,"question_ids":["q1","q2","q3","q4","q5"]},
#  "verified":true}
```

The command validates the envelope with the same rules the app enforces (`type`, `version: 1`, non-empty `questions`, unique ids, at least two options with unique values, `correctValue` matching one option, `passThreshold` in `1..len(questions)`, an `intro` that is a Tiptap doc if present). Every violated rule is listed; there is no bypass flag. Only the `assignment` key is sent, so lessons, SLTs, and the introduction are untouched. Assignments are editable in any module status — only SLTs lock after DRAFT. After the update the module is re-fetched and the stored `content_json` is deep-compared to the file; a mismatch or a degraded read-back exits 1 with `kind: verify`, which means the update was applied but should be inspected.

### Image Handling

**Exported images:** Downloaded to `assets/` with a `.image-manifest.json` mapping filenames to their original CDN URLs. On re-import, the manifest restores the original URLs — no re-upload needed.

**New images:** Place new images in `assets/` and reference them in markdown as `![alt](assets/filename.png)`. On import, new images (not in the manifest) are automatically uploaded to the CDN via the app server. The manifest is updated on disk so future imports don't re-upload.

**Supported formats:** PNG, JPEG, GIF, WebP (max 5MB per image).

### Round-Trip Workflow

```bash
# 1. Export
andamio course export <course-id> <module-code>

# 2. Edit locally
vim compiled/my-course/101/lesson-1.md

# 3. Add new images (optional)
cp diagram.png compiled/my-course/101/assets/

# 4. Import back
andamio course import compiled/my-course/101 --course-id <course-id>
```

### Use Cases

- **Local editing:** Edit course content in your preferred editor
- **Version control:** Track course materials in git
- **Round-trip editing:** Export → modify → import
- **Lesson coach integration:** Import modules compiled by lesson-coach
- **Bulk content updates:** Edit multiple lessons at once, import all changes atomically

## Course Creation Workflow

The full workflow for creating a course from scratch. Each step builds on the previous one.

### 1. Create Course On-Chain

```bash
andamio tx run /v2/tx/instance/owner/course/create \
  --body '{"alias":"my-alias","teachers":["my-alias"],"initiator_data":{"change_address":"addr_test1...","used_addresses":["addr_test1..."]}}' \
  --skey ./payment.skey \
  --tx-type course_create
```

This creates the course on-chain and **auto-registers** it in the DB. The response includes the `course_id`.

### 2. Set Course Metadata

```bash
andamio course owner update --course-id <id> --title "My Course" --description "..." --public
```

Use `update`, not `create` or `register` — the course was already registered by the TX in step 1.

### 3. Prepare Module Content

Use [andamio-lesson-coach](https://github.com/Andamio-Platform/andamio-lesson-coach-v2) to create and compile modules, or write markdown files manually following the directory structure in [Course Import/Export](#course-importexport).

### 4. Import Modules to DB

```bash
andamio course import-all ./compiled/my-course --course-id <id> --create
```

Creates DRAFT modules with content. The `--create` flag creates new modules (omit it when updating existing ones).

### 5. Publish Modules On-Chain

```bash
andamio tx run /v2/tx/course/teacher/modules/manage \
  --body-file manage-modules.json \
  --skey ./payment.skey \
  --tx-type modules_manage \
  --instance-id <course-id>
```

### 6. Link On-Chain Modules to DB

For each module, link the on-chain module (identified by `slt_hash`) to the DB module:

```bash
andamio course teacher register-module \
  --course-id <id> --module-code 101 --slt-hash <hash>
```

Get slt_hashes from the TX response or by inspecting the on-chain state.

### 7. Set Modules to DRAFT for Content Import

`register-module` sets the module status to APPROVED, but content import requires DRAFT status:

```bash
andamio course teacher update-module-status \
  --course-id <id> --module-code 101 --status DRAFT
```

### 8. Re-Import Content into Linked Modules

```bash
andamio course import-all ./compiled/my-course --course-id <id>
```

This time without `--create` — the modules already exist. Content (lessons, intro, assignment, SLTs) is imported into the linked modules.

### Common Gotchas

- **`register-module` sets APPROVED**: You must set status back to DRAFT before importing content (step 7). Import skips SLT updates for non-DRAFT modules.
- **Module hash ordering is non-deterministic**: On-chain token names (slt_hashes) don't sort in the same order as your module codes. Check the register response to map hashes to codes.
- **`publish-module` is for DB→chain linking, not on-chain publishing**: To publish modules on-chain, use `tx run` with `modules_manage`. The `publish-module` command links an existing on-chain module to a DB record.
- **`course owner create` vs `update`**: After `tx run` with `course_create`, the course is auto-registered. Use `update` to set metadata. `create` is only needed when auto-registration failed.

## Project Tasks

Project tasks are on-chain bounties that project managers create to reward contributors. All task commands require wallet authentication (`andamio user login`) and manager access on the project.

### Setup

```bash
# Find your project ID once, use it everywhere
export PROJECT_ID=$(andamio project list --output json | jq -r '.data[0].project_id')
```

### CRUD

```bash
# List tasks
andamio project task list "$PROJECT_ID"

# Create a task
andamio project task create "$PROJECT_ID" \
  --title "Implement wallet connect" \
  --lovelace 5000000 \
  --expiration 2026-06-01

# Get a task by index
andamio project task get 1 --project-id "$PROJECT_ID"

# Update fields on a DRAFT task
andamio project task update 1 --project-id "$PROJECT_ID" --lovelace 7000000

# Delete a DRAFT task
andamio project task delete 1 --project-id "$PROJECT_ID"
```

### Link to a GitHub Issue

Use `--github-issue` to prefix the title with the issue reference:

```bash
andamio project task create "$PROJECT_ID" \
  --title "Add dark mode toggle" \
  --github-issue "Andamio-Platform/andamio-cli#42" \
  --lovelace 5000000 \
  --expiration 2026-06-01
```

The stored title becomes `[Andamio-Platform/andamio-cli#42] Add dark mode toggle`.

### GitHub + Andamio Pipeline

All task commands work without a TTY — they compose cleanly with `gh` in scripts and CI:

```bash
# Create one andamio task per open GitHub issue
gh issue list --repo org/repo --json number,title --jq '.[]' | \
while IFS= read -r issue; do
  NUMBER=$(echo "$issue" | jq -r '.number')
  TITLE=$(echo "$issue" | jq -r '.title')
  andamio project task create "$PROJECT_ID" \
    --title "$TITLE" \
    --github-issue "org/repo#$NUMBER" \
    --lovelace 5000000 \
    --expiration 2026-06-01
done
```

### Task Import/Export

Export all tasks to Markdown, edit locally, and reimport:

```bash
# Export
andamio project task export "$PROJECT_ID"
# Creates: tasks/<project-slug>/001-task-title.md, 002-...

# Edit tasks/<project-slug>/*.md in your editor

# Dry run
andamio project task import "$PROJECT_ID" --dry-run

# Import
andamio project task import "$PROJECT_ID"
```

Each exported file has YAML frontmatter (`title`, `lovelace`, `expiration_time`, `index`, `project_id`) and a Markdown body. Files without an `index` field create new tasks on import.

Only DRAFT tasks can be updated or deleted. ACTIVE and COMPLETED tasks are skipped during import.

## Networks

The CLI works with two Cardano networks. Start on preprod for development.

| | Preprod (default) | Mainnet |
|---|---|---|
| API | `https://preprod.api.andamio.io` | `https://mainnet.api.andamio.io` |
| App | [preprod.app.andamio.io](https://preprod.app.andamio.io) | [app.andamio.io](https://app.andamio.io) |
| API key | [preprod.app.andamio.io/api-setup](https://preprod.app.andamio.io/api-setup) | [app.andamio.io/api-setup](https://app.andamio.io/api-setup) |
| Access Token | Free (test ADA) | Requires real ADA |

Switch networks:

```bash
andamio config set-url https://mainnet.api.andamio.io
```

**Important:**
- API keys are network-specific — a preprod key won't work on mainnet
- Wallet auth (`user login`) connects to the app matching your current network
- You need a separate Access Token on each network
- When switching networks, re-authenticate: `andamio auth login --api-key <mainnet-key>`

## Output Formats

All commands support `--output` (`-o`) flag:

```bash
andamio course list                  # Default text output
andamio course list -o json          # JSON (for scripting/piping)
andamio course list -o csv           # CSV
andamio course list -o markdown      # Markdown table
```

## Configuration

Config is stored at `~/.andamio/config.json`:

```json
{
  "api_key": "your-api-key",
  "base_url": "https://preprod.api.andamio.io",
  "user_jwt": "eyJ...",
  "jwt_expires_at": "2026-03-14T12:00:00Z",
  "user_alias": "your-alias",
  "user_id": "user-uuid"
}
```

The `user_*` fields are populated automatically by `andamio user login`.

Available environments:
- `https://preprod.api.andamio.io` (default)
- `https://mainnet.api.andamio.io`

## Development

```bash
# Build
go build -o andamio ./cmd/andamio

# Build with version info
go build -ldflags "-X main.version=0.1.0 -X main.commit=$(git rev-parse --short HEAD) -X main.date=$(date -u +%Y-%m-%dT%H:%M:%SZ)" -o andamio ./cmd/andamio

# Fetch latest API spec
./andamio spec fetch

# Run locally
./andamio --help
./andamio --version
```

### Releasing

```bash
./scripts/release.sh          # Auto-bump patch version
./scripts/release.sh 0.2.0    # Specific version
```

See [CLAUDE.md](CLAUDE.md) for architecture details, command patterns, and how to add new endpoints.

## Documentation

Full documentation: [docs.andamio.io/docs/guides/developers/cli](https://docs.andamio.io/docs/guides/developers/cli)

## License

MIT
