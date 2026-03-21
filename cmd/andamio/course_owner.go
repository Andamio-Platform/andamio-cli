package main

import (
	"fmt"
	"os"

	"github.com/Andamio-Platform/andamio-cli/internal/client"
	"github.com/Andamio-Platform/andamio-cli/internal/config"
	"github.com/Andamio-Platform/andamio-cli/internal/output"
	"github.com/spf13/cobra"
)

var courseOwnerCmd = &cobra.Command{
	Use:               "owner",
	Short:             "Course owner operations (requires user login)",
	PersistentPreRunE: jwtAuthPreRunE,
}

var courseOwnerListCmd = &cobra.Command{
	Use:   "list",
	Short: "List courses you own",
	RunE: func(cmd *cobra.Command, args []string) error {
		return printList(
			"/api/v2/course/owner/courses/list",
			"No owned courses found.",
			"content.title", "course_id", true,
		)
	},
}

var courseOwnerCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new course",
	Long: `Create a new off-chain course record.

For on-chain course creation, use: andamio tx run /v2/tx/instance/owner/course/create
Then register the course with: andamio course owner register

Examples:
  andamio course owner create --title "Introduction to Cardano"
  andamio course owner create --title "My Course" --description "Learn things" --public`,
	RunE: runCourseOwnerCreate,
}

var courseOwnerUpdateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update course metadata",
	Long: `Update an existing course's metadata. Only specified flags are updated; omitted fields are unchanged.

Examples:
  andamio course owner update --course-id <id> --title "New Title"
  andamio course owner update --course-id <id> --description "Updated description" --public=false`,
	RunE: runCourseOwnerUpdate,
}

var courseOwnerRegisterCmd = &cobra.Command{
	Use:   "register",
	Short: "Register an on-chain course with off-chain metadata",
	Long: `Register a course that has been created on-chain. Links the on-chain record
to off-chain metadata (title, description, etc.).

Typical flow:
  1. andamio tx run /v2/tx/instance/owner/course/create --body '...' --skey ... --tx-type course_create
  2. andamio course owner register --course-id <id>

Examples:
  andamio course owner register --course-id <id>
  andamio course owner register --course-id <id> --title "My Course" --public`,
	RunE: runCourseOwnerRegister,
}

var courseOwnerTeachersCmd = &cobra.Command{
	Use:   "teachers",
	Short: "Update the teacher list for a course",
	Long: `Set the list of teachers for a course. Pass one or more --teacher flags with user aliases.

This replaces the full teacher list — include all desired teachers.

Examples:
  andamio course owner teachers --course-id <id> --teacher alice --teacher bob`,
	RunE: runCourseOwnerTeachers,
}

func init() {
	courseCmd.AddCommand(courseOwnerCmd)
	courseOwnerCmd.AddCommand(courseOwnerListCmd)
	courseOwnerCmd.AddCommand(courseOwnerCreateCmd)
	courseOwnerCmd.AddCommand(courseOwnerUpdateCmd)
	courseOwnerCmd.AddCommand(courseOwnerRegisterCmd)
	courseOwnerCmd.AddCommand(courseOwnerTeachersCmd)

	// create flags
	courseOwnerCreateCmd.Flags().String("title", "", "Course title (required)")
	courseOwnerCreateCmd.MarkFlagRequired("title")
	courseOwnerCreateCmd.Flags().String("description", "", "Course description")
	courseOwnerCreateCmd.Flags().String("image-url", "", "Course image URL")
	courseOwnerCreateCmd.Flags().String("video-url", "", "Course video URL")
	courseOwnerCreateCmd.Flags().String("category", "", "Course category")
	courseOwnerCreateCmd.Flags().Bool("public", false, "Make course publicly visible")

	// update flags
	courseOwnerUpdateCmd.Flags().String("course-id", "", "Course ID (required)")
	courseOwnerUpdateCmd.MarkFlagRequired("course-id")
	courseOwnerUpdateCmd.Flags().String("title", "", "Course title")
	courseOwnerUpdateCmd.Flags().String("description", "", "Course description")
	courseOwnerUpdateCmd.Flags().String("image-url", "", "Course image URL")
	courseOwnerUpdateCmd.Flags().String("video-url", "", "Course video URL")
	courseOwnerUpdateCmd.Flags().Bool("live", false, "Set course live status")
	courseOwnerUpdateCmd.Flags().Bool("public", false, "Set course public visibility")

	// register flags
	courseOwnerRegisterCmd.Flags().String("course-id", "", "Course ID (required)")
	courseOwnerRegisterCmd.MarkFlagRequired("course-id")
	courseOwnerRegisterCmd.Flags().String("tx-hash", "", "Transaction hash from on-chain creation")
	courseOwnerRegisterCmd.Flags().String("title", "", "Course title")
	courseOwnerRegisterCmd.Flags().String("description", "", "Course description")
	courseOwnerRegisterCmd.Flags().String("image-url", "", "Course image URL")
	courseOwnerRegisterCmd.Flags().String("video-url", "", "Course video URL")
	courseOwnerRegisterCmd.Flags().String("category", "", "Course category")
	courseOwnerRegisterCmd.Flags().Bool("public", false, "Make course publicly visible")

	// teachers flags
	courseOwnerTeachersCmd.Flags().String("course-id", "", "Course ID (required)")
	courseOwnerTeachersCmd.MarkFlagRequired("course-id")
	courseOwnerTeachersCmd.Flags().StringArray("teacher", nil, "Teacher alias (repeatable)")
	courseOwnerTeachersCmd.MarkFlagRequired("teacher")
}

