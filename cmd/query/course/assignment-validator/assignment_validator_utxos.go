package assignment_validator

import (
	"fmt"

	"github.com/spf13/cobra"
)

var AssignmentValidatorUtxosCmd = &cobra.Command{
	Use:   "assignment-validator-utxos",
	Short: "Check alias availability",
	Long:  `Check whether a given alias is available.`,
	Run: func(cmd *cobra.Command, args []string) {
		if alias == "" {
			fmt.Println("Please provide an alias using --alias flag")
			return
		}
		fmt.Printf("Checking availability for alias: %s\n", alias)
		// Your alias availability logic here
	},
}

func init() {
	AssignmentValidatorUtxosCmd.Flags().StringVar(&alias, "alias", "", "Alias to check availability for")
}
