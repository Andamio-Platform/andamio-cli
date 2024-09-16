package course_creator_actions

import (
	"fmt"

	"github.com/Andamio-Platform/andamio-cli/internal/client"
	"github.com/spf13/cobra"
)

var DenyAssignmentCmd = &cobra.Command{
	Use:   "deny-assignment",
	Short: "Build transaction",
	Long:  `Build transactions.`,
	Run: func(cmd *cobra.Command, args []string) {
		if userAccessToken == "" {
			fmt.Println("Please provide an userAccessToken using --userAccessToken flag")
			return
		}
		if studentAlias == "" {
			fmt.Println("Please provide an studentAlias using --studentAlias flag")
			return
		}
		if policy == "" {
			fmt.Println("Please provide an policy using --policy flag")
			return
		}
		client.GetDenyAssignment(userAccessToken, studentAlias, policy)
	},
}

func init() {
	DenyAssignmentCmd.Flags().StringVar(&userAccessToken, "userAccessToken", "", "")
	DenyAssignmentCmd.Flags().StringVar(&studentAlias, "studentAlias", "", "")
	DenyAssignmentCmd.Flags().StringVar(&policy, "policy", "", "")
}
