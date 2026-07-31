package config

import (
	"encoding/base64"
	"encoding/json"
	"testing"
	"time"
)

// testJWT builds a structurally valid unsigned JWT whose payload is the
// given claims object. The signature segment is garbage — TokenExpiry never
// verifies it.
func testJWT(t *testing.T, claims map[string]interface{}) string {
	t.Helper()
	payload, err := json.Marshal(claims)
	if err != nil {
		t.Fatalf("marshal claims: %v", err)
	}
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))
	return header + "." + base64.RawURLEncoding.EncodeToString(payload) + ".sig"
}

func TestTokenExpiry_DecodesExp(t *testing.T) {
	exp := time.Now().Add(1 * time.Hour).Unix()
	token := testJWT(t, map[string]interface{}{"exp": exp, "alias": "someone"})

	got, ok := TokenExpiry(token)
	if !ok {
		t.Fatal("expected ok=true for decodable token")
	}
	if got.Unix() != exp {
		t.Errorf("expiry = %d, want %d", got.Unix(), exp)
	}
}

func TestTokenExpiry_ToleratesPaddedSegment(t *testing.T) {
	exp := time.Now().Add(1 * time.Hour).Unix()
	payload, _ := json.Marshal(map[string]interface{}{"exp": exp})
	padded := base64.URLEncoding.EncodeToString(payload) // padded variant
	token := "h." + padded + ".s"

	if _, ok := TokenExpiry(token); !ok {
		t.Error("expected padded base64url payload to decode")
	}
}

func TestTokenExpiry_Undecodable(t *testing.T) {
	cases := map[string]string{
		"non-jwt fixture":  "test-jwt",
		"empty":            "",
		"two segments":     "aaaa.bbbb",
		"four segments":    "a.b.c.d",
		"bad base64":       "h.!!!!.s",
		"non-json payload": "h." + base64.RawURLEncoding.EncodeToString([]byte("not json")) + ".s",
		"whitespace":       "   ",
	}
	for name, token := range cases {
		if _, ok := TokenExpiry(token); ok {
			t.Errorf("%s: expected ok=false", name)
		}
	}
}

func TestTokenExpiry_ExpClaimEdgeCases(t *testing.T) {
	now := time.Now()
	cases := []struct {
		name   string
		claims map[string]interface{}
		wantOK bool
	}{
		{"missing exp", map[string]interface{}{"alias": "x"}, false},
		{"exp as string", map[string]interface{}{"exp": "12345"}, false},
		{"exp non-numeric", map[string]interface{}{"exp": true}, false},
		{"exp zero", map[string]interface{}{"exp": 0}, false},
		{"exp negative", map[string]interface{}{"exp": -100}, false},
		{"exp absurdly large", map[string]interface{}{"exp": 1e30}, false},
		{"exp float seconds", map[string]interface{}{"exp": float64(now.Unix()) + 0.5}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, ok := TokenExpiry(testJWT(t, tc.claims))
			if ok != tc.wantOK {
				t.Errorf("ok = %v, want %v", ok, tc.wantOK)
			}
		})
	}
}

func TestTokenExpired_SkewBoundary(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	cases := []struct {
		name    string
		exp     time.Time
		expired bool
	}{
		{"long past", now.Add(-24 * time.Hour), true},
		{"just past", now.Add(-1 * time.Second), true},
		{"inside skew window", now.Add(ExpirySkew - time.Second), true},
		{"exactly exp-skew boundary", now.Add(ExpirySkew), true},
		{"one second beyond skew", now.Add(ExpirySkew + time.Second), false},
		{"far future", now.Add(1 * time.Hour), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			token := testJWT(t, map[string]interface{}{"exp": tc.exp.Unix()})
			if got := TokenExpired(token, now); got != tc.expired {
				t.Errorf("TokenExpired = %v, want %v", got, tc.expired)
			}
		})
	}
}

func TestTokenExpired_UndecodableNeverExpired(t *testing.T) {
	if TokenExpired("test-jwt", time.Now()) {
		t.Error("undecodable token must not report expired (fail open)")
	}
}

func TestConfigAccessors_ExpiryPredicates(t *testing.T) {
	now := time.Now()
	expired := testJWT(t, map[string]interface{}{"exp": now.Add(-1 * time.Hour).Unix()})
	fresh := testJWT(t, map[string]interface{}{"exp": now.Add(1 * time.Hour).Unix()})

	cfg := &Config{UserJWT: expired, DevJWT: fresh}
	if !cfg.UserJWTExpired(now) {
		t.Error("UserJWTExpired = false, want true")
	}
	if cfg.DevJWTExpired(now) {
		t.Error("DevJWTExpired = true, want false")
	}
	if cfg.HasFreshUserAuth(now) {
		t.Error("HasFreshUserAuth = true for expired JWT, want false")
	}

	cfg = &Config{UserJWT: fresh}
	if !cfg.HasFreshUserAuth(now) {
		t.Error("HasFreshUserAuth = false for fresh JWT, want true")
	}

	// Empty and undecodable slots.
	cfg = &Config{}
	if cfg.UserJWTExpired(now) || cfg.DevJWTExpired(now) {
		t.Error("empty slots must not report expired")
	}
	if cfg.HasFreshUserAuth(now) {
		t.Error("HasFreshUserAuth = true with no JWT, want false")
	}
	cfg = &Config{UserJWT: "test-jwt"}
	if !cfg.HasFreshUserAuth(now) {
		t.Error("HasFreshUserAuth must treat undecodable as fresh (fail open)")
	}
}

func TestUserJWTFromEnv(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	t.Setenv("ANDAMIO_JWT", "env-token")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !cfg.UserJWTFromEnv() {
		t.Error("UserJWTFromEnv = false for env-injected JWT, want true")
	}

	// Rotation: code overwrites the slot → no longer env-sourced.
	cfg.UserJWT = "rotated-token"
	if cfg.UserJWTFromEnv() {
		t.Error("UserJWTFromEnv = true after rotation, want false")
	}
}

func TestUserJWTFromEnv_NoEnv(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("ANDAMIO_JWT", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	cfg.UserJWT = "stored-token"
	if cfg.UserJWTFromEnv() {
		t.Error("UserJWTFromEnv = true for stored JWT, want false")
	}
}
