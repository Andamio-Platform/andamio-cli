package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Andamio-Platform/andamio-cli/internal/cardano"
	"github.com/Andamio-Platform/andamio-cli/internal/client"
	"github.com/Andamio-Platform/andamio-cli/internal/config"
	"github.com/Andamio-Platform/andamio-cli/internal/output"
	"github.com/spf13/cobra"
)

const (
	tasksManageEndpoint = "/v2/tx/project/manager/tasks/manage"
	tasksManageTxType   = "tasks_manage"

	// maxProjectContentLength is the on-chain cap on a task's project_content,
	// which is its title.
	maxProjectContentLength = 140

	// millisecondEpochThreshold separates a seconds timestamp from a
	// milliseconds one (2000-01-01 in ms), as the Andamio app does.
	millisecondEpochThreshold = 946684800000
)

var projectTaskMintCmd = &cobra.Command{
	Use:   "mint <project-id>",
	Short: "Mint draft tasks on-chain (tasks_manage transaction)",
	Long: `Mint one or more DRAFT tasks on-chain. Name the tasks by index; the command
reads each task's title, reward and expiration from its draft and builds the
tasks_manage transaction body itself.

Each task's on-chain project_content is its title as stored, which must be 140
characters or fewer. Only DRAFT tasks can be minted.

--deposit-lovelace and --deposit-token set deposit_value, what your wallet adds
to the project treasury in the same transaction. With neither, deposit_value
is sent empty and the treasury must already cover the rewards being minted.
The command prints the rewards the tasks lock, so you can choose a deposit.

This is an on-chain transaction. Like 'tx run', the command runs the full
lifecycle: build, sign with your local .skey, submit, register and poll for
confirmation. --skey defaults to the 'wallet create' default wallet.

Use --dry-run to print the request body without building anything.

Find task indexes with: andamio project task list <project-id>

Examples:
  andamio project task mint <project-id> --alias manager-01 --index 1 --skey ./payment.skey
  andamio project task mint <project-id> --alias manager-01 --index 1 --index 2 --deposit-lovelace 10000000
  andamio project task mint <project-id> --alias manager-01 --index 3 --dry-run --output json`,
	Args: cobra.ExactArgs(1),
	RunE: runProjectTaskMint,
}

func init() {
	projectTaskCmd.AddCommand(projectTaskMintCmd)

	f := projectTaskMintCmd.Flags()
	f.String("alias", "", "Your Manager alias (required)")
	projectTaskMintCmd.MarkFlagRequired("alias")
	f.IntSlice("index", nil, "Index of a DRAFT task to mint (repeatable, required)")
	projectTaskMintCmd.MarkFlagRequired("index")
	f.Int64("deposit-lovelace", 0, "Lovelace to deposit into the project treasury with this transaction")
	f.StringArray("deposit-token", nil, `Native asset to deposit (repeatable, format: "policy_id,asset_name,quantity")`)
	addManageTxFlags(projectTaskMintCmd)
}

// taskDataV2 is one entry of ManageTasksTxRequest.tasks_to_add. No omitempty:
// every key is sent, native_assets as [] when there are none.
type taskDataV2 struct {
	ProjectContent  string          `json:"project_content"`
	ExpirationPosix int64           `json:"expiration_posix"`
	LovelaceAmount  int64           `json:"lovelace_amount"`
	NativeAssets    [][]interface{} `json:"native_assets"`
}

// manageTasksBody is ManageTasksTxRequest. Every key is always sent.
type manageTasksBody struct {
	Alias              string          `json:"alias"`
	ProjectID          string          `json:"project_id"`
	ContributorStateID string          `json:"contributor_state_id"`
	TasksToAdd         []taskDataV2    `json:"tasks_to_add"`
	TasksToRemove      []taskDataV2    `json:"tasks_to_remove"`
	DepositValue       [][]interface{} `json:"deposit_value"`
}

// MintTaskSummary names each task in a mint body by its draft index, with the
// task hash its data produces on chain.
type MintTaskSummary struct {
	TaskIndex      int    `json:"task_index"`
	TaskHash       string `json:"task_hash"`
	LovelaceAmount int64  `json:"lovelace_amount"`
}

// mintTaskAsset is one native asset of a draft task reward.
type mintTaskAsset struct {
	PolicyID  string
	AssetName string // hex
	Quantity  int64
}

// mintTaskSource is what task mint reads from one draft task.
type mintTaskSource struct {
	Index           int
	Status          string
	Title           string
	ExpirationPosix int64 // milliseconds
	LovelaceAmount  int64
	Assets          []mintTaskAsset
}

// assetClass renders a native asset the way the tx API names one:
// "<policy_id>.<asset_name_hex>", or the bare policy id for an empty name.
func assetClass(policyID, assetNameHex string) string {
	if assetNameHex == "" {
		return policyID
	}
	return policyID + "." + assetNameHex
}

