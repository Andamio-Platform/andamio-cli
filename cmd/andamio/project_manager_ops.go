package main

import "github.com/spf13/cobra"

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
	return printListPost(
		cmd.Context(),
		"/api/v2/project/manager/commitments/list",
		map[string]string{"project_id": projectID},
		"No pending assessments found.",
		"content.title", "commitment_id",
	)
}
