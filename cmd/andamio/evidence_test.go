package main

import (
	"encoding/json"
	"testing"
)

func TestWrapEvidence_PlainText(t *testing.T) {
	doc, hash, err := wrapEvidence("My evidence submission")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Must be a Tiptap document
	if doc["type"] != "doc" {
		t.Errorf("expected type=doc, got %v", doc["type"])
	}
	content, ok := doc["content"].([]interface{})
	if !ok || len(content) == 0 {
		t.Fatal("expected non-empty content array")
	}

	// Hash must be 64-char hex (32 bytes = Blake2b-256)
	if len(hash) != 64 {
		t.Errorf("expected 64-char hex hash, got %d chars: %s", len(hash), hash)
	}

	// Must survive JSON round-trip
	if _, err := json.Marshal(doc); err != nil {
		t.Fatalf("document is not JSON-serializable: %v", err)
	}
}

func TestWrapEvidence_URL(t *testing.T) {
	doc, hash, err := wrapEvidence("https://github.com/user/repo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if doc["type"] != "doc" {
		t.Errorf("expected type=doc, got %v", doc["type"])
	}
	if len(hash) != 64 {
		t.Errorf("expected 64-char hex hash, got %d", len(hash))
	}
}

func TestWrapEvidence_MarkdownList(t *testing.T) {
	input := "- item 1\n- item 2\n- item 3"
	doc, _, err := wrapEvidence(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should contain a bulletList node
	content, ok := doc["content"].([]interface{})
	if !ok {
		t.Fatal("expected content array")
	}
	found := false
	for _, node := range content {
		if m, ok := node.(map[string]interface{}); ok && m["type"] == "bulletList" {
			found = true
			break
		}
	}
	if !found {
		jsonBytes, _ := json.Marshal(doc)
		t.Errorf("expected bulletList node in output, got: %s", string(jsonBytes))
	}
}

func TestWrapEvidence_Unicode(t *testing.T) {
	input := "Evidence with CJK: \u4f60\u597d and emoji"
	doc, hash, err := wrapEvidence(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if doc["type"] != "doc" {
		t.Errorf("expected type=doc, got %v", doc["type"])
	}
	if len(hash) != 64 {
		t.Errorf("expected 64-char hash, got %d", len(hash))
	}
}

func TestWrapEvidence_SpecialChars(t *testing.T) {
	input := `He said "hello" and used a \ backslash`
	doc, _, err := wrapEvidence(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Must survive JSON round-trip
	jsonBytes, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("not JSON-serializable: %v", err)
	}
	var roundtrip map[string]interface{}
	if err := json.Unmarshal(jsonBytes, &roundtrip); err != nil {
		t.Fatalf("JSON round-trip failed: %v", err)
	}
}

func TestWrapEvidence_Determinism(t *testing.T) {
	input := "Deterministic evidence test"
	_, hash1, err := wrapEvidence(input)
	if err != nil {
		t.Fatalf("first call: %v", err)
	}
	_, hash2, err := wrapEvidence(input)
	if err != nil {
		t.Fatalf("second call: %v", err)
	}
	if hash1 != hash2 {
		t.Errorf("hashes differ for same input: %s vs %s", hash1, hash2)
	}
}
