---
status: pending
priority: p2
issue_id: "043"
tags: [auth, wallet, ux, identity]
dependencies: []
---

# Consolidated Auth Status & a Real Identity Model (wallet vs. JWT account)

## Problem

The CLI carries three separate credential types — API key, user JWT, dev
JWT — each with its own expiry and network scoping, plus (as of `wallet
create`, todo 041) a fourth, entirely separate notion of identity: whichever
local wallet you're set up to sign with. Nothing ties these together, and
nothing shows you all of them at once.

Concretely, this surfaced two ways during `feat/wallet-create` E2E testing
(17 Sep 2026 session, see `clients/projects/andamio/NOTES.md` in the ops
repo):

1. **`dev login` and `user login` are both browser flows that link a
   mainnet CIP-30 wallet**, but they authenticate to different things — the
   developer portal (mainnet-only, regardless of target environment) vs.
   whichever gateway is actually configured. Nothing in the CLI's UX
   explains this distinction; it took digging through `andamio-docs` mid-session
   to confirm dev-portal auth is mainnet-only unconditionally. This exact
   ambiguity already caused a real bug once before: `config.BaseURL` was
   found silently pointed at mainnet while `user_alias` implied preprod
   (2026-09-13 session), because nothing surfaces a mismatch between what
   you're configured against and what you're authenticated as.

2. **`andamio user me` answers from the JWT, not the local wallet.** Once
   `wallet create` exists, "which account am I" has at least two possible
   answers — the wallet you're locally set up to sign transactions with, and
   the account your stored JWT says you are — and they can silently diverge
   (different wallet entirely, or same wallet but expired/stale JWT). `user
   me` picks one implicitly with no indication that's what it's doing.

There's also no single command to see all of it at once: `andamio user
status` covers one of the four credential pieces; there's nothing equivalent
for API key, dev JWT, or the local wallet's own state (which alias/Access
Token, if any, it actually holds).

## Proposed Solution (needs its own design pass, not a quick patch)

This is bigger than a single fix and should NOT be folded into
`feat/wallet-create` or any other in-flight branch — it touches the CLI's
identity model, not just one command's output. Two directions worth
separating:

1. **Consolidated status command** — something like `andamio status` or
   `andamio auth status` that shows, in one place: API key (present? which
   network's prefix?), user JWT (present? expiry? which gateway it was
   issued against?), dev JWT (present? expiry?), and the local default
   wallet (address, whether it holds an Access Token/alias). This alone
   would have made both incidents above visible immediately instead of
   requiring manual cross-referencing.

2. **An explicit identity model** — decide what "the current account" means
   when local wallet and JWT-derived identity disagree, and make `user me`
   (and anything else that implicitly picks one) either show both explicitly
   or warn on mismatch, rather than silently preferring the JWT. This is the
   harder, more architectural piece — probably worth a design doc/plan
   before touching code, given it affects every command that currently
   assumes "the JWT is who I am."

## Why This Matters

- The dev/user login confusion isn't hypothetical — it already produced a
  real bug once (2026-09-13 network mismatch)
- `wallet create` (todo 041) added a whole new identity axis (local wallet)
  without the rest of the CLI's auth surface catching up to acknowledge it
  exists
- Silent implicit choices (which JWT, which wallet) are exactly the kind of
  thing that's invisible until it produces a wrong result far downstream —
  same shape of problem as the wallet signing-key bug (todo 041 / commit
  `7a6ec12`), just at the identity layer instead of the crypto layer

## References

- `clients/projects/andamio/NOTES.md` (ops repo) — "Known UX Gaps" #6,
  2026-09-17 session notes (dev-login/user-login walkthrough, `user me`
  example)
- 2026-09-13 session notes (same file) — the network-mismatch bug this
  connects to
- todo 041 (`wallet create`) — introduced the local-wallet identity axis
  this todo is about reconciling with JWT-based identity
- todo 030 (auto-detect network from API key prefix) — adjacent but
  narrower; doesn't cover the JWT/wallet identity question

## Notes

- Explicitly NOT scoped for `feat/wallet-create` — file separately, work on
  its own branch, once write access to the repo is restored (blocked as of
  17 Sep 2026 — see NOTES.md).
- Worth checking whether `andamio dev keys list` / `andamio user status` /
  `andamio auth login --api-key` already return enough raw data that a
  consolidated `status` command is mostly aggregation, or whether new
  gateway calls are needed.
