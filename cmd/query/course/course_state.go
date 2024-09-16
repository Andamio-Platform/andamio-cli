package course

import (
	"fmt"
	"os"

	course_state "github.com/Andamio-Platform/andamio-cli/cmd/query/course/course-state"
	"github.com/spf13/cobra"
)

var CourseStateCmd = &cobra.Command{
	Use:   "course-state",
	Short: "",
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
	CourseStateCmd.AddCommand(course_state.CourseStateAddressCmd)
	CourseStateCmd.AddCommand(course_state.CourseStateUtxoCmd)
	CourseStateCmd.AddCommand(course_state.CourseStateUtxosCmd)
	CourseStateCmd.AddCommand(course_state.DecodedCourseStateDatumCmd)
}
