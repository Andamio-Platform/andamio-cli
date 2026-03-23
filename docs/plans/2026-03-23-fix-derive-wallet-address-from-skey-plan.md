---
title: "fix: Derive wallet address from skey during headless login"
type: fix
status: completed
date: 2026-03-23
origin: GitHub Issue #40
---

# Derive Wallet Address from Skey During Headless Login

## Problem

`andamio user login --skey ./payment.skey --alias otter` does not store a wallet address in config when the API returns `null` for `cardano_bech32_addr`. This blocks `course student commit-tx` and `project contributor commit-tx`, which require `cfg.UserAddress` for the `initiator_data` build body field.

Re-logging in doesn't fix it because the address comes from the API, not the key.

## Proposed Solution

During headless login, if the API doesn't return an address, derive the enterprise address locally from the skey's public key. The CLI already has all the primitives:

1. `cardano.LoadSigningKey(path)` returns `(privKey, pubKey, error)`
2. `blake2b224(pubKey)` computes the 28-byte key hash
3. `btcsuite/btcd/btcutil/bech32` is already a transitive dependency (via Bursa)

### Cardano Enterprise Address Format

An enterprise address (no staking component) is:

```
header_byte || blake2b_224(pubKey)
```

- Testnet (preprod): header = `0x60`, HRP = `addr_test`
- Mainnet: header = `0x61`, HRP = `addr`

Bech32 encoding of that 29-byte payload with the appropriate HRP gives the address.

### Network Detection

Derive from `cfg.BaseURL`:
- Contains `preprod` or `testnet` -> testnet
- Contains `mainnet` -> mainnet
- Default (localhost, unknown) -> testnet (safe default)

## Implementation

### File: `internal/cardano/address.go` (new)

```go
func DeriveEnterpriseAddress(pubKey ed25519.PublicKey, isMainnet bool) (string, error) {
    keyHash := blake2b224(pubKey)

    header := byte(0x60) // testnet enterprise
    if isMainnet {
        header = 0x61 // mainnet enterprise
    }

    payload := append([]byte{header}, keyHash...)

    // Bech32 encode
    conv, err := bech32.ConvertBits(payload, 8, 5, true)
    if err != nil {
        return "", fmt.Errorf("bech32 convert: %w", err)
    }

    hrp := "addr_test"
    if isMainnet {
        hrp = "addr"
    }

    addr, err := bech32.Encode(hrp, conv)
    if err != nil {
        return "", fmt.Errorf("bech32 encode: %w", err)
    }
    return addr, nil
}
```

### File: `internal/config/config.go`

```go
func (c *Config) IsMainnet() bool {
    return strings.Contains(c.BaseURL, "mainnet")
}
```

### File: `cmd/andamio/user.go` (modify `runHeadlessLogin`)

After the existing address-from-API block (line 335-337), add fallback derivation:

```go
if tokenResp.User.CardanoBech32Addr != nil {
    cfg.UserAddress = *tokenResp.User.CardanoBech32Addr
}
// Derive address from skey if API didn't return one
if cfg.UserAddress == "" {
    _, pubKey, err := cardano.LoadSigningKey(skeyPath)
    if err == nil {
        derived, err := cardano.DeriveEnterpriseAddress(pubKey, cfg.IsMainnet())
        if err == nil {
            cfg.UserAddress = derived
            if !isJSON {
                fmt.Fprintf(os.Stderr, "Derived address from signing key: %s\n", derived)
            }
        }
    }
}
```

Note: `skeyPath` is already in scope from the function parameter. No need to reload the key — but `LoadSigningKey` is cheap and the login flow already loaded it once for `SignMessage`. A minor optimization would be to pass the already-loaded pubKey, but it's not worth the API change.

## Acceptance Criteria

- [x] Headless login derives enterprise address when API returns null
- [x] Correct testnet address (`addr_test1...`) for preprod
- [x] Correct mainnet address (`addr1...`) for mainnet
- [x] Derived address matches the `.addr` file from `cardano-cli` for the same key
- [x] `commit-tx` commands work after headless login without manual config editing
- [x] API-returned address still takes precedence when available
- [x] `--output json` includes the derived address
- [x] `DeriveEnterpriseAddress` has unit tests (known test vector)

## Context

- `btcsuite/btcd/btcutil/bech32` is available as transitive dependency via Bursa
- `blake2b224` is already implemented in `internal/cardano/sign.go:206`
- The key hash is already computed and stored as `cfg.UserKeyHash` during login
- The `warnSkeyMismatch` helper in `helpers.go` already uses `PubKeyHash` for validation

## Sources

- GitHub Issue #40
- `internal/cardano/sign.go:206` — `blake2b224()` function
- `internal/cardano/sign.go:213` — `PubKeyHash()` function
- `cmd/andamio/user.go:335-337` — current address storage from API
- CIP-19 address format: header byte + key hash, bech32 encoded
