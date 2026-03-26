package cardano

import (
	"strings"
	"testing"
)

func TestComputeSltHash_OnChainVector(t *testing.T) {
	slts := []string{
		"I can set up a Typescript development environment.",
		"I can use Github CLI to create an issue.",
		"I can run the Andamio T3 App Template locally.",
	}

	hash := ComputeSltHash(slts)
	expected := "eff7d90a6ed2eaf32b523efb25d95f748166158bcce048717a4920478be052cf"

	if hash != expected {
		t.Errorf("hash mismatch:\n  got:      %s\n  expected: %s", hash, expected)
	}
}

func TestComputeSltHash_EmptySlts(t *testing.T) {
	hash := ComputeSltHash([]string{})
	if hash == "" {
		t.Fatal("expected non-empty hash for empty SLT list")
	}
	// Empty indefinite array: 9f ff -> should produce a consistent hash
	if len(hash) != 64 {
		t.Errorf("expected 64-char hex hash, got %d chars", len(hash))
	}
}

func TestComputeSltHash_ChunkedLongString(t *testing.T) {
	// A string > 64 bytes should trigger chunked encoding
	longSlt := strings.Repeat("A", 65)
	hash := ComputeSltHash([]string{longSlt})

	// Must differ from a short string hash
	shortHash := ComputeSltHash([]string{"A"})
	if hash == shortHash {
		t.Error("long and short SLT should produce different hashes")
	}
	if len(hash) != 64 {
		t.Errorf("expected 64-char hex hash, got %d chars", len(hash))
	}
}

func TestComputeSltHash_OrderMatters(t *testing.T) {
	hash1 := ComputeSltHash([]string{"Alpha", "Beta"})
	hash2 := ComputeSltHash([]string{"Beta", "Alpha"})

	if hash1 == hash2 {
		t.Error("different SLT order should produce different hashes")
	}
}
