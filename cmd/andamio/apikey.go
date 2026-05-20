package main

import (
	"context"
	"fmt"

	"github.com/Andamio-Platform/andamio-cli/internal/apierr"
	"github.com/Andamio-Platform/andamio-cli/internal/config"
	"github.com/Andamio-Platform/andamio-cli/internal/output"
	"github.com/spf13/cobra"
)

var apikeyCmd = &cobra.Command{
	Use:   "apikey",
	Short: "API key management",
	Long: `Developer API key management.

Subcommands query the developer-portal apikey surface
(/api/v2/apikey/developer/*), which is a dual-credential gateway surface:

  - X-API-Key (gateway V2AuthMiddleware)        → andamio auth login --api-key <key>
  - Authorization: Bearer <devJWT>              → andamio dev login --skey <path> --alias <name> --address <bech32>

The wallet/user JWT slot (` + "`user login`" + `) is NOT accepted on this surface
— the gateway's developerJWTAuth middleware rejects it. Run BOTH login commands
before invoking apikey subcommands; an empty slot short-circuits with an
actionable hint pointing at the missing command.`,
}

var apikeyUsageCmd = &cobra.Command{
	Use:   "usage",
	Short: "Get API key usage stats",
	Long: `Get developer API key usage stats.

Requires BOTH an API key and developer authentication. Run:
  andamio auth login --api-key <key>
  andamio dev login --skey <path> --alias <name> --address <bech32>`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return fmt.Errorf("failed to load config: %w", err)
		}
		return runAPIKeyJSON(cmd.Context(), cfg, "/api/v2/apikey/developer/usage/get")
	},
}

var apikeyProfileCmd = &cobra.Command{
	Use:   "profile",
	Short: "Get API key profile",
	Long: `Get developer API key profile.

Requires BOTH an API key and developer authentication. Run:
  andamio auth login --api-key <key>
  andamio dev login --skey <path> --alias <name> --address <bech32>`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return fmt.Errorf("failed to load config: %w", err)
		}
		return runAPIKeyJSON(cmd.Context(), cfg, "/api/v2/apikey/developer/profile/get")
	},
}

func init() {
	rootCmd.AddCommand(apikeyCmd)
	apikeyCmd.AddCommand(apikeyUsageCmd)
	apikeyCmd.AddCommand(apikeyProfileCmd)
}

// runAPIKeyJSON GETs a developer-portal apikey endpoint and prints the JSON
// body. The `/api/v2/apikey/developer/*` surface carries the same dual
// middleware stack as `/api/v2/keys` (andamio-api main_router.go): the
// outer `v2Group` `V2AuthMiddleware` requires `X-API-Key`, and the inner
// `v2ApiKeyGroup.Use(developerJWTAuth)` requires `Authorization: Bearer
// <devJWT>`. Both headers must ride on the request — so we reuse
// `devKeysClient`, which clones cfg, keeps `X-API-Key`, and promotes
// `DevJWT` into the JWT slot. An API key or wallet/user JWT alone yields
// 401 "Authorization header with Developer JWT required" from the gateway.
//
// History: this helper previously stripped the JWT and sent only
// `X-API-Key` (the correct shape when the gateway *rejected* dev JWTs on
// this surface — see docs/solutions/.../cli-apikey-auth-isolation...md).
// The gateway later moved these routes behind `developerJWTAuth`, inverting
// that requirement; routing through `devKeysClient` realigns the CLI and
// surfaces a `dev login` hint when the dev slot is empty.
func runAPIKeyJSON(ctx context.Context, cfg *config.Config, path string) error {
	// Pre-check the API key locally so an operator who ran `dev login` but
	// not `auth login --api-key` gets an actionable CLI hint instead of a
	// raw gateway 401 from V2AuthMiddleware. Mirrors the older getAPIKeyJSON
	// guard this command used to carry. Scoped here rather than inside the
	// shared devKeysClient so `dev keys` semantics (which surface the API-key
	// requirement via its own gateway 401) remain unchanged.
	if cfg.APIKey == "" {
		return &apierr.AuthError{
			Message: "apikey commands require an API key. Run 'andamio auth login --api-key <key>'",
		}
	}
	c, err := devKeysClient(cfg)
	if err != nil {
		return err
	}
	var result map[string]interface{}
	if err := c.Get(ctx, path, &result); err != nil {
		return err
	}
	return output.PrintJSON(result)
}
