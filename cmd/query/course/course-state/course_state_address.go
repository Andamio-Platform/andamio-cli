package course_state

import (
	"github.com/Andamio-Platform/andamio-cli/internal/client"
	"github.com/spf13/cobra"
)

var policy string

var CourseStateAddressCmd = &cobra.Command{
	Use:   "course-state-address",
	Short: "",
	Long:  ``,
	Run: func(cmd *cobra.Command, args []string) {
		client.GetCourseStateAddress(policy)
	},
}

func init() {
	CourseStateAddressCmd.Flags().StringVar(&policy, "policy", "", "")

	CourseStateAddressCmd.MarkFlagRequired("policy")
}
