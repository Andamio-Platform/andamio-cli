---
title: "Refactoring Go CLI commands to reduce boilerplate and improve scalability"
date: 2026-03-16
category: architecture
tags:
  - refactoring
  - go
  - cli
  - cobra
  - dry-principle
  - maintainability
components:
  - cmd/andamio/teacher.go
  - cmd/andamio/course.go
  - cmd/andamio/project.go
symptoms:
  - Duplicated config loading and client initialization across commands
  - Per-command authentication checks scattered throughout codebase
  - Repetitive list formatting logic in multiple command files
  - Growing maintenance burden as new commands are added
root_cause: "Initial command implementations used copy-paste patterns without extracting common functionality into shared helpers or utilizing Cobra's PersistentPreRunE hooks for cross-cutting concerns"
severity: medium
time_to_resolve: "30-60 minutes"
---

# Refactoring Go CLI Commands for Scalability

## Problem

The CLI had significant code duplication across list commands. Each command repeated the same boilerplate pattern:

1. Load config
2. Create HTTP client
3. Make GET/POST request
4. Extract `data` array from response
5. Handle empty results
6. Convert to typed slice
7. Print with output formatter

Additionally, commands requiring authentication (like teacher operations) had to individually check for valid user credentials, leading to scattered auth logic.

**Symptoms observed:**
- 20+ lines of nearly identical code in `course.go`, `project.go`, and `teacher.go`
- Per-command `HasUserAuth()` checks that would need duplication for each new endpoint
- Three-level command hierarchies (`teacher courses list`) when two levels sufficed

## Solution

### 1. Extracted `printList` Helper Function

A single helper in `cmd/andamio/course.go` encapsulates the entire list-fetch-print pattern:

```go
func printList(path, emptyMsg, titleKey, idKey string, usePost bool) error {
    cfg, err := config.Load()
    if err != nil {
        return err
    }
    c := client.New(cfg)
    var response map[string]interface{}
    var reqErr error
    if usePost {
        reqErr = c.Post(path, nil, &response)
    } else {
        reqErr = c.Get(path, &response)
    }
    if reqErr != nil {
        return reqErr
    }
    data, ok := response["data"].([]interface{})
    if !ok || len(data) == 0 {
        fmt.Println(emptyMsg)
        return nil
    }
    items := make([]map[string]interface{}, 0, len(data))
    for _, item := range data {
        if m, ok := item.(map[string]interface{}); ok {
            items = append(items, m)
        }
    }
    return output.PrintList(items, titleKey, idKey)
}
```

**Parameters:**
- `path` - API endpoint path
- `emptyMsg` - Message shown when no results found
- `titleKey` - Dot-notation key for display title (e.g., `"content.title"`)
- `idKey` - Dot-notation key for item ID
- `usePost` - Whether to use POST instead of GET

### 2. Centralized Authentication via `PersistentPreRunE`

Instead of checking auth in every command, the parent command validates authentication once. All subcommands inherit this check automatically:

```go
var teacherCmd = &cobra.Command{
    Use:   "teacher",
    Short: "Teacher operations (requires user login)",
    PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
        // Chain to root's PersistentPreRunE (sets output format)
        if err := rootCmd.PersistentPreRunE(cmd, args); err != nil {
            return err
        }
        // Auth check - runs once, applies to all subcommands
        cfg, err := config.Load()
        if err != nil {
            return err
        }
        if !cfg.HasUserAuth() {
            return fmt.Errorf("not authenticated. Run 'andamio user login' first")
        }
        return nil
    },
}
```

### 3. Flattened Command Hierarchy

Commands were simplified from three levels to two:

| Before | After |
|--------|-------|
| `teacher courses list` | `teacher courses` |
| `teacher assignments list` | `teacher assignments` |

## Impact

