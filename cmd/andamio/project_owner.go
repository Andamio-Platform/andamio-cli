package main

import (
	"fmt"
	"os"

	"github.com/Andamio-Platform/andamio-cli/internal/client"
	"github.com/Andamio-Platform/andamio-cli/internal/config"
	"github.com/Andamio-Platform/andamio-cli/internal/output"
	"github.com/spf13/cobra"
)

var projectOwnerCmd = &cobra.Command{
	Use:               "owner",
	Short:             "Project owner operations (requires user login)",
	PersistentPreRunE: jwtAuthPreRunE,
}

var projectOwnerListCmd = &cobra.Command{
	Use:   "list",
	Short: "List projects you own",
	RunE: func(cmd *cobra.Command, args []string) error {
		return printList(
			"/api/v2/project/owner/projects/list",
			"No owned projects found.",
			"content.title", "project_id", true,
		)
	},
}

var projectOwnerCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new project",
	Long: `Create a new off-chain project record.

For on-chain project creation, use: andamio tx run /v2/tx/instance/owner/project/create
Then register the project with: andamio project owner register

Examples:
  andamio project owner create --title "Community Development"
  andamio project owner create --title "My Project" --description "Build things" --public`,
	RunE: runProjectOwnerCreate,
}

var projectOwnerUpdateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update project metadata",
	Long: `Update an existing project's metadata. Only specified flags are updated; omitted fields are unchanged.

Examples:
  andamio project owner update --project-id <id> --title "New Title"
  andamio project owner update --project-id <id> --description "Updated description" --public=false`,
	RunE: runProjectOwnerUpdate,
}

var projectOwnerRegisterCmd = &cobra.Command{
	Use:   "register",
	Short: "Register an on-chain project with off-chain metadata",
	Long: `Register a project that has been created on-chain. Links the on-chain record
to off-chain metadata (title, description, etc.).

Typical flow:
  1. andamio tx run /v2/tx/instance/owner/project/create --body '...' --skey ... --tx-type project_create
  2. andamio project owner register --project-id <id>

Examples:
  andamio project owner register --project-id <id>
  andamio project owner register --project-id <id> --title "My Project" --public`,
	RunE: runProjectOwnerRegister,
}

func init() {
	projectCmd.AddCommand(projectOwnerCmd)
	projectOwnerCmd.AddCommand(projectOwnerListCmd)
	projectOwnerCmd.AddCommand(projectOwnerCreateCmd)
	projectOwnerCmd.AddCommand(projectOwnerUpdateCmd)
	projectOwnerCmd.AddCommand(projectOwnerRegisterCmd)

	// create flags
	projectOwnerCreateCmd.Flags().String("title", "", "Project title (required)")
	projectOwnerCreateCmd.MarkFlagRequired("title")
	projectOwnerCreateCmd.Flags().String("description", "", "Project description")
	projectOwnerCreateCmd.Flags().String("image-url", "", "Project image URL")
	projectOwnerCreateCmd.Flags().String("video-url", "", "Project video URL")
	projectOwnerCreateCmd.Flags().String("category", "", "Project category")
	projectOwnerCreateCmd.Flags().Bool("public", false, "Make project publicly visible")

	// update flags
	projectOwnerUpdateCmd.Flags().String("project-id", "", "Project ID (required)")
	projectOwnerUpdateCmd.MarkFlagRequired("project-id")
	projectOwnerUpdateCmd.Flags().String("title", "", "Project title")
	projectOwnerUpdateCmd.Flags().String("description", "", "Project description")
	projectOwnerUpdateCmd.Flags().String("image-url", "", "Project image URL")
	projectOwnerUpdateCmd.Flags().String("video-url", "", "Project video URL")
	projectOwnerUpdateCmd.Flags().Bool("public", false, "Set project public visibility")

	// register flags
	projectOwnerRegisterCmd.Flags().String("project-id", "", "Project ID (required)")
	projectOwnerRegisterCmd.MarkFlagRequired("project-id")
	projectOwnerRegisterCmd.Flags().String("tx-hash", "", "Transaction hash from on-chain creation")
	projectOwnerRegisterCmd.Flags().String("title", "", "Project title")
	projectOwnerRegisterCmd.Flags().String("description", "", "Project description")
	projectOwnerRegisterCmd.Flags().String("image-url", "", "Project image URL")
	projectOwnerRegisterCmd.Flags().String("video-url", "", "Project video URL")
	projectOwnerRegisterCmd.Flags().String("category", "", "Project category")
	projectOwnerRegisterCmd.Flags().Bool("public", false, "Make project publicly visible")
}

