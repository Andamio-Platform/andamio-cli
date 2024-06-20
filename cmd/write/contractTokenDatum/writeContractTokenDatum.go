package writeContractTokenDatum

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
)

// Input structures
type InputProjectData struct {
	Title      string `json:"title"`
	Expiration int64  `json:"expiration"`
	Ada        int    `json:"ada"`
	Gimbals    int    `json:"gimbals"`
}

type InputContractTokenData struct {
	ContributorPolicyIds []string           `json:"contributorPolicyIds"`
	Projects             []InputProjectData `json:"projects"`
}

// Output structures
type OutputDatumField struct {
	Constructor int                `json:"constructor,omitempty"`
	Fields      []interface{}      `json:"fields,omitempty"`
	Bytes       string             `json:"bytes,omitempty"`
	Int         int64              `json:"int,omitempty"`
	List        []OutputDatumField `json:"list,omitempty"`
}

type OutputDatum struct {
	Constructor int                `json:"constructor"`
	Fields      []OutputDatumField `json:"fields"`
}

func writeContractTokenDatum(inputFileName string) {
	inputFilePath := inputFileName
	ctInputName := inputFileName[:len(inputFileName)-5]
	outputFilePath := ctInputName + "-contract-token-datum.json"

	// Read input JSON file
	InputContractTokenData := readInputJSON(inputFilePath)

	// Transform the data
	outputDatum := transformData(InputContractTokenData)

	// Write output JSON file
	writeOutputJSON(outputFilePath, outputDatum)

	fmt.Println("Data transformation complete. Output written to", outputFilePath)
}

// Function to read input JSON file
func readInputJSON(filename string) InputContractTokenData {
	file, err := os.Open(filename)
	if err != nil {
		log.Fatalf("Failed to open input file: %v", err)
	}
	defer file.Close()

	byteValue, err := io.ReadAll(file)
	if err != nil {
		log.Fatalf("Failed to read input file: %v", err)
	}

	var InputContractTokenData InputContractTokenData
	if err := json.Unmarshal(byteValue, &InputContractTokenData); err != nil {
		log.Fatalf("Failed to unmarshal input JSON: %v", err)
	}

	return InputContractTokenData
}

// Function to transform the input data
func transformData(input InputContractTokenData) OutputDatum {
	var projectList []OutputDatumField

	for _, project := range input.Projects {
		projectHexTitle := hex.EncodeToString([]byte(project.Title))
		projectField := OutputDatumField{
			Constructor: 0,
			Fields: []interface{}{
				OutputDatumField{Bytes: projectHexTitle},
				OutputDatumField{Int: project.Expiration},
				OutputDatumField{Int: int64(project.Ada) * 1000000},
				OutputDatumField{Int: int64(project.Gimbals) * 1000000},
			},
		}
		projectList = append(projectList, projectField)
	}

	policyList := []OutputDatumField{}
	for _, policy := range input.ContributorPolicyIds {
		policyField := OutputDatumField{Bytes: policy}
		policyList = append(policyList, policyField)
	}

	OutputDatumFields := []OutputDatumField{
		{
			Constructor: 0,
			Fields: []interface{}{
				OutputDatumField{List: projectList},
				OutputDatumField{List: policyList},
			},
		},
	}

	return OutputDatum{
		Constructor: 1,
		Fields:      OutputDatumFields,
	}
}

// Function to write output JSON file
func writeOutputJSON(filename string, outputDatum OutputDatum) {
	byteValue, err := json.MarshalIndent(outputDatum, "", "  ")
	if err != nil {
		log.Fatalf("Failed to marshal output JSON: %v", err)
	}

	if err := os.WriteFile(filename, byteValue, 0644); err != nil {
		log.Fatalf("Failed to write output file: %v", err)
	}
}
