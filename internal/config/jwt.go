package config

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"time"
)

// ExpirySkew is the clock-skew allowance applied when deciding whether a
// token is expired. The direction is deliberately conservative: a token is
// treated as expired starting at exp - ExpirySkew, so the CLI never sends a
// token the gateway might already reject because our clock runs behind. The
// cost is a fail-fast that can fire up to 30 seconds early, which is the
// cheap side of the trade — the bug class this closes is "we sent a token
// the gateway rejects and the whole request 401s" (issue #134).
const ExpirySkew = 30 * time.Second

// maxExpSeconds bounds the exp claim to a sane range (year ~36812). Values
// outside (0, maxExpSeconds) are treated as undecodable rather than risking
// implementation-defined float→int conversion on absurd inputs.
const maxExpSeconds = 1 << 40

// TokenExpiry decodes the exp claim from a JWT's payload segment without
// verifying the signature — the token is the CLI's own stored credential and
// the decoded value only drives a send/don't-send decision; the gateway
// remains the authority. Returns ok=false for anything undecodable: not
// three dot-separated segments, bad base64, non-JSON payload, missing or
// non-numeric exp, or an exp outside the sane range. ok=false means "no
// expiry knowable", never "expired" — callers fail open so a decoder edge
// case can't lock anyone out (and non-JWT test fixtures keep working).
func TokenExpiry(token string) (time.Time, bool) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return time.Time{}, false
	}
	// JWTs use raw (unpadded) base64url per RFC 7515, but tolerate padded
	// segments from hand-built or non-conforming producers.
	payload, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(parts[1], "="))
	if err != nil {
		return time.Time{}, false
	}
	// *float64 is deliberately strict: a JSON string ("12345") or any other
	// non-number exp fails the whole decode, landing on ok=false.
	var claims struct {
		Exp *float64 `json:"exp"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return time.Time{}, false
	}
	if claims.Exp == nil {
		return time.Time{}, false
	}
	secs := *claims.Exp
	if secs <= 0 || secs >= maxExpSeconds {
		return time.Time{}, false
	}
	return time.Unix(int64(secs), 0), true
}

// TokenExpired reports whether the token is locally known to be expired at
// now, applying ExpirySkew in the conservative direction (expired from
// exp - ExpirySkew onward). Undecodable tokens are never expired — see
// TokenExpiry's fail-open contract.
func TokenExpired(token string, now time.Time) bool {
	exp, ok := TokenExpiry(token)
	if !ok {
		return false
	}
	return !now.Before(exp.Add(-ExpirySkew))
}

// UserJWTExpired reports whether the stored user JWT is locally known to be
// expired at now. False when no JWT is stored or its expiry is unknowable.
func (c *Config) UserJWTExpired(now time.Time) bool {
	return c.UserJWT != "" && TokenExpired(c.UserJWT, now)
}

// DevJWTExpired reports whether the stored developer JWT is locally known to
// be expired at now. False when no dev JWT is stored or its expiry is
// unknowable.
func (c *Config) DevJWTExpired(now time.Time) bool {
	return c.DevJWT != "" && TokenExpired(c.DevJWT, now)
}

// UserJWTFromEnv reports whether the current user JWT value was injected via
// ANDAMIO_JWT at Load time (and has not been rotated since). Callers use
// this to pick the right recovery hint: "update or unset ANDAMIO_JWT"
// instead of "run 'andamio user login'" — a fresh login is shadowed by the
// env var on the very next Load, so pointing an env-sourced user at login
// would send them in a loop.
func (c *Config) UserJWTFromEnv() bool {
	return c.envInjected.UserJWT != "" && c.UserJWT == c.envInjected.UserJWT
}

// DevJWTFromEnv reports whether the current developer JWT value was injected
// via ANDAMIO_DEV_JWT at Load time (and has not been rotated since). Same
// rationale as UserJWTFromEnv: an env-shadowed slot needs an env-flavored
// recovery hint, because a rotated token is re-shadowed on the next Load.
func (c *Config) DevJWTFromEnv() bool {
	return c.envInjected.DevJWT != "" && c.DevJWT == c.envInjected.DevJWT
}

// HasFreshUserAuth reports whether a user JWT is present and not locally
// known to be expired. Endpoint-routing decisions (teacher-vs-user endpoint
// selection) use this instead of HasUserAuth so a dead JWT doesn't route a
// request onto a JWT-preferred endpoint that will then 401 once the client
// layer drops the expired token.
func (c *Config) HasFreshUserAuth(now time.Time) bool {
	return c.HasUserAuth() && !c.UserJWTExpired(now)
}
