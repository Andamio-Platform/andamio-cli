package main

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/Andamio-Platform/andamio-cli/internal/cardano"
	"github.com/Andamio-Platform/andamio-cli/internal/client"
	"github.com/Andamio-Platform/andamio-cli/internal/config"
	"github.com/Andamio-Platform/andamio-cli/internal/output"
	"github.com/spf13/cobra"
)

const (
	modulesManageEndpoint = "/v2/tx/course/teacher/modules/manage"
	modulesManageTxType   = "modules_manage"
)

var courseTeacherMintModulesCmd = &cobra.Command{
	Use:   "mint-modules",
	Short: "Mint imported modules on-chain (modules_manage transaction)",
	Long: `Mint one or more imported modules on-chain. Name the modules by code; the
command reads each module's SLTs from its database record and builds the
modules_manage transaction body itself.

The body carries the SLT text exactly as the database holds it, in slt_index
order, so the minted module's hash is the hash its record carries. Before
anything is built, the command computes that hash locally and refuses a module
whose stored slt_hash differs, one that is already ON_CHAIN, and one with no
SLTs.

This is an on-chain transaction. Like 'tx run', the command runs the full
lifecycle: build, sign with your local .skey, submit, register and poll for
confirmation. --skey defaults to the 'wallet create' default wallet.

Use --dry-run to print the request body without building anything. The body
can then be inspected, or built on its own with 'andamio tx build'.

Find module codes with: andamio course modules <course-id> --output json

Examples:
  andamio course teacher mint-modules --course-id <id> --alias teacher-01 --module-code 101 --skey ./payment.skey
  andamio course teacher mint-modules --course-id <id> --alias teacher-01 --module-code 101 --module-code 102
  andamio course teacher mint-modules --course-id <id> --alias teacher-01 --module-code 101 --dry-run --output json`,
	Args: cobra.NoArgs,
	RunE: runCourseTeacherMintModules,
}

func init() {
	courseTeacherCmd.AddCommand(courseTeacherMintModulesCmd)

	f := courseTeacherMintModulesCmd.Flags()
	f.String("course-id", "", "Course ID (required)")
	courseTeacherMintModulesCmd.MarkFlagRequired("course-id")
	f.String("alias", "", "Your Teacher alias (required)")
	courseTeacherMintModulesCmd.MarkFlagRequired("alias")
	f.StringArray("module-code", nil, "Code of an imported module to mint (repeatable, required)")
	courseTeacherMintModulesCmd.MarkFlagRequired("module-code")
	addManageTxFlags(courseTeacherMintModulesCmd)
}

// addManageTxFlags registers the transaction flags shared by the commands that
// build a manage body and hand it to executeTxLifecycle. They mirror 'tx run'.
func addManageTxFlags(cmd *cobra.Command) {
	f := cmd.Flags()
	f.String("skey", "", "Path to Cardano .skey file for signing; defaults to the wallet from 'andamio wallet create' if omitted")
	f.String("submit-url", "", "Override submit API URL (falls back to config)")
	f.StringArray("submit-header", nil, "Additional submit headers (repeatable, format: \"Key: Value\")")
	f.StringArray("metadata", nil, "Metadata for registration (repeatable, format: key=value)")
	f.Bool("no-wait", false, "Exit after registration without polling for confirmation")
	f.Duration("timeout", 10*time.Minute, "Max time to wait for confirmation")
	f.Bool("dry-run", false, "Print the request body and stop: nothing is built, signed or submitted")
}

// mintModuleV2 is one entry of ManageModulesTxRequest.modules_to_add. No
// omitempty anywhere: the gateway refuses a body with a key missing, even when
// the list it would carry is empty.
type mintModuleV2 struct {
	SLTs                   []string `json:"slts"`
	AllowedStudentStateIDs []string `json:"allowed_student_state_ids"`
	PrereqCredentialHashes []string `json:"prereq_credential_hashes"`
}

