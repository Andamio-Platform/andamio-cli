package transaction

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var TransactionCmd = &cobra.Command{
	Use:   "transaction",
	Short: "build transaction",
	Long: `Build Andamio transactions. Use subcommands to build specific transactions.

Use andamio-cli
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
