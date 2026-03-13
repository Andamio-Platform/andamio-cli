package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Andamio-Platform/andamio-cli/internal/config"
	"github.com/pkg/browser"
	"github.com/spf13/cobra"
)

var userCmd = &cobra.Command{
	Use:   "user",
	Short: "User information and authentication",
}

var userMeCmd = &cobra.Command{
	Use:   "me",
	Short: "Get current user info",
	RunE: func(cmd *cobra.Command, args []string) error {
		return getJSON("/api/v1/user/me")
	},
}

var userUsageCmd = &cobra.Command{
	Use:   "usage",
	Short: "Get user usage stats",
	RunE: func(cmd *cobra.Command, args []string) error {
		return getJSON("/api/v1/user/usage")
	},
}

var userExistsCmd = &cobra.Command{
	Use:   "exists <alias>",
	Short: "Check if user exists by alias",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return getJSON("/api/v2/user/exists/" + args[0])
	},
}

var userLoginCmd = &cobra.Command{
	Use:   "login",
	Short: "Authenticate via browser wallet signing",
	Long: `Opens your browser to sign in with your Cardano wallet.

This authenticates you as an Access Token holder, enabling you to:
- Edit courses you own
- Manage course modules and content
- Edit projects you manage

The CLI starts a local server, opens your browser for wallet signing,
then stores the resulting JWT for future API calls.`,
	RunE: runUserLogin,
}

var userLogoutCmd = &cobra.Command{
	Use:   "logout",
	Short: "Clear stored user authentication",
	RunE:  runUserLogout,
}

var userStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show authentication status",
	RunE:  runUserStatus,
}

func init() {
	rootCmd.AddCommand(userCmd)
	userCmd.AddCommand(userMeCmd)
	userCmd.AddCommand(userUsageCmd)
	userCmd.AddCommand(userExistsCmd)
	userCmd.AddCommand(userLoginCmd)
	userCmd.AddCommand(userLogoutCmd)
	userCmd.AddCommand(userStatusCmd)
}

// generateState creates a cryptographically secure random state parameter
func generateState() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(bytes), nil
}

// authCallbackResult holds the result from the browser callback
type authCallbackResult struct {
	JWT       string
	ExpiresAt string
	Alias     string
	UserID    string
	Error     string
}

func runUserLogin(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Check if already authenticated
	if cfg.HasUserAuth() {
		fmt.Printf("Already authenticated as: %s\n", cfg.UserAlias)
		fmt.Println("Run 'andamio user logout' first to re-authenticate.")
		return nil
	}

	// Generate state for CSRF protection
	state, err := generateState()
	if err != nil {
		return fmt.Errorf("failed to generate state: %w", err)
	}

	// Start local HTTP server on ephemeral port
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return fmt.Errorf("failed to start local server: %w", err)
	}
	defer listener.Close()

	port := listener.Addr().(*net.TCPAddr).Port
	redirectURI := fmt.Sprintf("http://127.0.0.1:%d/callback", port)

	// Channel to receive the callback result
	resultChan := make(chan authCallbackResult, 1)

	// Set up callback handler
	mux := http.NewServeMux()
	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		result := authCallbackResult{}

		// Check for error
		if errParam := r.URL.Query().Get("error"); errParam != "" {
			result.Error = errParam
			resultChan <- result
			w.Header().Set("Content-Type", "text/html")
			fmt.Fprintf(w, authFailureHTML(errParam))
			return
		}

		// Validate state
		returnedState := r.URL.Query().Get("state")
		if returnedState != state {
			result.Error = "invalid state parameter"
			resultChan <- result
			w.Header().Set("Content-Type", "text/html")
			fmt.Fprintf(w, authFailureHTML("Security validation failed"))
			return
		}

		// Extract JWT and user info
		result.JWT = r.URL.Query().Get("jwt")
		result.ExpiresAt = r.URL.Query().Get("expires_at")
		result.Alias = r.URL.Query().Get("alias")
		result.UserID = r.URL.Query().Get("user_id")

		if result.JWT == "" {
			result.Error = "no JWT received"
			resultChan <- result
			w.Header().Set("Content-Type", "text/html")
			fmt.Fprintf(w, authFailureHTML("Authentication failed - no token received"))
			return
		}

		resultChan <- result
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprintf(w, authSuccessHTML(result.Alias))
	})

	server := &http.Server{Handler: mux}

	// Start server in background
	go func() {
		server.Serve(listener)
	}()

	// Build auth URL - the app's CLI auth page
	authURL := buildAuthURL(cfg.BaseURL, redirectURI, state)

	fmt.Println("Opening browser for authentication...")
	fmt.Printf("If browser doesn't open, visit: %s\n\n", authURL)

	// Open browser
	if err := browser.OpenURL(authURL); err != nil {
		fmt.Printf("Failed to open browser automatically: %v\n", err)
		fmt.Printf("Please open this URL manually: %s\n", authURL)
	}

	fmt.Println("Waiting for authentication...")

	// Wait for callback with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	var result authCallbackResult
	select {
	case result = <-resultChan:
		// Got result, continue
	case <-ctx.Done():
		server.Shutdown(context.Background())
		return fmt.Errorf("authentication timed out after 5 minutes")
	}

	// Shutdown server
	server.Shutdown(context.Background())

	if result.Error != "" {
		return fmt.Errorf("authentication failed: %s", result.Error)
	}

	// Save JWT to config
	cfg.UserJWT = result.JWT
	cfg.JWTExpiresAt = result.ExpiresAt
	cfg.UserAlias = result.Alias
	cfg.UserID = result.UserID

	if err := config.Save(cfg); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	fmt.Println()
	fmt.Printf("Successfully authenticated as: %s\n", result.Alias)
	if result.ExpiresAt != "" {
		fmt.Printf("Session expires: %s\n", result.ExpiresAt)
	}
	fmt.Println("\nYou can now use owner commands to edit courses and projects.")

	return nil
}

