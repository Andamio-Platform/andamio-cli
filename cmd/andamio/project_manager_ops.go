package main

import (
	"fmt"
	"os"

	"github.com/Andamio-Platform/andamio-cli/internal/client"
	"github.com/Andamio-Platform/andamio-cli/internal/config"
	"github.com/Andamio-Platform/andamio-cli/internal/output"
	"github.com/spf13/cobra"
)

// projectManagerCmd is the nested "project manager" subgroup.
// The existing top-level "manager" command stays as-is.
var projectManagerCmd = &cobra.Command{
	Use:               "manager",
	Short:             "Project manager operations (requires user login)",
	PersistentPreRunE: jwtAuthPreRunE,
}

var projectManagerCommitmentsCmd = &cobra.Command{
	Use:   "commitments",
	Short: "List pending task assessments",
	Long: `List task commitments awaiting assessment for a project.

Find your project IDs with: andamio project list --output json

Examples:
  andamio project manager commitments --project-id <id>`,
	RunE: runProjectManagerCommitments,
}

func init() {
	projectCmd.AddCommand(projectManagerCmd)
	projectManagerCmd.AddCommand(projectManagerCommitmentsCmd)

	projectManagerCommitmentsCmd.Flags().String("project-id", "", "Project ID (required)")
	projectManagerCommitmentsCmd.MarkFlagRequired("project-id")
}

func runProjectManagerCommitments(cmd *cobra.Command, args []string) error {
	projectID, _ := cmd.Flags().GetString("project-id")
	isJSON := output.GetFormat() == output.FormatJSON

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	c := client.New(cfg)
	payload := map[string]string{"project_id": projectID}
	var resp map[string]interface{}
	if err := c.Post("/api/v2/project/manager/commitments/list", payload, &resp); err != nil {
		return fmt.Errorf("failed to list commitments: %w", err)
	}

	if isJSON {
		return output.PrintJSON(resp)
	}

	data, ok := resp["data"].([]interface{})
	if !ok || len(data) == 0 {
		fmt.Fprintln(os.Stderr, "No pending assessments found.")
		return nil
	}

	items := make([]map[string]interface{}, 0, len(data))
	for _, item := range data {
		if m, ok := item.(map[string]interface{}); ok {
			items = append(items, m)
		}
	}

	return output.PrintList(items, "content.title", "commitment_id")
}
