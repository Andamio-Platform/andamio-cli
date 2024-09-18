package module_info

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

type Slt struct {
	SltId      string `json:"sltId"`
	SltContent string `json:"sltContent"`
}

type Module struct {
	ModuleId          string `json:"moduleId"`
	Slts              []Slt  `json:"slts"`
	AssignmentContent string `json:"assignmentContent"`
}

func writeModuleInfo(markdownFilePath string, jsonFilePath string) error {
	file, err := os.Open(markdownFilePath)
	if err != nil {
		return fmt.Errorf("error opening markdown file: %v", err)
	}
	defer file.Close()

	var modules []Module
	var currentModule *Module

	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		if strings.HasPrefix(line, "#") {
			// Skip the title line
			continue
		}

		if strings.Contains(line, ":") {
			// This line is a module header (e.g., "101: Getting Started")
			parts := strings.SplitN(line, ":", 2)
			moduleId := strings.TrimSpace(parts[0])
			currentModule = &Module{
				ModuleId:          moduleId,
				Slts:              []Slt{},
				AssignmentContent: moduleId,
			}
			modules = append(modules, *currentModule)
		} else if currentModule != nil && strings.HasPrefix(line, currentModule.ModuleId) {
			// This line is an SLT (e.g., "101.1 I can set up a development environment.")
			parts := strings.SplitN(line, " ", 2)
			sltId := strings.Split(parts[0], ".")[1]
			sltContent := strings.TrimSpace(parts[1])
			slt := Slt{
				SltId:      sltId,
				SltContent: sltContent,
			}
			// Update the slts array in the last module
			modules[len(modules)-1].Slts = append(modules[len(modules)-1].Slts, slt)
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("error reading markdown file: %v", err)
	}

	// Convert the modules to JSON
	jsonData, err := json.MarshalIndent(modules, "", "  ")
	if err != nil {
		return fmt.Errorf("error marshaling to JSON: %v", err)
	}

	// Write JSON data to file
	err = os.WriteFile(jsonFilePath, jsonData, 0644)
	if err != nil {
		return fmt.Errorf("error writing JSON file: %v", err)
	}

	return nil
}

// func writeModuleInfo() {
// 	// Example usage
// 	markdownFilePath := "input.md"
// 	jsonFilePath := "output.json"
//
// 	err := parseMarkdownToJSON(markdownFilePath, jsonFilePath)
// 	if err != nil {
// 		fmt.Printf("Error: %v\n", err)
// 	} else {
// 		fmt.Println("JSON file created successfully!")
// 	}
// }
