package course_state

import (
	"github.com/Andamio-Platform/andamio-cli/internal/client"
	"github.com/spf13/cobra"
)

var CourseStateUtxosCmd = &cobra.Command{
	Use:   "course-state-utxos",
	Short: "View all course state utxos for course with specified policy",
	Long:  ``,
	Run: func(cmd *cobra.Command, args []string) {
		client.GetCourseStateUtxos(policy)
	},
}

func init() {
	CourseStateUtxosCmd.Flags().StringVar(&policy, "policy", "", "")

	CourseStateUtxosCmd.MarkFlagRequired("policy")
}
