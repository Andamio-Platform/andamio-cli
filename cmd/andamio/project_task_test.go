package main

import (
	"testing"
)

func TestParseTokenFlags(t *testing.T) {
	tests := []struct {
		name    string
		input   []string
		want    []TaskToken
		wantErr string
	}{
		{
			name:  "single token",
			input: []string{"abc123,XP,50"},
			want:  []TaskToken{{PolicyID: "abc123", AssetName: "XP", Quantity: "50"}},
		},
		{
			name:  "multiple tokens",
			input: []string{"abc123,XP,50", "def456,RewardToken,100"},
			want: []TaskToken{
				{PolicyID: "abc123", AssetName: "XP", Quantity: "50"},
				{PolicyID: "def456", AssetName: "RewardToken", Quantity: "100"},
			},
		},
		{
			name:  "empty asset name allowed",
			input: []string{"abc123,,50"},
			want:  []TaskToken{{PolicyID: "abc123", AssetName: "", Quantity: "50"}},
		},
		{
			name:  "whitespace trimmed",
			input: []string{"abc123 , XP , 50"},
			want:  []TaskToken{{PolicyID: "abc123", AssetName: "XP", Quantity: "50"}},
		},
		{
			name:    "wrong field count - too few",
			input:   []string{"abc123,XP"},
			wantErr: `invalid --token format "abc123,XP"`,
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
			name:    "empty quantity",
			input:   []string{"abc123,XP,"},
			wantErr: "quantity cannot be empty",
		},
		{
			name:    "non-numeric quantity",
			input:   []string{"abc123,XP,abc"},
			wantErr: `invalid --token quantity "abc"`,
		},
		{
			name:    "negative quantity",
			input:   []string{"abc123,XP,-5"},
			wantErr: `invalid --token quantity "-5"`,
		},
		{
			name:    "duplicate token",
			input:   []string{"abc123,XP,50", "abc123,XP,100"},
			wantErr: "duplicate token",
		},
		{
			name:  "extra commas preserved in quantity field via SplitN",
			input: []string{"abc123,XP,50,extra"},
			// SplitN(v, ",", 3) means "50,extra" is the quantity field, which fails validation
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
				if !contains(err.Error(), tt.wantErr) {
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

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
