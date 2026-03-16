package main

import (
	"net/url"

	"github.com/spf13/cobra"
)

var txCmd = &cobra.Command{
	Use:   "tx",
	Short: "Transaction operations",
}

var txPendingCmd = &cobra.Command{
	Use:   "pending",
	Short: "List pending transactions",
	RunE: func(cmd *cobra.Command, args []string) error {
		return getJSON("/api/v2/tx/pending")
	},
}

var txTypesCmd = &cobra.Command{
	Use:   "types",
	Short: "List transaction types",
	RunE: func(cmd *cobra.Command, args []string) error {
		return getJSON("/api/v2/tx/types")
	},
}

var txStatusCmd = &cobra.Command{
	Use:   "status <tx-hash>",
	Short: "Get transaction status",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return getJSON("/api/v2/tx/status/" + url.PathEscape(args[0]))
	},
}

func init() {
	rootCmd.AddCommand(txCmd)
	txCmd.AddCommand(txPendingCmd)
	txCmd.AddCommand(txTypesCmd)
	txCmd.AddCommand(txStatusCmd)
}
