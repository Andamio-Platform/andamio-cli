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
	EscrowHash           string             `json:"escrowHash"`
}

// Output structures
type OutputDatumConstructor struct {
	Constructor int           `json:"constructor"`
	Fields      []interface{} `json:"fields,omitempty"`
}

type OutputDatumField struct {
	Bytes string             `json:"bytes,omitempty"`
	Int   int64              `json:"int,omitempty"`
	List  []OutputDatumField `json:"list,omitempty"`
}

type OutputDatumConstructorList struct {
	List []OutputDatumConstructor `json:"list"`
}

type OutputDatum struct {
	Constructor int                      `json:"constructor"`
	Fields      []OutputDatumConstructor `json:"fields"`
}

func writeContractTokenDatum(inputFileName string) {
	inputFilePath := inputFileName
	ctInputName := inputFileName[:len(inputFileName)-5]
	datumOutputFilePath := ctInputName + "-contract-token-datum.json"
	redeemerOutputFilePath := ctInputName + "-manage-redeemer.json"

	// Read input JSON file
	InputContractTokenData := readInputJSON(inputFilePath)

	// Write Datum
	outputDatum := writeDataToContractTokenDatum(InputContractTokenData)

	// Write Redeemer
	outputRedeemer := writeDataToManageRedeemer(InputContractTokenData)

	// Write output JSON file
	writeOutputJSON(datumOutputFilePath, outputDatum)
	writeOutputJSON(redeemerOutputFilePath, outputRedeemer)

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

// Function to write Contract Token Datum
func writeDataToContractTokenDatum(input InputContractTokenData) OutputDatum {
	var projectList []OutputDatumConstructor

	for _, project := range input.Projects {
		projectHexTitle := hex.EncodeToString([]byte(project.Title))
		projectField := OutputDatumConstructor{
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

	OutputDatumFields := []OutputDatumConstructor{
		{
			Constructor: 0,
			Fields: []interface{}{
				OutputDatumConstructorList{List: projectList},
				OutputDatumField{List: policyList},
			},
		},
	}

	return OutputDatum{
		Constructor: 1,
		Fields:      OutputDatumFields,
	}
}

// Function to write Manage redeemer
func writeDataToManageRedeemer(input InputContractTokenData) OutputDatum {
	var projectList []OutputDatumConstructor

	for _, project := range input.Projects {
		projectHexTitle := hex.EncodeToString([]byte(project.Title))
		projectField := OutputDatumConstructor{
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

	OutputDatumFields := []OutputDatumConstructor{
		{
			Constructor: 0,
			Fields: []interface{}{
				OutputDatumConstructorList{List: projectList},
				OutputDatumField{List: policyList},
				OutputDatumField{Bytes: input.EscrowHash},
			},
		},
	}

	return OutputDatum{
		Constructor: 2,
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
	fmt.Println("Output written to", filename)

}
