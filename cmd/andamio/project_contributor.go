package main

import (
	"fmt"
	"os"

	"github.com/Andamio-Platform/andamio-cli/internal/client"
	"github.com/Andamio-Platform/andamio-cli/internal/config"
	"github.com/Andamio-Platform/andamio-cli/internal/output"
	"github.com/spf13/cobra"
)

var projectContributorCmd = &cobra.Command{
	Use:               "contributor",
	Short:             "Project contributor operations (requires user login)",
	PersistentPreRunE: jwtAuthPreRunE,
}

var projectContributorListCmd = &cobra.Command{
	Use:   "list",
	Short: "List projects you contribute to",
	RunE: func(cmd *cobra.Command, args []string) error {
		return printList(
			"/api/v2/project/contributor/projects/list",
			"No contributor projects found.",
			"content.title", "project_id", true,
		)
	},
}

var projectContributorCommitmentsCmd = &cobra.Command{
	Use:   "commitments",
	Short: "List your task commitments",
	RunE: func(cmd *cobra.Command, args []string) error {
		return printList(
			"/api/v2/project/contributor/commitments/list",
			"No commitments found.",
			"content.title", "commitment_id", true,
		)
	},
}

var projectContributorCommitmentCmd = &cobra.Command{
	Use:   "commitment",
	Short: "Get a specific task commitment",
	Long: `Get details for a specific task commitment.

Examples:
  andamio project contributor commitment --project-id <id> --task-index 3`,
	RunE: runProjectContributorCommitment,
}

var projectContributorCommitCmd = &cobra.Command{
	Use:   "commit",
	Short: "Commit to a task",
	Long: `Create a new commitment to a project task.

Examples:
  andamio project contributor commit --project-id <id> --task-index 3`,
	RunE: runTaskHashAction("/api/v2/project/contributor/commitment/create", "Committing to task"),
}

var projectContributorUpdateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update task commitment evidence",
	Long: `Update the evidence for your task commitment.

Examples:
  andamio project contributor update --project-id <id> --task-index 3 --evidence "https://github.com/..."`,
	RunE: runProjectContributorUpdate,
}

var projectContributorDeleteCmd = &cobra.Command{
	Use:   "delete",
	Short: "Delete a task commitment",
	Long: `Withdraw your commitment to a project task.

Examples:
  andamio project contributor delete --project-id <id> --task-index 3`,
	RunE: runTaskHashAction("/api/v2/project/contributor/commitment/delete", "Deleting commitment"),
}

func init() {
	projectCmd.AddCommand(projectContributorCmd)
	projectContributorCmd.AddCommand(projectContributorListCmd)
	projectContributorCmd.AddCommand(projectContributorCommitmentsCmd)
	projectContributorCmd.AddCommand(projectContributorCommitmentCmd)
	projectContributorCmd.AddCommand(projectContributorCommitCmd)
	projectContributorCmd.AddCommand(projectContributorUpdateCmd)
	projectContributorCmd.AddCommand(projectContributorDeleteCmd)

	// Shared flags for task-specific commands
	for _, cmd := range []*cobra.Command{
		projectContributorCommitmentCmd,
		projectContributorCommitCmd,
		projectContributorDeleteCmd,
	} {
		cmd.Flags().String("project-id", "", "Project ID (required)")
		cmd.MarkFlagRequired("project-id")
		cmd.Flags().String("task-index", "", "Task index (use --task-hash for chain-only tasks)")
		cmd.Flags().String("task-hash", "", "Task hash (use instead of --task-index for chain-only tasks)")
	}

	// Update flags (add --evidence / --evidence-file + --task-hash)
	projectContributorUpdateCmd.Flags().String("project-id", "", "Project ID (required)")
	projectContributorUpdateCmd.MarkFlagRequired("project-id")
	projectContributorUpdateCmd.Flags().String("task-index", "", "Task index (use --task-hash for chain-only tasks)")
	projectContributorUpdateCmd.Flags().String("task-hash", "", "Task hash (use instead of --task-index for chain-only tasks)")
	projectContributorUpdateCmd.Flags().String("evidence", "", "Evidence text or URL (Markdown supported)")
	projectContributorUpdateCmd.Flags().String("evidence-file", "", "Path to evidence file (Markdown)")
}

