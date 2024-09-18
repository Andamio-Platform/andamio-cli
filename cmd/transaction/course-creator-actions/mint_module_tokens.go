package course_creator_actions

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"regexp"

	"github.com/Andamio-Platform/andamio-cli/internal/client"
	"github.com/spf13/cobra"
)

var (
	moduleInfos     string
	moduleInfosFile string
)

var MintModuleTokensCmd = &cobra.Command{
	Use:   "mint-module-tokens",
	Short: "Publish course credential criteria on-chain",
	Long: `
About:
Before a student can commit to an assignment, the course creator must publish credential criteria on-chain.

This transaction mints course module tokens specifying Student Learning Targets (SLTs) and an assignment for each course module.

The transaction must be signed by the holder of userAccessToken.

  `,
	Run: func(cmd *cobra.Command, args []string) {
		if moduleInfosFile != "" {
			var err error
			moduleInfos, err = jsonFileToString(moduleInfosFile)
			if err != nil {
				fmt.Printf("Error reading JSON from file: %v\n", err)
				os.Exit(1) // Exit with error if the file reading fails
			}
			fmt.Println("Using json file input")
			fmt.Println(moduleInfos)
		}
		client.GetMintModuleTokens(userAccessToken, policy, moduleInfos)
	},
}

func init() {
	MintModuleTokensCmd.Flags().StringVar(&userAccessToken, "userAccessToken", "", "Cardano Asset ID of teacher access token. The wallet holding this asset must sign the generated transaction.")
	MintModuleTokensCmd.Flags().StringVar(&policy, "policy", "", "Course NFT policy id")
	MintModuleTokensCmd.Flags().StringVar(&moduleInfos, "moduleInfos", "", "List of course module information. Use andamio-cli write module-info to generate valid module-info")
	MintModuleTokensCmd.Flags().StringVar(&moduleInfosFile, "moduleInfosFile", "", "Path to JSON file with valid Module Info")

	// Required Flags
	MintModuleTokensCmd.MarkFlagRequired("userAccessToken")
	MintModuleTokensCmd.MarkFlagRequired("policy")
}

type Slt struct {
	SltId      string `json:"sltId"`
	SltContent string `json:"sltContent"`
}

type Module struct {
	ModuleId          string `json:"moduleId"`
	Slts              []Slt  `json:"slts"`
	AssignmentContent string `json:"assignmentContent"`
}

func jsonFileToString(jsonFilePath string) (string, error) {
	// Open the JSON file
	file, err := os.Open(jsonFilePath)
	if err != nil {
		return "", fmt.Errorf("error opening JSON file: %v", err)
	}
	defer file.Close()

	// Read the entire file into a byte slice
	jsonData, err := io.ReadAll(file)
	if err != nil {
		return "", fmt.Errorf("error reading JSON file: %v", err)
	}

	// Unmarshal the byte slice into a slice of Module structs to maintain order
	var modules []Module
	err = json.Unmarshal(jsonData, &modules)
	if err != nil {
		return "", fmt.Errorf("error unmarshaling JSON data: %v", err)
	}

	// Marshal the data back into a compact JSON string (no pretty printing or line breaks)
	jsonString, err := json.Marshal(modules)
	if err != nil {
		return "", fmt.Errorf("error marshaling JSON to string: %v", err)
	}

	// Convert the JSON string to a format without quotation marks around keys
	// Use regular expressions to remove the quotes from the keys
	re := regexp.MustCompile(`\"([a-zA-Z0-9_]+)\":`)
	result := re.ReplaceAllString(string(jsonString), `$1:`)

	return result, nil
}
