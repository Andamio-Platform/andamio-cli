package main

import (
	"context"
	"crypto/ed25519"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/Andamio-Platform/andamio-cli/internal/apierr"
	"github.com/Andamio-Platform/andamio-cli/internal/cardano"
	"github.com/Andamio-Platform/andamio-cli/internal/client"
	"github.com/Andamio-Platform/andamio-cli/internal/config"
	"github.com/Andamio-Platform/andamio-cli/internal/output"
	"github.com/spf13/cobra"
)

// Gateway endpoint paths for the CIP-30-verified developer login flow shipped
// in andamio-api #410. The pair (session → complete) mirrors the existing
// developer-registration shape but mints a 60-minute RS256 developer JWT plus
// a 30-day single-use rotation refresh token. The legacy lookup-only path
// `/v2/auth/developer/account/login` returns 410 Gone when the gateway's
// kill-switch flag is on (default true) — the CLI does not call it.
//
// The developer JWT is the credential `/v2/keys` and other developer-portal
// endpoints accept under BearerAuth. Wallet-scoped (user) JWTs are not
// accepted by the developer-JWT middleware and vice versa; this is why the
// CLI keeps the two slots distinct in Config (UserJWT vs DevJWT).
const (
	devLoginSessionPath  = "/api/v2/auth/developer/login/session"
	devLoginCompletePath = "/api/v2/auth/developer/login/complete"
	devTokenRefreshPath  = "/api/v2/auth/developer/token/refresh"
)

var devCmd = &cobra.Command{
	Use:   "dev",
	Short: "Developer-portal operations (login, manage API keys)",
	Long: `Developer-portal commands operate on the developer JWT slot — distinct
from the wallet/user JWT used by course/project commands. The dev JWT is
required for /v2/keys and other developer-scoped endpoints.

Run 'andamio dev login --skey <path> --alias <name> --address <bech32>' to
mint one. The flow mirrors 'user login --skey' but binds the resulting JWT
to your developer account rather than your end-user account.

Environment:
  ANDAMIO_DEV_JWT             Override the stored developer JWT for this
                              process. Parallel to ANDAMIO_JWT for the user
                              slot. Useful for one-off scripted requests.
  ANDAMIO_DEV_REFRESH_TOKEN   Override the stored 30-day rotation refresh
                              token. Lets ephemeral CI/CD agents inject a
                              rotation credential without committing it to
                              the image, run 'dev refresh' once, and read
                              the rotated token from the resulting config.
                              NOTE: env-sourced values are written to
                              ~/.andamio/config.json on the next config
                              save (every successful login, refresh, or
                              logout triggers a save). For truly ephemeral
                              runs, point HOME at a tmpfs or remove the
                              .andamio directory on exit.`,
}

var devLoginCmd = &cobra.Command{
	Use:   "login",
	Short: "Authenticate as a developer via wallet signing (headless CIP-8)",
	Long: `Mint a developer JWT by signing a gateway nonce with your wallet.

Mirrors the headless 'user login --skey' flow but binds the resulting JWT
to your developer account. The developer JWT is required for /v2/keys and
other developer-portal endpoints.

Examples:
  andamio dev login --skey ./payment.skey --alias myalias --address $(cat wallet.addr)`,
	Args: cobra.NoArgs,
	RunE: runDevLogin,
}

var devLogoutCmd = &cobra.Command{
	Use:   "logout",
	Short: "Clear stored developer JWT and refresh token",
	Long: `Clear the stored developer JWT and refresh token. Does not affect the
wallet/user JWT — 'andamio user logout' clears that slot independently.

After logout, 'dev refresh' will fail; re-run 'dev login' to mint a new
session.

Caveat: if ANDAMIO_DEV_REFRESH_TOKEN (or ANDAMIO_DEV_JWT) is exported in
your environment, logout clears the on-disk slot but the next CLI
invocation re-injects the env value via Load(). For ephemeral CI/CD
runs that need true logout, unset the env var(s) before relying on
logout, or use a tmpfs HOME and discard the directory on exit.`,
	Args: cobra.NoArgs,
	RunE: runDevLogout,
}

