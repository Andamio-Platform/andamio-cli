---
title: Remove DeriveEnterpriseAddress from login
type: fix
status: completed
date: 2026-03-24
---

# fix: Remove DeriveEnterpriseAddress from login — address is a local concern

## Problem

`andamio user login --skey` derives an enterprise address via `DeriveEnterpriseAddress` and stores it in `cfg.UserAddress`. This is wrong:

1. **Enterprise addresses don't match base addresses.** Wallets with staking keys hold funds at base addresses (`addr_test1q...`), not enterprise addresses (`addr_test1v...`). The derived address points to an empty UTxO set, causing `COLLATERAL_NOT_FOUND` errors.

2. **Address is not the CLI's concern.** The composable `tx run` flow takes `--body` where the user provides `initiator_data.used_addresses` directly. No CLI command reads `cfg.UserAddress`. The address was only written during login and displayed — never consumed.

## Acceptance Criteria

- [x] `DeriveEnterpriseAddress` function and `internal/cardano/address.go` deleted
- [x] `internal/cardano/address_test.go` deleted
- [x] `UserAddress` field removed from `Config` struct in `internal/config/config.go`
- [x] `UserAddress` clearing removed from `ClearUserAuth()` in `internal/config/config.go`
- [x] Address derivation fallback removed from `runHeadlessLogin` in `cmd/andamio/user.go` (lines 340-351)
- [x] `CardanoBech32Addr` field removed from response struct in `cmd/andamio/user.go` (line 313) — dead code without a storage target
- [x] `"address"` removed from JSON output of headless login (line 361)
- [x] `"Address:"` removed from text output of headless login (line 368)
- [x] `go mod tidy` run to demote `btcsuite/btcd/btcutil` from direct to indirect dependency
- [x] `go build ./cmd/andamio` and `go test ./...` pass
- [x] Login still stores JWT, alias, user ID, key hash

## Context

### Why this exists

Address derivation was added in PR #41 (issue #40) as a fallback when the API returned null for `cardano_bech32_addr` during headless login. At the time, `commit-tx` commands needed a stored address. PR #47 subsequently removed `commit-tx` commands, making `UserAddress` completely unused at runtime.

### What stays

| Field | Stored in | Used by |
|-------|-----------|---------|
| `UserJWT` | `config.json` | All JWT-authenticated commands |
| `UserAlias` | `config.json` | `user status` display |
| `UserID` | `config.json` | `user status` display |
| `UserKeyHash` | `config.json` | Persisted for future validation use |
| `JWTExpiresAt` | `config.json` | Session expiry warnings |

### What goes

| Item | File | Lines |
|------|------|-------|
| `DeriveEnterpriseAddress` function | `internal/cardano/address.go` | entire file (43 lines) |
| Address derivation tests | `internal/cardano/address_test.go` | entire file (87 lines) |
| `UserAddress` config field | `internal/config/config.go` | line 19 |
| `UserAddress` clearing | `internal/config/config.go:ClearUserAuth()` | line 30 |
| Address derivation fallback | `cmd/andamio/user.go:runHeadlessLogin` | lines 335-351 |
| `CardanoBech32Addr` response field | `cmd/andamio/user.go` | line 313 |
| `"address"` in JSON output | `cmd/andamio/user.go` | line 361 |
| `"Address:"` in text output | `cmd/andamio/user.go` | line 368 |

### JSON schema change

The `--output json` response from `andamio user login --skey ... --alias ...` drops the `"address"` field:

**Before:** `{"alias":"...","user_id":"...","address":"...","key_hash":"..."}`
**After:** `{"alias":"...","user_id":"...","key_hash":"..."}`

This is acceptable because:
- The address value was actively harmful (enterprise != base), so consumers were getting wrong data
- No CLI command consumed the stored address
- Document the change in release notes

### Backward compatibility

Existing `~/.andamio/config.json` files with a `"user_address"` key: Go's `json.Unmarshal` silently ignores unknown fields, so `config.Load()` will discard the value. The key disappears on next `config.Save()`. This is benign since nothing reads it.

### Dependency cleanup

`address.go` is the only direct import of `github.com/btcsuite/btcd/btcutil/bech32`. After deletion, `go mod tidy` will demote it from direct to indirect (it remains a transitive dependency via Bursa/gouroboros). The `blake2b224` helper in `sign.go` is used by `sign.go` itself and is not affected.

## Sources

- GitHub Issue: #48
- Related brainstorm: [docs/brainstorms/2026-03-18-cli-wallet-signing-brainstorm.md](../brainstorms/2026-03-18-cli-wallet-signing-brainstorm.md) — established that address is the user's concern, not the CLI's
- Original implementation: PR #41 (issue #40) — added address derivation
- Subsequent removal of consumer: PR #47 — removed `commit-tx` commands that were the only motivation for storing address
- Solution doc: [docs/solutions/feature-implementations/cli-onchain-commitment-commands-and-address-derivation.md](../solutions/feature-implementations/cli-onchain-commitment-commands-and-address-derivation.md) — Section 5 documents the feature being removed
