package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"net/url"
	"os"
	"regexp"
	"time"

	"github.com/Andamio-Platform/andamio-cli/internal/apierr"
	"github.com/Andamio-Platform/andamio-cli/internal/client"
	"github.com/Andamio-Platform/andamio-cli/internal/config"
	"github.com/Andamio-Platform/andamio-cli/internal/output"
	"github.com/spf13/cobra"
)

// devKeyIDPattern matches the canonical UUID format the gateway emits for
// developer key ids. Validation is structural (not strict per RFC 4122) —
// any 8-4-4-4-12 hex sequence is accepted; deeper checks are gateway-side.
// The point of validating client-side is to catch typos and shell-expansion
// accidents — a stray `?` truncates the path, a `..` segment is forwarded
// literally (verified via httptest), and an empty `$ID` builds the list
// path with DELETE — before they hit the wire as confusing 4xx errors or,
// worse, target a different resource than the user named.
var devKeyIDPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

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
	// `devCfg := *cfg` is a shallow copy — `SubmitHeaders` is the only map
	// field on Config and would otherwise share the underlying map pointer
	// with the source. Today client.New does not read submit headers and
	// devKeysClient does not mutate them, so the shared pointer is safe.
	// But the auth-isolation contract this helper exists to enforce should
	// be defended structurally, not by current-implementation invariants.
	// Any future field added to Config that holds a reference type needs
	// the same explicit handling.
	devCfg.SubmitHeaders = maps.Clone(cfg.SubmitHeaders)
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

// devRawKey wraps a freshly minted API key string. The underlying value is
// usable via direct print (string-aliased type — fmt.Println prints the
// string verbatim, equality with "" works), but the LogValue() method makes
// accidental log emission impossible: any slog handler that captures a
// struct containing a devRawKey field renders it as "[redacted]" instead
// of the underlying secret. Mirrors the gateway's keys_viewmodels.RawKey
// type — defense-in-depth backstop in case a future refactor adds slog
// instrumentation around the create flow without re-reviewing what's safe
// to log. The compiler does not enforce no-string-cast, so a PR review
// still verifies new code does not unwrap to log it.
type devRawKey string

// LogValue implements slog.LogValuer.
func (k devRawKey) LogValue() slog.Value { return slog.StringValue("[redacted]") }

// devKeysDeleteResult is the typed `dev keys delete --output json` envelope.
// `id` echoes the deleted resource so cleanup pipelines can correlate
// against their own state; `deleted: true` is the success signal (404s
// surface as a returned error rather than `deleted: false`). Mirrors
// `devLogoutResult` — both commands have a single-bool success contract
// that benefits from a typed envelope so a future copy-edit cannot rename
// or drop a key without a compile error.
type devKeysDeleteResult struct {
	ID      string `json:"id"`
	Deleted bool   `json:"deleted"`
}

// devKeysCreateResponse mirrors keys_viewmodels.CreateKeyResponse. The Key
// field carries the raw API key — emitted to the user EXACTLY ONCE per
// the gateway's contract. Never persisted; the user is responsible for
// capturing it. Typed as devRawKey so a slog-based logger that captures
// the response struct redacts automatically.
type devKeysCreateResponse struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Environment string    `json:"environment"`
	Key         devRawKey `json:"key"`
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

	// Metadata + WARNING ride on stderr in BOTH modes. JSON consumers that
	// don't want the noise pipe `2>/dev/null`; humans running `--output
	// json` interactively for a one-off still see the one-time-use warning,
	// which is the single most important thing about this command. Pattern
	// matches `gh auth token`.
	fmt.Fprintf(os.Stderr, "Developer API key created (id: %s, environment: %s, name: %s).\n", resp.ID, resp.Environment, resp.Name)
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "WARNING: this is the only time the raw key value is returned. Store it immediately —")
	fmt.Fprintln(os.Stderr, "subsequent 'dev keys list' calls return only the last4 hint, not the full key.")
	fmt.Fprintln(os.Stderr, "")

	if output.GetFormat() == output.FormatJSON {
		return output.PrintJSON(resp)
	}

	// Text mode: raw key on stdout so `dev keys create … | pbcopy` captures
	// the key alone. Stderr already carries the metadata.
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
	if !devKeyIDPattern.MatchString(id) {
		return fmt.Errorf("invalid developer key id %q: must be a UUID returned by 'andamio dev keys list'", id)
	}
	c, err := devKeysClient(cfg)
	if err != nil {
		return err
	}

	// url.PathEscape is defense-in-depth — the UUID regex above already
	// rules out reserved characters, but encoding before formatting means a
	// future relaxation of the regex (or a different id format) cannot
	// reintroduce the URL-injection class.
	if err := c.Delete(ctx, fmt.Sprintf(devKeysDeletePathFmt, url.PathEscape(id)), nil); err != nil {
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
		return output.PrintJSON(devKeysDeleteResult{ID: id, Deleted: true})
	}
	fmt.Fprintf(os.Stderr, "Developer key %s revoked.\n", id)
	return nil
}