// buildManageTasksBody builds the tasks_manage body that mints the given
// draft tasks and deposits the given value.
func buildManageTasksBody(alias, projectID, contributorStateID string, tasks []mintTaskSource, deposit [][]interface{}) (manageTasksBody, []MintTaskSummary, error) {
	if contributorStateID == "" {
		return manageTasksBody{}, nil, fmt.Errorf("project %s has no contributor_state_id (may not be on-chain yet)", projectID)
	}
	if deposit == nil {
		deposit = [][]interface{}{}
	}
	body := manageTasksBody{
		Alias:              alias,
		ProjectID:          projectID,
		ContributorStateID: contributorStateID,
		TasksToAdd:         make([]taskDataV2, 0, len(tasks)),
		TasksToRemove:      []taskDataV2{},
		DepositValue:       deposit,
	}
	summaries := make([]MintTaskSummary, 0, len(tasks))

	for _, t := range tasks {
		if !strings.EqualFold(t.Status, "DRAFT") {
			status := t.Status
			if status == "" {
				status = "unknown"
			}
			return manageTasksBody{}, nil, fmt.Errorf("task %d is %s; only DRAFT tasks can be minted", t.Index, status)
		}
		if t.Title == "" {
			return manageTasksBody{}, nil, fmt.Errorf("task %d has no title, which is its on-chain content", t.Index)
		}
		if n := utf8.RuneCountInString(t.Title); n > maxProjectContentLength {
			return manageTasksBody{}, nil, fmt.Errorf("task %d: its title is %d characters and the on-chain limit is %d. Shorten it with: andamio project task update %d --project-id %s --title \"...\"", t.Index, n, maxProjectContentLength, t.Index, projectID)
		}
		if t.ExpirationPosix <= 0 {
			return manageTasksBody{}, nil, fmt.Errorf("task %d has no expiration", t.Index)
		}
		if t.LovelaceAmount < 0 {
			return manageTasksBody{}, nil, fmt.Errorf("task %d has a negative lovelace amount", t.Index)
		}

		assets := make([][]interface{}, 0, len(t.Assets))
		hashAssets := make([]cardano.NativeAsset, 0, len(t.Assets))
		for _, a := range t.Assets {
			if a.Quantity < 0 {
				return manageTasksBody{}, nil, fmt.Errorf("task %d has a negative quantity for asset %s", t.Index, assetClass(a.PolicyID, a.AssetName))
			}
			assets = append(assets, []interface{}{assetClass(a.PolicyID, a.AssetName), a.Quantity})
			hashAssets = append(hashAssets, cardano.NativeAsset{PolicyID: a.PolicyID, TokenName: a.AssetName, Quantity: uint64(a.Quantity)})
		}

		body.TasksToAdd = append(body.TasksToAdd, taskDataV2{
			ProjectContent:  t.Title,
			ExpirationPosix: t.ExpirationPosix,
			LovelaceAmount:  t.LovelaceAmount,
			NativeAssets:    assets,
		})

		hash, err := cardano.ComputeTaskHash(cardano.TaskData{
			ProjectContent: t.Title,
			ExpirationTime: uint64(t.ExpirationPosix),
			LovelaceAmount: uint64(t.LovelaceAmount),
			NativeAssets:   hashAssets,
		})
		if err != nil {
			return manageTasksBody{}, nil, fmt.Errorf("task %d: %w", t.Index, err)
		}
		summaries = append(summaries, MintTaskSummary{TaskIndex: t.Index, TaskHash: hash, LovelaceAmount: t.LovelaceAmount})
	}
	return body, summaries, nil
}

// buildDepositValue turns --deposit-lovelace and --deposit-token into a
// deposit_value list. Nothing to deposit is an empty list, never null.
func buildDepositValue(lovelace int64, tokens []TaskToken) ([][]interface{}, error) {
	if lovelace < 0 {
		return nil, fmt.Errorf("--deposit-lovelace must be non-negative, got %d", lovelace)
	}
	out := [][]interface{}{}
	if lovelace > 0 {
		out = append(out, []interface{}{"lovelace", lovelace})
	}
	for _, t := range tokens {
		q, err := strconv.ParseInt(t.Quantity, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid --deposit-token quantity %q", t.Quantity)
		}
		if q == 0 {
			continue
		}
		out = append(out, []interface{}{assetClass(t.PolicyID, t.AssetName), q})
	}
	return out, nil
}

// numberField reads an integer the gateway may send as a JSON number or a
// numeric string.
func numberField(m map[string]interface{}, name string) (int64, bool) {
	switch v := m[name].(type) {
	case float64:
		return int64(v), true
	case string:
		n, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
		if err == nil {
			return n, true
		}
	}
	return 0, false
}

