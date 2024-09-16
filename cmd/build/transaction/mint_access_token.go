package transaction

import (
	"github.com/Andamio-Platform/andamio-cli/internal/client"
	"github.com/spf13/cobra"
)

var (
	userAddress string
	alias       string
	userInfo    string
)

var MintAccessTokenCmd = &cobra.Command{
	Use:   "mint-access-token",
	Short: "Mint an Andamio Access Token",
	Long: `
------------------ Build Transaction ------------------
An access token is required to interact with Andamio. This transaction will mint an access token.

-------------------- Example Usage --------------------
andamio-cli build transaction mint-access-token \ 
  --userAddress addr_test1... \ 
  --alias yourTokenName \
  --userInfo "Andamio Token"
-------------------------------------------------------
  `,
	Run: func(cmd *cobra.Command, args []string) {
		client.GetMintAccessToken(userAddress, alias, userInfo)
	},
}

func init() {
	MintAccessTokenCmd.Flags().StringVar(&userAddress, "userAddress", "", "Preprod wallet address that will sign the transaction")
	MintAccessTokenCmd.Flags().StringVar(&alias, "alias", "", "Access Token Alias")
	MintAccessTokenCmd.Flags().StringVar(&userInfo, "userInfo", "", "Network user info. Can be any string.")

	// Mark flags as required
	MintAccessTokenCmd.MarkFlagRequired("userAddress")
	MintAccessTokenCmd.MarkFlagRequired("alias")
	MintAccessTokenCmd.MarkFlagRequired("userInfo")
}