**Before refactoring** - a typical list command (~20 lines):
```go
var teacherCoursesListCmd = &cobra.Command{
    Use:   "list",
    Short: "List courses you teach",
    RunE: func(cmd *cobra.Command, args []string) error {
        cfg, err := config.Load()
        if err != nil {
            return err
        }
        if !cfg.HasUserAuth() {
            return fmt.Errorf("not authenticated")
        }
        c := client.New(cfg)
        var response map[string]interface{}
        if err := c.Post("/api/v2/course/teacher/courses/list", &response); err != nil {
            return err
        }
        data, ok := response["data"].([]interface{})
        if !ok || len(data) == 0 {
            fmt.Println("No courses found")
            return nil
        }
        items := make([]map[string]interface{}, 0, len(data))
        for _, item := range data {
            if m, ok := item.(map[string]interface{}); ok {
                items = append(items, m)
            }
        }
        return output.PrintList(items, "content.title", "course_id")
    },
}
```

**After refactoring** - same functionality (~5 lines):
```go
var teacherCoursesCmd = &cobra.Command{
    Use:   "courses",
    Short: "List courses you teach",
    RunE: func(cmd *cobra.Command, args []string) error {
        return printList(
            "/api/v2/course/teacher/courses/list",
            "No courses found where you are a teacher.",
            "content.title", "course_id", true,
        )
    },
}
```

**Results:**
- **75% reduction** in per-command code (20 lines to 5 lines)
- **Single point of auth enforcement** - no more scattered `HasUserAuth()` checks
- **24 lines removed** from `project.go` alone
- **Simpler command structure** - fewer nested subcommands to navigate

## Prevention

### Guidelines for Adding New Commands

**1. Use existing helpers first**

Before writing any new command code, check if an existing helper handles your use case:

| Use Case | Helper |
|----------|--------|
| Simple GET requests | `getJSON(path)` |
| Simple POST requests | `postJSON(path)` |
| List endpoints with formatted output | `printList(path, emptyMsg, titleKey, idKey, usePost)` |

**2. Never duplicate these patterns**

- Config loading (`config.Load()`)
- Client instantiation (`client.New(cfg)`)
- Output formatting (`output.PrintJSON()`, `output.PrintList()`)
- Auth header injection (handled automatically by client)

### When to Create Helpers vs Inline Code

**Create a helper when:**
- The same 3+ lines appear in more than one command
- The logic involves error handling that should be consistent
- The pattern will likely be reused by future commands

**Keep inline when:**
- The logic is specific to one command's business requirements
- Adding a helper would require passing 4+ parameters

### Checklist for Reviewing New CLI Commands

**Structure:**
- [ ] Command registered via `init()` function
- [ ] Command hierarchy is max 2 levels deep
- [ ] Uses existing helpers where applicable
- [ ] No duplicated config/client/output boilerplate

**Auth & Security:**
- [ ] Auth requirements handled by `PersistentPreRunE` on parent command
- [ ] Sensitive data never logged or printed
- [ ] User input validated before use in URLs

**Output:**
- [ ] Respects global `--output` flag
- [ ] Uses `output.PrintJSON()` for single objects
- [ ] Uses `output.PrintList()` for collections
- [ ] Error messages are actionable

### Best Practices for Cobra Command Structure

**1. Flat over deep**
```go
// GOOD: 2 levels
rootCmd.AddCommand(courseCmd)
courseCmd.AddCommand(courseListCmd)

// AVOID: 3+ levels without purpose
```

**2. Group related auth in PersistentPreRunE**
```go
var teacherCmd = &cobra.Command{
    Use: "teacher",
    PersistentPreRunE: requireUserAuth,
}
```

**3. Keep Run functions small** - delegate to helpers

**4. Return errors from RunE** rather than calling `os.Exit()` directly

## Related Documentation

- [CLI Output Format Flag](../feature-implementations/cli-output-format-flag.md) - Documents the `PersistentPreRunE` pattern for middleware-style output format handling
- [CLI API Auth Middleware](../integration-issues/cli-api-auth-middleware-mismatch.md) - Documents the `postJSON` helper function
- [CLI Security Hardening](../security-issues/cli-security-hardening-input-validation.md) - Documents URL path escaping patterns
- [CLAUDE.md](/CLAUDE.md) - Primary architecture guide covering package layout and command patterns

## Key Helpers Reference

```go
// course.go - Centralized helpers

getJSON(path string) error                                              // Simple GET endpoints
postJSON(path string) error                                             // Simple POST endpoints (no body)
printList(path, emptyMsg, titleKey, idKey string, usePost bool) error   // List formatting
```
