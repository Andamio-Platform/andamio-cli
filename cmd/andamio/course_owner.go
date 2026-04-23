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
			cmd.Context(),
			"/api/v2/course/owner/courses/list",
			"No owned courses found.",
			"content.title", "course_id", true,
		)
	},
}

var courseOwnerCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create off-chain course record (after on-chain creation)",
	Long: `Create the off-chain metadata record for a course that has already been created on-chain.

Note: In most cases, 'andamio tx run' with course_create auto-registers the course in the DB.
Use 'andamio course owner update' to set metadata after that. This command is only needed when
the auto-registration did not occur (e.g., the TX confirmed but DB update failed).

Requires --course-id (from the on-chain NFT policy) and --pending-tx-hash.

Typical workflow:
  1. andamio tx run /v2/tx/instance/owner/course/create ...  (creates on-chain, auto-registers)
  2. andamio course owner update --course-id <id> --title ...  (set metadata)

Examples:
  andamio course owner create --course-id abc123 --pending-tx-hash tx123 --title "Introduction to Cardano"
  andamio course owner create --course-id abc123 --pending-tx-hash tx123 --description "Learn things" --public`,
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
  andamio course owner register --course-id <id> --title "My Course"
  andamio course owner register --course-id <id> --title "My Course" --public`,
	RunE: runCourseOwnerRegister,
}

var courseOwnerTeachersCmd = &cobra.Command{
	Use:   "teachers",
	Short: "Update the teacher list for a course",
	Long: `Add or remove teachers from a course. Use --add and --remove flags with user aliases.

Examples:
  andamio course owner teachers --course-id <id> --add alice --add bob
  andamio course owner teachers --course-id <id> --remove charlie
  andamio course owner teachers --course-id <id> --add alice --remove charlie`,
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
	courseOwnerCreateCmd.Flags().String("course-id", "", "Course ID (required — derived from on-chain NFT policy)")
	courseOwnerCreateCmd.MarkFlagRequired("course-id")
	courseOwnerCreateCmd.Flags().String("title", "", "Course title")
	courseOwnerCreateCmd.Flags().String("description", "", "Course description")
	courseOwnerCreateCmd.Flags().String("image-url", "", "Course image URL")
	courseOwnerCreateCmd.Flags().String("video-url", "", "Course video URL")
	courseOwnerCreateCmd.Flags().String("category", "", "Course category")
	courseOwnerCreateCmd.Flags().Bool("public", false, "Make course publicly visible")
	courseOwnerCreateCmd.Flags().String("pending-tx-hash", "", "Transaction hash of the pending on-chain creation (required)")
	courseOwnerCreateCmd.MarkFlagRequired("pending-tx-hash")

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
	courseOwnerRegisterCmd.Flags().String("title", "", "Course title (required)")
	courseOwnerRegisterCmd.MarkFlagRequired("title")
	courseOwnerRegisterCmd.Flags().String("description", "", "Course description")
	courseOwnerRegisterCmd.Flags().String("image-url", "", "Course image URL")
	courseOwnerRegisterCmd.Flags().String("video-url", "", "Course video URL")
	courseOwnerRegisterCmd.Flags().String("category", "", "Course category")
	courseOwnerRegisterCmd.Flags().Bool("public", false, "Make course publicly visible")

	// teachers flags
	courseOwnerTeachersCmd.Flags().String("course-id", "", "Course ID (required)")
	courseOwnerTeachersCmd.MarkFlagRequired("course-id")
	courseOwnerTeachersCmd.Flags().StringArray("add", nil, "Teacher alias to add (repeatable)")
	courseOwnerTeachersCmd.Flags().StringArray("remove", nil, "Teacher alias to remove (repeatable)")
}

func runCourseOwnerCreate(cmd *cobra.Command, args []string) error {
	courseID, _ := cmd.Flags().GetString("course-id")
	title, _ := cmd.Flags().GetString("title")
	description, _ := cmd.Flags().GetString("description")
	imageURL, _ := cmd.Flags().GetString("image-url")
	videoURL, _ := cmd.Flags().GetString("video-url")
	category, _ := cmd.Flags().GetString("category")
	isPublic, _ := cmd.Flags().GetBool("public")
	pendingTxHash, _ := cmd.Flags().GetString("pending-tx-hash")
	isJSON := output.GetFormat() == output.FormatJSON

	payload := map[string]interface{}{
		"course_id":       courseID,
		"pending_tx_hash": pendingTxHash,
	}
	if title != "" {
		payload["title"] = title
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
		fmt.Fprintf(os.Stderr, "Creating course: %s\n", courseID)
	}

	c := client.New(cfg)
	var resp map[string]interface{}
	if err := c.Post(cmd.Context(), "/api/v2/course/owner/course/create", payload, &resp); err != nil {
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
	if err := c.Post(cmd.Context(), "/api/v2/course/owner/course/update", payload, &resp); err != nil {
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
	title, _ := cmd.Flags().GetString("title")
	if title == "" {
		return fmt.Errorf("--title must not be empty")
	}
	payload["title"] = title
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
	if err := c.Post(cmd.Context(), "/api/v2/course/owner/course/register", payload, &resp); err != nil {
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
	addTeachers, _ := cmd.Flags().GetStringArray("add")
	removeTeachers, _ := cmd.Flags().GetStringArray("remove")
	isJSON := output.GetFormat() == output.FormatJSON

	if len(addTeachers) == 0 && len(removeTeachers) == 0 {
		return fmt.Errorf("specify at least one of --add or --remove. Use 'andamio user exists <alias>' to verify aliases")
	}

	// Filter empty strings from alias arrays
	addTeachers = filterEmpty(addTeachers)
	removeTeachers = filterEmpty(removeTeachers)
	if len(addTeachers) == 0 && len(removeTeachers) == 0 {
		return fmt.Errorf("all provided aliases are empty")
	}

	payload := map[string]interface{}{
		"course_id": courseID,
	}
	if len(addTeachers) > 0 {
		payload["add"] = addTeachers
	}
	if len(removeTeachers) > 0 {
		payload["remove"] = removeTeachers
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
	if err := c.Post(cmd.Context(), "/api/v2/course/owner/teachers/update", payload, &resp); err != nil {
		return fmt.Errorf("failed to update teachers: %w", err)
	}

	if isJSON {
		return output.PrintJSON(resp)
	}

	fmt.Fprintf(os.Stderr, "Teachers updated.\n")
	return nil
}

func filterEmpty(ss []string) []string {
	out := ss[:0]
	for _, s := range ss {
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}
