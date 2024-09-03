package course_state

import (
	"fmt"

	"github.com/Andamio-Platform/andamio-cli/internal/client"
	"github.com/spf13/cobra"
)

var policy string

var CourseStateAddressCmd = &cobra.Command{
	Use:   "course-state-address",
	Short: "",
	Long:  ``,
	Run: func(cmd *cobra.Command, args []string) {
		if policy == "" {
			fmt.Println("Please provide an policy using --policy flag")
			return
		}

		client.GetCourseStateAddress(policy)
	},
}

func init() {
	CourseStateAddressCmd.Flags().StringVar(&policy, "policy", "", "")
}
