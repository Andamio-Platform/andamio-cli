package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"

	"github.com/Andamio-Platform/andamio-cli/internal/apierr"
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

var projectManagerQualifiedContributorsCmd = &cobra.Command{
	Use:   "qualified-contributors",
	Short: "List aliases qualified to commit to the project's tasks",
	Long: `List aliases qualified to commit to the project's tasks.

An alias is qualified iff they hold every (course_id, slt_hash) prerequisite
pair declared in the project's current on-chain state. This is a reverse
lookup over the credential graph — the chain-side gate remains the source of
truth for who can actually commit.

Results are capped at 500 aliases; when exceeded, the response carries
truncated=true. In text mode this surfaces as a stderr warning line; in
JSON mode the flag is passed through on the envelope.

Find your project IDs with: andamio project list --output json

Examples:
  andamio project manager qualified-contributors --project-id <id>
  andamio project manager qualified-contributors --project-id <id> --output json`,
	RunE: runProjectManagerQualifiedContributors,
}

func init() {
	projectCmd.AddCommand(projectManagerCmd)
	projectManagerCmd.AddCommand(projectManagerCommitmentsCmd)
	projectManagerCmd.AddCommand(projectManagerQualifiedContributorsCmd)

	projectManagerCommitmentsCmd.Flags().String("project-id", "", "Project ID (required)")
	projectManagerCommitmentsCmd.MarkFlagRequired("project-id")

	projectManagerQualifiedContributorsCmd.Flags().String("project-id", "", "Project ID (required)")
	projectManagerQualifiedContributorsCmd.MarkFlagRequired("project-id")
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

// qualifiedContributorsResponse matches the gateway envelope.
// Field tags follow the gateway contract from andamio-api v2.3 PR #380.
type qualifiedContributorsResponse struct {
	ProjectID  string   `json:"projectId"`
	Aliases    []string `json:"aliases"`
	TotalCount int      `json:"totalCount"`
	Truncated  bool     `json:"truncated"`
}

func runProjectManagerQualifiedContributors(cmd *cobra.Command, args []string) error {
	projectID, _ := cmd.Flags().GetString("project-id")

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	c := client.New(cfg)

	resp, err := fetchQualifiedContributors(cmd.Context(), c, projectID)
	if err != nil {
		return err
	}

	if output.GetFormat() == output.FormatJSON {
		return output.PrintJSON(resp)
	}
	return renderQualifiedContributorsText(resp, os.Stdout, os.Stderr)
}

// renderQualifiedContributorsText writes the text-mode output: aliases to
// stdout, empty-result notice and truncated warning to stderr. The empty
// notice and truncated warning are independent — when the gateway returns
// aliases=[] with truncated=true (per the gateway contract this is rare
// but representable), both stderr lines fire in that order.
func renderQualifiedContributorsText(resp qualifiedContributorsResponse, stdout, stderr io.Writer) error {
	for _, alias := range resp.Aliases {
		fmt.Fprintln(stdout, alias)
	}
	if len(resp.Aliases) == 0 {
		fmt.Fprintln(stderr, "No qualified contributors found.")
	}
	if resp.Truncated {
		fmt.Fprintln(stderr, "warning: result truncated at 500 aliases")
	}
	return nil
}

// fetchQualifiedContributors performs the GET and remaps errors. Split from the
// Cobra handler so tests can drive it with a stubbed server without reaching
// into filesystem config.
func fetchQualifiedContributors(ctx context.Context, c *client.Client, projectID string) (qualifiedContributorsResponse, error) {
	path := "/api/v2/project/manager/contributors/get-qualified?" + url.Values{"project_id": {projectID}}.Encode()
	var resp qualifiedContributorsResponse
	if err := c.Get(ctx, path, &resp); err != nil {
		return qualifiedContributorsResponse{}, remapQualifiedContributorsError(err, projectID)
	}
	return resp, nil
}

// remapQualifiedContributorsError rewrites the gateway's typed errors into
// user-facing hints. Only 403, 404, and 502 get rewritten; every other error
// (401, 500, 503, 504, transport failures) bubbles unchanged.
func remapQualifiedContributorsError(err error, projectID string) error {
	var authErr *apierr.AuthError
	if errors.As(err, &authErr) && authErr.HTTPStatus == 403 {
		return &apierr.AuthError{
			HTTPStatus: 403,
			Message:    fmt.Sprintf("not a manager of project %s", projectID),
		}
	}
	var notFound *apierr.NotFoundError
	if errors.As(err, &notFound) {
		return &apierr.NotFoundError{Message: fmt.Sprintf("project %s not found", projectID)}
	}
	var serverErr *apierr.ServerError
	if errors.As(err, &serverErr) && serverErr.Status == 502 {
		return &apierr.ServerError{Status: 502, Message: "scan temporarily unavailable, retry later"}
	}
	return err
}
