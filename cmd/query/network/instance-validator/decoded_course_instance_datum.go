package instance_validator

import (
	"fmt"

	"github.com/Andamio-Platform/andamio-cli/internal/client"
	"github.com/spf13/cobra"
)

var DecodedCourseInstanceDatumCmd = &cobra.Command{
	Use:   "decoded-course-instance-datum",
	Short: "Check policy availability",
	Long:  `Check whether a given policy is available.`,
	Run: func(cmd *cobra.Command, args []string) {
		if policy == "" {
			fmt.Println("Please provide an policy using --policy flag")
			return
		}
		fmt.Printf("Checking availability for policy: %s\n", policy)
		// Your policy availability logic here
		client.GetDecodedCourseInstanceDatum(policy)
	},
}

func init() {
	DecodedCourseInstanceDatumCmd.Flags().StringVar(&policy, "policy", "", "policy to check availability for")
}
