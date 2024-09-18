/*
Copyright © 2024 NAME HERE <EMAIL ADDRESS>
*/
package query

import (
	"github.com/Andamio-Platform/andamio-cli/cmd/query/course"
	courseInstances "github.com/Andamio-Platform/andamio-cli/cmd/query/course-instances"
	globalState "github.com/Andamio-Platform/andamio-cli/cmd/query/global-state"
	"github.com/Andamio-Platform/andamio-cli/cmd/query/network"
	"github.com/Andamio-Platform/andamio-cli/cmd/query/tip"
	"github.com/spf13/cobra"
)

// queryCmd represents the query command
var QueryCmd = &cobra.Command{
	Use:   "query",
	Short: "Andamio Network queries",
	Long: `
The Andamio Network is home to valuable, public data that becomes even 
more valuable when you have tools to make sense of it. Andamio CLI gives 
developers instant access to queries and transactions, making it easier 
to build new tools on Andamio.

The queries provided here serve two purposes:
 1. Opening access to Andamio network data
 2. Providing examples for Cardano Go Project-Based Learning Course

`,
	Run: func(cmd *cobra.Command, args []string) {
		if len(args) == 0 {
			cmd.Help()
			return
		}
	},
}

func addQuerySubcommandIslands() {
	// WriteCmd.AddCommand(playground.PlaygroundCmd)
	QueryCmd.AddCommand(tip.TipCmd)
	QueryCmd.AddCommand(globalState.GlobalStateCmd)
	QueryCmd.AddCommand(courseInstances.CourseInstanceCmd)
	QueryCmd.AddCommand(course.CourseCmd)
	QueryCmd.AddCommand(network.NetworkCmd)
}

func init() {
	addQuerySubcommandIslands()

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// queryCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// queryCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}
