package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/Andamio-Platform/andamio-cli/internal/config"
	"github.com/spf13/cobra"
)

var specCmd = &cobra.Command{
	Use:   "spec",
	Short: "Manage OpenAPI spec",
}

var specFetchCmd = &cobra.Command{
	Use:   "fetch",
	Short: "Fetch OpenAPI spec from the API",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return err
		}

		specURL := cfg.BaseURL + "/api/v1/docs/doc.json"
		fmt.Printf("Fetching spec from %s...\n", specURL)

		resp, err := http.Get(specURL)
		if err != nil {
			return fmt.Errorf("failed to fetch spec: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			return fmt.Errorf("API error %d: %s", resp.StatusCode, string(body))
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

		// Extract metadata
		info, _ := spec["info"].(map[string]interface{})
		version := info["version"]
		title := info["title"]

		fmt.Printf("Saved to %s\n", outPath)
		fmt.Printf("API: %s v%s\n", title, version)

		return nil
	},
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
				resp, err := http.Get(specURL)
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

		for path, methods := range paths {
			if filter != "" && !strings.Contains(path, filter) {
				continue
			}
			methodsMap, _ := methods.(map[string]interface{})
			for method := range methodsMap {
				fmt.Printf("%s %s\n", method, path)
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
