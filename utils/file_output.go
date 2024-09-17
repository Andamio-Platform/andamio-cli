package utils

import (
	"encoding/json"
	"fmt"
	"log"
	"os"

	"github.com/spf13/cobra"
)

// SaveOutputToFile marshals the response to JSON and writes it to a file if --out-file is provided.
func SaveOutputToFile(cmd *cobra.Command, response interface{}) {
	// Get the value of the --out-file flag
	outFile, _ := cmd.Flags().GetString("out-file")

	// If the --out-file flag is provided, write the response to the file as JSON
	if outFile != "" {
		// Marshal the response into JSON format
		jsonData, err := json.MarshalIndent(response, "", "  ") // Pretty print with indent
		if err != nil {
			log.Fatalf("Failed to marshal response to JSON: %v", err)
		}

		// Write the JSON data to the specified file
		err = os.WriteFile(outFile, jsonData, 0644)
		if err != nil {
			log.Fatalf("Failed to write response to file: %v", err)
		}
		fmt.Printf("Response saved as JSON to file: %s\n", outFile)
	} else {
		// If no --out-file is provided, print the response to stdout
		fmt.Println(response)
	}
}

// AddOutFileFlag adds the --out-file flag to a command
func AddOutFileFlag(cmd *cobra.Command) {
	cmd.Flags().String("out-file", "", "Optional: specify a JSON file to save the response")
}
