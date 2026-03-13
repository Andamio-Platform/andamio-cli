---
title: Add output format flag to Andamio CLI for multiple formats (JSON, CSV, Markdown, text)
date: 2026-03-13
problem_type: feature_implementation
component: andamio-cli
tags: [cli, go, cobra, output-formatting, data-export]
severity: enhancement
symptoms:
  - CLI only supported plaintext output
  - No way to export data in machine-readable formats for scripting/integration
  - Users unable to pipe CLI output to other tools requiring JSON or CSV
root_cause: Missing output format abstraction and formatter implementation
resolution_time: ~15 minutes
implementation_files:
  - internal/output/output.go (new)
  - cmd/andamio/main.go (modified)
  - cmd/andamio/course.go (modified)
  - cmd/andamio/project.go (modified)
key_features:
  - Global -o/--output flag supporting text|json|csv|markdown
  - Format-specific printers for different data types
  - Nested key path support for extracting fields (e.g., content.title)
  - List command formatting with headers and values
status: completed
---

# CLI Output Format Flag Implementation

## Problem

The Andamio CLI needed to support multiple output formats beyond plain text to enable better integration with scripts, data pipelines, and human-readable documentation.

## Solution

Created a centralized output package with a global persistent flag that applies to all commands.

### Implementation

1. **New output package** (`internal/output/output.go`):
   - Format enum: `text`, `json`, `csv`, `markdown`
   - `SetFormat(string) error` - parses format string
   - `PrintJSON(interface{}) error` - outputs raw data in selected format
   - `PrintList(items, titleKey, idKey)` - outputs lists with title/id columns
   - Supports nested key extraction via dot notation ("content.title")

2. **Global flag in main.go**:
   ```go
   rootCmd.PersistentFlags().StringVarP(&outputFormat, "output", "o", "text",
       "Output format: text, json, csv, markdown")
   ```

3. **PersistentPreRunE hook** sets format before any command runs

4. **List commands** use `output.PrintList(items, "content.title", "course_id")`

5. **Detail commands** use `output.PrintJSON(result)`

## Usage Examples

```bash
# Default text format
./andamio course list
# - My First Course (abc123)
# - Advanced Go Programming (def456)

# JSON format
./andamio course list -o json
# [{"course_id": "abc123", "content": {"title": "My First Course"}}, ...]

# CSV format
./andamio course list -o csv
# title,id
# My First Course,abc123
# Advanced Go Programming,def456

# Markdown format
./andamio course list -o markdown
# | Title | ID |
# |-------|-----|
# | My First Course | abc123 |
```

## Best Practices for Future Commands

### List Commands
Always use the centralized formatter:
```go
items := make([]map[string]interface{}, 0, len(data))
for _, item := range data {
    if course, ok := item.(map[string]interface{}); ok {
        items = append(items, course)
    }
}
return output.PrintList(items, "content.title", "course_id")
```

### Detail Commands
Use the getJSON helper or PrintJSON directly:
```go
return output.PrintJSON(result)
```

### Nested Key Support
Dot notation works for extracting nested values:
```go
output.PrintList(items, "user.profile.name", "id")
output.PrintList(items, "metadata.content.title", "uuid")
```

## Testing Checklist

- [ ] Test all four output formats for each command
- [ ] Verify CSV escaping for commas and quotes
- [ ] Verify markdown table formatting
- [ ] Test with empty result sets
- [ ] Test with special characters (emoji, unicode)

## Extension Points

### Adding New Formats (e.g., YAML)

1. Add constant in `internal/output/output.go`
2. Update `SetFormat()` switch statement
3. Implement formatter function
4. Update dispatcher functions
5. Update help text in `main.go`

### Customizing List Columns

For commands needing more than title/id:
```go
func PrintListWithColumns(items []map[string]interface{}, columns map[string]string) error
```

## Related Documentation

- [README.md](/README.md) - CLI usage documentation
- [CLAUDE.md](/CLAUDE.md) - Developer architecture guide
- [Getting Started Skill](/.claude/skills/getting-started/SKILL.md) - Interactive walkthrough
