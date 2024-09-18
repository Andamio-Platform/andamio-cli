package transaction

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var TransactionCmd = &cobra.Command{
	Use:   "transaction",
	Short: "build transaction",
	Long: `The Andamio Network is home to valuable, public data that becomes even 
more valuable when you have tools to make sense of it. Andamio CLI gives 
developers instant access to transactions, making it easier to explore possibilities
and build new tools on Andamio.

This release of andamio-cli features transactions for
1. Minting access tokens
2. Student interactions
3. Course creator interactions

Transactions for Andamio contributors will be added in a future release.
  `,
	Run: func(cmd *cobra.Command, args []string) {
		// If no arguments are passed, show help
		if len(args) == 0 {
			cmd.Help()
			return
		}

		// If an invalid subcommand is passed, show an error message
		fmt.Printf("Error: '%s' is not a valid subcommand for 'transaction'\n", args[0])
		fmt.Println("Run './andamio-cli build transaction --help' for available subcommands.")
		os.Exit(1) // Exit with a non-zero status to indicate an error
	},
}

func init() {
	TransactionCmd.AddCommand(MintAccessTokenCmd)
	TransactionCmd.AddCommand(StudentActionsCmd)
	TransactionCmd.AddCommand(CourseCreatorActionsCmd)
}
