package instance_validator

import (
	"github.com/Andamio-Platform/andamio-cli/internal/client"
	"github.com/spf13/cobra"
)

var DecodedCourseInstanceDatumCmd = &cobra.Command{
	Use:   "decoded-course-instance-datum",
	Short: "",
	Long:  ``,
	Run: func(cmd *cobra.Command, args []string) {
		client.GetDecodedCourseInstanceDatum(policy)
	},
}

func init() {
	DecodedCourseInstanceDatumCmd.Flags().StringVar(&policy, "policy", "", "")

	DecodedCourseInstanceDatumCmd.MarkFlagRequired("policy")
}
