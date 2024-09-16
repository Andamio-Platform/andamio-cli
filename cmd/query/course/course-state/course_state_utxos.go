package course_state

import (
	"fmt"

	"github.com/Andamio-Platform/andamio-cli/internal/client"
	"github.com/spf13/cobra"
)

var CourseStateUtxosCmd = &cobra.Command{
	Use:   "course-state-utxos",
	Short: "",
	Long:  ``,
	Run: func(cmd *cobra.Command, args []string) {
		if policy == "" {
			fmt.Println("Please provide an policy using --policy flag")
			return
		}

		client.GetCourseStateUtxos(policy)
	},
}

func init() {
	CourseStateUtxosCmd.Flags().StringVar(&policy, "policy", "", "")
}
