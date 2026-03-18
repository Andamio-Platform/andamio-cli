package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/Andamio-Platform/andamio-cli/internal/config"
	"github.com/Andamio-Platform/andamio-cli/internal/output"
	"github.com/spf13/cobra"
)

// specHTTPClient is a dedicated client for spec fetching with proper timeout
var specHTTPClient = &http.Client{Timeout: 30 * time.Second}

var specCmd = &cobra.Command{
	Use:   "spec",
	Short: "Manage OpenAPI spec",
}

var specFetchCmd = &cobra.Command{
	Use:   "fetch",
	Short: "Fetch OpenAPI spec from the API",
	RunE: func(cmd *cobra.Command, args []string) error {
		isJSON := output.GetFormat() == output.FormatJSON

		cfg, err := config.Load()
		if err != nil {
			return err
		}

		specURL := cfg.BaseURL + "/api/v1/docs/doc.json"
		if !isJSON {
			fmt.Fprintf(os.Stderr, "Fetching spec from %s...\n", specURL)
		}

		resp, err := specHTTPClient.Get(specURL)
		if err != nil {
			return fmt.Errorf("failed to fetch spec: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			errMsg := string(body)
			if len(errMsg) > 500 {
				errMsg = errMsg[:500] + "... (truncated)"
			}
			return fmt.Errorf("API error %d: %s", resp.StatusCode, errMsg)
		}

		var spec map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&spec); err != nil {
			return fmt.Errorf("failed to parse spec: %w", err)
		}

		// Save to ./openapi.json
		outPath := "openapi.json"
		data, err := json.MarshalIndent(spec, "", "  ")
		if err != nil {
			return err
		}

		if err := os.WriteFile(outPath, data, 0644); err != nil {
			return err
		}

		info, _ := spec["info"].(map[string]interface{})
		apiVersion, _ := info["version"].(string)
		apiTitle, _ := info["title"].(string)

		if isJSON {
			return output.PrintJSON(map[string]interface{}{
				"path":        outPath,
				"api_version": apiVersion,
				"api_title":   apiTitle,
			})
		}

		fmt.Fprintf(os.Stderr, "Saved to %s\n", outPath)
		fmt.Fprintf(os.Stderr, "API: %s v%s\n", apiTitle, apiVersion)
		return nil
	},
}

type specPathEntry struct {
	Method  string `json:"method"`
	Path    string `json:"path"`
	Summary string `json:"summary"`
}

var specPathsCmd = &cobra.Command{
	Use:   "paths",
	Short: "List available API paths",
	RunE: func(cmd *cobra.Command, args []string) error {
		// Try local file first
		specPath := "openapi.json"
		data, err := os.ReadFile(specPath)
		if err != nil {
			if os.IsNotExist(err) {
				// Fetch from API
				cfg, err := config.Load()
				if err != nil {
					return err
				}

				specURL := cfg.BaseURL + "/api/v1/docs/doc.json"
				resp, err := specHTTPClient.Get(specURL)
				if err != nil {
					return fmt.Errorf("failed to fetch spec: %w", err)
				}
				defer resp.Body.Close()

				data, err = io.ReadAll(resp.Body)
				if err != nil {
					return err
				}
			} else {
				return err
			}
		}

		var spec map[string]interface{}
		if err := json.Unmarshal(data, &spec); err != nil {
			return err
		}

		paths, ok := spec["paths"].(map[string]interface{})
		if !ok {
			return fmt.Errorf("no paths found in spec")
		}

		filter, _ := cmd.Flags().GetString("filter")

		var entries []specPathEntry
		for path, methods := range paths {
			if filter != "" && !strings.Contains(path, filter) {
				continue
			}
			methodsMap, _ := methods.(map[string]interface{})
			for method, methodData := range methodsMap {
				summary := ""
				if md, ok := methodData.(map[string]interface{}); ok {
					summary, _ = md["summary"].(string)
				}
				entries = append(entries, specPathEntry{
					Method:  strings.ToUpper(method),
					Path:    path,
					Summary: summary,
				})
			}
		}

		sort.Slice(entries, func(i, j int) bool {
			if entries[i].Path != entries[j].Path {
				return entries[i].Path < entries[j].Path
			}
			return entries[i].Method < entries[j].Method
		})

		if output.GetFormat() == output.FormatJSON {
			return output.PrintJSON(entries)
		}

		for _, e := range entries {
			if e.Summary != "" {
				fmt.Printf("%s %s — %s\n", e.Method, e.Path, e.Summary)
			} else {
				fmt.Printf("%s %s\n", e.Method, e.Path)
			}
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(specCmd)
	specCmd.AddCommand(specFetchCmd)
	specCmd.AddCommand(specPathsCmd)

	specPathsCmd.Flags().String("filter", "", "Filter paths by pattern")
}
