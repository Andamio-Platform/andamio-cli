package course_state

import (
	"github.com/Andamio-Platform/andamio-cli/internal/client"
	"github.com/spf13/cobra"
)

var CourseStateUtxosCmd = &cobra.Command{
	Use:   "course-state-utxos",
	Short: "",
	Long:  ``,
	Run: func(cmd *cobra.Command, args []string) {
		client.GetCourseStateUtxos(policy)
	},
}

func init() {
	CourseStateUtxosCmd.Flags().StringVar(&policy, "policy", "", "")

	CourseStateUtxoCmd.MarkFlagRequired("policy")
}
