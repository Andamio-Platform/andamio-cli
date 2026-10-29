package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// devPortalDeprecationNotice is printed on stderr by every command in
// devPortalDeprecatedCommands. Key management moved to the app's developer
// portal, and /v2/keys left Andamio API's public contract in 2.6.0.
const devPortalDeprecationNotice = "Deprecated: API keys are managed at https://app.andamio.io/api-setup. This command is removed in andamio CLI 2.0.0."

// devPortalDeprecatedCommands is every command removed in 2.0.0 with the
// developer-portal surface: the whole dev group, and apikey, which needs the
// dev JWT that only 'dev login' mints. At 2.0.0 these move to retired.go.
//
// The notice is printed by wrapping RunE, not by cobra's Deprecated field.
// Cobra hides a Deprecated command from help, which would leave 'andamio dev
// --help' listing no subcommands while they still work. PersistentPreRunE is
// not used either: a child's replaces the root's, which parses --output.
var devPortalDeprecatedCommands = []*cobra.Command{
	devLoginCmd,
	devLogoutCmd,
	devRefreshCmd,
	devStatusCmd,
	devKeysListCmd,
	devKeysCreateCmd,
	devKeysDeleteCmd,
	apikeyUsageCmd,
	apikeyProfileCmd,
}

func init() {
	for _, cmd := range devPortalDeprecatedCommands {
		run := cmd.RunE
		cmd.RunE = func(cmd *cobra.Command, args []string) error {
			// Printed in every output mode and before any request, so a run
			// that then fails on auth still says where keys are managed.
			fmt.Fprintln(os.Stderr, devPortalDeprecationNotice)
			return run(cmd, args)
		}
	}
}
