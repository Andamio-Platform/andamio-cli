# Getting Started with Andamio CLI

Walk a developer through installing and using the Andamio CLI.

## Interactive Walkthrough

Guide the user step-by-step. Pause after each section and ask if they want to continue or skip ahead.

### 1. Install & Verify

Check if the CLI is installed:

```bash
andamio --version
```

If not installed, offer two paths:

**Download a release** (no Go required):
```bash
# Check latest version at https://github.com/Andamio-Platform/andamio-cli/releases/latest
VERSION=0.1.0
curl -sLO "https://github.com/Andamio-Platform/andamio-cli/releases/download/v${VERSION}/andamio_${VERSION}_darwin_arm64.tar.gz"
tar xzf "andamio_${VERSION}_darwin_arm64.tar.gz"
sudo mv andamio /usr/local/bin/
```

**Build from source** (requires Go 1.21+):
```bash
go install github.com/Andamio-Platform/andamio-cli/cmd/andamio@latest
```

Or build locally in this repo:
```bash
go build -o andamio ./cmd/andamio
```

### 2. Configure Authentication

The CLI supports two auth methods. Start with an API key for read access:

```bash
# Get an API key from https://preprod.app.andamio.io/api-setup
andamio auth login --api-key <your-api-key>
andamio auth status
```

For edit access (course/project owners), authenticate with your Cardano wallet:

```bash
andamio user login
# Opens browser → connect wallet → sign message → JWT stored automatically
andamio user status   # Shows both API key and JWT status
```

### 3. Check Configuration

```bash
andamio config show
```

Default environment is preprod. To switch:
```bash
andamio config set-url https://mainnet.api.andamio.io  # mainnet
andamio config set-url https://preprod.api.andamio.io  # back to preprod
```

Config is stored at `~/.andamio/config.json`.

### 4. Explore Courses

Walk through the course hierarchy:

```bash
# List all courses you have access to
andamio course list

# Get details on a specific course
andamio course get <course-id>

# See the modules in a course
andamio course modules <course-id>

# Drill into a module's SLTs (Student Learning Targets)
andamio course slts <course-id> <module-code>

# Read a specific lesson
andamio course lesson <course-id> <module-code> <slt-index>

# Read module introduction and assignment
andamio course intro <course-id> <module-code>
andamio course assignment <course-id> <module-code>
```

### 5. Output Formats

Every command supports multiple output formats via the `-o` flag:

```bash
andamio course list                # Default text: "- Title (ID)"
andamio course list -o json        # Raw JSON for scripting
andamio course list -o csv         # CSV for spreadsheets
andamio course list -o markdown    # Markdown tables for docs
```

This makes the CLI useful for both interactive use and piping to other tools.

### 6. Discover API Endpoints

Use the spec commands to see what the API offers:

```bash
andamio spec fetch                    # Download OpenAPI spec
andamio spec paths                    # List all endpoints
andamio spec paths --filter course    # Filter by keyword
andamio spec paths --filter project
```

### 7. Projects & Transactions

```bash
# Browse projects
andamio project list
andamio project get <project-id>

# Check transaction status
andamio tx types
andamio tx pending
andamio tx status <tx-hash>
```

### 8. Next Steps

Based on the developer's role, suggest next steps:

**Course creators:**
- Explore existing course content with the commands above
- Content sync is coming — `sync pull` / `sync push` for local editing (see `docs/PLAN-content-sync.md`)

**App builders:**
- Fork the [app template](https://github.com/Andamio-Platform/andamio-app-template) for a UI
- Use CLI alongside the template for quick data checks

**CLI contributors:**
- `andamio spec paths --filter <keyword>` to find unimplemented endpoints
- Add commands following the pattern in `cmd/andamio/course.go`
- See CLAUDE.md for architecture and command patterns

**Documentation:**
- Full CLI docs at [andamio-docs](https://docs.andamio.io/docs/guides/developers/cli)
