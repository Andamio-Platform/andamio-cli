---
title: "feat: Add CLI wallet transaction signing"
type: feat
status: completed
date: 2026-03-18
origin: docs/brainstorms/2026-03-18-cli-wallet-signing-brainstorm.md
---

# feat: Add CLI wallet transaction signing

## Overview

Add local Cardano transaction signing to the Andamio CLI. Developers pass a `.skey` file to sign API-built transactions and submit them directly to the Cardano network — no browser wallet required.

Four composable commands handle the full transaction lifecycle:

```
tx build → tx sign → tx submit → tx register
```

Each command is independent, supports `--output json`, and communicates through flags — fully scriptable, no interactive input.

## Problem Statement / Motivation

Today, all Andamio on-chain operations require a browser wallet (CIP-30). This blocks:

- **Scripting**: Developers can't automate transaction workflows from the terminal
- **CI/CD**: No path to automated deployments or batch operations
- **Developer tooling**: Building on Andamio requires constant browser context-switching

The API already returns unsigned CBOR from 17 tx-building endpoints. The missing piece is local signing and submission.

## Proposed Solution

Use the existing API transaction endpoints (which return `{ "unsigned_tx": "<cbor_hex>" }`) and add CLI-side signing via [Bursa](https://github.com/blinklabs-io/bursa) (Blink Labs) for key loading + Go stdlib for ed25519 signing.

(see brainstorm: `docs/brainstorms/2026-03-18-cli-wallet-signing-brainstorm.md`)

### Architecture

```
┌─────────────┐     ┌──────────────┐     ┌──────────────┐     ┌──────────────┐
│  tx build   │────▶│   tx sign    │────▶│  tx submit   │────▶│ tx register  │
│             │     │              │     │              │     │              │
│ POST to API │     │ Load .skey   │     │ POST CBOR to │     │ POST hash to │
│ Returns     │     │ Hash body    │     │ submit API   │     │ Andamio API  │
│ unsigned_tx │     │ Sign ed25519 │     │ Returns hash │     │ For tracking │
│             │     │ Add witness  │     │              │     │              │
└─────────────┘     └──────────────┘     └──────────────┘     └──────────────┘
     JWT                 local              HTTP POST              JWT
   required            no network         any provider           required
```

### Command Surface

#### `andamio tx build <endpoint> --body <json> [--body-file <path>]`

POST to an Andamio API tx-building endpoint. Returns the full API response (includes `unsigned_tx` plus any endpoint-specific fields like `course_id`, `project_id`, `slt_hashes`).

```bash
andamio tx build /v2/tx/global/user/access-token/mint \
  --body '{"alias":"dev1","initiator_data":"addr_test1..."}' \
  --output json
# → { "unsigned_tx": "84a4..." }

andamio tx build /v2/tx/instance/owner/course/create \
  --body-file create-course.json \
  --output json
# → { "course_id": "abc123...", "unsigned_tx": "84a4..." }
```

| Flag | Required | Description |
|------|----------|-------------|
| `<endpoint>` | yes (arg) | API path, e.g. `/v2/tx/global/user/access-token/mint` |
| `--body` | one of | Inline JSON request body |
| `--body-file` | one of | Path to JSON file (mutually exclusive with `--body`) |

Auth: JWT required. Uses existing `internal/client` POST.

#### `andamio tx sign --tx <cbor_hex> [--tx-file <path>] --skey <path>`

Sign an unsigned transaction locally. Outputs the signed transaction CBOR and tx hash.

```bash
andamio tx sign --tx 84a4... --skey ./payment.skey --output json
# → { "signed_tx": "84a4...", "tx_hash": "abc123..." }

andamio tx sign --tx-file unsigned.cbor --skey ./payment.skey --output json
# → { "signed_tx": "84a4...", "tx_hash": "abc123..." }
```

| Flag | Required | Description |
|------|----------|-------------|
| `--tx` | one of | Unsigned transaction CBOR hex string |
| `--tx-file` | one of | Path to file containing CBOR hex (mutually exclusive with `--tx`) |
| `--skey` | yes | Path to `.skey` file (cardano-cli JSON envelope format) |

Auth: None (purely local operation). No network required.

**Signing process:**
1. Load `.skey` via Bursa `LoadKeyFromFile()` → get raw ed25519 key bytes
2. Decode CBOR hex → extract raw body bytes (index 0 of top-level array) **without re-encoding**
3. Hash body bytes with Blake2b-256
4. Sign hash with ed25519
5. **Pre-flight check**: Extract `required_signers` (CBOR body map key 14) if present, compare `blake2b-224(vkey)` — warn if mismatch
6. Extract existing witness set (index 1), insert VKey witness `[vkey(32), signature(64)]` at map key 0 — **merge, don't replace**
7. Re-assemble transaction: `[original_body_bytes, updated_witness_set, is_valid, auxiliary_data]`
8. Compute tx hash = Blake2b-256 of original body bytes
9. Verify signature before output: `ed25519.Verify(vkey, bodyHash, signature)`
10. Output signed CBOR hex + tx hash

**Critical: CBOR byte preservation.** The body bytes extracted from the unsigned transaction must be hashed as-is. Decoding and re-encoding CBOR can change integer widths, array lengths, or map ordering — which changes the hash and invalidates the signature. Extract raw bytes using CBOR array element boundaries.

#### `andamio tx submit --tx <cbor_hex> [--tx-file <path>] --submit-url <url> [--submit-header <header>]`

Submit a signed transaction to the Cardano network via a submit API.

```bash
andamio tx submit \
  --tx 84a4... \
  --submit-url https://cardano-mainnet.blockfrost.io/api/tx/submit \
  --submit-header "project_id: preprodABC123" \
  --output json
# → { "tx_hash": "abc123..." }
```

| Flag | Required | Description |
|------|----------|-------------|
| `--tx` | one of | Signed transaction CBOR hex string |
| `--tx-file` | one of | Path to file containing signed CBOR hex |
| `--submit-url` | yes* | Submit API URL (*falls back to config `submit_url`) |
| `--submit-header` | no | Additional HTTP header (repeatable, e.g. `"project_id: abc"`) |

Auth: None against Andamio API. Submit API auth via `--submit-header`.

**HTTP details:**
- `POST <submit-url>` with `Content-Type: application/cbor`
- Body: raw CBOR bytes (decoded from hex)
- Does NOT use `internal/client` — uses a dedicated submit client (different content type, no Andamio auth headers, different base URL)
- Passes through error responses from the submit API (provider-specific formats)

#### `andamio tx register --tx-hash <hash> --tx-type <type> [--instance-id <id>]`

Register a submitted transaction with the Andamio API for async tracking.

```bash
andamio tx register --tx-hash abc123... --tx-type access_token_mint --output json
```

| Flag | Required | Description |
|------|----------|-------------|
| `--tx-hash` | yes | 64-character hex transaction hash |
| `--tx-type` | yes | Transaction type (use `andamio tx types` to list valid values) |
| `--instance-id` | no | Course or project ID (for types that return one during build) |

Auth: JWT required.

#### `andamio config set-submit-url <url>`

Persist a default submit URL in config.

```bash
andamio config set-submit-url https://cardano-mainnet.blockfrost.io/api/tx/submit
```

URL validation: require HTTPS for non-localhost, allow any domain (no `andamio.io` restriction). Warn to stderr for HTTP URLs.

### Convenience Script

`scripts/tx-flow.sh` wraps the full pipeline:

```bash
#!/usr/bin/env bash
# Usage: ./scripts/tx-flow.sh <endpoint> <body-json> <skey-path> <tx-type> [instance-id]
set -euo pipefail

# Build
RESULT=$(andamio tx build "$1" --body "$2" --output json)
UNSIGNED_TX=$(echo "$RESULT" | jq -r '.unsigned_tx')

# Sign
SIGNED=$(andamio tx sign --tx "$UNSIGNED_TX" --skey "$3" --output json)
TX_HASH=$(echo "$SIGNED" | jq -r '.tx_hash')
SIGNED_TX=$(echo "$SIGNED" | jq -r '.signed_tx')

# Always echo hash before submit so it's never lost
echo "Transaction hash: $TX_HASH" >&2

# Submit
andamio tx submit --tx "$SIGNED_TX" --output json

# Register
REGISTER_FLAGS="--tx-hash $TX_HASH --tx-type $4"
if [ -n "${5:-}" ]; then
  REGISTER_FLAGS="$REGISTER_FLAGS --instance-id $5"
fi
andamio tx register $REGISTER_FLAGS --output json

# Poll
echo "Polling for confirmation..." >&2
andamio tx status "$TX_HASH"
```

## Technical Considerations

### New Dependencies

| Library | Purpose | Module Path |
|---------|---------|-------------|
| Bursa | `.skey` file loading, key extraction | `github.com/blinklabs-io/bursa` |
| cbor/v2 | CBOR decode (raw byte extraction) | `github.com/fxamacker/cbor/v2` |
| blake2b | Transaction body + key hashing | `golang.org/x/crypto/blake2b` |
| ed25519 | Signing + verification | `crypto/ed25519` (stdlib) |

Bursa pulls in `fxamacker/cbor/v2` transitively, so only two explicit additions to `go.mod`: Bursa and `golang.org/x/crypto`.

### Config Changes

Add to `Config` struct in `internal/config/config.go`:

```go
SubmitURL string `json:"submit_url,omitempty"`
```

- Env var override: `ANDAMIO_SUBMIT_URL`
- Separate validation from `ValidateBaseURL` — allow any domain, require HTTPS for non-localhost
- Add to `config show` output

### HTTP Client for Submit

The existing `internal/client` sends JSON with Andamio auth headers. `tx submit` needs:
- `Content-Type: application/cbor` with raw bytes
- No `X-API-Key` or `Authorization` headers
- Custom headers via `--submit-header`
- Different base URL (submit API, not Andamio API)

Create `internal/submit/client.go` — a focused HTTP client for Cardano submit API calls. Keeps it separate from the Andamio API client.

### Security

- `.skey` file permission check: warn to stderr if group-readable or world-readable (Unix only)
- No key material in stdout, logs, or error messages
- Pre-flight `required_signers` check catches wrong-key errors before submission
- Post-sign verification (`ed25519.Verify`) catches corrupted keys or CBOR bugs
- Submit URL: HTTPS required for non-localhost, warn for HTTP
- (ref: `docs/solutions/security-issues/cli-security-hardening-input-validation.md`)

### CBOR Handling — The Critical Detail

Cardano transaction CBOR must be handled with extreme care:

1. **Never re-encode the body for hashing.** Extract raw bytes from the top-level CBOR array at index 0. Hash those exact bytes.
2. **Witness set merging.** Some API-built transactions include script witnesses, redeemers, or datums in the witness set. The CLI must decode the existing witness set map, insert the VKey witness at key 0, and preserve all other entries.
3. **Transaction structure:** `[body, witness_set, is_valid, auxiliary_data]` — CBOR array of 4 elements.
4. **VKey witness format:** `[vkey_bytes(32), signature_bytes(64)]` — CBOR array, inserted as an element of the set at witness map key 0.

### Auth Requirements

| Command | Auth | Reason |
|---------|------|--------|
| `tx build` | JWT | Write operation against Andamio API |
| `tx sign` | None | Purely local |
| `tx submit` | None (Andamio) | External submit API, auth via `--submit-header` |
| `tx register` | JWT | Write operation against Andamio API |

Use per-command `PreRunE` auth guard (not on `txCmd` parent, since existing `tx pending/types/status` are unauthenticated).

### Error Handling

| Failure | Command | Behavior |
|---------|---------|----------|
| JWT expired | `tx build`, `tx register` | Exit 3, `AuthError` with re-auth instructions |
| `.skey` not found | `tx sign` | Exit 1, "file not found: <path>" |
| `.skey` parse error | `tx sign` | Exit 1, "failed to parse .skey: <detail>" |
| Invalid CBOR hex | `tx sign`, `tx submit` | Exit 1, "invalid CBOR hex" |
| Key mismatch (pre-flight) | `tx sign` | Warning to stderr, proceed (signer may not be in `required_signers`) |
| Signature verification failed | `tx sign` | Exit 1, "signature verification failed — possible key corruption" |
| Submit API unreachable | `tx submit` | Exit 1, "failed to connect to <url>" |
| Transaction rejected by node | `tx submit` | Exit 1, pass through submit API error body |
| Duplicate registration | `tx register` | Pass through API response (let API decide) |

All errors output as `{"error": "..."}` in JSON mode (existing `main.go` behavior).

## Acceptance Criteria

### Functional Requirements

- [ ] `tx build <endpoint> --body <json>` POSTs to API and returns full response including `unsigned_tx`
- [ ] `tx build` supports `--body-file` for JSON from file
- [ ] `tx sign --tx <hex> --skey <path>` produces valid signed transaction CBOR
- [ ] `tx sign` preserves existing witness set entries (merge, not replace)
- [ ] `tx sign` performs pre-flight `required_signers` check and warns on mismatch
- [ ] `tx sign` verifies signature before output
- [ ] `tx submit --tx <hex> --submit-url <url>` POSTs `application/cbor` to submit API
- [ ] `tx submit` supports `--submit-header` (repeatable) for provider auth
- [ ] `tx submit` falls back to config `submit_url` when `--submit-url` flag omitted
- [ ] `tx register --tx-hash <hash> --tx-type <type>` registers with Andamio API
- [ ] `tx register` supports optional `--instance-id`
- [ ] `config set-submit-url <url>` persists to config with HTTPS validation
- [ ] `config show` displays `submit_url` when set
- [ ] All new commands support `--output json` with stable schemas
- [ ] `--tx-file` alternative works for `tx sign` and `tx submit`
- [ ] Auth guard on `tx build` and `tx register` (JWT required)
- [ ] No auth required for `tx sign` (local) and `tx submit` (external)
- [ ] `.skey` file permission warning on Unix when too permissive
- [ ] Progress messages to stderr, structured output to stdout only

### Testing Requirements

- [ ] Unit test: CBOR body byte extraction preserves original bytes (round-trip hash comparison)
- [ ] Unit test: VKey witness insertion into empty witness set
- [ ] Unit test: VKey witness merge into witness set with existing script witnesses
- [ ] Unit test: `.skey` file parsing via Bursa (valid key, invalid format, missing file)
- [ ] Unit test: Blake2b-256 hashing produces expected Cardano tx hash for known transaction
- [ ] Unit test: `--body` / `--body-file` mutual exclusivity
- [ ] Unit test: `--tx` / `--tx-file` mutual exclusivity
- [ ] Integration test: Full pipeline with a known unsigned tx → sign → verify signature is valid

## Implementation Phases

### Phase 1: Foundation

New files and config changes. No new commands yet.

- [x] Add `SubmitURL` to `Config` struct with env var override (`ANDAMIO_SUBMIT_URL`)
- [x] Add `ValidateSubmitURL()` — HTTPS required for non-localhost, any domain allowed
- [x] Add `config set-submit-url` command — `cmd/andamio/config.go`
- [x] Update `config show` to display `submit_url`
- [x] Add dependencies: `github.com/blinklabs-io/bursa`, `golang.org/x/crypto`
- [x] Create `internal/submit/client.go` — HTTP client for Cardano submit API (`application/cbor`, custom headers, no Andamio auth)
- [x] Create `internal/cardano/sign.go` — CBOR body extraction, Blake2b hashing, ed25519 signing, witness assembly, signature verification

### Phase 2: Commands

Four new commands, one file each.

- [x] `cmd/andamio/tx_build.go` — `tx build` command with `--body`/`--body-file`, JWT auth guard, returns full API response
- [x] `cmd/andamio/tx_sign.go` — `tx sign` command with `--tx`/`--tx-file`/`--skey`, CBOR signing via `internal/cardano`, pre-flight check, signature verification
- [x] `cmd/andamio/tx_submit.go` — `tx submit` command with `--tx`/`--tx-file`/`--submit-url`/`--submit-header`, uses `internal/submit`
- [x] `cmd/andamio/tx_register.go` — `tx register` command with `--tx-hash`/`--tx-type`/`--instance-id`, JWT auth guard

### Phase 3: Polish

- [x] `scripts/tx-flow.sh` convenience script
- [x] `.skey` file permission check (warn if too permissive)
- [x] Tests for `internal/cardano/sign.go` (CBOR handling, signing, witness assembly)
- [ ] Tests for `internal/submit/client.go`
- [x] Update CLAUDE.md command reference table

## Dependencies & Risks

| Risk | Impact | Mitigation |
|------|--------|------------|
| CBOR re-encoding invalidates signatures | Critical — all signing breaks | Extract raw body bytes, never re-encode for hashing |
| Witness set replacement destroys script witnesses | Critical — Plutus txs break | Decode existing witness set, merge VKey witness at key 0 |
| Bursa API changes | Low — stable library, v0.16 | Pin version in go.mod |
| Submit API provider differences | Medium — error format varies | Pass through raw error, don't parse provider-specific formats |
| Large CBOR hex exceeds shell arg limits | Medium — affects scripting | `--tx-file` and `--body-file` flags |

## Sources & References

### Origin

- **Brainstorm document:** [docs/brainstorms/2026-03-18-cli-wallet-signing-brainstorm.md](docs/brainstorms/2026-03-18-cli-wallet-signing-brainstorm.md) — Key decisions: API-built/locally-signed architecture, Bursa for key loading, composable individual commands, configurable submit URL, `.skey` files first.

### Internal References

- Config system: `internal/config/config.go`
- HTTP client pattern: `internal/client/client.go`
- Auth guard pattern: `cmd/andamio/teacher.go:12-25`, `cmd/andamio/project_task.go:19-32`
- Existing tx commands: `cmd/andamio/tx.go`
- Security hardening learnings: `docs/solutions/security-issues/cli-security-hardening-input-validation.md`
- Auth header learnings: `docs/solutions/integration-issues/cli-api-auth-middleware-mismatch.md`
- TX hash learnings: `andamio-api/docs/solutions/architecture-patterns/task-hash-self-healing-wholesale-reconciliation.md`

### External References

- Bursa library: https://github.com/blinklabs-io/bursa
- Cardano CBOR transaction spec: CIP-0021 (transaction structure)
- Cardano submit API spec: cardano-submit-api HTTP interface
- fxamacker/cbor/v2: https://github.com/fxamacker/cbor