var devRefreshCmd = &cobra.Command{
	Use:   "refresh",
	Short: "Rotate the developer JWT using the stored refresh token",
	Long: `Use the stored 30-day refresh token to mint a new 60-minute developer
JWT. Both tokens rotate atomically — the old refresh token is invalidated
server-side after a successful refresh, and the new pair is persisted to
config.

The refresh-token rotation is single-use server-side. If the rotation fails
on the gateway side AND the compensating revoke also fails, the gateway logs
a critical alert; the CLI sees a 5xx and a re-run will mint cleanly.

Examples:
  andamio dev refresh
  andamio dev refresh --output json`,
	Args: cobra.NoArgs,
	RunE: runDevRefresh,
}

var devStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show developer authentication status",
	Long: `Show whether a developer JWT is stored and (if a known expiry was
returned at login) when it expires. Reports independently of 'user status' —
the two slots are distinct.`,
	Args: cobra.NoArgs,
	RunE: runDevStatus,
}

func init() {
	rootCmd.AddCommand(devCmd)
	devCmd.AddCommand(devLoginCmd)
	devCmd.AddCommand(devLogoutCmd)
	devCmd.AddCommand(devRefreshCmd)
	devCmd.AddCommand(devStatusCmd)

	devLoginCmd.Flags().String("skey", "", "Path to .skey file (required for headless mode)")
	devLoginCmd.Flags().String("alias", "", "Developer access-token alias (required for headless mode)")
	devLoginCmd.Flags().String("address", "", "Bech32 wallet address bound to the access-token alias (required for headless mode)")
}

// runDevLogin dispatches between the browser-wallet flow (no flags) and the
// headless .skey flow (all three of --skey/--alias/--address). The
// discriminator is `cmd.Flags().Changed(...)`, NOT empty-string equality:
// `--skey ""` from an unset shell variable sets the value to empty but
// Changed() still returns true, which correctly routes to headless mode
// (where LoadSigningKey then fails with the existing "missing skey" error
// rather than mistakenly triggering the browser flow). See plan
// docs/plans/2026-05-22-001-feat-browser-based-dev-login-plan.md.
func runDevLogin(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	skeyProvided := cmd.Flags().Changed("skey")
	aliasProvided := cmd.Flags().Changed("alias")
	addrProvided := cmd.Flags().Changed("address")

	switch {
	case !skeyProvided && !aliasProvided && !addrProvided:
		// Unit 2 replaces this stub with runDevLoginBrowser.
		return fmt.Errorf("browser-flow dev login not yet implemented in this commit — use --skey/--alias/--address for now")
	case skeyProvided && aliasProvided && addrProvided:
		skeyPath, _ := cmd.Flags().GetString("skey")
		alias, _ := cmd.Flags().GetString("alias")
		address, _ := cmd.Flags().GetString("address")

		privKey, pubKey, err := cardano.LoadSigningKey(skeyPath)
		if err != nil {
			return fmt.Errorf("failed to load signing key: %w", err)
		}
		return runDevHeadlessLogin(cmd.Context(), cfg, privKey, pubKey, skeyPath, alias, address)
	default:
		// Partial-flag invocation. Name both modes so the operator can fix
		// forward in either direction without re-reading the help text.
		missing := []string{}
		if !skeyProvided {
			missing = append(missing, "--skey")
		}
		if !aliasProvided {
			missing = append(missing, "--alias")
		}
		if !addrProvided {
			missing = append(missing, "--address")
		}
		return fmt.Errorf("dev login requires either no flags (browser mode) or all three of --skey/--alias/--address (headless mode); missing: %v", missing)
	}
}

// devSessionResult is the typed `--output json` envelope shape for both
// `dev login` and `dev refresh`. Single struct so the two commands cannot
// drift on field naming (e.g., `refresh_expires_at` vs `refresh_token_expires_at`),
// and so a future copy-edit that drops or renames a key is a compile error
// rather than a silent contract break. Field names match `devStatusResult`
// so a script can read `refresh_token_expires_at` from any of the three
// commands and use the same path.
//
// CRITICAL: this struct must NEVER carry the JWT or refresh-token bodies.
// Tokens belong on disk (~/.andamio/config.json at 0600), not on stdout.
// `TestRunDevHeadlessLogin_JSONOutputShape` and
// `TestRunDevRefreshFlow_JSONOutputShape` enforce this — both decode
// captured stdout and assert the literal token bodies are absent.
type devSessionResult struct {
	Alias                 string `json:"alias"`
	DevID                 string `json:"dev_id"`
	Tier                  string `json:"tier,omitempty"`
	KeyHash               string `json:"key_hash,omitempty"`
	JWTExpiresAt          string `json:"jwt_expires_at"`
	RefreshTokenExpiresAt string `json:"refresh_token_expires_at"`
}

