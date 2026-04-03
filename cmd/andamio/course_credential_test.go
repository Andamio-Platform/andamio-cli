package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Andamio-Platform/andamio-cli/internal/cardano"
)

func TestRunCredentialComputeHash_FromFlags(t *testing.T) {
	slts := []string{
		"I can set up a Typescript development environment.",
		"I can use Github CLI to create an issue.",
		"I can run the Andamio T3 App Template locally.",
	}
	expected := "eff7d90a6ed2eaf32b523efb25d95f748166158bcce048717a4920478be052cf"

	hash := cardano.ComputeSltHash(slts)
	if hash != expected {
		t.Errorf("hash mismatch: got %s, want %s", hash, expected)
	}
}

func TestRunCredentialComputeHash_FromOutlineFile(t *testing.T) {
	content := `---
title: "Test Module"
code: "101"
---

# Test Module

## SLTs

1. I can set up a Typescript development environment.
2. I can use Github CLI to create an issue.
3. I can run the Andamio T3 App Template locally.
`
	dir := t.TempDir()
	path := filepath.Join(dir, "outline.md")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	slts := parseSLTsFromOutline(content)
	if len(slts) != 3 {
		t.Fatalf("expected 3 SLTs, got %d", len(slts))
	}

	hash := cardano.ComputeSltHash(slts)
	expected := "eff7d90a6ed2eaf32b523efb25d95f748166158bcce048717a4920478be052cf"
	if hash != expected {
		t.Errorf("hash mismatch: got %s, want %s", hash, expected)
	}
}

func TestRunCredentialComputeHash_NoSLTsInFile(t *testing.T) {
	content := `---
title: "Empty Module"
code: "102"
---

# Empty Module

No SLTs section here.
`
	slts := parseSLTsFromOutline(content)
	if len(slts) != 0 {
		t.Errorf("expected 0 SLTs, got %d", len(slts))
	}
}

func TestRunCredentialComputeHash_Deterministic(t *testing.T) {
	slts := []string{"Alpha", "Beta"}
	hash1 := cardano.ComputeSltHash(slts)
	hash2 := cardano.ComputeSltHash(slts)
	if hash1 != hash2 {
		t.Errorf("hash not deterministic: %s != %s", hash1, hash2)
	}
}
