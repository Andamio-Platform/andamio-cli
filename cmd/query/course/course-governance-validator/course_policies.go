package course_governance_validator

import (
	"fmt"

	"github.com/Andamio-Platform/andamio-cli/internal/client"
	"github.com/spf13/cobra"
)

var CoursePoliciesCmd = &cobra.Command{
	Use:   "course-policies",
	Short: "",
	Long:  ``,
	Run: func(cmd *cobra.Command, args []string) {
		if alias == "" {
			fmt.Println("Please provide an alias using --alias flag")
			return
		}

		client.GetCoursePolicies(alias)
	},
}

func init() {
	CoursePoliciesCmd.Flags().StringVar(&alias, "alias", "", "")
}
