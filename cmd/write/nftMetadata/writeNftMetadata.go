package nftMetadata

import (
	"bufio"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// Define some types:

type Asset struct {
	Name        string   `json:"name"`
	Image       []string `json:"image"`
	MediaType   string   `json:"mediaType"`
	Description []string `json:"description"`
	Files       []File   `json:"files"`
}

type File struct {
	MediaType string   `json:"mediaType"`
	Src       []string `json:"src"`
}

type Metadata struct {
	PolicyID map[string]map[string]Asset `json:"721"`
}

// Helper functions - we'll come back to these in a minute:

func stringToHexString(s string) string {
	return hex.EncodeToString([]byte(s))
}

func splitStringForMetadata(input string) []string {
	const chunkSize = 56
	var result []string

	// Calculate the number of chunks needed.
	numChunks := len(input) / chunkSize
	if len(input)%chunkSize != 0 {
		numChunks++
	}

	for i := 0; i < numChunks; i++ {
		start := i * chunkSize
		end := start + chunkSize
		if end > len(input) {
			end = len(input)
		}
		result = append(result, input[start:end])
	}

	return result
}

// Take input in the terminal
// Write a metadata.json file
// How to improve this:
// 1. Take parameters so we can call writeNftFile() from other functions
// 2. Create NFT metadata with multiple assets of the same policy_id

func writeNftFile() {
	reader := bufio.NewReader(os.Stdin)

	fmt.Print("Enter policy_id: ")
	policyID, _ := reader.ReadString('\n')
	policyID = strings.TrimSpace(policyID)

	fmt.Print("Enter asset_name: ")
	assetName, _ := reader.ReadString('\n')
	assetName = strings.TrimSpace(assetName)
	assetHex := stringToHexString(assetName)

	fmt.Print("Enter name: ")
	name, _ := reader.ReadString('\n')
	name = strings.TrimSpace(name)

	fmt.Print("Enter image URL: ")
	imageRaw, _ := reader.ReadString('\n')
	imageRaw = strings.TrimSpace(imageRaw)
	image := splitStringForMetadata(imageRaw)

	fmt.Print("Enter mediaType: ")
	mediaType, _ := reader.ReadString('\n')
	mediaType = strings.TrimSpace(mediaType)

	fmt.Print("Enter description: ")
	descriptionRaw, _ := reader.ReadString('\n')
	descriptionRaw = strings.TrimSpace(descriptionRaw)
	description := splitStringForMetadata(descriptionRaw)

	asset := Asset{
		Name:        name,
		Image:       image,
		MediaType:   mediaType,
		Description: description,
		Files: []File{
			{
				MediaType: mediaType,
				Src:       image,
			},
		},
	}

	metadata := Metadata{
		PolicyID: map[string]map[string]Asset{
			policyID: {
				assetHex: asset,
			},
		},
	}

	jsonData, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		fmt.Println("Error marshalling JSON:", err)
		return
	}

	if err := os.WriteFile("metadata.json", jsonData, 0644); err != nil {
		fmt.Println("Error writing file:", err)
		return
	}

	fmt.Println("metadata.json has been created successfully.")
}