// secureLoginResponse mirrors andamio-api's `auth_viewmodels.SecureLoginResponse`
// shape — the body returned by both `/login/complete` and `/token/refresh`.
// JWT and refresh-token expiries are nested inside the respective objects
// rather than top-level so we can keep the two clocks straight in cfg + status.
type secureLoginResponse struct {
	UserID string `json:"user_id"`
	Alias  string `json:"alias"`
	Tier   string `json:"tier"`
	JWT    struct {
		Token     string `json:"token"`
		ExpiresAt string `json:"expires_at"`
	} `json:"jwt"`
	RefreshToken struct {
		Token     string `json:"token"`
		ExpiresAt string `json:"expires_at"`
	} `json:"refresh_token"`
}

// runDevHeadlessLogin is the testable core of `andamio dev login`. Split from
// runDevLogin so unit tests can inject an ephemeral ed25519 keypair without
// staging a real .skey file. skeyPath is taken purely for the human-readable
// stderr signing message and otherwise has no effect on the flow.
//
// Wire shape sourced from andamio-api #410 (`auth_viewmodels.LoginSessionRequest`,
// `LoginCompleteRequest`, `SecureLoginResponse`).
func runDevHeadlessLogin(ctx context.Context, cfg *config.Config, privKey ed25519.PrivateKey, pubKey ed25519.PublicKey, skeyPath, alias, address string) error {
	isJSON := output.GetFormat() == output.FormatJSON

	c := client.New(cfg)

	// Step 1: Open login session keyed to (alias, wallet_address). The gateway
	// looks up the developer account, persists a 5-min nonce against the
	// (user_id, wallet_address) pair, and returns the nonce for signing. The
	// alias+address bind here, not at /complete — the gateway rejects a
	// /complete that uses a session created against a different binding.
	if !isJSON {
		fmt.Fprintf(os.Stderr, "Requesting developer login session...\n")
	}
	sessionReq := map[string]string{
		"alias":          alias,
		"wallet_address": address,
	}
	var session struct {
		SessionID string `json:"session_id"`
		Nonce     string `json:"nonce"`
		ExpiresAt string `json:"expires_at"`
	}
	if err := c.Post(ctx, devLoginSessionPath, sessionReq, &session); err != nil {
		return fmt.Errorf("failed to open developer login session: %w", err)
	}
	if session.Nonce == "" || session.SessionID == "" {
		return fmt.Errorf("invalid login session response: missing nonce or session_id")
	}

	// Step 2: Sign the nonce with the wallet's signing key (CIP-8). The
	// gateway's complete handler verifies the signature against the
	// wallet_address recorded at session creation, so the signing key must
	// match that address.
	if !isJSON {
		fmt.Fprintf(os.Stderr, "Signing nonce with %s...\n", skeyPath)
	}
	signResult, err := cardano.SignMessage([]byte(session.Nonce), privKey, pubKey)
	if err != nil {
		return fmt.Errorf("failed to sign nonce: %w", err)
	}

	// Step 2b: Detect a session that expired during signing (slow hardware
	// wallet, OS sleep, debugger pause). Without this guard, the gateway's
	// /complete returns 401 and the CLI surfaces the misleading "wallet
	// address does not match the .skey" hint, accusing the user's flags
	// when the actual cause is a clock issue. Skip when ExpiresAt is empty
	// or unparseable — let the gateway have the final word in those cases.
	if session.ExpiresAt != "" {
		if expiresAt, err := time.Parse(time.RFC3339, session.ExpiresAt); err == nil && time.Now().After(expiresAt) {
			return fmt.Errorf("developer login session expired during signing (sessions are valid for 5 minutes; signing took longer). Re-run 'andamio dev login --skey <path> --alias <name> --address <bech32>' to start fresh")
		}
	}

	// Step 3: Submit signature. Body carries only session_id + signature —
	// alias and address are already bound to the session server-side.
	if !isJSON {
		fmt.Fprintf(os.Stderr, "Submitting signature...\n")
	}
	completeReq := map[string]interface{}{
		"session_id": session.SessionID,
		"signature": map[string]string{
			"key":       signResult.Key,
			"signature": signResult.Signature,
		},
	}
	var tokenResp secureLoginResponse
	if err := c.Post(ctx, devLoginCompletePath, completeReq, &tokenResp); err != nil {
		// 401 at /complete almost always means the wallet that signed the
		// nonce does not match the address recorded at session creation —
		// i.e., the .skey and --address flags belong to different wallets.
		// Surface that hypothesis up front so users don't waste a retry.
		// Underlying typed error stays reachable via errors.As.
		var authErr *apierr.AuthError
		if errors.As(err, &authErr) && authErr.HTTPStatus == 401 {
			return fmt.Errorf("developer authentication failed (likely the wallet address does not match the .skey signing key — re-check --address and --skey): %w", err)
		}
		return fmt.Errorf("developer authentication failed: %w", err)
	}
	if tokenResp.JWT.Token == "" {
		return fmt.Errorf("developer authentication failed: no JWT received")
	}
	if tokenResp.RefreshToken.Token == "" {
		// Refresh token is the durable credential; refusing to persist a
		// session without one prevents a confusing future `dev refresh` that
		// blames the user for the gateway's omission.
		return fmt.Errorf("developer authentication failed: no refresh token received")
	}

	// Step 4: Persist all four moving parts of the dev session — JWT (60-min),
	// refresh token (30-day, single-use), tier (surfaced in `dev status`), and
	// the canonical alias/user_id from the gateway response.
	//
	// ClearDevAuth before persistDevSession makes login a clean overwrite: a
	// re-login switching accounts cannot inherit the prior session's DevAlias
	// or DevKeyHash if a future gateway response shape omits a field that the
	// existing slot has populated. Refresh's call to persistDevSession is a
	// deliberate merge (preserves DevKeyHash, no re-sign happened) and stays.
	cfg.ClearDevAuth()
	persistDevSession(cfg, &tokenResp, signResult.KeyHash, alias)
	if err := config.Save(cfg); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	if isJSON {
		return output.PrintJSON(devSessionResult{
			Alias:                 cfg.DevAlias,
			DevID:                 cfg.DevID,
			Tier:                  cfg.DevTier,
			KeyHash:               signResult.KeyHash,
			JWTExpiresAt:          cfg.DevJWTExpiresAt,
			RefreshTokenExpiresAt: cfg.DevRefreshTokenExpiresAt,
		})
	}
	fmt.Fprintf(os.Stderr, "\nAuthenticated as developer: %s", cfg.DevAlias)
	if cfg.DevTier != "" {
		fmt.Fprintf(os.Stderr, " (tier: %s)", cfg.DevTier)
	}
	fmt.Fprintln(os.Stderr)
	if cfg.DevID != "" {
		fmt.Fprintf(os.Stderr, "Developer ID: %s\n", cfg.DevID)
	}
	fmt.Fprintln(os.Stderr, "\nDeveloper JWT (60 min) + refresh token (30 days) stored.")
	fmt.Fprintln(os.Stderr, "Run 'andamio dev refresh' before the JWT expires to rotate without re-signing.")
	return nil
}

