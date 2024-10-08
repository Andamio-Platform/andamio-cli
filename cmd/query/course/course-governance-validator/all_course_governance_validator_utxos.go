package course_governance_validator

import (
	"github.com/Andamio-Platform/andamio-cli/internal/client"
	"github.com/spf13/cobra"
)

var alias string

var AllCourseGovernanceValidatorUtxosCmd = &cobra.Command{
	Use:   "all-course-governance-validator-utxos",
	Short: "View all course governance utxos",
	Long:  ``,
	Run: func(cmd *cobra.Command, args []string) {
		client.GetAllCourseGovernanceValidatorUtxos()
	},
}
