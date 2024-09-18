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
	Short: "Mint an Andamio access token",
	Long: `
About:
An access token is required to interact with Andamio. 

This transaction mints a unique Andamio access token. This transaction will fail if access token name is already minted.

  `,
	Run: func(cmd *cobra.Command, args []string) {
		client.GetMintAccessToken(userAddress, alias, userInfo)
	},
}

func init() {
	MintAccessTokenCmd.Flags().StringVar(&userAddress, "userAddress", "", "Preprod wallet address to receive access token. Minting transaction requires a signature from this address.")
	MintAccessTokenCmd.Flags().StringVar(&alias, "alias", "", "Unique access token name")
	MintAccessTokenCmd.Flags().StringVar(&userInfo, "userInfo", "new Andamio access token", "Optional string.")

	// Required flags
	MintAccessTokenCmd.MarkFlagRequired("userAddress")
	MintAccessTokenCmd.MarkFlagRequired("alias")
}
