---
title: "Remove interactive stdin picker to maximize CLI composability"
date: 2026-03-18
module: [cmd/andamio, project-task-commands, resolveProject]
tags: [cobra, cli, composability, stdin, non-interactive, ci, scripting, stdout-stderr, pipes]
problem_type: architecture
symptoms:
  - "project task list/create/export/import hang indefinitely in CI and bash scripts"
  - "commands block waiting for stdin input when project-id is not provided"
  - "progress messages mixed into stdout, breaking jq and other downstream pipe consumers"
  - "cobra.MaximumNArgs(1) allowed omitting project-id, silently triggering interactive picker"
---

# Remove Interactive Stdin Picker to Maximize CLI Composability

## Problem

Four of seven `project task` commands (`list`, `create`, `export`, `import`) had an interactive stdin picker that activated when `<project-id>` was omitted. The picker used `bufio.NewScanner(os.Stdin)`, which blocks on any non-TTY context — CI pipelines, `$(...)` subshells, agent tool calls, bash scripts.

Simultaneously, progress messages ("Exporting 3 tasks...", "Task created.") were written to `os.Stdout` instead of `os.Stderr`, polluting the data stream for pipe consumers.

The team identified composability as a key deliverable: the CLI must work in bash scripts alongside `gh` and `jq` with no human interaction required.

### Symptoms

- `andamio project task list | jq .` hangs indefinitely (waiting for picker input)
- `PROJECT_ID=$(andamio project task export ...)` captures progress strings along with any IDs
- Commands in CI exit with `"no input received"` error (empty stdin with no TTY)
- `--output json` didn't help — the picker printed before any format check ran

## Root Cause

`resolveProject()` in `cmd/andamio/project_task.go` mixed two concerns: argument resolution and discoverability. When `args[0]` was absent it fell through to a `bufio.Scanner` picker:

```go
// BEFORE: interactive picker triggered on missing arg
if len(args) == 0 {
    fmt.Println("Your managed projects:")
    for i, p := range projects {
        fmt.Printf("  %d. %s  [%s]\n", i+1, title, p.ProjectID)
    }
    fmt.Print("\nSelect project number: ")
    scanner := bufio.NewScanner(os.Stdin) // blocks forever without TTY
    ...
}
```

`cobra.MaximumNArgs(1)` allowed Cobra to accept zero arguments, which meant the picker path was always reachable. And progress messages used bare `fmt.Printf` (stdout) with no stderr alternative.

## Solution

Three targeted changes, no new dependencies.

### 1. Rewrite `resolveProject()` — remove picker, fail fast with guidance

```go
// AFTER: fail fast, teach the user how to discover IDs
func resolveProject(c *client.Client, args []string) (*managerProject, []managerProject, error) {
    projects, err := fetchManagerProjects(c)
    if err != nil {
        return nil, nil, err
    }
    if len(projects) == 0 {
        return nil, nil, fmt.Errorf("no managed projects found")
    }
    for i := range projects {
        if projects[i].ProjectID == args[0] {
            return &projects[i], projects, nil
        }
    }
    return nil, nil, fmt.Errorf(
        "project %s not found in your managed projects\n\nList your projects with:\n  andamio project list --output json",
        args[0],
    )
}
```

The `bufio` and `strings` imports (only used by the picker) were removed entirely.

### 2. Enforce required args at the Cobra level — `ExactArgs(1)` + `<project-id>` in Use string

```go
// BEFORE
var projectTaskListCmd = &cobra.Command{
    Use:  "list [project-id]",     // square brackets = optional
    Args: cobra.MaximumNArgs(1),   // 0 or 1 accepted → picker activates at 0
    ...
}

// AFTER
var projectTaskListCmd = &cobra.Command{
    Use:  "list <project-id>",     // angle brackets = required, shows in --help
    Args: cobra.ExactArgs(1),      // Cobra rejects 0 args before RunE is called
    Long: `...
Find your project IDs with: andamio project list --output json`,
    ...
}
```

Applied to all four affected commands: `list`, `create`, `export`, `import`.

### 3. Route all progress messages to stderr

```go
// BEFORE: progress to stdout, contaminating pipes
fmt.Printf("Exporting %d tasks to %s/\n", len(items), outDir)
fmt.Printf("  %s\n", filename)
fmt.Printf("Task created successfully.\n")

// AFTER: progress to stderr, gated on !isJSON
if !isJSON {
    fmt.Fprintf(os.Stderr, "Exporting %d tasks to %s/\n", len(items), outDir)
    fmt.Fprintf(os.Stderr, "  %s\n", filename)
}
fmt.Fprintf(os.Stderr, "Task created successfully.\n")
```

The `!isJSON` gate means `--output json` also silences stderr — the caller gets only structured JSON on stdout, nothing else.

Applied across `project_task.go`, `project_task_export.go`, `project_task_import.go`.

## Composable Workflow (After Fix)

