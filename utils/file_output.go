package utils

import (
	"encoding/json"
	"fmt"
	"log"
	"os"

	"github.com/spf13/cobra"
)

// SaveOutputToFile marshals the response to JSON and writes it to a file if --out-file is provided
func SaveOutputToFile(cmd *cobra.Command, response interface{}) {
	// Get the value of the --out-file flag
	outFile, _ := cmd.Flags().GetString("out-file")

	// Marshal the response into formatted JSON (pretty print with indent)
	jsonData, err := json.MarshalIndent(response, "", "  ")
	if err != nil {
		log.Fatalf("Failed to marshal response to JSON: %v", err)
	}

	// If the --out-file flag is provided, write the response to the file
	if outFile != "" {
		err = os.WriteFile(outFile, jsonData, 0644)
		if err != nil {
			log.Fatalf("Failed to write response to file: %v", err)
		}
		fmt.Printf("Response saved as JSON to file: %s\n", outFile)
	} else {
		// If no --out-file is provided, print the formatted JSON to stdout
		fmt.Println(string(jsonData))
	}
}

// AddOutFileFlag adds the --out-file flag to a command
func AddOutFileFlag(cmd *cobra.Command) {
	cmd.Flags().String("out-file", "", "Optional: specify a JSON file to save the response")
}
