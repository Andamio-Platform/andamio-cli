package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/Andamio-Platform/andamio-cli/internal/apierr"
	"github.com/Andamio-Platform/andamio-cli/internal/client"
	"github.com/Andamio-Platform/andamio-cli/internal/config"
	"github.com/Andamio-Platform/andamio-cli/internal/output"
	"github.com/spf13/cobra"
)

// PR-B (#80 second slice) — wraps andamio-api's `/v2/keys` developer-portal
// surface (added in #410, separate from the legacy `/apikey/developer/key/*`
// POST-with-body routes). All three endpoints require the developer JWT
// minted by `andamio dev login`; the gateway's `developerJWTAuth` middleware
// rejects wallet/user JWTs and api-keys-only.
//
// Routing approach: clone the loaded cfg, clear `APIKey`, promote `DevJWT`
// into `UserJWT` so `internal/client.setHeaders` emits exactly one credential
// (`Authorization: Bearer <devJWT>`). Mirrors `cmd/andamio/apikey.go`'s clone
// pattern. Tracked design choice in #84 item 3 — kept consistent with the
// existing clone helper rather than introducing a parallel `Client.SetDevJWT`
// API surface that wouldn't pay for itself today.
const (
	devKeysListPath   = "/api/v2/keys"
	devKeysCreatePath = "/api/v2/keys"
	// devKeysDeletePathFmt formats the per-id revoke path. Path arg is the
	// local UUID returned in `dev keys list` / `dev keys create`.
	devKeysDeletePathFmt = "/api/v2/keys/%s"
)

var devKeysCmd = &cobra.Command{
	Use:   "keys",
	Short: "Manage developer API keys (mainnet + preprod)",
	Long: `Developer API key management.

Wraps the gateway's /v2/keys surface — list, create, and delete API keys
across both mainnet and preprod environments. Authenticates with the
developer JWT minted by 'andamio dev login'; api-key + wallet-JWT slots
are not accepted by this endpoint family.

Run 'andamio dev login --skey <path> --alias <name> --address <bech32>'
first if you have not yet minted a developer JWT.`,
	Args: cobra.NoArgs,
}

var devKeysListCmd = &cobra.Command{
	Use:   "list",
	Short: "List developer API keys (mainnet + preprod, unified)",
	Long: `List all active developer API keys. Returns a unified view across
mainnet and preprod environments, sorted newest-first. Mainnet entries do
not carry the last4 hint (legacy storage shape); preprod entries do.`,
	Args: cobra.NoArgs,
	RunE: runDevKeysList,
}

var devKeysCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a developer API key",
	Long: `Create a new developer API key in the requested environment.

The raw key value is returned EXACTLY ONCE in the response — store it
immediately. Subsequent 'dev keys list' calls return only the last4 hint
and metadata; the full key is not retrievable.

Examples:
  andamio dev keys create --name "preprod-bot" --environment preprod
  andamio dev keys create --name "mainnet-prod" --environment mainnet --output json`,
	Args: cobra.NoArgs,
	RunE: runDevKeysCreate,
}

var devKeysDeleteCmd = &cobra.Command{
	Use:   "delete <id>",
	Short: "Revoke a developer API key by id",
	Long: `Revoke a developer API key. The id is the local UUID returned in
'dev keys list' / 'dev keys create'. Both mainnet and preprod ids are
accepted — the gateway routes the revoke to the correct environment.

A 404 is returned for both unknown ids and ids owned by another developer
(intentionally indistinguishable, per the gateway's threat model).`,
	Args: cobra.ExactArgs(1),
	RunE: runDevKeysDelete,
}

func init() {
	devCmd.AddCommand(devKeysCmd)
	devKeysCmd.AddCommand(devKeysListCmd)
	devKeysCmd.AddCommand(devKeysCreateCmd)
	devKeysCmd.AddCommand(devKeysDeleteCmd)

	devKeysCreateCmd.Flags().String("name", "", "Human-readable key label (3-64 chars, required)")
	devKeysCreateCmd.Flags().String("environment", "", "Target environment: mainnet or preprod (required)")
	devKeysCreateCmd.MarkFlagRequired("name")
	devKeysCreateCmd.MarkFlagRequired("environment")
}

