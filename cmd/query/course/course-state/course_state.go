package course_state

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var CourseStateCmd = &cobra.Command{
	Use:   "course-state",
	Short: "View course details",
	Long:  `.`,
	Run: func(cmd *cobra.Command, args []string) {
		// If no arguments are passed, show help
		if len(args) == 0 {
			cmd.Help()
			return
		}

		// If an invalid subcommand is passed, show an error message
		fmt.Printf("Error: '%s' is not a valid subcommand for 'course-state'\n", args[0])
		fmt.Println("Run './andamio-cli query course course-state --help' for available subcommands.")
		os.Exit(1) // Exit with a non-zero status to indicate an error
	},
}

func init() {
	CourseStateCmd.AddCommand(CourseStateAddressCmd)
	CourseStateCmd.AddCommand(CourseStateUtxoCmd)
	CourseStateCmd.AddCommand(CourseStateUtxosCmd)
	CourseStateCmd.AddCommand(DecodedCourseStateDatumCmd)
}
