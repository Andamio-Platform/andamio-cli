package main

import "testing"

// TestWarningHidesDBContent pins the gateway's merged-read warning texts
// (andamio-api internal/orchestration/warnings.go, and the wording before
// andamio-api#893). If the gateway rewords them again, this fails here
// rather than during a chain outage (#186).
func TestWarningHidesDBContent(t *testing.T) {
	tests := []struct {
		name    string
		warning string
		want    bool
	}{
		// Since andamio-api#893.
		{"on-chain missing", "On-chain state not available", false},
		{"on-chain missing, showing off-chain", "On-chain state not available, showing off-chain content only", false},
		{"on-chain missing, showing history", "On-chain state not available, showing off-chain history only", false},
		{"off-chain missing, showing on-chain", "Off-chain content not available, showing on-chain data only", true},

		// Before andamio-api#893, for gateways that have not deployed it.
		{"old andamioscan unavailable", "Andamioscan unavailable, showing database data only", false},
		{"old db api unavailable", "DB API unavailable, showing on-chain data only", true},

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