func runCourseOwnerCreate(cmd *cobra.Command, args []string) error {
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
		fmt.Fprintf(os.Stderr, "Creating course: %s\n", title)
	}

	c := client.New(cfg)
	var resp map[string]interface{}
	if err := c.Post("/api/v2/course/owner/course/create", payload, &resp); err != nil {
		return fmt.Errorf("failed to create course: %w", err)
	}

	if isJSON {
		return output.PrintJSON(resp)
	}

	fmt.Fprintf(os.Stderr, "Course created.\n")
	if id, ok := resp["course_id"].(string); ok {
		fmt.Printf("course_id: %s\n", id)
	}
	return nil
}

func runCourseOwnerUpdate(cmd *cobra.Command, args []string) error {
	courseID, _ := cmd.Flags().GetString("course-id")
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
	if cmd.Flags().Changed("live") {
		v, _ := cmd.Flags().GetBool("live")
		data["live"] = v
	}
	if cmd.Flags().Changed("public") {
		v, _ := cmd.Flags().GetBool("public")
		data["is_public"] = v
	}

	if len(data) == 0 {
		return fmt.Errorf("no fields to update. Specify at least one of: --title, --description, --image-url, --video-url, --live, --public")
	}

	payload := map[string]interface{}{
		"course_id": courseID,
		"data":      data,
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	if !isJSON {
		fmt.Fprintf(os.Stderr, "Updating course %s\n", courseID)
	}

	c := client.New(cfg)
	var resp map[string]interface{}
	if err := c.Post("/api/v2/course/owner/course/update", payload, &resp); err != nil {
		return fmt.Errorf("failed to update course: %w", err)
	}

	if isJSON {
		return output.PrintJSON(resp)
	}

	fmt.Fprintf(os.Stderr, "Course updated.\n")
	return nil
}

func runCourseOwnerRegister(cmd *cobra.Command, args []string) error {
	courseID, _ := cmd.Flags().GetString("course-id")
	isJSON := output.GetFormat() == output.FormatJSON

	payload := map[string]interface{}{
		"course_id": courseID,
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
		fmt.Fprintf(os.Stderr, "Registering course %s\n", courseID)
	}

	c := client.New(cfg)
	var resp map[string]interface{}
	if err := c.Post("/api/v2/course/owner/course/register", payload, &resp); err != nil {
		return fmt.Errorf("failed to register course: %w", err)
	}

	if isJSON {
		return output.PrintJSON(resp)
	}

	fmt.Fprintf(os.Stderr, "Course registered.\n")
	return nil
}

func runCourseOwnerTeachers(cmd *cobra.Command, args []string) error {
	courseID, _ := cmd.Flags().GetString("course-id")
	teachers, _ := cmd.Flags().GetStringArray("teacher")
	isJSON := output.GetFormat() == output.FormatJSON

	payload := map[string]interface{}{
		"course_id": courseID,
		"teachers":  teachers,
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	if !isJSON {
		fmt.Fprintf(os.Stderr, "Updating teachers for course %s\n", courseID)
	}

	c := client.New(cfg)
	var resp map[string]interface{}
	if err := c.Post("/api/v2/course/owner/teachers/update", payload, &resp); err != nil {
		return fmt.Errorf("failed to update teachers: %w", err)
	}

	if isJSON {
		return output.PrintJSON(resp)
	}

	fmt.Fprintf(os.Stderr, "Teachers updated.\n")
	return nil
}
