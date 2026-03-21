package main

import (
	"strings"
	"testing"
)

// Valid 56-char hex policy IDs for tests
const (
	testPolicyA = "722c475bebb106799b109fc95301c9b796e1a37b6afc601359d54a04"
	testPolicyB = "a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2e3f4a5b6c7d8"
)

func TestParseTokenFlags(t *testing.T) {
	tests := []struct {
		name    string
		input   []string
		want    []TaskToken
		wantErr string
	}{
		{
			name:  "single token - human-readable auto-hex-encoded",
			input: []string{testPolicyA + ",XP,50"},
			want:  []TaskToken{{PolicyID: testPolicyA, AssetName: "5850", Quantity: "50"}},
		},
		{
			name:  "multiple tokens - auto-hex-encoded",
			input: []string{testPolicyA + ",XP,50", testPolicyB + ",RewardToken,100"},
			want: []TaskToken{
				{PolicyID: testPolicyA, AssetName: "5850", Quantity: "50"},
				{PolicyID: testPolicyB, AssetName: "526577617264546f6b656e", Quantity: "100"},
			},
		},
		{
			name:  "already-hex asset name passed through",
			input: []string{testPolicyA + ",5850,50"},
			want:  []TaskToken{{PolicyID: testPolicyA, AssetName: "5850", Quantity: "50"}},
		},
		{
			name:  "empty asset name allowed",
			input: []string{testPolicyA + ",,50"},
			want:  []TaskToken{{PolicyID: testPolicyA, AssetName: "", Quantity: "50"}},
		},
		{
			name:  "whitespace trimmed then hex-encoded",
			input: []string{testPolicyA + " , XP , 50"},
			want:  []TaskToken{{PolicyID: testPolicyA, AssetName: "5850", Quantity: "50"}},
		},
		{
			name:  "odd-length hex-like string is encoded (not valid hex)",
			input: []string{testPolicyA + ",ABC,50"},
			want:  []TaskToken{{PolicyID: testPolicyA, AssetName: "414243", Quantity: "50"}},
		},
		{
			name:    "wrong field count - too few",
			input:   []string{testPolicyA + ",XP"},
			wantErr: `invalid --token format`,
		},
		{
			name:    "single value no commas",
			input:   []string{"bad"},
			wantErr: `invalid --token format "bad"`,
		},
		{
			name:    "empty policy_id",
			input:   []string{",XP,50"},
			wantErr: "policy_id cannot be empty",
		},
		{
			name:    "policy_id wrong length",
			input:   []string{"abc123,XP,50"},
			wantErr: "must be 56 hex characters",
		},
		{
			name:    "policy_id not hex",
			input:   []string{"zzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzz,XP,50"},
			wantErr: "must be hexadecimal",
		},
		{
			name:    "empty quantity",
			input:   []string{testPolicyA + ",XP,"},
			wantErr: "quantity cannot be empty",
		},
		{
			name:    "non-numeric quantity",
			input:   []string{testPolicyA + ",XP,abc"},
			wantErr: `invalid --token quantity "abc"`,
		},
		{
			name:    "negative quantity",
			input:   []string{testPolicyA + ",XP,-5"},
			wantErr: `invalid --token quantity "-5"`,
		},
		{
			name:    "duplicate token (after hex encoding)",
			input:   []string{testPolicyA + ",XP,50", testPolicyA + ",XP,100"},
			wantErr: "duplicate token",
		},
		{
			name:    "extra commas preserved in quantity field via SplitN",
			input:   []string{testPolicyA + ",XP,50,extra"},
			wantErr: `invalid --token quantity "50,extra"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseTokenFlags(tt.input)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("expected error containing %q, got %q", tt.wantErr, err.Error())
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("got %d tokens, want %d", len(got), len(tt.want))
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("token[%d] = %+v, want %+v", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestHexEncodeAssetName(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"human-readable", "XP", "5850"},
		{"already hex", "5850", "5850"},
		{"empty", "", ""},
		{"long hex", "526577617264546f6b656e", "526577617264546f6b656e"},
		{"odd-length not hex", "ABC", "414243"},
		{"unicode", "caf\u00e9", "636166c3a9"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := hexEncodeAssetName(tt.input)
			if got != tt.want {
				t.Errorf("hexEncodeAssetName(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestHexDecodeAssetName(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"decode XP", "5850", "XP"},
		{"decode RewardToken", "526577617264546f6b656e", "RewardToken"},
		{"empty", "", ""},
		{"invalid hex", "zzzz", "zzzz"},
		{"already text (odd length)", "XP", "XP"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := hexDecodeAssetName(tt.input)
			if got != tt.want {
				t.Errorf("hexDecodeAssetName(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestHexRoundTrip(t *testing.T) {
	// Encode then decode should return original
	names := []string{"XP", "RewardToken", "MyToken123"}
	for _, name := range names {
		encoded := hexEncodeAssetName(name)
		decoded := hexDecodeAssetName(encoded)
		if decoded != name {
			t.Errorf("round-trip failed for %q: encoded=%q decoded=%q", name, encoded, decoded)
		}
	}
}