```bash
#!/usr/bin/env bash
set -euo pipefail

# Step 1: Discover project IDs — pure data on stdout, no TTY needed
PROJECT_ID=$(andamio project list --output json | jq -r '.data[0].project_id')

# Step 2: List tasks — progress to stderr, JSON data to stdout
andamio project task list "$PROJECT_ID" --output json | jq '.data[].content.title'

# Step 3: Create tasks from GitHub issues — works in CI
gh issue list --repo org/repo --json number,title --jq '.[]' | while IFS= read -r issue; do
  NUMBER=$(echo "$issue" | jq -r '.number')
  TITLE=$(echo "$issue" | jq -r '.title')
  andamio project task create "$PROJECT_ID" \
    --title "$TITLE" \
    --github-issue "org/repo#$NUMBER" \
    --lovelace 5000000 \
    --expiration 2026-06-01
done

# Step 4: Export, edit, reimport
andamio project task export "$PROJECT_ID"
andamio project task import "$PROJECT_ID" --dry-run
andamio project task import "$PROJECT_ID"
```

All steps work without a TTY. Progress narration stays on stderr and doesn't appear in captured output.

## Composability Rules (Now in CLAUDE.md)

These rules are codified in `CLAUDE.md` under **Composability Rules**:

1. **No interactive pickers.** If a required argument is omitted, return an error that tells the user how to discover valid values. Use `cobra.ExactArgs(N)` to enforce at the framework level.
2. **Progress to stderr.** Use `fmt.Fprintf(os.Stderr, ...)`. Gate with `if !isJSON` to suppress in JSON mode.
3. **Data to stdout only.** Structured output (tables, JSON, CSV, Markdown) via the `output` package — nothing else touches stdout.
4. **Required args are required.** Never use `cobra.MaximumNArgs` for args the command cannot function without.
5. **`--output json` is the scripting surface.** All list/get commands must support it with stable schemas.

## Prevention

### Adding new commands — checklist

- [ ] Required arguments use `cobra.ExactArgs(N)` — never `cobra.MaximumNArgs` paired with a runtime picker
- [ ] `Use:` string uses `<angle-brackets>` for required args, `[square-brackets]` only for truly optional ones
- [ ] All progress/status text goes to `fmt.Fprintf(os.Stderr, ...)`
- [ ] Structured output goes to stdout via `output.PrintJSON` or `output.PrintList`
- [ ] `--output json` is implemented and the output is clean (`| jq .` works without error)
- [ ] `if !isJSON` gate wraps all stderr progress to suppress in JSON mode
- [ ] Command works when piped: `andamio <cmd> <args> | wc -l` should not hang

### Code review — immediate red flags

```go
bufio.NewReader(os.Stdin)   // interactive prompt — never in production commands
fmt.Scan(                    // blocks stdin
fmt.Printf("Fetching...")   // progress to stdout — use Fprintf(os.Stderr, ...)
cobra.MaximumNArgs(1)       // paired with a picker → ban
cobra.ArbitraryArgs         // almost always wrong for composable commands
os.Exit(1)                  // use return err from RunE instead
```

### Anti-pattern: "helpful" optional arg with interactive fallback

```go
// WRONG — breaks every non-interactive caller
RunE: func(cmd *cobra.Command, args []string) error {
    var id string
    if len(args) == 0 {
        id = runInteractivePicker() // hangs in pipes, CI, scripts
    } else {
        id = args[0]
    }
    ...
}

// RIGHT — fail fast, teach discovery
Args: cobra.ExactArgs(1),
// Error message: "project-id required\n\nList your projects:\n  andamio project list --output json"
```

### Detection signals

- Command hangs when piped: `andamio project task list | wc -l` blocks
- `jq` receives mixed text/JSON because progress leaked to stdout
- CI job exits with `"no input received"` (empty stdin, no TTY)
- Command behaves differently in terminal vs. script — always a red flag
- Import of `bufio` or `golang.org/x/term` in a command file

### Composability smoke test

Every command should pass all three:

```bash
andamio <cmd> <args>                         # human-readable text, clean exit
andamio <cmd> <args> --output json | jq .   # machine-readable, no noise
andamio <cmd> <args> 2>/dev/null | wc -l    # no hang, no mixed output
```

## Files Changed

| File | Change |
|------|--------|
| `cmd/andamio/project_task.go` | Remove picker from `resolveProject()`, `ExactArgs(1)` on list/create, progress → stderr, remove `bufio`/`strings` imports |
| `cmd/andamio/project_task_export.go` | `ExactArgs(1)` on export, all progress → stderr |
| `cmd/andamio/project_task_import.go` | `ExactArgs(1)` on import, all progress → stderr |
| `CLAUDE.md` | Add Composability Rules section |

## Related

- `docs/solutions/architecture/command-structure-refactoring.md` — command patterns, `printList` helper, auth middleware, flat command hierarchy
- `docs/solutions/feature-implementations/cli-output-format-flag.md` — `--output` flag design, `PersistentPreRunE`, `PrintJSON`/`PrintList`, the JSON scripting surface
- `docs/solutions/feature-implementations/cli-course-module-management-commands.md` — `ImportParams` pattern, `--dry-run`, sentinel errors; reused by task import/export
- `docs/solutions/feature-implementations/cli-course-export-tiptap-conversion.md` — `tiptapToMarkdown()` and `writeFileAtomic()` reused by task export
- `docs/plans/2026-03-18-refactor-cli-composability-remove-interactive-pickers-plan.md` — the full plan that drove this change, including `gh`/`gcloud` design rationale