// persistDevSession copies a SecureLoginResponse + the locally-derived key
// hash into Config. Pulled out so login and refresh share one persistence
// rule and `dev refresh` cannot drift from `dev login` on which fields it
// updates. fallbackAlias covers the (currently unobserved) case where the
// gateway returns an empty alias — refresh has no alias to fall back to, so
// callers from refresh pass the existing cfg.DevAlias.
func persistDevSession(cfg *config.Config, resp *secureLoginResponse, keyHash, fallbackAlias string) {
	cfg.DevJWT = resp.JWT.Token
	cfg.DevJWTExpiresAt = resp.JWT.ExpiresAt
	cfg.DevRefreshToken = resp.RefreshToken.Token
	cfg.DevRefreshTokenExpiresAt = resp.RefreshToken.ExpiresAt
	if resp.Alias != "" {
		cfg.DevAlias = resp.Alias
	} else if fallbackAlias != "" {
		cfg.DevAlias = fallbackAlias
	}
	cfg.DevID = resp.UserID
	cfg.DevTier = resp.Tier
	if keyHash != "" {
		cfg.DevKeyHash = keyHash
	}
}

// devLogoutResult is the typed `dev logout --output json` envelope. The
// `cleared` flag distinguishes "nothing was stored, nothing to do" (false)
// from "credentials were present and have been wiped" (true) — agents
// scripting cleanup branch on this without parsing stderr.
type devLogoutResult struct {
	Cleared bool `json:"cleared"`
}

