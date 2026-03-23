package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"html"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"os"

	"github.com/Andamio-Platform/andamio-cli/internal/cardano"
	"github.com/Andamio-Platform/andamio-cli/internal/client"
	"github.com/Andamio-Platform/andamio-cli/internal/config"
	"github.com/Andamio-Platform/andamio-cli/internal/output"
	"github.com/pkg/browser"
	"github.com/spf13/cobra"
)

var userCmd = &cobra.Command{
	Use:   "user",
	Short: "User information and authentication",
}

var userMeCmd = &cobra.Command{
	Use:   "me",
	Short: "Get current user dashboard",
	RunE:  runUserMe,
}

var userExistsCmd = &cobra.Command{
	Use:   "exists <alias>",
	Short: "Check if user exists by alias",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return getJSON("/api/v2/user/exists/" + url.PathEscape(args[0]))
	},
}

var userLoginCmd = &cobra.Command{
	Use:   "login",
	Short: "Authenticate via browser wallet signing or .skey file",
	Long: `Authenticate to use owner and manager commands.

Browser login (default):
  andamio user login
  Opens your browser for wallet signing.

Headless login (for CI/CD, scripting, agents):
  andamio user login --skey ./payment.skey --alias myalias
  Signs a nonce with your .skey file — no browser needed.`,
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
	userCmd.AddCommand(userExistsCmd)
	userCmd.AddCommand(userLoginCmd)
	userCmd.AddCommand(userLogoutCmd)
	userCmd.AddCommand(userStatusCmd)

	// Headless login flags
	userLoginCmd.Flags().String("skey", "", "Path to .skey file for headless authentication (no browser)")
	userLoginCmd.Flags().String("alias", "", "Andamio alias (required with --skey)")
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

	// Headless login via --skey
	skeyPath, _ := cmd.Flags().GetString("skey")
	if skeyPath != "" {
		alias, _ := cmd.Flags().GetString("alias")
		if alias == "" {
			return fmt.Errorf("--alias is required with --skey\n\nCheck aliases with: andamio user exists <alias>")
		}
		return runHeadlessLogin(cfg, skeyPath, alias)
	}

	// Check if already authenticated (browser flow only)
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
		// Only accept GET requests
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		result := authCallbackResult{}

		// Check for error
		if errParam := r.URL.Query().Get("error"); errParam != "" {
			result.Error = errParam
			resultChan <- result
			w.Header().Set("Content-Type", "text/html")
			fmt.Fprint(w, authFailureHTML(errParam))
			return
		}

		// Validate state
		returnedState := r.URL.Query().Get("state")
		if returnedState != state {
			result.Error = "invalid state parameter"
			resultChan <- result
			w.Header().Set("Content-Type", "text/html")
			fmt.Fprint(w, authFailureHTML("Security validation failed"))
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
			fmt.Fprint(w, authFailureHTML("Authentication failed - no token received"))
			return
		}

		resultChan <- result
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, authSuccessHTML(result.Alias))
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

