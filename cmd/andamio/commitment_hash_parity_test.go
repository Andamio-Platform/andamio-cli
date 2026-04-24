package main

import (
	"encoding/hex"
	"encoding/json"
	"testing"

	"github.com/Andamio-Platform/andamio-cli/internal/cardano"
)

// TestCommitmentHashParity_CoreTSVector pins one shared vector against the
// @andamio/core TypeScript implementation of computeCommitmentHash. Both
// sides must produce the SAME 64-char hex hash for this exact input — any
// drift in normalizeForHashing (key sorting, string trimming, null handling)
// or in the JSON serialization / Blake2b-256 step on either side will break
// this test.
//
// Why the vector matters: CLI-submitted evidence must hash to the same
// value the gateway stores on-chain. If the Go and TS implementations drift,
// users see "evidence does not match on-chain commitment" errors with no
// deterministic way to diagnose. This test catches drift at CI time instead.
//
// The canonical input exercises:
//   - a Tiptap doc envelope (type, content)
//   - nested arrays (content → [paragraph] → content → [text])
//   - a mix of object keys that sort non-trivially ("attrs" vs "content" vs
//     "marks" vs "text" vs "type") — key-sort drift would break the hash
//   - a string with interior whitespace (not just leading/trailing) so the
//     trim rule is exercised without being a no-op
//   - a `marks` array with an attribute object nested under it
//
// Vector source of truth: compute via
//
//	node -e 'import("@andamio/core/hashing").then(m =>
//	  console.log(m.computeCommitmentHash(<JSON below>)))'
//
// Verified 2026-04-24 against andamio-core@<current> built at
// ~/projects/01-projects/andamio-core/dist/utils/hashing/index.mjs.
// If this test ever fails after a @andamio/core release, re-run the TS
// computation and update `want` — then open a follow-up to investigate
// why the normalization/hash contract changed.
func TestCommitmentHashParity_CoreTSVector(t *testing.T) {
	doc := map[string]interface{}{
		"type": "doc",
		"content": []interface{}{
			map[string]interface{}{
				"type": "paragraph",
				"content": []interface{}{
					map[string]interface{}{
						"type": "text",
						"text": "My evidence submission",
						"marks": []interface{}{
							map[string]interface{}{
								"type":  "bold",
								"attrs": map[string]interface{}{"level": 1},
							},
						},
					},
				},
			},
		},
	}

	normalized := normalizeForHashing(doc)
	b, err := json.Marshal(normalized)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	got := hex.EncodeToString(cardano.Blake2b256(b))

	// Emitted by @andamio/core computeCommitmentHash on the exact input above.
	const want = "8bd3d0b5a9c157005616a34f3a6ec7ba5d4b4961cc277d408ddac8e86a17434f"

	if got != want {
		t.Errorf("Go-TS parity mismatch on canonical Tiptap doc:\n  go  = %s\n  ts  = %s\n  json = %s", got, want, string(b))
	}
}
