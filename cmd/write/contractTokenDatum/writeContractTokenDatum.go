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
type InputProject struct {
	Title      string `json:"title"`
	Expiration int64  `json:"expiration"`
	Ada        int    `json:"ada"`
	Gimbals    int    `json:"gimbals"`
}

type InputData struct {
	ContributorPolicyIds []string       `json:"contributorPolicyIds"`
	Projects             []InputProject `json:"projects"`
}

// Output structures
type OutputField struct {
	Constructor int           `json:"constructor"`
	Fields      []interface{} `json:"fields,omitempty"`
	Bytes       string        `json:"bytes,omitempty"`
	Int         int64         `json:"int,omitempty"`
	List        []OutputField `json:"list,omitempty"`
}

type OutputData struct {
	Constructor int           `json:"constructor"`
	Fields      []OutputField `json:"fields"`
}

func writeContractTokenDatum() {
	inputFilePath := "input.json"
	outputFilePath := "output.json"

	// Read input JSON file
	inputData := readInputJSON(inputFilePath)

	// Transform the data
	outputData := transformData(inputData)

	// Write output JSON file
	writeOutputJSON(outputFilePath, outputData)

	fmt.Println("Data transformation complete. Output written to", outputFilePath)
}

// Function to read input JSON file
func readInputJSON(filename string) InputData {
	file, err := os.Open(filename)
	if err != nil {
		log.Fatalf("Failed to open input file: %v", err)
	}
	defer file.Close()

	byteValue, err := io.ReadAll(file)
	if err != nil {
		log.Fatalf("Failed to read input file: %v", err)
	}

	var inputData InputData
	if err := json.Unmarshal(byteValue, &inputData); err != nil {
		log.Fatalf("Failed to unmarshal input JSON: %v", err)
	}

	return inputData
}

// Function to transform the input data
func transformData(input InputData) OutputData {
	var projectList []OutputField

	for _, project := range input.Projects {
		projectHexTitle := hex.EncodeToString([]byte(project.Title))
		projectField := OutputField{
			Constructor: 0,
			Fields: []interface{}{
				OutputField{Bytes: projectHexTitle},
				OutputField{Int: project.Expiration},
				OutputField{Int: int64(project.Ada) * 1000000},
				OutputField{Int: int64(project.Gimbals) * 1000000},
			},
		}
		projectList = append(projectList, projectField)
	}

	policyList := []OutputField{}
	for _, policy := range input.ContributorPolicyIds {
		policyField := OutputField{Bytes: policy}
		policyList = append(policyList, policyField)
	}

	outputFields := []OutputField{
		{
			Constructor: 0,
			Fields: []interface{}{
				OutputField{List: projectList},
				OutputField{List: policyList},
			},
		},
	}

	return OutputData{
		Constructor: 1,
		Fields:      outputFields,
	}
}

// Function to write output JSON file
func writeOutputJSON(filename string, outputData OutputData) {
	byteValue, err := json.MarshalIndent(outputData, "", "  ")
	if err != nil {
		log.Fatalf("Failed to marshal output JSON: %v", err)
	}

	if err := os.WriteFile(filename, byteValue, 0644); err != nil {
		log.Fatalf("Failed to write output file: %v", err)
	}
}
