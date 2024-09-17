package course_state

import (
	"github.com/Andamio-Platform/andamio-cli/internal/client"
	"github.com/spf13/cobra"
)

var DecodedCourseStateDatumCmd = &cobra.Command{
	Use:   "decoded-course-state-datum",
	Short: "",
	Long:  ``,
	Run: func(cmd *cobra.Command, args []string) {
		client.GetDecodedCourseStateDatum(policy, alias)
	},
}

func init() {
	DecodedCourseStateDatumCmd.Flags().StringVar(&alias, "alias", "", "")
	DecodedCourseStateDatumCmd.Flags().StringVar(&policy, "policy", "", "")

	DecodedCourseStateDatumCmd.MarkFlagRequired("alias")
	DecodedCourseStateDatumCmd.MarkFlagRequired("policy")
}