// devKeysClient builds an HTTP client with auth headers scoped to the
// developer JWT only. Clones the loaded cfg, clears `APIKey` so X-API-Key
// is not also sent (the gateway's `developerJWTAuth` middleware rejects
// dual-credential requests — past pain in
// docs/solutions/integration-issues/cli-apikey-auth-isolation-and-content-404-ux.md),
// and promotes `DevJWT` into the `UserJWT` slot so `setHeaders` emits
// `Authorization: Bearer <devJWT>` via the existing path. The original cfg
// is not mutated — callers that subsequently `Save()` write the unchanged
// dev/user/api-key state back to disk.
//
// Returns a typed AuthError if the dev slot is empty, with an inline hint
// pointing at `dev login`.
func devKeysClient(cfg *config.Config) (*client.Client, error) {
	if !cfg.HasDevAuth() {
		return nil, &apierr.AuthError{
			Message: "developer authentication required. Run 'andamio dev login --skey <path> --alias <name> --address <bech32>' first",
		}
	}
	devCfg := *cfg
	devCfg.APIKey = ""
	devCfg.UserJWT = cfg.DevJWT
	return client.New(&devCfg), nil
}

// devKeyListItem mirrors andamio-api's keys_viewmodels.KeyListItem one-for-one.
// Decoded from the gateway's `{keys: [...]}` envelope.
type devKeyListItem struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Environment string    `json:"environment"`
	Last4       string    `json:"last4"`
	CreatedAt   time.Time `json:"created_at"`
}

type devKeysListResponse struct {
	Keys []devKeyListItem `json:"keys"`
}

func runDevKeysList(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}
	return runDevKeysListFlow(cmd.Context(), cfg)
}

// runDevKeysListFlow is the testable core of `dev keys list`. Split from
// runDevKeysList so unit tests can pass an explicit context (cmd.Context()
// is nil when RunE is invoked outside cobra.Execute) and a stubbed cfg
// without going through config.Load.
func runDevKeysListFlow(ctx context.Context, cfg *config.Config) error {
	c, err := devKeysClient(cfg)
	if err != nil {
		return err
	}

	var resp devKeysListResponse
	if err := c.Get(ctx, devKeysListPath, &resp); err != nil {
		return fmt.Errorf("list developer keys failed: %w", err)
	}

	if output.GetFormat() == output.FormatJSON {
		// Pass the gateway shape through verbatim so scripts can read
		// `.keys[].environment`, `.keys[].last4`, etc. with `jq`.
		return output.PrintJSON(resp)
	}

	if len(resp.Keys) == 0 {
		fmt.Fprintln(os.Stderr, "No developer API keys.")
		fmt.Fprintln(os.Stderr, "Run 'andamio dev keys create --name <label> --environment <mainnet|preprod>' to create one.")
		return nil
	}
	for _, k := range resp.Keys {
		// Mainnet entries lack last4 (legacy table stores only bcrypt + prefix shard);
		// surface "—" to keep columns aligned without misrepresenting the absence.
		last4 := k.Last4
		if last4 == "" {
			last4 = "—"
		}
		fmt.Printf("%s  %-8s  ...%s  %s  %s\n", k.ID, k.Environment, last4, k.CreatedAt.Local().Format("2006-01-02 15:04"), k.Name)
	}
	return nil
}

// devKeysCreateRequest matches keys_viewmodels.CreateKeyRequest. Validation
// is server-side (`min=3,max=64`, `oneof=mainnet preprod`); the CLI does
// not duplicate it client-side because the gateway is the source of truth
// and a client-side check would drift on policy changes.
type devKeysCreateRequest struct {
	Name        string `json:"name"`
	Environment string `json:"environment"`
}

