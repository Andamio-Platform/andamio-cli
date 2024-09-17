package instance_validator

import (
	"github.com/Andamio-Platform/andamio-cli/internal/client"
	"github.com/spf13/cobra"
)

var CourseInstanceUtxoCmd = &cobra.Command{
	Use:   "course-instance-utxo",
	Short: "",
	Long:  ``,
	Run: func(cmd *cobra.Command, args []string) {
		client.GetCourseInstanceUtxo(policy)
	},
}

func init() {
	CourseInstanceUtxoCmd.Flags().StringVar(&policy, "policy", "", "")

	CourseInstanceUtxoCmd.MarkFlagRequired("policy")
}
