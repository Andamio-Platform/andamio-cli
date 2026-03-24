package cardano

import (
	"fmt"
	"testing"
)

// On-chain test vectors from @andamio/core task-hash.test.ts
// Project 490e6da6be3dbfae3baa8431351dc148dd8bdebc62e2dd7772675e76
// All have expiration_time: 1782792000000 and native_assets: []
var onChainVectors = []struct {
	title    string
	lovelace uint64
	hash     string
}{
	{"Introduce Yourself", 5000000, "b1e5c9234e8a4481da7cb3fb525fc54430f8df127ab9f10464ddc8a4e7560614"},
	{"Review the Docs", 8000000, "9d113eafdbe599d624c1ae3e545083e3ec7a053e14ebb6cb730eb3fb59eb3363"},
	{"Find a Typo", 5000000, "c79b778c46a26148c5a33ad669b3452ecf0263539270513003abef73c5858cb2"},
	{"Attend a Sync Call", 8000000, "090391c308370ca1846e6cf39641dc975e8b2f3e370fb812f61bebcacb6902aa"},
	{"Test a Feature", 10000000, "801eae4957a456034025e61f23f2a508eb8a6e15f8d55edb239712033ff06d18"},
	{"Write a How-To", 15000000, "b6ac09b203c7a81d1cd819bc6064eec2f713e64a6cc5a2fac16f864fcfeee949"},
	{"Propose an Improvement", 5000000, "eb14effb2a81bece91708a2fb2478bd36711b06804f1fa5fca049d0a9192c784"},
}

const deadline = uint64(1782792000000)

func TestComputeTaskHash_OnChainVectors(t *testing.T) {
	for _, v := range onChainVectors {
		t.Run(v.title, func(t *testing.T) {
			task := TaskData{
				ProjectContent: v.title,
				ExpirationTime: deadline,
				LovelaceAmount: v.lovelace,
				NativeAssets:   nil,
			}
			hash, err := ComputeTaskHash(task)
			if err != nil {
				t.Fatalf("ComputeTaskHash error: %v", err)
			}
			if hash != v.hash {
				t.Errorf("hash mismatch for %q:\n  got:  %s\n  want: %s", v.title, hash, v.hash)
			}
		})
	}
}

func TestDebugTaskBytes(t *testing.T) {
	// From @andamio/core: "Hi", deadline=1, lovelace=2, assets=[] → d8799f424869010280ff
	task := TaskData{
		ProjectContent: "Hi",
		ExpirationTime: 1,
		LovelaceAmount: 2,
		NativeAssets:   nil,
	}
	bytes, err := DebugTaskBytes(task)
	if err != nil {
		t.Fatalf("DebugTaskBytes error: %v", err)
	}
	want := "d8799f424869010280ff"
	if bytes != want {
		t.Errorf("bytes mismatch:\n  got:  %s\n  want: %s", bytes, want)
	}
}

func TestComputeTaskHash_EmptyContent(t *testing.T) {
	task := TaskData{
		ProjectContent: "",
		ExpirationTime: 0,
		LovelaceAmount: 0,
		NativeAssets:   nil,
	}
	hash, err := ComputeTaskHash(task)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if len(hash) != 64 {
		t.Errorf("hash length = %d, want 64", len(hash))
	}
}

func TestComputeTaskHash_NativeAssets(t *testing.T) {
	task := TaskData{
		ProjectContent: "Test",
		ExpirationTime: 1,
		LovelaceAmount: 1,
		NativeAssets: []NativeAsset{
			{
				PolicyID:  "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
				TokenName: "01",
				Quantity:  1,
			},
		},
	}
	hash, err := ComputeTaskHash(task)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if len(hash) != 64 {
		t.Errorf("hash length = %d, want 64", len(hash))
	}

	// Verify assets are actually included by comparing with empty-assets version
	taskNoAssets := task
	taskNoAssets.NativeAssets = nil
	hashNoAssets, _ := ComputeTaskHash(taskNoAssets)
	if hash == hashNoAssets {
		t.Errorf("hash with assets should differ from hash without assets")
	}
}

func TestComputeTaskHash_MultipleAssets(t *testing.T) {
	policyA := "aa" + "00000000000000000000000000000000000000000000000000000000"[2:]
	policyB := "bb" + "00000000000000000000000000000000000000000000000000000000"[2:]
	task := TaskData{
		ProjectContent: "",
		ExpirationTime: 0,
		LovelaceAmount: 0,
		NativeAssets: []NativeAsset{
			{PolicyID: policyA, TokenName: "01", Quantity: 1},
			{PolicyID: policyB, TokenName: "02", Quantity: 2},
		},
	}
	bytes, err := DebugTaskBytes(task)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	// Should contain both policy IDs in order
	if len(bytes) == 0 {
		t.Fatal("empty bytes")
	}
	// Assets list should NOT be 0x80 (empty)
	if bytes[len(bytes)-4:len(bytes)-2] == "80" {
		t.Error("assets should not be encoded as empty array")
	}
}

func TestComputeTaskHash_UnicodeNFC(t *testing.T) {
	// cafe with combining acute accent vs precomposed — should produce same hash
	task1 := TaskData{
		ProjectContent: "cafe\u0301", // e + combining acute
		ExpirationTime: 1,
		LovelaceAmount: 1,
	}
	task2 := TaskData{
		ProjectContent: "caf\u00e9", // precomposed e
		ExpirationTime: 1,
		LovelaceAmount: 1,
	}
	hash1, _ := ComputeTaskHash(task1)
	hash2, _ := ComputeTaskHash(task2)
	if hash1 != hash2 {
		t.Errorf("NFC normalization failed: %s != %s", hash1, hash2)
	}
}

func TestComputeTaskHash_Validation(t *testing.T) {
	t.Run("rejects content over 140 chars", func(t *testing.T) {
		task := TaskData{ProjectContent: string(make([]byte, 141))}
		_, err := ComputeTaskHash(task)
		if err == nil {
			t.Error("expected error for content > 140 chars")
		}
	})

	t.Run("rejects invalid policyId length", func(t *testing.T) {
		task := TaskData{
			NativeAssets: []NativeAsset{{PolicyID: "abc", TokenName: "", Quantity: 1}},
		}
		_, err := ComputeTaskHash(task)
		if err == nil {
			t.Error("expected error for short policyId")
		}
	})

	t.Run("rejects odd-length tokenName", func(t *testing.T) {
		task := TaskData{
			NativeAssets: []NativeAsset{{
				PolicyID:  "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
				TokenName: "abc",
				Quantity:  1,
			}},
		}
		_, err := ComputeTaskHash(task)
		if err == nil {
			t.Error("expected error for odd-length tokenName")
		}
	})
}

func TestEncodeCBORUint(t *testing.T) {
	tests := []struct {
		input uint64
		want  string
	}{
		{0, "00"},
		{1, "01"},
		{23, "17"},
		{24, "1818"},
		{255, "18ff"},
		{256, "190100"},
		{65535, "19ffff"},
		{65536, "1a00010000"},
		{0x12345678, "1a12345678"},
		{4294967295, "1affffffff"},
		{4294967296, "1b0000000100000000"},
		{1782792000000, "1b0000019f16af1200"}, // the test vector deadline
	}
	for _, tt := range tests {
		got := encodeCBORUint(tt.input)
		gotHex := ""
		for _, b := range got {
			gotHex += fmt.Sprintf("%02x", b)
		}
		if gotHex != tt.want {
			t.Errorf("encodeCBORUint(%d) = %s, want %s", tt.input, gotHex, tt.want)
		}
	}
}