// devKeysCreateResponse mirrors keys_viewmodels.CreateKeyResponse. The Key
// field carries the raw API key — emitted to the user EXACTLY ONCE per
// the gateway's contract. Never logged, never persisted; the user is
// responsible for capturing it.
type devKeysCreateResponse struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Environment string    `json:"environment"`
	Key         string    `json:"key"`
	Last4       string    `json:"last4"`
	CreatedAt   time.Time `json:"created_at"`
}

func runDevKeysCreate(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}
	name, _ := cmd.Flags().GetString("name")
	environment, _ := cmd.Flags().GetString("environment")
	return runDevKeysCreateFlow(cmd.Context(), cfg, name, environment)
}

// runDevKeysCreateFlow is the testable core of `dev keys create`. See
// runDevKeysListFlow for the split rationale.
func runDevKeysCreateFlow(ctx context.Context, cfg *config.Config, name, environment string) error {
	c, err := devKeysClient(cfg)
	if err != nil {
		return err
	}

	var resp devKeysCreateResponse
	if err := c.Post(ctx, devKeysCreatePath, devKeysCreateRequest{
		Name:        name,
		Environment: environment,
	}, &resp); err != nil {
		// 422 with `tier_limit_exceeded`/`invalid_environment` body codes
		// surfaces via apierr.* — the gateway-side stable error codes are
		// preserved verbatim in the message so scripts that match on them
		// continue to work.
		return fmt.Errorf("create developer key failed: %w", err)
	}
	if resp.Key == "" {
		// Defensive: a 200 with no `key` field in the body means the
		// gateway dropped the raw value (which would be unrecoverable).
		// Refuse to silently succeed — the developer would have a key
		// they cannot use.
		return fmt.Errorf("create developer key failed: gateway returned no key value (the raw key is unrecoverable; this is a gateway bug)")
	}

	if output.GetFormat() == output.FormatJSON {
		return output.PrintJSON(resp)
	}

	// Text mode: surface the raw key on stdout (so it can be piped/captured)
	// and the warning on stderr (so it doesn't pollute the captured key).
	// Pattern matches `gh auth token` etc.
	fmt.Fprintf(os.Stderr, "Developer API key created (id: %s, environment: %s, name: %s).\n", resp.ID, resp.Environment, resp.Name)
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "WARNING: this is the only time the raw key value is returned. Store it immediately —")
	fmt.Fprintln(os.Stderr, "subsequent 'dev keys list' calls return only the last4 hint, not the full key.")
	fmt.Fprintln(os.Stderr, "")
	fmt.Println(resp.Key)
	return nil
}

func runDevKeysDelete(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}
	return runDevKeysDeleteFlow(cmd.Context(), cfg, args[0])
}

// runDevKeysDeleteFlow is the testable core of `dev keys delete`. See
// runDevKeysListFlow for the split rationale.
func runDevKeysDeleteFlow(ctx context.Context, cfg *config.Config, id string) error {
	c, err := devKeysClient(cfg)
	if err != nil {
		return err
	}

	if err := c.Delete(ctx, fmt.Sprintf(devKeysDeletePathFmt, id)); err != nil {
		// 404 from this endpoint covers both "not found" and "owned by
		// another developer" — gateway treats them identically by design.
		// Surface that fact so users don't waste time debugging an id
		// they typo'd vs an id that's not theirs.
		var nfErr *apierr.NotFoundError
		if errors.As(err, &nfErr) {
			return fmt.Errorf("developer key %s not found (or not owned by your developer account)", id)
		}
		return fmt.Errorf("delete developer key failed: %w", err)
	}

	if output.GetFormat() == output.FormatJSON {
		return output.PrintJSON(map[string]interface{}{
			"id":      id,
			"deleted": true,
		})
	}
	fmt.Fprintf(os.Stderr, "Developer key %s revoked.\n", id)
	return nil
}
