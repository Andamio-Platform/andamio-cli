package courseInstances

import (
	"fmt"

	"github.com/spf13/cobra"
)

var CourseInstanceCmd = &cobra.Command{
	Use:   "course-instances",
	Short: "Example for Cardano Go PBL",
	Long:  ``,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("Querying Andamio Course Instances...")
		courseInstance()
	},
}

func init() {
	// AdderPublisherCmd.AddCommand(walletToWallet.WalletToWalletCmd)
}
