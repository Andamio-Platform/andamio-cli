package network

import (
	"fmt"
	"os"

	instance_validator "github.com/Andamio-Platform/andamio-cli/cmd/query/network/instance-validator"
	"github.com/spf13/cobra"
)

var InstanceValidatorCmd = &cobra.Command{
	Use:   "instance-validator",
	Short: "change this",
	Long:  `change this.`,
	Run: func(cmd *cobra.Command, args []string) {

		// If no arguments are passed, show help
		if len(args) == 0 {
			cmd.Help()
			return
		}

		// If an invalid subcommand is passed, show an error message
		fmt.Printf("Error: '%s' is not a valid subcommand for 'network'\n", args[0])
		fmt.Println("Run './andamio-cli query network --help' for available subcommands.")
		os.Exit(1) // Exit with a non-zero status to indicate an error
	},
}

func init() {
	InstanceValidatorCmd.AddCommand(instance_validator.AllInstanceValidatorUtxosCmd)
	InstanceValidatorCmd.AddCommand(instance_validator.AllCourseInstanceUtxosCmd)
	InstanceValidatorCmd.AddCommand(instance_validator.AssignmentValidatorRefUtxoCmd)
	InstanceValidatorCmd.AddCommand(instance_validator.CourseInstanceUtxoCmd)
	InstanceValidatorCmd.AddCommand(instance_validator.DecodedCourseInstanceDatumCmd)
	InstanceValidatorCmd.AddCommand(instance_validator.LocalStatePolicyRefUtxoCmd)
	InstanceValidatorCmd.AddCommand(instance_validator.LocalStateValidatorRefUtxoCmd)
	InstanceValidatorCmd.AddCommand(instance_validator.ModuleTokenPolicyRefUtxoCmd)
	InstanceValidatorCmd.AddCommand(instance_validator.ModuleValidatorRefUtxoCmd)
}
