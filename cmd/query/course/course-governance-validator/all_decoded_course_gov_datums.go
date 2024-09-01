package course_governance_validator

import (
	"github.com/Andamio-Platform/andamio-cli/internal/client"
	"github.com/spf13/cobra"
)

var AllDecodedCourseGovDatumsCmd = &cobra.Command{
	Use:   "all-decoded-course-gov-datums",
	Short: "Check alias availability",
	Long:  `Check whether a given alias is available.`,
	Run: func(cmd *cobra.Command, args []string) {
		client.GetAllDecodedCourseGovDatums()
	},
}
