# Andamio CLI

CLI for interacting with the Andamio Protocol.

## Installation

```bash
go install github.com/Andamio-Platform/andamio-cli/cmd/andamio@latest
```

## Quick Start

```bash
# Authenticate
andamio auth login --api-key <your-api-key>

# List courses
andamio course list

# Export a course
andamio course export <course-id>

# Query credentials
andamio query credential <stake-key>
```

## Commands

### `andamio auth`

- `andamio auth login --api-key <key>` — Store your API key
- `andamio auth status` — Check authentication status

### `andamio course`

- `andamio course list` — List available courses
- `andamio course export <id>` — Export course data as JSON

### `andamio query`

- `andamio query credential <stake-key>` — Query credentials for a stake key

## Configuration

Config is stored at `~/.andamio/config.json`:

```json
{
  "api_key": "your-api-key",
  "base_url": "https://api.andamio.io"
}
```

## Development

```bash
# Build
go build -o andamio ./cmd/andamio

# Run locally
./andamio --help
```