// updateModuleV2 is one entry of ManageModulesTxRequest.modules_to_update.
// mint-modules never sends one; the type exists so the list is typed.
type updateModuleV2 struct {
	SltHash                string   `json:"slt_hash"`
	AllowedStudentStateIDs []string `json:"allowed_student_state_ids"`
	PrereqCredentialHashes []string `json:"prereq_credential_hashes"`
}

// manageModulesBody is ManageModulesTxRequest. Every key is always sent.
type manageModulesBody struct {
	Alias           string           `json:"alias"`
	CourseID        string           `json:"course_id"`
	ModulesToAdd    []mintModuleV2   `json:"modules_to_add"`
	ModulesToUpdate []updateModuleV2 `json:"modules_to_update"`
	ModulesToRemove []string         `json:"modules_to_remove"`
}

// mintModuleSource is what mint-modules reads from one module's DB record.
type mintModuleSource struct {
	Code       string
	Status     string
	StoredHash string // the record's slt_hash; empty for a draft without one
	SLTs       []string
}

// MintModuleSummary names each module in a mint body by code, with the hash
// its SLTs produce. The body itself carries no module code.
type MintModuleSummary struct {
	ModuleCode string `json:"module_code"`
	SltHash    string `json:"slt_hash"`
	SLTCount   int    `json:"slt_count"`
}

// ManageBodyPreview is the --dry-run output of mint-modules and task mint.
type ManageBodyPreview struct {
	DryRun   bool                `json:"dry_run"`
	Endpoint string              `json:"endpoint"`
	TxType   string              `json:"tx_type"`
	Body     interface{}         `json:"body"`
	Modules  []MintModuleSummary `json:"modules,omitempty"`
	Tasks    []MintTaskSummary   `json:"tasks,omitempty"`
}

// buildManageModulesBody builds the modules_manage body that mints the given
// modules. The SLT strings are copied as they are: no trimming, no
// normalization, so the hash the chain computes is the hash of the DB text.
func buildManageModulesBody(alias, courseID string, modules []mintModuleSource) (manageModulesBody, []MintModuleSummary, error) {
	body := manageModulesBody{
		Alias:           alias,
		CourseID:        courseID,
		ModulesToAdd:    make([]mintModuleV2, 0, len(modules)),
		ModulesToUpdate: []updateModuleV2{},
		ModulesToRemove: []string{},
	}
	summaries := make([]MintModuleSummary, 0, len(modules))

	for _, m := range modules {
		if strings.EqualFold(m.Status, "ON_CHAIN") {
			return manageModulesBody{}, nil, fmt.Errorf("module %s is already ON_CHAIN; there is nothing to mint", m.Code)
		}
		if len(m.SLTs) == 0 {
			return manageModulesBody{}, nil, fmt.Errorf("module %s has no SLTs in its record. Import its outline first: andamio course import <dir> --course-id %s", m.Code, courseID)
		}
		computed := cardano.ComputeSltHash(m.SLTs)
		if m.StoredHash != "" && !strings.EqualFold(m.StoredHash, computed) {
			return manageModulesBody{}, nil, fmt.Errorf("module %s: its record carries slt_hash %s but its SLTs hash to %s. Minting it would create a module the record does not match; re-import the module before minting", m.Code, m.StoredHash, computed)
		}

		slts := make([]string, len(m.SLTs))
		copy(slts, m.SLTs)
		body.ModulesToAdd = append(body.ModulesToAdd, mintModuleV2{
			SLTs:                   slts,
			AllowedStudentStateIDs: []string{},
			PrereqCredentialHashes: []string{},
		})
		summaries = append(summaries, MintModuleSummary{
			ModuleCode: m.Code,
			SltHash:    computed,
			SLTCount:   len(slts),
		})
	}
	return body, summaries, nil
}