func runDevLogout(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	// Gate on ANY persisted dev credential, not just the JWT. If the user
	// has only a refresh token (env-override path, manual config edit, or
	// any future code path that outlives the 60-min JWT but persists the
	// 30-day rotation credential), HasDevAuth() returns false but the
	// durable credential is still on disk. Clearing only when the JWT is
	// present would silently strand the refresh token — the more valuable
	// of the two — directly contradicting the command's promise.
	hadCredentials := cfg.HasDevAuth() || cfg.DevRefreshToken != ""
	isJSON := output.GetFormat() == output.FormatJSON
	if !hadCredentials {
		if isJSON {
			return output.PrintJSON(devLogoutResult{Cleared: false})
		}
		fmt.Fprintln(os.Stderr, "No developer credentials stored.")
		return nil
	}
	cfg.ClearDevAuth()
	if err := config.Save(cfg); err != nil {
		return err
	}
	if isJSON {
		return output.PrintJSON(devLogoutResult{Cleared: true})
	}
	fmt.Fprintln(os.Stderr, "Developer JWT and refresh token cleared.")
	return nil
}

func runDevRefresh(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}
	if cfg.DevRefreshToken == "" {
		return fmt.Errorf("no refresh token stored. Run 'andamio dev login --skey <path> --alias <name> --address <bech32>' first")
	}
	return runDevRefreshFlow(cmd.Context(), cfg)
}

// runDevRefreshFlow is the testable core of `andamio dev refresh`. Split from
// runDevRefresh so unit tests can stub the gateway response without touching
// real config.
func runDevRefreshFlow(ctx context.Context, cfg *config.Config) error {
	isJSON := output.GetFormat() == output.FormatJSON

	c := client.New(cfg)

	if !isJSON {
		fmt.Fprintln(os.Stderr, "Rotating developer JWT...")
	}

	refreshReq := map[string]string{"refresh_token": cfg.DevRefreshToken}
	var tokenResp secureLoginResponse
	if err := c.Post(ctx, devTokenRefreshPath, refreshReq, &tokenResp); err != nil {
		// 401 specifically means the refresh token is dead server-side —
		// expired, revoked, or already rotated by another process. The
		// stored token is now misleading: `dev status` would still show it
		// as valid based on the persisted expiry. Clear the dev slot so
		// state matches reality and the next `dev status` reports the
		// truth. Other AuthError statuses (403) are NOT cleared — those
		// could be transient policy decisions on the gateway side.
		var authErr *apierr.AuthError
		if errors.As(err, &authErr) && authErr.HTTPStatus == 401 {
			cfg.ClearDevAuth()
			if saveErr := config.Save(cfg); saveErr != nil {
				// Both the refresh and the cleanup failed. Surface both so
				// the user can choose to manually clear ~/.andamio/config.json.
				return fmt.Errorf("refresh token rejected (%w); cleanup of stale config also failed (%v) — manually run 'andamio dev logout' or remove ~/.andamio/config.json, then 'andamio dev login --skey <path> --alias <name> --address <bech32>' to re-authenticate", err, saveErr)
			}
			return fmt.Errorf("refresh token rejected (%w); stored dev credentials cleared. Run 'andamio dev login --skey <path> --alias <name> --address <bech32>' to re-authenticate", err)
		}
		return fmt.Errorf("token refresh failed: %w", err)
	}
	if tokenResp.JWT.Token == "" {
		return fmt.Errorf("token refresh failed: no JWT received")
	}
	if tokenResp.RefreshToken.Token == "" {
		return fmt.Errorf("token refresh failed: no refresh token in rotation response")
	}

	// Refresh keeps the existing key hash (we did not re-sign) and falls back
	// to the existing alias if the gateway does not echo it.
	persistDevSession(cfg, &tokenResp, cfg.DevKeyHash, cfg.DevAlias)
	if err := config.Save(cfg); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	if isJSON {
		// Refresh did not re-sign, so KeyHash is intentionally absent from
		// the envelope (the existing key hash on disk is unchanged; agents
		// that need it should read `dev status --output json`).
		return output.PrintJSON(devSessionResult{
			Alias:                 cfg.DevAlias,
			DevID:                 cfg.DevID,
			Tier:                  cfg.DevTier,
			JWTExpiresAt:          cfg.DevJWTExpiresAt,
			RefreshTokenExpiresAt: cfg.DevRefreshTokenExpiresAt,
		})
	}
	fmt.Fprintf(os.Stderr, "Developer JWT rotated (alias: %s).\n", cfg.DevAlias)
	if cfg.DevJWTExpiresAt != "" {
		fmt.Fprintf(os.Stderr, "New JWT expires at: %s\n", cfg.DevJWTExpiresAt)
	}
	if cfg.DevRefreshTokenExpiresAt != "" {
		fmt.Fprintf(os.Stderr, "New refresh token expires at: %s\n", cfg.DevRefreshTokenExpiresAt)
	}
	return nil
}

