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
andamio wallet create [--name default] [--network preprod|mainnet] [--output-dir <path>] [--no-write-mnemonic] [--output json]
```

- Generates a fresh BIP-39 mnemonic
- Derives payment (and stake) keys from it
- **Default location, zero flags required:** `~/.andamio/wallet/<name>/`
  (`--name` defaults to `default`) — mirrors the CLI's one existing
  filesystem convention, `~/.andamio/config.json`
  (`internal/config/config.go:199`). `--output-dir` overrides for a fully
  custom path. This matters because the CLI is meant to work as a dev-kit
  download-and-go tool — requiring a flag before the first command works
  defeats that.
- Writes `payment.skey` / `payment.vkey` (and `stake.skey` / `stake.vkey`) in
  the same cardano-cli JSON envelope format `tx sign` already reads via
  `LoadKeyFromFile`
- **Mnemonic handling:** write `mnemonic.txt` (`0600`) alongside the keys by
  default, plus a one-time stderr warning banner. This is a deliberate
  departure from "shown once, never stored" wallet-generator convention —
  see the Composability Rules discussion below for why. `--no-write-mnemonic`
  opts back into shown-once-only for anyone who wants it (e.g. mainnet use)
- Prints the derived payment address to stdout (JSON: `{"address": "...",
  "payment_skey_path": "...", "payment_vkey_path": "...", "mnemonic_path":
  "..."}`)
- `tx sign` should default `--skey` to `~/.andamio/wallet/default/payment.skey`
  when the flag is omitted and the file exists (still a flag, still
  overridable — a default lookup, not a prompt)

No key state is stored in CLI *config* (`~/.andamio/config.json` stays
untouched) — same "fully explicit, fully scriptable" posture as
`tx sign --skey` already has (see the 2026-03-18 wallet-signing plan's Key
Decisions). This only adds a way to produce the file that flag already
expects, at a predictable default path.

### Why write the mnemonic to disk (Composability Rules constraint)

`CLAUDE.md`'s Composability Rules require every command to work without a
TTY and forbid stdin reads or interactive pickers — the whole CLI is built
so no command ever blocks waiting on a keypress. That rules out the usual
"type yes to confirm you saved this" gate a wallet generator would normally
use before proceeding. A single non-blocking invocation can't safely make
the mnemonic recoverable only if the user happened to copy it fast enough —
so instead of an interactive confirmation, `wallet create` persists the
mnemonic itself (0600, same directory as the keys), and the command stays a
single atomic action with no exception carved into the no-prompts rule.
This is consistent with the CLI being preprod/dev-kit-first
(`internal/config/config.go`'s default `BaseURL` is preprod) rather than a
production wallet manager; `--no-write-mnemonic` covers anyone who wants
stricter mainnet handling.

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

## Paradigm shift: wallet *kind*, not one wallet story

This todo also exposes something that was previously conflated. `user
login`/`dev login`'s CIP-30 browser-wallet connection is **authentication
only** — it has never produced a signing key, and was never meant to (that
confusion is exactly what prompted this todo). Once the CLI can hold its own
key, "which wallet do I sign with" becomes a real choice with three distinct
answers, not one flow:

| Kind | Key location | Setup needed | Fits |
|---|---|---|---|
| `local` (this todo, new) | CLI-generated, on disk at `~/.andamio/wallet/` | None — works out of the box | Default dev-kit experience, scripting, CI |
| `file` (exists today, `--skey <path>`) | Developer's own `.skey` from elsewhere (`cardano-cli`, hardware wallet export, etc.) | Developer already has one | CI/automation with externally-managed keys |
| `browser` (todo 029, CIP-30 signing — not built) | Never touches disk, stays in Eternl/Lace/Nami | Browser + wallet extension | Less technical / non-developer users, mainnet safety-conscious use |

Worth a `--wallet-type`/config `wallet.kind` flag once more than one of
these exists, so `tx sign` can pick a default without the user re-specifying
`--skey` every time. Not required for v1 of *this* todo (which only builds
`local`), but the command surface (`wallet create`, `--skey` flag naming,
etc.) should be chosen with this three-way split in mind rather than assuming
`local` is the only kind that will ever exist.

## References

- Plan: `docs/plans/2026-03-18-feat-cli-wallet-transaction-signing-plan.md` (shipped `tx sign`/`build`/`submit`/`register`, assumed a `.skey` already exists)
- Brainstorm: `docs/brainstorms/2026-03-18-cli-wallet-signing-brainstorm.md`
- Related but distinct: todo 029 (browser/CIP-30 signing — an alternative to holding a `.skey` at all, not a way to generate one)
- `docs/TX-LOOP-COMMAND-INVENTORY.md` — "No hot-wallet integration for transaction signing" finding, listed as unscoped future work
- Manual workaround this replaces: `docker run --rm inputoutput/cardano-addresses key from-recovery-phrase Shelley` (documented in the ops repo's `clients/projects/andamio/NOTES.md`)
- Bursa source: `github.com/blinklabs-io/bursa` — `GenerateMnemonic`, `NewWallet`, `ExtractKeyFiles`, `WithNetwork` (checked directly against v0.16.0 in the local module cache)

## Notes

- Mnemonic handling resolved: persisted to disk (`0600`) by default rather
  than gated behind a confirmation prompt — see "Why write the mnemonic to
  disk" above. No exception to the no-interactive-prompts rule needed.
  `mnemonic.txt` should get the same permission warning `tx sign` already
  applies to `.skey` files if it ends up group/world-readable.
- Default storage path resolved: `~/.andamio/wallet/<name>/`, mirroring
  `~/.andamio/config.json`. `--output-dir` remains for a fully custom
  location.
- `.skey` file permission hardening should match what `tx sign` already does
  (warn if group/world-readable) — see the security section of the March plan.
- Should probably also support deriving from an *existing* mnemonic
  (`--from-mnemonic`, read from stdin/env, never a flag) for recovery — but
  that's a natural follow-up, not required for v1.
- `wallet.kind`/`--wallet-type` (see Paradigm shift section) is out of scope
  for v1 — file it as a separate follow-up todo once `browser` (todo 029)
  actually exists and there are two-plus kinds to choose between.
