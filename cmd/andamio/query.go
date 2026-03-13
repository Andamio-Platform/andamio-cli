package main

import (
	"encoding/json"
	"fmt"

	"github.com/Andamio-Platform/andamio-cli/internal/client"
	"github.com/Andamio-Platform/andamio-cli/internal/config"
	"github.com/spf13/cobra"
)

var queryCmd = &cobra.Command{
	Use:   "query",
	Short: "Query on-chain and off-chain data",
	Long:  `Query credentials, tasks, and other data from the Andamio platform.`,
}

var queryCredentialCmd = &cobra.Command{
	Use:   "credential [stake-key]",
	Short: "Query credentials for a stake key",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		stakeKey := args[0]

		cfg, err := config.Load()
		if err != nil {
			return err
		}

		c := client.New(cfg)
		var credentials []map[string]interface{}
		if err := c.Get("/api/v2/credentials?stakeKey="+stakeKey, &credentials); err != nil {
			return err
		}

		if len(credentials) == 0 {
			fmt.Println("No credentials found for this stake key.")
			return nil
		}

		output, err := json.MarshalIndent(credentials, "", "  ")
		if err != nil {
			return err
		}

		fmt.Println(string(output))
		return nil
	},
}

func init() {
	rootCmd.AddCommand(queryCmd)
	queryCmd.AddCommand(queryCredentialCmd)
}
