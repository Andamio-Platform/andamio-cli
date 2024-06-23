package courseInstances

import (
	"fmt"

	"github.com/spf13/cobra"
)

var CourseInstanceCmd = &cobra.Command{
	Use:   "course-instances",
	Short: "List course instances on Andamio Network",
	Long: `
On the Andamio Network, a course instance is represented first with a UTxO.
This query provides quick access to those UTxOs.	
	`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("Querying Andamio Course Instances...")
		courseInstance()
	},
}

func init() {
	// AdderPublisherCmd.AddCommand(walletToWallet.WalletToWalletCmd)
}