func runUserLogout(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	if !cfg.HasUserAuth() {
		fmt.Println("Not currently authenticated as a user.")
		return nil
	}

	alias := cfg.UserAlias
	cfg.ClearUserAuth()

	if err := config.Save(cfg); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	fmt.Printf("Logged out successfully (was: %s)\n", alias)
	return nil
}

func runUserStatus(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	fmt.Println("Authentication Status")
	fmt.Println("---------------------")

	// API Key status
	if cfg.APIKey != "" {
		fmt.Printf("API Key: %s... (configured)\n", cfg.APIKey[:min(8, len(cfg.APIKey))])
	} else {
		fmt.Println("API Key: not configured")
	}

	fmt.Printf("Base URL: %s\n", cfg.BaseURL)
	fmt.Println()

	// User JWT status
	if cfg.HasUserAuth() {
		fmt.Printf("User: %s\n", cfg.UserAlias)
		fmt.Printf("User ID: %s\n", cfg.UserID)

		if cfg.JWTExpiresAt != "" {
			expiresAt, err := time.Parse(time.RFC3339, cfg.JWTExpiresAt)
			if err == nil {
				now := time.Now()
				if now.After(expiresAt) {
					fmt.Printf("Session: EXPIRED (at %s)\n", expiresAt.Local().Format(time.RFC1123))
					fmt.Println("\nRun 'andamio user logout && andamio user login' to re-authenticate.")
				} else {
					remaining := expiresAt.Sub(now)
					fmt.Printf("Session: valid until %s (%s remaining)\n",
						expiresAt.Local().Format(time.RFC1123),
						formatDuration(remaining))
				}
			} else {
				fmt.Printf("Session expires: %s\n", cfg.JWTExpiresAt)
			}
		} else {
			fmt.Println("Session: active (no expiry info)")
		}
	} else {
		fmt.Println("User: not authenticated")
		fmt.Println("\nRun 'andamio user login' to authenticate with your wallet.")
	}

	return nil
}

// buildAuthURL constructs the authentication URL for the app's CLI auth page
func buildAuthURL(baseURL, redirectURI, state string) string {
	// Convert API base URL to app URL
	// e.g., https://preprod.api.andamio.io -> https://preprod.app.andamio.io
	appURL := strings.Replace(baseURL, ".api.", ".app.", 1)

	// Build the auth URL with query params
	params := url.Values{}
	params.Set("redirect_uri", redirectURI)
	params.Set("state", state)

	return fmt.Sprintf("%s/auth/cli?%s", appURL, params.Encode())
}

// formatDuration formats a duration in a human-readable way
func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%d seconds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%d minutes", int(d.Minutes()))
	}
	hours := int(d.Hours())
	if hours < 24 {
		return fmt.Sprintf("%d hours", hours)
	}
	days := hours / 24
	return fmt.Sprintf("%d days", days)
}

// authSuccessHTML returns HTML for successful authentication
func authSuccessHTML(alias string) string {
	return fmt.Sprintf(`<!DOCTYPE html>
<html>
<head>
    <title>Andamio CLI - Authenticated</title>
    <style>
        body { font-family: system-ui, sans-serif; max-width: 600px; margin: 100px auto; text-align: center; }
        .success { color: #10b981; font-size: 48px; }
        h1 { color: #1f2937; }
        p { color: #6b7280; }
    </style>
</head>
<body>
    <div class="success">&#10004;</div>
    <h1>Authentication Successful</h1>
    <p>Welcome, <strong>%s</strong>!</p>
    <p>You can close this window and return to the terminal.</p>
</body>
</html>`, alias)
}

// authFailureHTML returns HTML for failed authentication
func authFailureHTML(errMsg string) string {
	return fmt.Sprintf(`<!DOCTYPE html>
<html>
<head>
    <title>Andamio CLI - Authentication Failed</title>
    <style>
        body { font-family: system-ui, sans-serif; max-width: 600px; margin: 100px auto; text-align: center; }
        .error { color: #ef4444; font-size: 48px; }
        h1 { color: #1f2937; }
        p { color: #6b7280; }
    </style>
</head>
<body>
    <div class="error">&#10008;</div>
    <h1>Authentication Failed</h1>
    <p>%s</p>
    <p>Please close this window and try again.</p>
</body>
</html>`, errMsg)
}

// parseJWTExpiry extracts the expiry time from a JWT (without verifying signature)
func parseJWTExpiry(jwt string) (time.Time, error) {
	parts := strings.Split(jwt, ".")
	if len(parts) != 3 {
		return time.Time{}, fmt.Errorf("invalid JWT format")
	}

	// Decode payload (second part)
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return time.Time{}, fmt.Errorf("failed to decode JWT payload: %w", err)
	}

	var claims struct {
		Exp int64 `json:"exp"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return time.Time{}, fmt.Errorf("failed to parse JWT claims: %w", err)
	}

	if claims.Exp == 0 {
		return time.Time{}, fmt.Errorf("no expiry in JWT")
	}

	return time.Unix(claims.Exp, 0), nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