// taskSourceFromItem reads one entry of the manager tasks list
// (MergedTaskListItem).
func taskSourceFromItem(item map[string]interface{}) (mintTaskSource, bool) {
	idx, ok := numberField(item, "task_index")
	if !ok {
		content, _ := item["content"].(map[string]interface{})
		if content == nil {
			return mintTaskSource{}, false
		}
		if idx, ok = numberField(content, "task_index"); !ok {
			return mintTaskSource{}, false
		}
	}
	src := mintTaskSource{Index: int(idx)}
	src.Status, _ = item["task_status"].(string)
	if content, ok := item["content"].(map[string]interface{}); ok {
		src.Title, _ = content["title"].(string)
	}
	src.LovelaceAmount, _ = numberField(item, "lovelace_amount")

	if exp, ok := numberField(item, "expiration_posix"); ok && exp > 0 {
		src.ExpirationPosix = exp
	} else if iso, ok := item["expiration"].(string); ok && iso != "" {
		if t, err := time.Parse(time.RFC3339, iso); err == nil {
			src.ExpirationPosix = t.UnixMilli()
		}
	}
	if src.ExpirationPosix > 0 && src.ExpirationPosix < millisecondEpochThreshold {
		src.ExpirationPosix *= 1000
	}

	if assets, ok := item["assets"].([]interface{}); ok {
		for _, raw := range assets {
			a, ok := raw.(map[string]interface{})
			if !ok {
				continue
			}
			policyID, _ := a["policy_id"].(string)
			name, _ := a["name"].(string)
			if name == "" {
				name, _ = a["asset_name"].(string)
			}
			qty, ok := numberField(a, "amount")
			if !ok {
				qty, _ = numberField(a, "quantity")
			}
			src.Assets = append(src.Assets, mintTaskAsset{
				PolicyID:  policyID,
				AssetName: hexEncodeAssetName(name),
				Quantity:  qty,
			})
		}
	}
	return src, true
}

func runProjectTaskMint(cmd *cobra.Command, args []string) error {
	projectID := args[0]
	alias, _ := cmd.Flags().GetString("alias")
	indexes, _ := cmd.Flags().GetIntSlice("index")
	depositLovelace, _ := cmd.Flags().GetInt64("deposit-lovelace")
	depositTokenFlags, _ := cmd.Flags().GetStringArray("deposit-token")
	dryRun, _ := cmd.Flags().GetBool("dry-run")
	isJSON := output.GetFormat() == output.FormatJSON

	if len(indexes) == 0 {
		return fmt.Errorf("--index is required. List tasks with: andamio project task list %s", projectID)
	}
	seen := make(map[int]bool, len(indexes))
	for _, i := range indexes {
		if seen[i] {
			return fmt.Errorf("task %d is named twice", i)
		}
		seen[i] = true
	}

	var depositTokens []TaskToken
	if len(depositTokenFlags) > 0 {
		var err error
		if depositTokens, err = parseTokenFlags(depositTokenFlags); err != nil {
			return fmt.Errorf("%s", strings.ReplaceAll(err.Error(), "--token", "--deposit-token"))
		}
	}
	deposit, err := buildDepositValue(depositLovelace, depositTokens)
	if err != nil {
		return err
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	c := client.New(cfg)
	ctx := cmd.Context()

	proj, err := resolveProject(ctx, c, args)
	if err != nil {
		return err
	}
	resp, err := fetchTasks(ctx, c, proj.ProjectID)
	if err != nil {
		return err
	}

	byIndex := make(map[int]mintTaskSource)
	for _, item := range extractTaskList(resp) {
		if src, ok := taskSourceFromItem(item); ok {
			if _, dup := byIndex[src.Index]; !dup {
				byIndex[src.Index] = src
			}
		}
	}
	sources := make([]mintTaskSource, 0, len(indexes))
	for _, i := range indexes {
		src, ok := byIndex[i]
		if !ok {
			if warning := metaWarning(resp); warning != "" {
				return fmt.Errorf("task %d not found in project %s, and the task list was degraded: %s", i, proj.ProjectID, warning)
			}
			return fmt.Errorf("task %d not found in project %s. List tasks with: andamio project task list %s", i, proj.ProjectID, proj.ProjectID)
		}
		sources = append(sources, src)
	}

	body, summaries, err := buildManageTasksBody(alias, proj.ProjectID, proj.ContributorStateID, sources, deposit)
	if err != nil {
		return err
	}

	var locked int64
	for _, s := range summaries {
		locked += s.LovelaceAmount
	}
	if !isJSON {
		fmt.Fprintf(os.Stderr, "Minting %d task(s) for project %s. Rewards locked: %d lovelace", len(summaries), proj.ProjectID, locked)
		if len(body.DepositValue) == 0 {
			fmt.Fprintf(os.Stderr, "; no deposit, so the treasury must cover them.\n")
		} else {
			fmt.Fprintf(os.Stderr, "; deposit: %v.\n", body.DepositValue)
		}
	}

	if dryRun {
		return printManageBodyPreview(ManageBodyPreview{
			DryRun:   true,
			Endpoint: tasksManageEndpoint,
			TxType:   tasksManageTxType,
			Body:     body,
			Tasks:    summaries,
		}, isJSON)
	}

	if !isJSON {
		for _, s := range summaries {
			fmt.Fprintf(os.Stderr, "  task %d: task_hash %s\n", s.TaskIndex, s.TaskHash)
		}
	}
	return runManageTx(cmd, cfg, c, tasksManageEndpoint, tasksManageTxType, proj.ProjectID, body, "Tasks minted.")
}
