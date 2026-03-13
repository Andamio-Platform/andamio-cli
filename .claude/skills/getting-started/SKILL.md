# Getting Started with Andamio CLI

Walk a developer through the Andamio CLI capabilities and how to extend it with skills.

## Interactive Walkthrough

Guide the user step-by-step through the following. Pause after each section and ask if they want to continue or skip ahead.

### 1. Check Setup

First, verify the CLI is built and configured:

```bash
./andamio --help
./andamio config show
```

If not built, run: `go build -o andamio ./cmd/andamio`

If no API key is set, tell them to get one from the Andamio platform and run:
```bash
./andamio auth login --api-key <their-key>
```

### 2. Explore Available Commands

Show the main command groups:

- **config** - Switch between preprod/mainnet environments
- **auth** - Manage API key authentication
- **spec** - Fetch and explore the OpenAPI spec
- **course** - List courses, modules, lessons, assignments
- **project** - List projects and details
- **user** - Current user info and usage
- **tx** - Transaction status and types
- **apikey** - API key usage stats

Demo a few commands:
```bash
./andamio course list
./andamio tx types
./andamio spec paths --filter course
```

### 3. Discover API Endpoints

Show how to use the spec commands to discover what's available:

```bash
./andamio spec fetch                    # Download latest OpenAPI spec
./andamio spec paths                    # List all endpoints
./andamio spec paths --filter project   # Filter by keyword
```

Explain that all GET endpoints from the API are implemented as CLI commands.

### 4. Switch Environments

Show how to switch between preprod and mainnet:

```bash
./andamio config show
./andamio config set-url https://mainnet.api.andamio.io
./andamio config show
./andamio config set-url https://preprod.api.andamio.io  # switch back
```

### 5. Skills Integration

Explain that this CLI integrates with Claude Code skills:

- **/getting-started** (this skill) - Interactive walkthrough
- Skills can automate common workflows
- Developers can create custom skills in `.claude/skills/`

Show the skill structure:
```
.claude/skills/
  getting-started/
    SKILL.md      # This file - defines the skill behavior
```

### 6. Next Steps

Suggest next steps based on their role:

**For Course Creators:**
- `./andamio course list` to see existing courses
- `./andamio course get <id>` for course details
- `./andamio course modules <id>` to see structure

**For Project Contributors:**
- `./andamio project list` to find projects
- `./andamio project get <id>` for project details

**For Developers extending the CLI:**
- `./andamio spec fetch` to get the latest API spec
- Add new commands following the pattern in `cmd/andamio/course.go`
- Use `getJSON()` helper for simple GET endpoints

Ask if they want to dive deeper into any area.
