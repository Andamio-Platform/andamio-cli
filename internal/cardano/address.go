package cardano

import (
	"crypto/ed25519"
	"fmt"

	"github.com/btcsuite/btcd/btcutil/bech32"
)

// DeriveEnterpriseAddress derives a Cardano enterprise address (no staking component)
// from an ed25519 public key. Uses blake2b-224 key hash with the appropriate
// network header byte and bech32 encoding per CIP-19.
//
// Testnet: header 0x60, HRP "addr_test"
// Mainnet: header 0x61, HRP "addr"
func DeriveEnterpriseAddress(pubKey ed25519.PublicKey, isMainnet bool) (string, error) {
	keyHash := blake2b224(pubKey)

	header := byte(0x60) // testnet enterprise address
	hrp := "addr_test"
	if isMainnet {
		header = 0x61
		hrp = "addr"
	}

	payload := make([]byte, 0, 29) // 1 header + 28 key hash
	payload = append(payload, header)
	payload = append(payload, keyHash...)

	// Convert 8-bit groups to 5-bit groups for bech32
	conv, err := bech32.ConvertBits(payload, 8, 5, true)
	if err != nil {
		return "", fmt.Errorf("bech32 bit conversion: %w", err)
	}

	addr, err := bech32.Encode(hrp, conv)
	if err != nil {
		return "", fmt.Errorf("bech32 encode: %w", err)
	}

	return addr, nil
}
