---
status: pending
priority: p3
issue_id: "029"
tags: [auth, tx, browser, cip-30, ux]
dependencies: []
---

# Browser-Based TX Signing via CIP-30

## Problem

Currently `tx run` and `tx sign` require a `.skey` file on disk. This means
users must extract their private key from their wallet and store it as a file —
a security risk, and a barrier for non-developer users who only have browser
wallets (Eternl, Lace, Nami).

## Proposed Solution

Instead of requiring a `.skey`, open a browser page, have the wallet sign the
unsigned CBOR via CIP-30, and return the signed CBOR to the CLI via a localhost
callback — the same pattern already used for `user login` and `dev login`.

### Flow

```
1. CLI calls POST /v2/tx/{operation} → API returns { "unsigned_tx": "<cbor_hex>" }
2. CLI starts ephemeral localhost server (same pattern as user login)
3. CLI opens browser to {appURL}/tx/sign?cbor=<hex>&redirect_uri=...&state=...
4. App page presents the TX to the user's browser wallet for CIP-30 signing
5. Wallet signs, app redirects back to localhost callback with signed CBOR
6. CLI receives signed CBOR, submits to Cardano network
7. CLI registers tx hash with Andamio API and polls for confirmation
```

Private keys never leave the wallet. No `.skey` file needed.

## Why This Matters

- Makes the CLI usable on mainnet without exposing private keys as files
- Lowers the barrier for non-developer Andamio users
- Consistent with how `user login` and `dev login` already work

## References

- Brainstorm: `docs/brainstorms/2026-03-18-cli-wallet-signing-brainstorm.md`
- Plan: `docs/plans/2026-03-13-feat-browser-wallet-authentication-plan.md`
- Requires a new `/tx/sign` page in `andamio-app-v2`

## Notes

- Large piece of work — spans CLI and app-v2
- `.skey` signing (`tx sign`, `tx run`) stays as the developer/CI path
- Browser signing would be an alternative flag or subcommand, not a replacement
