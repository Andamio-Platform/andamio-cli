package output

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
)

// Format represents the output format type
type Format string

const (
	FormatText     Format = "text"
	FormatJSON     Format = "json"
	FormatCSV      Format = "csv"
	FormatMarkdown Format = "markdown"
)

var currentFormat Format = FormatText

// SetFormat sets the global output format
func SetFormat(f string) error {
	switch strings.ToLower(f) {
	case "text", "":
		currentFormat = FormatText
	case "json":
		currentFormat = FormatJSON
	case "csv":
		currentFormat = FormatCSV
	case "markdown", "md":
		currentFormat = FormatMarkdown
	default:
		return fmt.Errorf("unsupported format: %s (use text, json, csv, or markdown)", f)
	}
	return nil
}

// GetFormat returns the current output format
func GetFormat() Format {
	return currentFormat
}

// PrintJSON outputs data based on the current format
func PrintJSON(data interface{}) error {
	switch currentFormat {
	case FormatJSON:
		return printAsJSON(data)
	case FormatCSV:
		return printAsCSV(data, os.Stdout)
	case FormatMarkdown:
		return printAsMarkdown(data)
	default:
		return printAsJSON(data)
	}
}

// PrintList outputs a list of items with title and id fields
func PrintList(items []map[string]interface{}, titleKey, idKey string) error {
	switch currentFormat {
	case FormatJSON:
		return printAsJSON(items)
	case FormatCSV:
		return printListAsCSV(items, titleKey, idKey)
	case FormatMarkdown:
		return printListAsMarkdown(items, titleKey, idKey)
	default:
		return printListAsText(items, titleKey, idKey)
	}
}

func printAsJSON(data interface{}) error {
	output, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(output))
	return nil
}

func printListAsText(items []map[string]interface{}, titleKey, idKey string) error {
	if len(items) == 0 {
		return nil
	}

	// Derive column headers from the key names (last segment of dot notation)
	titleHeader := titleKey
	if i := strings.LastIndex(titleHeader, "."); i >= 0 {
		titleHeader = titleHeader[i+1:]
	}
	idHeader := idKey

	// Find max widths
	titleWidth := len(titleHeader)
	idWidth := len(idHeader)
	for _, item := range items {
		if w := len(getNestedString(item, titleKey)); w > titleWidth {
			titleWidth = w
		}
		if w := len(getNestedString(item, idKey)); w > idWidth {
			idWidth = w
		}
	}

	// Print header
	fmt.Printf("%-*s  %s\n", titleWidth, strings.ToUpper(titleHeader), strings.ToUpper(idHeader))
	fmt.Printf("%-*s  %s\n", titleWidth, strings.Repeat("─", titleWidth), strings.Repeat("─", idWidth))

	// Print rows
	for _, item := range items {
		title := getNestedString(item, titleKey)
		id := getNestedString(item, idKey)
		fmt.Printf("%-*s  %s\n", titleWidth, title, id)
	}
	return nil
}

func printListAsCSV(items []map[string]interface{}, titleKey, idKey string) error {
	w := csv.NewWriter(os.Stdout)
	defer w.Flush()

	if err := w.Write([]string{"title", "id"}); err != nil {
		return err
	}

	for _, item := range items {
		title := getNestedString(item, titleKey)
		id := getNestedString(item, idKey)
		if err := w.Write([]string{title, id}); err != nil {
			return err
		}
	}
	return nil
}

func printListAsMarkdown(items []map[string]interface{}, titleKey, idKey string) error {
	fmt.Println("| Title | ID |")
	fmt.Println("|-------|-----|")
	for _, item := range items {
		title := getNestedString(item, titleKey)
		id := getNestedString(item, idKey)
		fmt.Printf("| %s | %s |\n", title, id)
	}
	return nil
}

func printAsCSV(data interface{}, w io.Writer) error {
	writer := csv.NewWriter(w)
	defer writer.Flush()

	switch v := data.(type) {
	case map[string]interface{}:
		return printMapAsCSV(v, writer)
	case []interface{}:
		if len(v) == 0 {
			return nil
		}
		return printSliceAsCSV(v, writer)
	default:
		return printAsJSON(data)
	}
}

func printMapAsCSV(m map[string]interface{}, w *csv.Writer) error {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	if err := w.Write(keys); err != nil {
		return err
	}

	values := make([]string, len(keys))
	for i, k := range keys {
		values[i] = fmt.Sprintf("%v", m[k])
	}
	return w.Write(values)
}

func printSliceAsCSV(items []interface{}, w *csv.Writer) error {
	if len(items) == 0 {
		return nil
	}

	first, ok := items[0].(map[string]interface{})
	if !ok {
		return printAsJSON(items)
	}

	keys := make([]string, 0, len(first))
	for k := range first {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	if err := w.Write(keys); err != nil {
		return err
	}

	for _, item := range items {
		m, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		values := make([]string, len(keys))
		for i, k := range keys {
			values[i] = fmt.Sprintf("%v", m[k])
		}
		if err := w.Write(values); err != nil {
			return err
		}
	}
	return nil
}

func printAsMarkdown(data interface{}) error {
	switch v := data.(type) {
	case map[string]interface{}:
		return printMapAsMarkdown(v)
	case []interface{}:
		return printSliceAsMarkdown(v)
	default:
		return printAsJSON(data)
	}
}

func printMapAsMarkdown(m map[string]interface{}) error {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	fmt.Println("| Key | Value |")
	fmt.Println("|-----|-------|")
	for _, k := range keys {
		fmt.Printf("| %s | %v |\n", k, m[k])
	}
	return nil
}

func printSliceAsMarkdown(items []interface{}) error {
	if len(items) == 0 {
		return nil
	}

	first, ok := items[0].(map[string]interface{})
	if !ok {
		return printAsJSON(items)
	}

	keys := make([]string, 0, len(first))
	for k := range first {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	fmt.Print("|")
	for _, k := range keys {
		fmt.Printf(" %s |", k)
	}
	fmt.Println()

	fmt.Print("|")
	for range keys {
		fmt.Print("-----|")
	}
	fmt.Println()

	for _, item := range items {
		m, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		fmt.Print("|")
		for _, k := range keys {
			fmt.Printf(" %v |", m[k])
		}
		fmt.Println()
	}
	return nil
}

// getNestedString extracts a string value, supporting dot notation for nested keys
func getNestedString(m map[string]interface{}, key string) string {
	parts := strings.Split(key, ".")
	current := m

	for i, part := range parts {
		val, ok := current[part]
		if !ok {
			return ""
		}

		if i == len(parts)-1 {
			if s, ok := val.(string); ok {
				return s
			}
			return fmt.Sprintf("%v", val)
		}

		current, ok = val.(map[string]interface{})
		if !ok {
			return ""
		}
	}
	return ""
}