// loadClientAndResolveTask loads config, creates a client, and resolves task_hash
// from --task-index or --task-hash flags.
func loadClientAndResolveTask(cmd *cobra.Command) (*client.Client, string, int, error) {
	projectID, _ := cmd.Flags().GetString("project-id")

	cfg, err := config.Load()
	if err != nil {
		return nil, "", 0, err
	}

	c := client.New(cfg)
	taskHash, taskIndex, err := resolveTaskHashFromFlags(cmd, c, projectID)
	if err != nil {
		return nil, "", 0, err
	}
	return c, taskHash, taskIndex, nil
}

func runProjectContributorCommitment(cmd *cobra.Command, args []string) error {
	projectID, _ := cmd.Flags().GetString("project-id")

	c, taskHash, _, err := loadClientAndResolveTask(cmd)
	if err != nil {
		return err
	}

	payload := map[string]string{
		"project_id": projectID,
		"task_hash":  taskHash,
	}
	var resp map[string]interface{}
	if err := c.Post("/api/v2/project/contributor/commitment/get", payload, &resp); err != nil {
		return fmt.Errorf("failed to get commitment: %w", err)
	}

	return output.PrintJSON(resp)
}

// runTaskHashAction returns a RunE for commands that resolve task_hash and POST with it.
func runTaskHashAction(endpoint, verb string) func(cmd *cobra.Command, args []string) error {
	return func(cmd *cobra.Command, args []string) error {
		isJSON := output.GetFormat() == output.FormatJSON

		c, taskHash, taskIndex, err := loadClientAndResolveTask(cmd)
		if err != nil {
			return err
		}

		if !isJSON {
			if taskIndex >= 0 {
				fmt.Fprintf(os.Stderr, "%s %d...\n", verb, taskIndex)
			} else {
				fmt.Fprintf(os.Stderr, "%s %s...\n", verb, taskHash[:16]+"...")
			}
		}

		payload := map[string]interface{}{
			"task_hash": taskHash,
		}

		var resp map[string]interface{}
		if err := c.Post(endpoint, payload, &resp); err != nil {
			return fmt.Errorf("failed: %w", err)
		}

		if isJSON {
			return output.PrintJSON(resp)
		}

		fmt.Fprintf(os.Stderr, "Done.\n")
		return nil
	}
}

// runProjectContributorUpdate sends evidence as Tiptap JSON with a Blake2b-256 content hash.
func runProjectContributorUpdate(cmd *cobra.Command, args []string) error {
	isJSON := output.GetFormat() == output.FormatJSON

	evidence, err := readEvidenceFlag(cmd)
	if err != nil {
		return err
	}

	tiptapDoc, evidenceHash, err := wrapEvidence(evidence)
	if err != nil {
		return fmt.Errorf("failed to format evidence: %w", err)
	}

	c, taskHash, taskIndex, err := loadClientAndResolveTask(cmd)
	if err != nil {
		return err
	}

	if !isJSON {
		if taskIndex >= 0 {
			fmt.Fprintf(os.Stderr, "Updating commitment evidence for task %d...\n", taskIndex)
		} else {
			fmt.Fprintf(os.Stderr, "Updating commitment evidence for task %s...\n", taskHash[:16]+"...")
		}
	}

	payload := map[string]interface{}{
		"task_hash":     taskHash,
		"evidence":      tiptapDoc,
		"evidence_hash": evidenceHash,
	}

	var resp map[string]interface{}
	if err := c.Post("/api/v2/project/contributor/commitment/update", payload, &resp); err != nil {
		return fmt.Errorf("failed to update commitment: %w", err)
	}

	if isJSON {
		return output.PrintJSON(resp)
	}

	fmt.Fprintf(os.Stderr, "Commitment updated.\n")
	return nil
}


