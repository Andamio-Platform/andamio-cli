package module_info

import (
	"fmt"

	"github.com/spf13/cobra"
)

var (
	inputFileName  string
	outputFileName string
)

var ModuleInfoCmd = &cobra.Command{
	Use:   "write-module-info",
	Short: "Write module info for use in module minting transaction",
	Long: `This is a helper function for 

andamio-cli transaction course-creator mint-module-tokens

The output of write-module-info can be used as --moduleInfos

The input is a markdown file structured like this:

# Course Title

101: Getting Started
101.1 I can set up a development environment.
101.2 I can compile a validator.

102: Using Transactions
102.1 I can build a transaction.
102.2 I can sign a transaction.
102.3 I can submit a transaction.

	`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("Writing contract token datum from %s...\n", inputFileName)
		writeModuleInfo(inputFileName, outputFileName)
	},
}

func init() {
	ModuleInfoCmd.Flags().StringVarP(&inputFileName, "inputFileName", "i", "input.md", "Name of input file")
	ModuleInfoCmd.Flags().StringVarP(&outputFileName, "outputFileName", "o", "output.json", "Name of output file")
}