// moduleSourceFromItem reads one entry of the teacher course-modules list.
// SLTs are ordered by slt_index, falling back to list order for an entry
// without one.
func moduleSourceFromItem(item map[string]interface{}) (mintModuleSource, bool) {
	content, ok := item["content"].(map[string]interface{})
	if !ok {
		return mintModuleSource{}, false
	}
	code, _ := content["course_module_code"].(string)
	if code == "" {
		return mintModuleSource{}, false
	}
	src := mintModuleSource{Code: code}
	src.Status, _ = content["module_status"].(string)
	src.StoredHash, _ = item["slt_hash"].(string)

	type indexed struct {
		pos, index int
		text       string
	}
	var entries []indexed
	if slts, ok := content["slts"].([]interface{}); ok {
		for pos, raw := range slts {
			s, ok := raw.(map[string]interface{})
			if !ok {
				continue
			}
			text, ok := s["slt_text"].(string)
			if !ok {
				continue
			}
			idx := pos + 1
			if v, ok := s["slt_index"].(float64); ok && v > 0 {
				idx = int(v)
			}
			entries = append(entries, indexed{pos: pos, index: idx, text: text})
		}
	}
	sort.SliceStable(entries, func(i, j int) bool { return entries[i].index < entries[j].index })
	for _, e := range entries {
		src.SLTs = append(src.SLTs, e.text)
	}
	return src, true
}

// fetchMintModuleSources reads the teacher modules list once and returns the
// named modules in the order they were named.
func fetchMintModuleSources(ctx context.Context, c *client.Client, courseID string, codes []string) ([]mintModuleSource, error) {
	var resp map[string]interface{}
	if err := c.PostWithRetry(ctx, "/api/v2/course/teacher/course-modules/list", map[string]string{"course_id": courseID}, &resp); err != nil {
		return nil, fmt.Errorf("failed to list modules for course %s: %w", courseID, err)
	}
	data, ok := resp["data"].([]interface{})
	if !ok {
		return nil, fmt.Errorf("unexpected response format from teacher modules list: missing data array")
	}

	byCode := make(map[string]mintModuleSource, len(data))
	for _, raw := range data {
		item, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		if src, ok := moduleSourceFromItem(item); ok {
			byCode[src.Code] = src
		}
	}

	out := make([]mintModuleSource, 0, len(codes))
	for _, code := range codes {
		src, ok := byCode[code]
		if !ok {
			if warning := metaWarning(resp); warning != "" && warningHidesDBContent(warning) {
				return nil, fmt.Errorf("%w: %s (module '%s' in course '%s' may exist but was not returned)", errDegradedRead, warning, code, courseID)
			}
			return nil, fmt.Errorf("%w: '%s' in course '%s'. List modules with: andamio course modules %s --output json", errModuleNotFound, code, courseID, courseID)
		}
		out = append(out, src)
	}
	return out, nil
}