func runProjectOwnerCreate(cmd *cobra.Command, args []string) error {
	title, _ := cmd.Flags().GetString("title")
	description, _ := cmd.Flags().GetString("description")
	imageURL, _ := cmd.Flags().GetString("image-url")
	videoURL, _ := cmd.Flags().GetString("video-url")
	category, _ := cmd.Flags().GetString("category")
	isPublic, _ := cmd.Flags().GetBool("public")
	isJSON := output.GetFormat() == output.FormatJSON

	payload := map[string]interface{}{
		"title": title,
	}
	if description != "" {
		payload["description"] = description
	}
	if imageURL != "" {
		payload["image_url"] = imageURL
	}
	if videoURL != "" {
		payload["video_url"] = videoURL
	}
	if category != "" {
		payload["category"] = category
	}
	if cmd.Flags().Changed("public") {
		payload["is_public"] = isPublic
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	if !isJSON {
		fmt.Fprintf(os.Stderr, "Creating project: %s\n", title)
	}

	c := client.New(cfg)
	var resp map[string]interface{}
	if err := c.Post("/api/v2/project/owner/project/create", payload, &resp); err != nil {
		return fmt.Errorf("failed to create project: %w", err)
	}

	if isJSON {
		return output.PrintJSON(resp)
	}

	fmt.Fprintf(os.Stderr, "Project created.\n")
	if id, ok := resp["project_id"].(string); ok {
		fmt.Printf("project_id: %s\n", id)
	}
	return nil
}

func runProjectOwnerUpdate(cmd *cobra.Command, args []string) error {
	projectID, _ := cmd.Flags().GetString("project-id")
	isJSON := output.GetFormat() == output.FormatJSON

	data := map[string]interface{}{}
	if cmd.Flags().Changed("title") {
		v, _ := cmd.Flags().GetString("title")
		data["title"] = v
	}
	if cmd.Flags().Changed("description") {
		v, _ := cmd.Flags().GetString("description")
		data["description"] = v
	}
	if cmd.Flags().Changed("image-url") {
		v, _ := cmd.Flags().GetString("image-url")
		data["image_url"] = v
	}
	if cmd.Flags().Changed("video-url") {
		v, _ := cmd.Flags().GetString("video-url")
		data["video_url"] = v
	}
	if cmd.Flags().Changed("public") {
		v, _ := cmd.Flags().GetBool("public")
		data["is_public"] = v
	}

	if len(data) == 0 {
		return fmt.Errorf("no fields to update. Specify at least one of: --title, --description, --image-url, --video-url, --public")
	}

	payload := map[string]interface{}{
		"project_id": projectID,
		"data":       data,
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	if !isJSON {
		fmt.Fprintf(os.Stderr, "Updating project %s\n", projectID)
	}

	c := client.New(cfg)
	var resp map[string]interface{}
	if err := c.Post("/api/v2/project/owner/project/update", payload, &resp); err != nil {
		return fmt.Errorf("failed to update project: %w", err)
	}

	if isJSON {
		return output.PrintJSON(resp)
	}

	fmt.Fprintf(os.Stderr, "Project updated.\n")
	return nil
}

func runProjectOwnerRegister(cmd *cobra.Command, args []string) error {
	projectID, _ := cmd.Flags().GetString("project-id")
	isJSON := output.GetFormat() == output.FormatJSON

	payload := map[string]interface{}{
		"project_id": projectID,
	}
	if v, _ := cmd.Flags().GetString("tx-hash"); v != "" {
		payload["tx_hash"] = v
	}
	if v, _ := cmd.Flags().GetString("title"); v != "" {
		payload["title"] = v
	}
	if v, _ := cmd.Flags().GetString("description"); v != "" {
		payload["description"] = v
	}
	if v, _ := cmd.Flags().GetString("image-url"); v != "" {
		payload["image_url"] = v
	}
	if v, _ := cmd.Flags().GetString("video-url"); v != "" {
		payload["video_url"] = v
	}
	if v, _ := cmd.Flags().GetString("category"); v != "" {
		payload["category"] = v
	}
	if cmd.Flags().Changed("public") {
		v, _ := cmd.Flags().GetBool("public")
		payload["is_public"] = v
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	if !isJSON {
		fmt.Fprintf(os.Stderr, "Registering project %s\n", projectID)
	}

	c := client.New(cfg)
	var resp map[string]interface{}
	if err := c.Post("/api/v2/project/owner/project/register", payload, &resp); err != nil {
		return fmt.Errorf("failed to register project: %w", err)
	}

	if isJSON {
		return output.PrintJSON(resp)
	}

	fmt.Fprintf(os.Stderr, "Project registered.\n")
	return nil
}
