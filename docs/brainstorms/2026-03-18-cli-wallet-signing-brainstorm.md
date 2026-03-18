# CLI Wallet Signing — Brainstorm

**Date**: 2026-03-18
**Status**: Draft
**Author**: James + Claude

## What We're Building

Local transaction signing for the Andamio CLI. Developers can pass a Cardano signing key (`.skey` file) to sign API-built transactions and submit them directly to the Cardano network — no browser wallet required.

**Target user**: Developers building on Andamio who want to script transaction workflows from the terminal.

### Flow

```
1. CLI calls POST /v2/tx/{operation} → API returns { "unsigned_tx": "<cbor_hex>" }
2. CLI loads .skey file via Bursa, signs the tx body hash locally (ed25519)
3. CLI submits signed tx to Cardano network via configurable submit API (HTTP POST, application/cbor)
4. CLI registers the tx hash with Andamio API: POST /v2/tx/register { tx_hash, tx_type }
5. CLI polls GET /v2/tx/status/{tx_hash} (or SSE stream) for confirmation
```

## Why This Approach

### API-built, locally signed

The API controls transaction construction (protocol logic, fee calculation, UTxO selection), while the developer's keys never leave their machine. This separation means:

- **Security**: Private keys stay local. No key upload to any server.
- **Correctness**: API handles Andamio protocol constraints. Developers don't need to know CBOR structure or Plutus script details.
- **Simplicity**: CLI only needs to sign and submit — not build transactions.

### .skey files (start here)

Cardano `.skey` files are the standard key format from `cardano-cli`. Developers who run nodes or use devnets already have these. Starting here avoids BIP-39 derivation complexity and hardware wallet integration.

Mnemonic and hardware wallet support can be added later as separate features.

### Bursa (Blink Labs) for key loading

