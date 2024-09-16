package instance_validator

import (
	"fmt"

	"github.com/Andamio-Platform/andamio-cli/internal/client"
	"github.com/spf13/cobra"
)

var CourseInstanceUtxoCmd = &cobra.Command{
	Use:   "course-instance-utxo",
	Short: "",
	Long:  ``,
	Run: func(cmd *cobra.Command, args []string) {
		if policy == "" {
			fmt.Println("Please provide an policy using --policy flag")
			return
		}

		client.GetCourseInstanceUtxo(policy)
	},
}

func init() {
	CourseInstanceUtxoCmd.Flags().StringVar(&policy, "policy", "", "")
}
