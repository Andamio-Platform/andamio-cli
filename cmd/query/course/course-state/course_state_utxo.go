package course_state

import (
	"fmt"

	"github.com/Andamio-Platform/andamio-cli/internal/client"
	"github.com/spf13/cobra"
)

var alias string

var CourseStateUtxoCmd = &cobra.Command{
	Use:   "course-state-utxo",
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

		client.GetCourseStateUtxo(policy, alias)
	},
}

func init() {
	CourseStateUtxoCmd.Flags().StringVar(&alias, "alias", "", "")
	CourseStateUtxoCmd.Flags().StringVar(&policy, "policy", "", "")
}
