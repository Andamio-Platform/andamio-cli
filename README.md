# Andamio CLI

CLI for interacting with the Andamio Protocol.

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
VERSION=0.3.0
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

- `course list` — List available courses
- `course get <course-id>` — Get course details
- `course modules <course-id>` — List modules for a course
- `course slts <course-id> <module-code>` — List SLTs for a module
- `course lesson <course-id> <module-code> <slt-index>` — Get lesson content
- `course assignment <course-id> <module-code>` — Get assignment
- `course intro <course-id> <module-code>` — Get module introduction
- `course export <course-id> <module-code>` — Export module to local directory
- `course import <path> --course-id <id>` — Import module from local directory

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
