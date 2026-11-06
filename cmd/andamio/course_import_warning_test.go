package main

import "testing"

// TestWarningHidesDBContent pins the merged-read warning texts the gateway
// sends today. The strings are hardcoded here, so this test does not catch a
// change of wording on the gateway side; it records which texts the CLI
// treats as on-chain-only (#186).
func TestWarningHidesDBContent(t *testing.T) {
	tests := []struct {
		name    string
		warning string
		want    bool
	}{
		// The gateway's current wording.
		{"on-chain missing", "On-chain state not available", false},
		{"on-chain missing, showing off-chain", "On-chain state not available, showing off-chain content only", false},
		{"on-chain missing, showing history", "On-chain state not available, showing off-chain history only", false},
		{"off-chain missing, showing on-chain", "Off-chain content not available, showing on-chain data only", true},

		// Case and surrounding space do not matter.
		{"lowercase with spaces", "  on-chain state not available, showing off-chain content only  ", false},

		// Anything unrecognised is treated as hiding content.
		{"unrecognised text", "Something else went wrong", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := warningHidesDBContent(tt.warning); got != tt.want {
				t.Errorf("warningHidesDBContent(%q) = %v, want %v", tt.warning, got, tt.want)
			}
		})
	}
}
