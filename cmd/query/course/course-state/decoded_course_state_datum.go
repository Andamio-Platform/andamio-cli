package course_state

import (
	"fmt"

	"github.com/Andamio-Platform/andamio-cli/internal/client"
	"github.com/spf13/cobra"
)

var DecodedCourseStateDatumCmd = &cobra.Command{
	Use:   "decoded-course-state-datum",
	Short: "",
	Long:  ``,
	Run: func(cmd *cobra.Command, args []string) {
		if policy == "" {
			fmt.Println("Please provide an policy using --policy flag")
			return
		}
		if alias == "" {
			fmt.Println("Please provide an alias using --alias flag")
			return
		}

		client.GetDecodedCourseStateDatum(policy, alias)
	},
}

func init() {
	DecodedCourseStateDatumCmd.Flags().StringVar(&alias, "alias", "", "")
	DecodedCourseStateDatumCmd.Flags().StringVar(&policy, "policy", "", "")
}
