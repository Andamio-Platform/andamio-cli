package course_state

import (
	"github.com/Andamio-Platform/andamio-cli/internal/client"
	"github.com/spf13/cobra"
)

var alias string

var CourseStateUtxoCmd = &cobra.Command{
	Use:   "course-state-utxo",
	Short: "View the courses status of specified alias in course with specified policy",
	Long:  ``,
	Run: func(cmd *cobra.Command, args []string) {
		client.GetCourseStateUtxo(policy, alias)
	},
}

func init() {
	CourseStateUtxoCmd.Flags().StringVar(&alias, "alias", "", "")
	CourseStateUtxoCmd.Flags().StringVar(&policy, "policy", "", "")

	CourseStateUtxoCmd.MarkFlagRequired("alias")
	CourseStateUtxoCmd.MarkFlagRequired("policy")
}
