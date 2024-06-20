package sync

import (
	"fmt"

	adderPublisher "github.com/Andamio-Platform/andamio-cli/cmd/sync/adder-library-starter-kit"
	"github.com/spf13/cobra"
)

var SyncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Sync building commands",
	Long:  ``,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println(`andamio-cli sync
		Usage: andamio-cli sync
					( example-sync
					)

		`)
	},
}

func init() {
	SyncCmd.AddCommand(adderPublisher.ExampleSyncCmd)
}
