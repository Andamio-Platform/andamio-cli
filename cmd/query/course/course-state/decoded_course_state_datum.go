package course_state

import (
	"andamio-cli/utils"
	"log"

	"github.com/Andamio-Platform/andamio-cli/internal/client"
	"github.com/spf13/cobra"
)

var DecodedCourseStateDatumCmd = &cobra.Command{
	Use:   "learner-status",
	Short: "View course datum for specified alias in course with specified policy",
	Long:  ``,
	Run: func(cmd *cobra.Command, args []string) {
		response, err := client.GetDecodedCourseStateDatum(policy, alias)
		if err != nil {
			log.Fatalf("Falied to get learner status: %v", err)
		}

		utils.SaveOutputToFile(cmd, response)
	},
}

func init() {
	DecodedCourseStateDatumCmd.Flags().StringVar(&alias, "alias", "", "")
	DecodedCourseStateDatumCmd.Flags().StringVar(&policy, "policy", "", "")

	DecodedCourseStateDatumCmd.MarkFlagRequired("alias")
	DecodedCourseStateDatumCmd.MarkFlagRequired("policy")

	utils.AddOutFileFlag(DecodedCourseStateDatumCmd)
}
