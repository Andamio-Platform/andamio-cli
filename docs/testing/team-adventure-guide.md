# Andamio CLI: Team Adventure Guide

Get set up with the Andamio CLI, explore courses, join a project, and create tasks — all from your terminal.

## Prerequisites

- Go 1.21+ installed
- An Andamio API key (ask James)
- A Cardano wallet with an Andamio Access Token (for manager operations)

## Chapter 1: Install and Connect

### Install the CLI

```bash
go install github.com/Andamio-Platform/andamio-cli/cmd/andamio@latest
```

Verify it works:

```bash
andamio --version
```

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

> **Known issue:** Free tier rate limits are too low for CLI usage ([andamio-ops#41](https://github.com/Andamio-Platform/andamio-ops/issues/41)). If you hit rate limits, ask James for a key with higher limits. Fix is config-only, pending team review.

> **Getting a preprod subscription via Stripe:** _Details in demo — James will walk through this live._

> **Why no tx submit?** The Andamio API doesn't expose a transaction submit endpoint — by design. In the app, CIP-30 wallets handle submission directly. From the CLI, you submit your own transactions. This is more secure, more decentralized, and means we don't have to run submission infrastructure. Your keys never leave your machine.
>
> To submit transactions, use any Cardano provider. For example with Blockfrost:
>
> ```bash
> curl "https://cardano-preprod.blockfrost.io/api/v0/tx/submit" \
>   -H "project_id: <your-blockfrost-key>" \
>   -H "Content-Type: application/cbor" \
>   --data-binary @tx.signed.cbor
> ```
>
> Ask James for a Blockfrost preprod API key if you need one for the demo.

## Chapter 2: Explore Courses

_Testing in progress..._

## Chapter 3: Wallet Login and Projects

_Testing in progress..._

## Chapter 4: Create and Manage Tasks

_Testing in progress..._

## Chapter 5: Agent Sign-Off Demo (CF Dev Office Hours — Mar 21)

5-minute live demo. The loop:

1. Human delegates a task (CLI)
2. Agent does the work
3. Agent signs off on the response — completing the task on-chain via CLI
4. Credential is verifiable: anyone can check what the agent attested to

This is the bridge: agents read credentials before accepting delegation, humans check attestation history before trusting output.