[Bursa](https://github.com/blinklabs-io/bursa) (Apache 2.0, actively maintained, v0.16.0 as of 2026-02-28) handles:
- Parsing cardano-cli JSON envelope format (`.skey` / `.vkey` files)
- Extracting raw ed25519 key bytes
- Compatible with the broader Blink Labs Go ecosystem (gouroboros for future node socket support)

Bursa does NOT sign transactions — it provides key material. Actual signing uses Go's ed25519 standard library on the transaction body hash.

### Direct node submission via configurable submit API

The signed transaction is submitted via HTTP POST to a configurable submit API endpoint. The Cardano submit API spec is standard across providers:

- `POST <endpoint>` with `Content-Type: application/cbor` body
- Works with Blockfrost, Maestro, self-hosted `cardano-submit-api`, or any compatible service

Future: Add gouroboros `LocalTxSubmission` for direct node socket communication.

## API Transaction Endpoints (Already Exist)

The Andamio API has **17 tx-building endpoints** that return `{ "unsigned_tx": "<cbor_hex>" }`. They are organized by system and role:

| Category | Endpoint | Description |
|----------|----------|-------------|
| **Global** | `POST /v2/tx/global/user/access-token/mint` | Mint access token |
| **Instance Owner** | `POST /v2/tx/instance/owner/course/create` | Create course (also returns `course_id`) |
| | `POST /v2/tx/instance/owner/project/create` | Create project (also returns `project_id`) |
| **Course Owner** | `POST /v2/tx/course/owner/teachers/manage` | Manage teachers |
| **Course Teacher** | `POST /v2/tx/course/teacher/modules/manage` | Manage modules (also returns `slt_hashes`) |
| | `POST /v2/tx/course/teacher/assignments/assess` | Assess assignments |
| **Course Student** | `POST /v2/tx/course/student/assignment/commit` | Commit to assignment |
| | `POST /v2/tx/course/student/assignment/update` | Update assignment |
| | `POST /v2/tx/course/student/credential/claim` | Claim course credential |
| **Project Owner** | `POST /v2/tx/project/owner/managers/manage` | Manage managers |
| | `POST /v2/tx/project/owner/contributor-blacklist/manage` | Manage blacklist |
| **Project Manager** | `POST /v2/tx/project/manager/tasks/manage` | Manage tasks |
| | `POST /v2/tx/project/manager/tasks/assess` | Assess tasks |
| **Project Contributor** | `POST /v2/tx/project/contributor/task/commit` | Commit to task |
| | `POST /v2/tx/project/contributor/task/action` | Task action |
| | `POST /v2/tx/project/contributor/credential/claim` | Claim project credential |
| **Project User** | `POST /v2/tx/project/user/treasury/add-funds` | Add funds to treasury |

### Transaction Lifecycle (Post-Submission)

| Step | Endpoint | Purpose |
|------|----------|---------|
| Register | `POST /v2/tx/register` | Register `{ tx_hash, tx_type, instance_id?, metadata? }` for async tracking |
| Poll | `GET /v2/tx/status/{tx_hash}` | Check status: `pending → confirmed → updated` (or `failed`/`expired`) |
| Stream | `GET /v2/tx/stream/{tx_hash}` | SSE stream for real-time status updates |
| Pending | `GET /v2/tx/pending` | List user's pending transactions |
| Types | `GET /v2/tx/types` | List valid `tx_type` values for register |

## Key Decisions

1. **No stored key state.** The `.skey` path is passed via `--skey` flag on every command. No keys in config. Fully explicit, fully scriptable.

2. **API builds, CLI signs.** The CLI never constructs transactions. It receives unsigned CBOR hex from the API's 17 existing tx-building endpoints, signs locally, and submits.

3. **Bursa for key loading.** Use Blink Labs' Bursa library to parse `.skey` files rather than hand-rolling cardano-cli envelope parsing.

4. **Ed25519 signing in Go.** Standard library `crypto/ed25519` for signing the transaction body hash. No need for a heavyweight crypto dependency.

5. **Configurable submit endpoint.** `--submit-url` flag with config persistence (`andamio config set-submit-url <url>`). Same pattern as base URL — store once, override with flag.

6. **Individual commands, composable by design.** Each step (`tx build`, `tx sign`, `tx submit`, `tx register`) is a separate command with JSON output. An example bash script is provided for the full flow.

7. **Start simple, extend later.** v1 = `.skey` + HTTP submit API. Future additions: mnemonic/seed phrases, hardware wallets, gouroboros node socket submission.

## Proposed Command Surface

Four individual commands, each with `--output json` support:

```bash
# 1. Build — request unsigned tx from API
andamio tx build /v2/tx/global/user/access-token/mint \
  --body '{"alias":"dev1","initiator_data":"addr_test1..."}' \
  --output json
# → { "unsigned_tx": "84a4..." }

# 2. Sign — sign unsigned CBOR with local .skey
andamio tx sign --tx 84a4... --skey ./payment.skey --output json
# → { "signed_tx": "84a4...", "tx_hash": "abc123..." }

# 3. Submit — submit signed tx to Cardano network
andamio tx submit --tx 84a4... --submit-url https://submit.example.com --output json
# → { "tx_hash": "abc123..." }

# 4. Register — register tx hash with Andamio for tracking
andamio tx register --tx-hash abc123... --tx-type access_token_mint --output json

# 5. Status — poll for confirmation (already exists)
andamio tx status abc123...
```

### Convenience Script

A bash script (`scripts/tx-flow.sh`) wraps the full pipeline:

```bash
#!/usr/bin/env bash
# Usage: ./scripts/tx-flow.sh <endpoint> <body-json> <skey-path> <tx-type>
set -euo pipefail

UNSIGNED=$(andamio tx build "$1" --body "$2" --output json | jq -r '.unsigned_tx')
SIGNED=$(andamio tx sign --tx "$UNSIGNED" --skey "$3" --output json)
TX_HASH=$(echo "$SIGNED" | jq -r '.tx_hash')
SIGNED_TX=$(echo "$SIGNED" | jq -r '.signed_tx')

andamio tx submit --tx "$SIGNED_TX" --output json
andamio tx register --tx-hash "$TX_HASH" --tx-type "$4"

echo "Submitted: $TX_HASH"
andamio tx status "$TX_HASH"
```

**Composability**: Each step reads from flags only (no stdin for key material). Steps are fully independent and pipeable via `--output json | jq`.

## Dependencies

| Library | Purpose | Module Path |
|---------|---------|-------------|
| Bursa | .skey file loading, key extraction | `github.com/blinklabs-io/bursa` |
| crypto/ed25519 | Transaction signing | stdlib |
| fxamacker/cbor/v2 | CBOR decode/encode (may come via Bursa) | `github.com/fxamacker/cbor/v2` |
| blake2b | Transaction body hashing (Blake2b-256) | `golang.org/x/crypto/blake2b` |

## Resolved Questions

1. **Which API endpoints return unsigned transactions?** → 17 POST endpoints under `/v2/tx/*`, organized by system (course/project/global) and role (owner/teacher/student/manager/contributor). All return `{ "unsigned_tx": "<cbor_hex>" }`.

2. **What happens after submission?** → CLI must register the tx hash with `POST /v2/tx/register` for async tracking. API polls Andamioscan for confirmation and updates state: `pending → confirmed → updated`.

## Resolved Questions

1. **Which API endpoints return unsigned transactions?** → 17 POST endpoints under `/v2/tx/*`, organized by system and role. All return `{ "unsigned_tx": "<cbor_hex>" }`.

2. **What happens after submission?** → CLI registers the tx hash with `POST /v2/tx/register` for async tracking. State: `pending → confirmed → updated`.

3. **Submit URL persistence?** → Yes, `andamio config set-submit-url <url>` stores it in config. `--submit-url` flag overrides. Same pattern as base URL.

4. **All-in-one vs separate steps?** → Individual commands (`tx build`, `tx sign`, `tx submit`, `tx register`) for composability. A convenience bash script wraps the full flow.

5. **CBOR format?** → Full transaction with empty witness set: `[body, witness_set, is_valid, auxiliary_data]`. CLI decodes, extracts body, Blake2b-256 hashes it, signs with ed25519, inserts `[vkey, signature]` into witness set map key 0, re-encodes.

6. **Witness assembly?** → Standard VKey witness: CBOR array `[vkey_bytes(32), signature_bytes(64)]` inserted into witness set map at key 0 (as a set/array of witnesses).
