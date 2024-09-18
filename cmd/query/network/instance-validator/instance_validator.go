package instance_validator

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var InstanceValidatorCmd = &cobra.Command{
	Use:   "instance-validator",
	Short: "View course instance details",
	Long:  ``,
	Run: func(cmd *cobra.Command, args []string) {
		// If no arguments are passed, show help
		if len(args) == 0 {
			cmd.Help()
			return
		}

		// If an invalid subcommand is passed, show an error message
		fmt.Printf("Error: '%s' is not a valid subcommand for 'instance-validator'\n", args[0])
		fmt.Println("Run './andamio-cli query network instance-validator --help' for available subcommands.")
		os.Exit(1) // Exit with a non-zero status to indicate an error
	},
}

func init() {
	InstanceValidatorCmd.AddCommand(AllInstanceValidatorUtxosCmd)
	InstanceValidatorCmd.AddCommand(AllCourseInstanceUtxosCmd)
	InstanceValidatorCmd.AddCommand(AssignmentValidatorRefUtxoCmd)
	InstanceValidatorCmd.AddCommand(CourseInstanceUtxoCmd)
	InstanceValidatorCmd.AddCommand(DecodedCourseInstanceDatumCmd)
	InstanceValidatorCmd.AddCommand(LocalStatePolicyRefUtxoCmd)
	InstanceValidatorCmd.AddCommand(LocalStateValidatorRefUtxoCmd)
	InstanceValidatorCmd.AddCommand(ModuleTokenPolicyRefUtxoCmd)
	InstanceValidatorCmd.AddCommand(ModuleValidatorRefUtxoCmd)
}
