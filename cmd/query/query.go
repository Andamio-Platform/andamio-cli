/*
Copyright © 2024 NAME HERE <EMAIL ADDRESS>
*/
package query

import (
	"fmt"

	courseInstances "github.com/Andamio-Platform/andamio-cli/cmd/query/course-instances"
	globalState "github.com/Andamio-Platform/andamio-cli/cmd/query/global-state"
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
developers instant access to helpful queries that allow users to gain 
insights about the network. In this phase of Andamio CLI development, we 
will focus on making it easy for anyone to make useful queries to the 
Andamio network.

The queries provided here serve two purposes:
 1. Protoyping for production release of Andamio CLI
 2. Examples for Cardano Go Project-Based Learning Course

`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("query called")
	},
}

func addQuerySubcommandIslands() {
	// WriteCmd.AddCommand(playground.PlaygroundCmd)
	QueryCmd.AddCommand(tip.TipCmd)
	QueryCmd.AddCommand(globalState.GlobalStateCmd)
	QueryCmd.AddCommand(courseInstances.CourseInstanceCmd)
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
