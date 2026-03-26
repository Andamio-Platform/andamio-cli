package main

import (
	"fmt"
	"net/url"
	"os"

	"github.com/Andamio-Platform/andamio-cli/internal/cardano"
	"github.com/Andamio-Platform/andamio-cli/internal/client"
	"github.com/Andamio-Platform/andamio-cli/internal/config"
	"github.com/Andamio-Platform/andamio-cli/internal/output"
	"github.com/spf13/cobra"
)

var courseCredentialCmd = &cobra.Command{
	Use:   "credential",
	Short: "Credential verification commands",
}

var courseCredentialVerifyHashCmd = &cobra.Command{
	Use:   "verify-hash <course-id>",
	Short: "Verify credential hashes match computed SLT hashes",
	Long: `Compute SLT hashes locally and compare against API-returned slt_hash values.

For each on-chain module, collects the SLT texts, encodes them as Plutus Data
CBOR (matching the on-chain validator), hashes with Blake2b-256, and compares
against the slt_hash stored in the API. Reports any mismatches.

Requires an API key or user authentication.

Examples:
  andamio course credential verify-hash <course-id>
  andamio course credential verify-hash <course-id> --output json`,
	Args: cobra.ExactArgs(1),
	RunE: runCredentialVerifyHash,
}

func init() {
	courseCmd.AddCommand(courseCredentialCmd)
	courseCredentialCmd.AddCommand(courseCredentialVerifyHashCmd)
}

func runCredentialVerifyHash(cmd *cobra.Command, args []string) error {
	courseID := args[0]
	isJSON := output.GetFormat() == output.FormatJSON

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	c := client.New(cfg)

	path := "/api/v2/course/user/modules/" + url.PathEscape(courseID)
	var resp map[string]interface{}
	if err := c.Get(path, &resp); err != nil {
		return fmt.Errorf("failed to list modules: %w", err)
	}

	modules, ok := resp["data"].([]interface{})
	if !ok || len(modules) == 0 {
		if isJSON {
			return output.PrintJSON(map[string]interface{}{"results": []interface{}{}, "mismatches": 0})
		}
		fmt.Fprintln(os.Stderr, "No modules found.")
		return nil
	}

	type verifyResult struct {
		SltHash      string   `json:"slt_hash"`
		ComputedHash string   `json:"computed_hash"`
		Match        bool     `json:"match"`
		SLTCount     int      `json:"slt_count"`
		SLTs         []string `json:"slts"`
		Error        string   `json:"error,omitempty"`
	}

	var results []verifyResult
	mismatches := 0

	for _, item := range modules {
		m, ok := item.(map[string]interface{})
		if !ok {
			continue
		}

		apiHash, _ := m["slt_hash"].(string)
		if apiHash == "" {
			continue // skip modules not yet on-chain
		}

		// on_chain_slts is a string array from the user endpoint
		var sltTexts []string
		if slts, ok := m["on_chain_slts"].([]interface{}); ok {
			for _, s := range slts {
				if text, ok := s.(string); ok {
					sltTexts = append(sltTexts, text)
				}
			}
		}

		r := verifyResult{
			SltHash:  apiHash,
			SLTCount: len(sltTexts),
			SLTs:     sltTexts,
		}

		if len(sltTexts) == 0 {
			r.Error = "no SLT texts found"
		} else {
			r.ComputedHash = cardano.ComputeSltHash(sltTexts)
			r.Match = r.ComputedHash == apiHash
		}

		if !r.Match {
			mismatches++
		}

		results = append(results, r)
	}

	if isJSON {
		return output.PrintJSON(map[string]interface{}{
			"results":    results,
			"total":      len(results),
			"mismatches": mismatches,
		})
	}

	// Text output
	for _, r := range results {
		status := "\u2713"
		if !r.Match {
			status = "\u2717"
		}
		label := r.SltHash
		if len(label) > 16 {
			label = label[:16] + "..."
		}
		fmt.Printf("%s %s (%d SLTs)\n", status, label, r.SLTCount)
		if r.Error != "" {
			fmt.Printf("    Error: %s\n", r.Error)
			continue
		}
		if !r.Match {
			fmt.Printf("    API hash:      %s\n", r.SltHash)
			fmt.Printf("    Computed hash:  %s\n", r.ComputedHash)
		}
	}

	if mismatches > 0 {
		fmt.Fprintf(os.Stderr, "\n%d of %d credentials have hash mismatches.\n", mismatches, len(results))
	} else if len(results) > 0 {
		fmt.Fprintf(os.Stderr, "\nAll %d credential hashes verified.\n", len(results))
	}

	return nil
}