func runCourseTeacherMintModules(cmd *cobra.Command, args []string) error {
	courseID, _ := cmd.Flags().GetString("course-id")
	alias, _ := cmd.Flags().GetString("alias")
	codes, _ := cmd.Flags().GetStringArray("module-code")
	dryRun, _ := cmd.Flags().GetBool("dry-run")
	isJSON := output.GetFormat() == output.FormatJSON

	codes = filterEmpty(codes)
	if len(codes) == 0 {
		return fmt.Errorf("--module-code is empty. List modules with: andamio course modules %s --output json", courseID)
	}
	seen := make(map[string]bool, len(codes))
	for _, code := range codes {
		if seen[code] {
			return fmt.Errorf("module %s is named twice", code)
		}
		seen[code] = true
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	c := client.New(cfg)

	sources, err := fetchMintModuleSources(cmd.Context(), c, courseID, codes)
	if err != nil {
		return err
	}
	body, summaries, err := buildManageModulesBody(alias, courseID, sources)
	if err != nil {
		return err
	}

	if dryRun {
		return printManageBodyPreview(ManageBodyPreview{
			DryRun:   true,
			Endpoint: modulesManageEndpoint,
			TxType:   modulesManageTxType,
			Body:     body,
			Modules:  summaries,
		}, isJSON)
	}

	if !isJSON {
		fmt.Fprintf(os.Stderr, "Minting %d module(s) for course %s\n", len(summaries), courseID)
		for _, s := range summaries {
			fmt.Fprintf(os.Stderr, "  %s: %d SLTs, slt_hash %s\n", s.ModuleCode, s.SLTCount, s.SltHash)
		}
	}

	return runManageTx(cmd, cfg, c, modulesManageEndpoint, modulesManageTxType, courseID, body, "Modules minted.")
}

// printManageBodyPreview writes a --dry-run preview. JSON mode prints the
// whole envelope; text mode prints the bare body on stdout, so it can be
// saved and passed to 'tx build --body-file', and the summary on stderr.
func printManageBodyPreview(p ManageBodyPreview, isJSON bool) error {
	if isJSON {
		return output.PrintJSON(p)
	}
	for _, m := range p.Modules {
		fmt.Fprintf(os.Stderr, "  %s: %d SLTs, slt_hash %s\n", m.ModuleCode, m.SLTCount, m.SltHash)
	}
	for _, t := range p.Tasks {
		fmt.Fprintf(os.Stderr, "  task %d: task_hash %s\n", t.TaskIndex, t.TaskHash)
	}
	fmt.Fprintf(os.Stderr, "Dry run: nothing was built. Endpoint %s, tx type %s.\n", p.Endpoint, p.TxType)
	return output.PrintJSON(p.Body)
}

// runManageTx hands a built body to the shared transaction lifecycle, reading
// the transaction flags registered by addManageTxFlags.
func runManageTx(cmd *cobra.Command, cfg *config.Config, c *client.Client, endpoint, txType, instanceID string, body interface{}, doneMsg string) error {
	skeyPath, _ := cmd.Flags().GetString("skey")
	submitURL, _ := cmd.Flags().GetString("submit-url")
	headers, _ := cmd.Flags().GetStringArray("submit-header")
	metadataFlags, _ := cmd.Flags().GetStringArray("metadata")
	noWait, _ := cmd.Flags().GetBool("no-wait")
	timeout, _ := cmd.Flags().GetDuration("timeout")
	isJSON := output.GetFormat() == output.FormatJSON

	if timeout <= 0 {
		return fmt.Errorf("--timeout must be positive (got %s)", timeout)
	}

	skeyPath, usedDefault, err := cardano.ResolveSkeyPath(skeyPath, cmd.Flags().Changed("skey"))
	if err != nil {
		return err
	}
	var warnings []string
	if usedDefault {
		msg := cardano.DefaultSkeyWarning(skeyPath)
		fmt.Fprintf(os.Stderr, "Warning: %s\n", msg)
		warnings = append(warnings, msg)
	}

	metadata, err := parseMetadataFlags(metadataFlags)
	if err != nil {
		return err
	}

	if err := checkJWTExpiry(cfg, isJSON); err != nil {
		return err
	}

	result, err := executeTxLifecycle(cmd.Context(), c, cfg, TxLifecycleParams{
		Endpoint:   endpoint,
		Body:       body,
		SkeyPath:   skeyPath,
		TxType:     txType,
		InstanceID: instanceID,
		Metadata:   metadata,
		NoWait:     noWait,
		Timeout:    timeout,
		SubmitURL:  submitURL,
		Headers:    headers,
		Warnings:   warnings,
	})
	if err != nil {
		return err
	}

	if isJSON && result.State != "registered" {
		return output.PrintJSON(result)
	}
	if !isJSON && result.State != "registered" {
		fmt.Fprintln(os.Stderr, doneMsg)
	}
	return nil
}
