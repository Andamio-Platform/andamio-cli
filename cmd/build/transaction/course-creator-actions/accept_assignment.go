package course_creator_actions

import (
	"fmt"

	"github.com/Andamio-Platform/andamio-cli/internal/client"
	"github.com/spf13/cobra"
)

var userAccessToken string
var studentAlias string
var policy string

var AcceptAssignmentCmd = &cobra.Command{
	Use:   "accept-assignment",
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
		client.GetAcceptAssignment(userAccessToken, studentAlias, policy)
	},
}

func init() {
	AcceptAssignmentCmd.Flags().StringVar(&userAccessToken, "userAccessToken", "", "userAccessToken to check availability for")
	AcceptAssignmentCmd.Flags().StringVar(&studentAlias, "studentAlias", "", "studentAlias to check availability for")
	AcceptAssignmentCmd.Flags().StringVar(&policy, "policy", "", "policy to check availability for")
}