// devStatusResult is the JSON envelope shape of `andamio dev status --output json`.
// Field set mirrors userStatusResult (api_key_set, base_url) plus the dev-JWT
// fields. Distinct envelope from user status so callers branch on the slot
// they care about without coupling.
//
// Two clocks: the JWT expires in ~60 minutes, the refresh token in 30 days.
// `*Expired` and `*RemainingSeconds` mirror the userStatusResult convention so
// scripts can branch deterministically on JSON without parsing timestamps.
type devStatusResult struct {
	APIKeySet             bool   `json:"api_key_set"`
	BaseURL               string `json:"base_url"`
	DevAuthenticated      bool   `json:"dev_authenticated"`
	DevAlias              string `json:"dev_alias,omitempty"`
	DevID                 string `json:"dev_id,omitempty"`
	DevTier               string `json:"dev_tier,omitempty"`
	DevKeyHash            string `json:"dev_key_hash,omitempty"`
	JWTExpiresAt          string `json:"jwt_expires_at,omitempty"`
	JWTExpired            *bool  `json:"jwt_expired,omitempty"`
	// JWTRemainingSeconds intentionally has NO omitempty: zero is a valid
	// signal (sub-second remaining — agents need to refresh immediately).
	// Suppressing zero would conflate "almost expired" with "no signal".
	// Branch on JWTExpired (*bool present iff parseable) to disambiguate
	// "not parseable" from "fully expired". Same for RefreshTokenRemainingSeconds.
	JWTRemainingSeconds          int64  `json:"jwt_remaining_seconds"`
	RefreshTokenStored           bool   `json:"refresh_token_stored"`
	RefreshTokenExpiresAt        string `json:"refresh_token_expires_at,omitempty"`
	RefreshTokenExpired          *bool  `json:"refresh_token_expired,omitempty"`
	RefreshTokenRemainingSeconds int64  `json:"refresh_token_remaining_seconds"`
}

