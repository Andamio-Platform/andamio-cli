package instance_validator

import (
	"github.com/Andamio-Platform/andamio-cli/internal/client"
	"github.com/spf13/cobra"
)

var AllCourseInstanceUtxosCmd = &cobra.Command{
	Use:   "all-course-instance-utxos",
	Short: "Check alias availability",
	Long:  `Check whether a given alias is available.`,
	Run: func(cmd *cobra.Command, args []string) {
		client.GetAllCourseInstanceUtxos()
	},
}