// runHeadlessLogin authenticates using a .skey file via CIP-8 message signing.
// Flow: get nonce → sign with .skey → validate signature → store JWT.
func runHeadlessLogin(cfg *config.Config, skeyPath, alias string) error {
	isJSON := output.GetFormat() == output.FormatJSON

	// Load signing key
	privKey, pubKey, err := cardano.LoadSigningKey(skeyPath)
	if err != nil {
		return fmt.Errorf("failed to load signing key: %w", err)
	}

	c := client.New(cfg)

	// Step 1: Get login session with nonce
	if !isJSON {
		fmt.Fprintf(os.Stderr, "Requesting login session...\n")
	}

	var session struct {
		ID        string `json:"id"`
		Nonce     string `json:"nonce"`
		ExpiresAt string `json:"expires_at"`
	}
	if err := c.Post("/api/v2/auth/login/session", nil, &session); err != nil {
		return fmt.Errorf("failed to get login session: %w", err)
	}

	if session.Nonce == "" || session.ID == "" {
		return fmt.Errorf("invalid login session response: missing nonce or session ID")
	}

	// Step 2: Sign the nonce using CIP-8
	if !isJSON {
		fmt.Fprintf(os.Stderr, "Signing nonce with %s...\n", skeyPath)
	}

	signResult, err := cardano.SignMessage([]byte(session.Nonce), privKey, pubKey)
	if err != nil {
		return fmt.Errorf("failed to sign nonce: %w", err)
	}

	// Step 3: Validate signature with API
	if !isJSON {
		fmt.Fprintf(os.Stderr, "Validating signature...\n")
	}

	validatePayload := map[string]interface{}{
		"id": session.ID,
		"signature": map[string]string{
			"signature": signResult.Signature,
			"key":       signResult.Key,
		},
	}

	var tokenResp struct {
		JWT  string `json:"jwt"`
		User struct {
			ID                string  `json:"id"`
			CardanoBech32Addr *string `json:"cardano_bech32_addr"`
			AccessTokenAlias  *string `json:"access_token_alias"`
		} `json:"user"`
	}
	if err := c.Post("/api/v2/auth/login/validate", validatePayload, &tokenResp); err != nil {
		return fmt.Errorf("authentication failed: %w", err)
	}

	if tokenResp.JWT == "" {
		return fmt.Errorf("authentication failed: no token received")
	}

	// Step 4: Store JWT in config
	cfg.UserJWT = tokenResp.JWT
	cfg.JWTExpiresAt = "" // headless flow does not return expiry; extract from JWT if needed
	// Use alias from response if available, fall back to flag
	if tokenResp.User.AccessTokenAlias != nil && *tokenResp.User.AccessTokenAlias != "" {
		cfg.UserAlias = *tokenResp.User.AccessTokenAlias
	} else {
		cfg.UserAlias = alias
	}
	cfg.UserID = tokenResp.User.ID
	if tokenResp.User.CardanoBech32Addr != nil {
		cfg.UserAddress = *tokenResp.User.CardanoBech32Addr
	}
	cfg.UserKeyHash = signResult.KeyHash

	// Derive enterprise address from skey if API didn't return one
	if cfg.UserAddress == "" {
		derived, deriveErr := cardano.DeriveEnterpriseAddress(pubKey, cfg.IsMainnet())
		if deriveErr == nil {
			cfg.UserAddress = derived
			if !isJSON {
				fmt.Fprintf(os.Stderr, "Derived address from signing key: %s\n", derived)
			}
		} else if !isJSON {
			fmt.Fprintf(os.Stderr, "Warning: could not derive address from signing key: %v\n", deriveErr)
		}
	}

	if err := config.Save(cfg); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	if isJSON {
		return output.PrintJSON(map[string]interface{}{
			"alias":    cfg.UserAlias,
			"user_id":  cfg.UserID,
			"address":  cfg.UserAddress,
			"key_hash": signResult.KeyHash,
		})
	}

	fmt.Fprintf(os.Stderr, "\nAuthenticated as: %s\n", cfg.UserAlias)
	fmt.Fprintf(os.Stderr, "User ID: %s\n", cfg.UserID)
	fmt.Fprintf(os.Stderr, "Address: %s\n", cfg.UserAddress)
	fmt.Fprintf(os.Stderr, "Key hash: %s\n", signResult.KeyHash)

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

type userStatusResult struct {
	APIKeySet              bool   `json:"api_key_set"`
	BaseURL                string `json:"base_url"`
	UserAuthenticated      bool   `json:"user_authenticated"`
	UserAlias              string `json:"user_alias,omitempty"`
	UserID                 string `json:"user_id,omitempty"`
	SessionExpiresAt       string `json:"session_expires_at,omitempty"`
	SessionExpired         *bool  `json:"session_expired,omitempty"`
	SessionRemainingSeconds int64 `json:"session_remaining_seconds,omitempty"`
}

func runUserStatus(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	if output.GetFormat() == output.FormatJSON {
		result := userStatusResult{
			APIKeySet:         cfg.APIKey != "",
			BaseURL:           cfg.BaseURL,
			UserAuthenticated: cfg.HasUserAuth(),
		}
		if cfg.HasUserAuth() {
			result.UserAlias = cfg.UserAlias
			result.UserID = cfg.UserID
			if cfg.JWTExpiresAt != "" {
				result.SessionExpiresAt = cfg.JWTExpiresAt
				if expiresAt, err := time.Parse(time.RFC3339, cfg.JWTExpiresAt); err == nil {
					now := time.Now()
					expired := now.After(expiresAt)
					result.SessionExpired = &expired
					if !expired {
						result.SessionRemainingSeconds = int64(expiresAt.Sub(now).Seconds())
					}
				}
			}
		}
		return output.PrintJSON(result)
	}

	fmt.Println("Authentication Status")
	fmt.Println("---------------------")

	// API Key status
	if cfg.APIKey != "" {
		fmt.Println("API Key: ****... (configured)")
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

func runUserMe(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	c := client.New(cfg)
	var result map[string]interface{}
	if err := c.Post("/api/v2/user/dashboard", nil, &result); err != nil {
		return err
	}

	// If JSON output requested, print raw JSON
	if output.GetFormat() == output.FormatJSON {
		return output.PrintJSON(result)
	}

	// Extract data from envelope
	data, ok := result["data"].(map[string]interface{})
	if !ok {
		return output.PrintJSON(result)
	}

	// Print formatted dashboard
	printDashboard(data)
	return nil
}

// ANSI color codes
const (
	cReset      = "\033[0m"
	cBold       = "\033[1m"
	cDim        = "\033[2m"
	cCyan       = "\033[36m"
	cGreen      = "\033[32m"
	cYellow     = "\033[33m"
	cMagenta    = "\033[35m"
	cBlue       = "\033[34m"
	cWhite      = "\033[97m"
	cBrightCyan = "\033[96m"
)

func printDashboard(data map[string]interface{}) {
	fmt.Println()

	// User info
	if user, ok := data["user"].(map[string]interface{}); ok {
		alias := getStr(user, "alias")
		fmt.Printf("%s%s◆ %s%s\n", cBold, cGreen, alias, cReset)
		fmt.Printf("%s%s━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━%s\n", cDim, cCyan, cReset)
	}

	// Counts summary
	if counts, ok := data["counts"].(map[string]interface{}); ok {
		fmt.Printf("\n%s%s📊 Summary%s\n", cBold, cYellow, cReset)
		enrolled := getInt(counts, "enrolled_courses")
		completed := getInt(counts, "completed_courses")
		teaching := getInt(counts, "teaching_courses")
		managing := getInt(counts, "managing_projects")
		contributing := getInt(counts, "contributing_projects")
		credentials := getInt(counts, "total_credentials")

		fmt.Printf("   %sCourses%s     %s%d%s enrolled  %s%d%s completed  %s%d%s teaching\n",
			cDim, cReset, cBold, enrolled, cReset, cBold, completed, cReset, cBold, teaching, cReset)
		fmt.Printf("   %sProjects%s    %s%d%s managing  %s%d%s contributing\n",
			cDim, cReset, cBold, managing, cReset, cBold, contributing, cReset)
		fmt.Printf("   %sCredentials%s %s%d%s earned\n",
			cDim, cReset, cBold, credentials, cReset)
	}

	// Teacher section
	if teacher, ok := data["teacher"].(map[string]interface{}); ok {
		if courses, ok := teacher["courses"].([]interface{}); ok && len(courses) > 0 {
			fmt.Printf("\n%s%s📚 Teaching%s\n", cBold, cMagenta, cReset)
			for _, c := range courses {
				if course, ok := c.(map[string]interface{}); ok {
					title := getStr(course, "title")
					if title == "" {
						title = "(untitled)"
					}
					fmt.Printf("   %s▸%s %s\n", cMagenta, cReset, title)
				}
			}
		}
		if pending := getInt(teacher, "total_pending_reviews"); pending > 0 {
			fmt.Printf("   %s%s⚠ %d pending reviews%s\n", cBold, cYellow, pending, cReset)
		}
	}

	// Student section
	if student, ok := data["student"].(map[string]interface{}); ok {
		if enrolled, ok := student["enrolled_courses"].([]interface{}); ok && len(enrolled) > 0 {
			fmt.Printf("\n%s%s🎓 Learning%s\n", cBold, cGreen, cReset)
			for _, c := range enrolled {
				if course, ok := c.(map[string]interface{}); ok {
					title := getStr(course, "title")
					if title == "" {
						title = "(untitled)"
					}
					fmt.Printf("   %s▸%s %s\n", cGreen, cReset, title)
				}
			}
		}
	}

	// Projects section
	if projects, ok := data["projects"].(map[string]interface{}); ok {
		if managing, ok := projects["managing"].([]interface{}); ok && len(managing) > 0 {
			fmt.Printf("\n%s%s🔧 Managing%s\n", cBold, cBlue, cReset)
			for _, p := range managing {
				if proj, ok := p.(map[string]interface{}); ok {
					title := getStr(proj, "title")
					if title == "" {
						title = "(untitled)"
					}
					fmt.Printf("   %s▸%s %s\n", cBlue, cReset, title)
				}
			}
		}
		if pending := getInt(projects, "total_pending_assessments"); pending > 0 {
			fmt.Printf("   %s%s⚠ %d pending assessments%s\n", cBold, cYellow, pending, cReset)
		}
	}

	fmt.Println()
}

func getStr(m map[string]interface{}, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

func getInt(m map[string]interface{}, key string) int {
	if v, ok := m[key].(float64); ok {
		return int(v)
	}
	return 0
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
</html>`, html.EscapeString(alias))
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
</html>`, html.EscapeString(errMsg))
}
