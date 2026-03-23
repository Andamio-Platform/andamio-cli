package cardano

import (
	"crypto/ed25519"
	"encoding/hex"
	"strings"
	"testing"
)

func TestDeriveEnterpriseAddress(t *testing.T) {
	// Use a deterministic seed to get a known key pair
	seed := make([]byte, 32)
	for i := range seed {
		seed[i] = byte(i)
	}
	privKey := ed25519.NewKeyFromSeed(seed)
	pubKey := privKey.Public().(ed25519.PublicKey)

	// Compute expected key hash for verification
	keyHash := blake2b224(pubKey)
	keyHashHex := hex.EncodeToString(keyHash)

	t.Run("testnet address", func(t *testing.T) {
		addr, err := DeriveEnterpriseAddress(pubKey, false)
		if err != nil {
			t.Fatalf("DeriveEnterpriseAddress(testnet) error: %v", err)
		}
		if !strings.HasPrefix(addr, "addr_test1") {
			t.Errorf("testnet address should start with addr_test1, got: %s", addr)
		}
		// Pinned test vector for deterministic seed [0,1,2,...,31]
		wantKeyHash := "27e38d0e19e3434e33fbd001d3fe04b5b76763f88acd625e0d770b43"
		if keyHashHex != wantKeyHash {
			t.Errorf("key hash = %s, want %s", keyHashHex, wantKeyHash)
		}
		wantAddr := "addr_test1vqn78rgwr835xn3nl0gqr5l7qj6mwemrlz9v6cj7p4mskscud5urh"
		if addr != wantAddr {
			t.Errorf("testnet address = %s, want %s", addr, wantAddr)
		}
	})

	t.Run("mainnet address", func(t *testing.T) {
		addr, err := DeriveEnterpriseAddress(pubKey, true)
		if err != nil {
			t.Fatalf("DeriveEnterpriseAddress(mainnet) error: %v", err)
		}
		if !strings.HasPrefix(addr, "addr1") {
			t.Errorf("mainnet address should start with addr1, got: %s", addr)
		}
		// Mainnet should NOT have _test prefix
		if strings.HasPrefix(addr, "addr_test") {
			t.Errorf("mainnet address should not have _test prefix, got: %s", addr)
		}
		t.Logf("mainnet address: %s", addr)
	})

	t.Run("deterministic output", func(t *testing.T) {
		addr1, _ := DeriveEnterpriseAddress(pubKey, false)
		addr2, _ := DeriveEnterpriseAddress(pubKey, false)
		if addr1 != addr2 {
			t.Errorf("same key should produce same address: %s vs %s", addr1, addr2)
		}
	})

	t.Run("different networks produce different addresses", func(t *testing.T) {
		testnet, _ := DeriveEnterpriseAddress(pubKey, false)
		mainnet, _ := DeriveEnterpriseAddress(pubKey, true)
		if testnet == mainnet {
			t.Errorf("testnet and mainnet addresses should differ")
		}
	})

	t.Run("different keys produce different addresses", func(t *testing.T) {
		seed2 := make([]byte, 32)
		for i := range seed2 {
			seed2[i] = byte(i + 100)
		}
		privKey2 := ed25519.NewKeyFromSeed(seed2)
		pubKey2 := privKey2.Public().(ed25519.PublicKey)

		addr1, _ := DeriveEnterpriseAddress(pubKey, false)
		addr2, _ := DeriveEnterpriseAddress(pubKey2, false)
		if addr1 == addr2 {
			t.Errorf("different keys should produce different addresses")
		}
	})
}
