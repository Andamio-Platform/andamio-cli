package course_governance_validator

import (
	"github.com/Andamio-Platform/andamio-cli/internal/client"
	"github.com/spf13/cobra"
)

var CoursePoliciesCmd = &cobra.Command{
	Use:   "course-policies",
	Short: "View a list of course policies where specified alias has creator access",
	Long:  ``,
	Run: func(cmd *cobra.Command, args []string) {
		client.GetCoursePolicies(alias)
	},
}

func init() {
	CoursePoliciesCmd.Flags().StringVar(&alias, "alias", "", "")

	CoursePoliciesCmd.MarkFlagRequired("alias")
}
