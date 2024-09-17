package course_governance_validator

import (
	"github.com/Andamio-Platform/andamio-cli/internal/client"
	"github.com/spf13/cobra"
)

var policy string

var CourseGovernanceValidatorUtxoCmd = &cobra.Command{
	Use:   "course-governance-validator-utxo",
	Short: "View course governance utxo for specified course policy",
	Long:  ``,
	Run: func(cmd *cobra.Command, args []string) {
		client.GetCourseGovernanceValidatorUtxo(policy)
	},
}

func init() {
	CourseGovernanceValidatorUtxoCmd.Flags().StringVar(&policy, "policy", "", "")

	CourseGovernanceValidatorUtxoCmd.MarkFlagRequired("policy")
}
