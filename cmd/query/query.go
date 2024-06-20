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
	Short: "A brief description of your command",
	Long: `A longer description that spans multiple lines and likely contains examples
and usage of using your command. For example:

Cobra is a CLI library for Go that empowers applications.
This application is a tool to generate the needed files
to quickly create a Cobra application.`,
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
