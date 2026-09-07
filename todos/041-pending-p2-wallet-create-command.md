---
status: pending
priority: p2
issue_id: "041"
tags: [wallet, keys, tx, signing, bursa, ux]
dependencies: []
---

# `wallet create` — Generate Signing Keys Locally

## Problem

`tx sign`/`tx run` require a `--skey` file, but the CLI has no way to produce
one. A developer who doesn't already run a Cardano node or have `cardano-cli`/
`cardano-address` installed has no path to a `.skey` at all today — the only
documented workaround (`clients/projects/andamio/NOTES.md` in the ops repo) is
deriving one from a mnemonic via `docker run --rm inputoutput/cardano-addresses
key from-recovery-phrase Shelley`, which requires Docker and IOG's separate
tool.

The existing browser-wallet connection (`user login`/`dev login`, CIP-30) is
authentication only — it does not produce a signing key the CLI can use, and
(per todo 029) CIP-30 signing is a separate, unbuilt, larger piece of work.
Right now there is no in-CLI path from "nothing" to "a `.skey` I can pass to
`tx sign`."

## Proposed Solution

Add `andamio wallet create`, generating a new mnemonic and deriving a full
CIP-1852 key set locally, entirely offline.

```bash
andamio wallet create --output-dir ./payment [--network preprod|mainnet] [--output json]
```

- Generates a fresh BIP-39 mnemonic
- Derives payment (and stake) keys from it
- Writes `payment.skey` / `payment.vkey` (and `stake.skey` / `stake.vkey`) to
  `--output-dir` in the same cardano-cli JSON envelope format `tx sign`
  already reads via `LoadKeyFromFile`
- Prints the mnemonic to stderr exactly once, with a loud one-time warning to
  write it down — never written to disk, never in `--output json`, never in
  logs
- Prints the derived payment address to stdout (JSON: `{"address": "...",
  "payment_skey_path": "...", "payment_vkey_path": "..."}`)

No key state is stored in CLI config — same "fully explicit, fully
scriptable" posture as `tx sign --skey` already has (see the 2026-03-18
wallet-signing plan's Key Decisions). This only adds a way to produce the
file that flag already expects.

## Implementation Option: Bursa (no new dependency)

`github.com/blinklabs-io/bursa` is already a direct dependency (v0.16.0,
`go.mod`), already used in `internal/cardano/sign.go` for *loading* keys, and
turns out to fully cover *generating* them too — this was't obvious until
checking the library source directly:

- `bursa.GenerateMnemonic()` — fresh BIP-39 mnemonic
- `bursa.NewWallet(mnemonic, opts...)` — derives root → account → payment /
  stake / drep / committee / pool-cold keys via CIP-1852, plus the payment
  and stake addresses (`bursa.WithNetwork("preprod"|"mainnet")` controls
  address prefix)
- `bursa.ExtractKeyFiles(wallet)` — returns a `map[string]string` of every
  key already formatted as a cardano-cli JSON envelope, ready to write
  straight to disk with the same filenames `tx sign --skey` expects

This means `wallet create` needs no new external dependency, no shell-out,
and no Docker — pure Go, same library the signing path already trusts. It
directly replaces the manual `docker run inputoutput/cardano-addresses`
workaround.

### Alternatives considered

| Option | Verdict |
|---|---|
| Shell out to IOG's `cardano-address` CLI (what the Docker workaround already does) | Works, but adds a binary/Docker dependency, is platform-specific, and is strictly redundant with what Bursa already does natively — no reason to prefer it |
| Hardware wallet (Ledger/Trezor) key generation | Out of scope — different problem (device-backed keys), no existing precedent in this CLI, large lift |
| UTXOS Wallet-as-a-Service / hosted custody (used elsewhere in Andamio's enterprise-sponsorship model, see `andamio-dev-kit-internal/docs/sponsorship/social-wallet-model.md`) | Wrong fit — that model is deliberately non-custodial from Andamio's side for *enterprise/user*-facing flows via social login; this CLI's whole design principle is the developer holds their own `.skey` locally, not a hosted wallet |

Bursa is the clear choice: zero new deps, consistent with the existing
`tx sign` code path, and it's what should have been reached for from the
start instead of the Docker workaround.

## Why This Matters

- Closes the actual on-ramp gap: `tx sign`/`tx run` are documented and built,
  but there was never a supported way to get the `.skey` they require
  in the first place
- Removes the Docker + IOG-tool dependency currently used ad hoc for this
- Keeps the whole tx pipeline (`wallet create` → `tx build` → `tx sign` →
  `tx submit` → `tx register`) self-contained in the CLI

## References

- Plan: `docs/plans/2026-03-18-feat-cli-wallet-transaction-signing-plan.md` (shipped `tx sign`/`build`/`submit`/`register`, assumed a `.skey` already exists)
- Brainstorm: `docs/brainstorms/2026-03-18-cli-wallet-signing-brainstorm.md`
- Related but distinct: todo 029 (browser/CIP-30 signing — an alternative to holding a `.skey` at all, not a way to generate one)
- `docs/TX-LOOP-COMMAND-INVENTORY.md` — "No hot-wallet integration for transaction signing" finding, listed as unscoped future work
- Manual workaround this replaces: `docker run --rm inputoutput/cardano-addresses key from-recovery-phrase Shelley` (documented in the ops repo's `clients/projects/andamio/NOTES.md`)
- Bursa source: `github.com/blinklabs-io/bursa` — `GenerateMnemonic`, `NewWallet`, `ExtractKeyFiles`, `WithNetwork` (checked directly against v0.16.0 in the local module cache)

## Notes

- Mnemonic display is the one irreversible/security-sensitive step — needs
  careful stderr handling so it can't end up in shell history, `--output json`,
  or a piped log. Consider requiring an explicit `--i-have-saved-the-mnemonic`
  confirmation flag or an interactive confirm before proceeding (note: the CLI
  otherwise has a hard "no interactive prompts" rule — this may need to be the
  one deliberate exception, or handled via a `--confirm` flag instead).
- `.skey` file permission hardening should match what `tx sign` already does
  (warn if group/world-readable) — see the security section of the March plan.
- Should probably also support deriving from an *existing* mnemonic
  (`--from-mnemonic`, read from stdin/env, never a flag) for recovery — but
  that's a natural follow-up, not required for v1.
