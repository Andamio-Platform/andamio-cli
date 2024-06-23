/*
Copyright © 2024 NAME HERE <EMAIL ADDRESS>
*/
package nftMetadata

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
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

// Helper functions

func splitStringForMetadata(input string) []string {
	const chunkSize = 56
	var result []string

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

// nftMetadataCmd represents the writeMetadata command
var NftMetadataCmd = &cobra.Command{
	Use:   "nft-metadata",
	Short: "Write NFT metadata",
	Long: `
Utility for writing simple NFT Metadata that adheres to CIP-25 standard.

andamio-cli write nft-medata
	--policyid
	--asset-name
	--name
	--image
	--media-type
	--description
	--out-file

	`,
	Run: func(cmd *cobra.Command, args []string) {
		policyID, _ := cmd.Flags().GetString("policyid")
		assetName, _ := cmd.Flags().GetString("asset-name")
		name, _ := cmd.Flags().GetString("name")
		imageRaw, _ := cmd.Flags().GetString("image")
		mediaType, _ := cmd.Flags().GetString("media-type")
		descriptionRaw, _ := cmd.Flags().GetString("description")
		outFile, _ := cmd.Flags().GetString("out-file")

		image := splitStringForMetadata(imageRaw)
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
					assetName: asset,
				},
			},
		}

		jsonData, err := json.MarshalIndent(metadata, "", "  ")
		if err != nil {
			fmt.Println("Error marshalling JSON:", err)
			return
		}

		if err := os.WriteFile(outFile, jsonData, 0644); err != nil {
			fmt.Println("Error writing file:", err)
			return
		}

		fmt.Println(outFile, "has been created successfully.")
	},
}

func init() {
	// rootCmd.AddCommand(writeMetadataCmd)

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// writeMetadataCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// writeMetadataCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
	NftMetadataCmd.Flags().String("policyid", "", "The policy ID")
	NftMetadataCmd.Flags().String("asset-name", "", "The asset name")
	NftMetadataCmd.Flags().String("name", "", "The name of the asset")
	NftMetadataCmd.Flags().String("image", "", "The image URL")
	NftMetadataCmd.Flags().String("media-type", "", "The media type")
	NftMetadataCmd.Flags().String("description", "", "The description")
	NftMetadataCmd.Flags().String("out-file", "", "Output file")

	NftMetadataCmd.MarkFlagRequired("policyid")
	NftMetadataCmd.MarkFlagRequired("asset-name")
	NftMetadataCmd.MarkFlagRequired("name")
	NftMetadataCmd.MarkFlagRequired("image")
	NftMetadataCmd.MarkFlagRequired("media-type")
	NftMetadataCmd.MarkFlagRequired("description")
	NftMetadataCmd.MarkFlagRequired("out-file")
}
