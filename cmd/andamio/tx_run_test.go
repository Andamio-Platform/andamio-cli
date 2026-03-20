package main

import "testing"

func TestParseMetadataFlags(t *testing.T) {
	tests := []struct {
		name    string
		flags   []string
		want    map[string]string
		wantErr bool
	}{
		{
			name:  "nil input",
			flags: nil,
			want:  nil,
		},
		{
			name:  "empty input",
			flags: []string{},
			want:  nil,
		},
		{
			name:  "single pair",
			flags: []string{"task_hash=abc123"},
			want:  map[string]string{"task_hash": "abc123"},
		},
		{
			name:  "multiple pairs",
			flags: []string{"task_hash=abc123", "role=teacher"},
			want:  map[string]string{"task_hash": "abc123", "role": "teacher"},
		},
		{
			name:  "value contains equals",
			flags: []string{"query=x=1&y=2"},
			want:  map[string]string{"query": "x=1&y=2"},
		},
		{
			name:  "empty value",
			flags: []string{"key="},
			want:  map[string]string{"key": ""},
		},
		{
			name:    "missing equals",
			flags:   []string{"noequals"},
			wantErr: true,
		},
		{
			name:    "empty key",
			flags:   []string{"=value"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseMetadataFlags(tt.flags)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseMetadataFlags() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				return
			}
			if tt.want == nil && got != nil {
				t.Errorf("parseMetadataFlags() = %v, want nil", got)
				return
			}
			if len(got) != len(tt.want) {
				t.Errorf("parseMetadataFlags() returned %d entries, want %d", len(got), len(tt.want))
				return
			}
			for k, v := range tt.want {
				if got[k] != v {
					t.Errorf("parseMetadataFlags()[%q] = %q, want %q", k, got[k], v)
				}
			}
		})
	}
}

func TestTruncateTxHash(t *testing.T) {
	tests := []struct {
		hash string
		want string
	}{
		{"abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789", "abcdef01"},
		{"short", "short"},
		{"12345678", "12345678"},
		{"123456789", "12345678"},
		{"", ""},
	}

	for _, tt := range tests {
		got := truncateTxHash(tt.hash)
		if got != tt.want {
			t.Errorf("truncateTxHash(%q) = %q, want %q", tt.hash, got, tt.want)
		}
	}
}
