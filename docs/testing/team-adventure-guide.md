# Andamio CLI: Team Adventure Guide

Get set up with the Andamio CLI, explore courses, join a project, and create tasks — all from your terminal.

## Prerequisites

- Go 1.21+ installed
- An Andamio API key (self-service — see Chapter 1)
- A Cardano wallet with an Andamio Access Token (for manager operations)

## Chapter 1: Install and Connect

### Install the CLI

Follow instructions in the README.md. Do the docs meet your expectations?

Verify it works:

```bash
andamio --version
```

### Get an API Key

We can test Stripe integration at https://preprod.app.andamio.io. Try to get a subscription:

1. Go to the preprod app and sign up for a subscription (Stripe test mode — no real charges)
2. Once subscribed, provision an API key from your developer dashboard
3. Copy the key

### Authenticate with your API key

```bash
andamio auth login --api-key <your-api-key>
```

Confirm everything is connected:

```bash
andamio auth status
# → Authenticated (API key configured)

andamio config show
# → Base URL: https://preprod.api.andamio.io
# → API Key:  ****... (configured)
```

You're in. The CLI is pointed at preprod and ready to go.

> **Team: try this!** API key provisioning is live. Go through the subscription + key creation flow on preprod and report back on any friction you hit.


## Chapter 2: Explore Courses

List available courses:

```bash
andamio course list
```

Pick one and dig in:

```bash
andamio course get <course-id>
andamio course modules <course-id>
andamio course slts <course-id> <module-code>
andamio course lesson <course-id> <module-code> <slt-index>
```

Try different output formats:

```bash
andamio course list --output json
andamio course list --output markdown
```

Everything composes. Pull a course ID and pipe it forward:

```bash
COURSE_ID=$(andamio course list --output json | jq -r '.data[0].course_id')
andamio course modules "$COURSE_ID" --output json
```

## Chapter 3: Wallet Login and Projects

API key auth gives you read access. For write operations — creating tasks, managing projects — you need a wallet login.

```bash
andamio user login
```

This opens your browser. Connect your Cardano wallet, sign the nonce, and the CLI receives a JWT.

Check your session:

```bash
andamio user status
```

Now explore projects:

```bash
andamio project list
andamio project get <project-id>
```

## Chapter 4: Create and Manage Tasks

List tasks for a project:

```bash
andamio project task list <project-id>
```

Create a task:

```bash
andamio project task create <project-id> \
  --title "Test task from CLI" \
  --lovelace 5000000 \
  --expiration 2026-06-01
```

Update and delete work the same way:

```bash
andamio project task update <index> --project-id <id> --title "Updated title"
andamio project task delete <index> --project-id <id>
```

Export tasks to Markdown, edit locally, import back:

```bash
andamio project task export <project-id>
# edit files in tasks/<slug>/
andamio project task import <project-id> --dry-run
andamio project task import <project-id>
```

## Chapter 5: Using Andamio in an Agent Harness

The CLI is designed for agents. Every command works without a TTY, returns structured JSON, and never prompts for input.

An agent can discover, act, and verify — all through the CLI:

```bash
# discover available work
andamio project task list <project-id> --output json

# read course material for context
andamio course lesson <course-id> <module> <slt> --output json

# create or update tasks
andamio project task create <project-id> --title "..." --lovelace 5000000

# sign and submit transactions
andamio tx sign --tx <hex> --skey agent.skey
andamio tx submit --tx <signed-hex>
```

The pattern: humans delegate via CLI, agents execute via CLI, credentials are verifiable on-chain. Try wiring it into your own agent loop and see what breaks.

## Chapter 6: Transactions

> **Still in development.** The tx commands work but the workflow is rough. Try them out and tell us what's missing.

Build a transaction:

```bash
andamio tx build <endpoint> --body '{"key": "value"}'
```

Sign it locally with your `.skey` file:

```bash
andamio tx sign --tx <unsigned-hex> --skey /path/to/payment.skey
```

Submit to a Cardano node:

```bash
andamio tx submit --tx <signed-hex>
```

Register it for tracking:

```bash
andamio tx register --tx-hash <hash> --tx-type <type>
```

Check status:

```bash
andamio tx status <hash>
andamio tx pending
```

The CLI doesn't submit through the Andamio API — you bring your own submit endpoint. This is intentional: your keys never leave your machine.

Configure a default submit URL so you don't have to pass it every time:

```bash
andamio config set-submit-url https://cardano-preprod.blockfrost.io/api/v0/tx/submit
```

Play with the full build → sign → submit → register loop. It's rough — feedback welcome.
