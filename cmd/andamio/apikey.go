package main

import (
	"github.com/spf13/cobra"
)

var apikeyCmd = &cobra.Command{
	Use:   "apikey",
	Short: "API key management",
}

var apikeyUsageCmd = &cobra.Command{
	Use:   "usage",
	Short: "Get API key usage stats",
	RunE: func(cmd *cobra.Command, args []string) error {
		return getJSON("/api/v2/apikey/developer/usage/get")
	},
}

var apikeyProfileCmd = &cobra.Command{
	Use:   "profile",
	Short: "Get API key profile",
	RunE: func(cmd *cobra.Command, args []string) error {
		return getJSON("/api/v2/apikey/developer/profile/get")
	},
}

func init() {
	rootCmd.AddCommand(apikeyCmd)
	apikeyCmd.AddCommand(apikeyUsageCmd)
	apikeyCmd.AddCommand(apikeyProfileCmd)
}