func runDevStatus(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	if output.GetFormat() == output.FormatJSON {
		result := devStatusResult{
			APIKeySet:          cfg.APIKey != "",
			BaseURL:            cfg.BaseURL,
			DevAuthenticated:   cfg.HasDevAuth(),
			RefreshTokenStored: cfg.DevRefreshToken != "",
		}
		if cfg.HasDevAuth() {
			result.DevAlias = cfg.DevAlias
			result.DevID = cfg.DevID
			result.DevTier = cfg.DevTier
			result.DevKeyHash = cfg.DevKeyHash
			result.JWTExpiresAt = cfg.DevJWTExpiresAt
			if expired, remaining, ok := timeUntil(cfg.DevJWTExpiresAt); ok {
				result.JWTExpired = &expired
				if !expired {
					result.JWTRemainingSeconds = int64(remaining.Seconds())
				}
			}
		}
		if cfg.DevRefreshToken != "" {
			result.RefreshTokenExpiresAt = cfg.DevRefreshTokenExpiresAt
			if expired, remaining, ok := timeUntil(cfg.DevRefreshTokenExpiresAt); ok {
				result.RefreshTokenExpired = &expired
				if !expired {
					result.RefreshTokenRemainingSeconds = int64(remaining.Seconds())
				}
			}
		}
		return output.PrintJSON(result)
	}

	fmt.Println("Developer Authentication Status")
	fmt.Println("-------------------------------")

	if cfg.APIKey != "" {
		fmt.Println("API Key: ****... (configured)")
	} else {
		fmt.Println("API Key: not configured")
	}
	fmt.Printf("Base URL: %s\n", cfg.BaseURL)
	fmt.Println()

	if !cfg.HasDevAuth() {
		fmt.Println("Developer: not authenticated")
		fmt.Println("\nRun 'andamio dev login --skey <path> --alias <name> --address <bech32>' to authenticate.")
		return nil
	}

	fmt.Printf("Developer: %s", cfg.DevAlias)
	if cfg.DevTier != "" {
		fmt.Printf(" (tier: %s)", cfg.DevTier)
	}
	fmt.Println()
	if cfg.DevID != "" {
		fmt.Printf("Developer ID: %s\n", cfg.DevID)
	}
	printExpiryLine("JWT", cfg.DevJWTExpiresAt, "Run 'andamio dev refresh' to rotate without re-signing.")
	if cfg.DevRefreshToken != "" {
		printExpiryLine("Refresh token", cfg.DevRefreshTokenExpiresAt, "Run 'andamio dev login ...' to re-authenticate.")
	} else {
		fmt.Println("Refresh token: not stored")
	}
	if cfg.DevKeyHash != "" {
		fmt.Printf("Key hash: %s\n", cfg.DevKeyHash)
	}
	return nil
}

// parseExpiry is the shared RFC3339 parse + expiry-comparison primitive
// behind timeUntil and printExpiryLine. Returns the parsed timestamp (zero
// when ok=false) plus a (remaining, expired, ok) tuple. ok=false means the
// input was empty or unparseable — callers should treat the value as absent
// rather than expired/valid.
func parseExpiry(rfc3339 string) (expiresAt time.Time, remaining time.Duration, expired, ok bool) {
	if rfc3339 == "" {
		return time.Time{}, 0, false, false
	}
	expiresAt, err := time.Parse(time.RFC3339, rfc3339)
	if err != nil {
		return time.Time{}, 0, false, false
	}
	now := time.Now()
	if now.After(expiresAt) {
		return expiresAt, 0, true, true
	}
	return expiresAt, expiresAt.Sub(now), false, true
}

// timeUntil returns the (expired, remaining, ok) projection of parseExpiry —
// the JSON envelope path doesn't need the parsed timestamp for display.
func timeUntil(rfc3339 string) (expired bool, remaining time.Duration, ok bool) {
	_, remaining, expired, ok = parseExpiry(rfc3339)
	return
}

// printExpiryLine prints "<label>: valid until <time> (<remaining>)" or
// "<label>: EXPIRED (at <time>)" + a follow-up hint, falling back to the raw
// timestamp when parsing fails. Shared between JWT and refresh-token rendering
// so both clocks present identically in `dev status`.
func printExpiryLine(label, rfc3339, expiredHint string) {
	if rfc3339 == "" {
		fmt.Printf("%s: active (no expiry info)\n", label)
		return
	}
	expiresAt, remaining, expired, ok := parseExpiry(rfc3339)
	if !ok {
		fmt.Printf("%s expires: %s\n", label, rfc3339)
		return
	}
	if expired {
		fmt.Printf("%s: EXPIRED (at %s)\n", label, expiresAt.Local().Format(time.RFC1123))
		if expiredHint != "" {
			fmt.Printf("  → %s\n", expiredHint)
		}
		return
	}
	fmt.Printf("%s: valid until %s (%s remaining)\n",
		label,
		expiresAt.Local().Format(time.RFC1123),
		formatDuration(remaining))
}
