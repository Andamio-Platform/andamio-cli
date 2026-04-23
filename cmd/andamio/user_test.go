package main

import "testing"

// TestSanitizeCallbackValue covers the browser-callback → config → user-status
// pipeline for missing/undefined fields. Locks issue #60's fix: "User ID:
// undefined" never reaches the user, whether they're reading text-mode
// `user status` or parsing `user status --output json`.
func TestSanitizeCallbackValue(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"empty stays empty", "", ""},
		{"normal value passes through", "usr_abc123", "usr_abc123"},
		{"leading/trailing whitespace trimmed", "  usr_abc  ", "usr_abc"},
		{"JavaScript undefined literal dropped", "undefined", ""},
		{"JavaScript null literal dropped", "null", ""},
		{"whitespace around undefined dropped", "  undefined  ", ""},
		{"case-sensitive: 'Undefined' is a real value", "Undefined", "Undefined"},
		{"case-sensitive: 'UNDEFINED' is a real value", "UNDEFINED", "UNDEFINED"},
		{"whitespace only collapses to empty", "   \t\n  ", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := sanitizeCallbackValue(tc.in)
			if got != tc.want {
				t.Errorf("sanitizeCallbackValue(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
