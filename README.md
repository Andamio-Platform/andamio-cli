# Andamio CLI

CLI for interacting with the Andamio Protocol.

## Installation

### Requirements

- Go 1.21+ ([install Go](https://go.dev/doc/install))

### Install

```bash
go install github.com/Andamio-Platform/andamio-cli/cmd/andamio@latest
```

Verify installation:
```bash
andamio --help
```

## Quick Start (Andamio Pioneers)

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

## Quick Start

```bash
# Set up API key (for read access)
andamio auth login --api-key <your-api-key>

# Authenticate with wallet (for edit access)
andamio user login

# List courses
andamio course list

# Get course details
andamio course get <course-id>
```

## Authentication

The CLI supports two authentication methods:

| Method | Use Case | How to Set Up |
|--------|----------|---------------|
| **API Key** | Read-only access to public endpoints | `andamio auth login --api-key <key>` |
| **User JWT** | Edit courses/projects you own | `andamio user login` |

### Getting a User JWT (Wallet Authentication)

> **Note:** Wallet authentication requires [andamio-app-v2#439](https://github.com/Andamio-Platform/andamio-app-v2/issues/439) to be deployed.

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

### `andamio project`

- `project list` — List available projects
- `project get <project-id>` — Get project details

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

# Fetch latest API spec
./andamio spec fetch

# Run locally
./andamio --help
```
