package course_state

import (
	"fmt"

	"github.com/Andamio-Platform/andamio-cli/internal/client"
	"github.com/spf13/cobra"
)

var alias string

var CourseStateUtxoCmd = &cobra.Command{
	Use:   "course-state-utxo",
	Short: "Check alias availability",
	Long:  `Check whether a given alias is available.`,
	Run: func(cmd *cobra.Command, args []string) {
		if policy == "" {
			fmt.Println("Please provide an policy using --policy flag")
			return
		}
		if alias == "" {
			fmt.Println("Please provide an alias using --alias flag")
			return
		}
		fmt.Printf("Checking availability for alias: %s\n", alias)
		// Your alias availability logic here
		client.GetCourseStateUtxo(policy, alias)
	},
}

func init() {
	CourseStateUtxoCmd.Flags().StringVar(&alias, "alias", "", "Alias to check availability for")
	CourseStateUtxoCmd.Flags().StringVar(&policy, "policy", "", "policy to check availability for")
}
