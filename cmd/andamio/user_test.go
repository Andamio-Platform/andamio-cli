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

// TestBuildAuthURL locks the API-host → app-host rewrite for the browser CLI
// auth page. The production host has no subdomain prefix (api.andamio.io), so
// a plain ".api." replace silently no-ops and sends the browser to the API
// gateway (which 404s on /auth/cli). Both host shapes must rewrite to .app.
func TestBuildAuthURL(t *testing.T) {
	cases := []struct {
		name    string
		baseURL string
		want    string
	}{
		{"production (no prefix)", "https://api.andamio.io", "https://app.andamio.io/auth/cli?redirect_uri=http%3A%2F%2F127.0.0.1%3A55880%2Fcallback&state=abc"},
		{"preprod (subdomain prefix)", "https://preprod.api.andamio.io", "https://preprod.app.andamio.io/auth/cli?redirect_uri=http%3A%2F%2F127.0.0.1%3A55880%2Fcallback&state=abc"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := buildAuthURL(tc.baseURL, "/auth/cli", "http://127.0.0.1:55880/callback", "abc")
			if got != tc.want {
				t.Errorf("buildAuthURL(%q) = %q, want %q", tc.baseURL, got, tc.want)
			}
		})
	}
}
