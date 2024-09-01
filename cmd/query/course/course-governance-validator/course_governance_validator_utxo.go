package course_governance_validator

import (
	"fmt"

	"github.com/spf13/cobra"
)

var CourseGovernanceValidatorUtxoCmd = &cobra.Command{
	Use:   "course-governance-validator-utxo",
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
	CourseGovernanceValidatorUtxoCmd.Flags().StringVar(&alias, "alias", "", "Alias to check availability for")
}
