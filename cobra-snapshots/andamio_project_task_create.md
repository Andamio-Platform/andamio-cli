## andamio project task create

Create a new task

### Synopsis

Create a new task for a project where you are a manager.

Find your project IDs with: andamio project list --output json

Examples:
  andamio project task create <project-id> --title "Build API" --lovelace 5000000 --expiration 2026-04-01T00:00:00Z
  andamio project task create <project-id> --title "Fix bug" --lovelace 2000000 --expiration 2026-04-01T00:00:00Z --github-issue "org/repo#42"
  andamio project task create <project-id> --title "Design system" --lovelace 5000000 --expiration 2026-04-01 --content-file task.md
  andamio project task create <project-id> --title "Earn XP" --lovelace 5000000 --expiration 2026-04-01 --token "policyid...,XP,50"

Requires user authentication via 'andamio user login'.

```
andamio project task create <project-id> [flags]
```

### Options

```
      --content string        Plain text task description
      --content-file string   Markdown file for rich task content (converted to Tiptap JSON)
      --expiration string     Expiration time in ISO 8601 format, e.g. 2026-04-01T00:00:00Z (required)
      --github-issue string   GitHub issue reference, e.g. org/repo#123 (prepended to title as [org/repo#123])
  -h, --help                  help for create
      --lovelace string       Lovelace reward amount, e.g. 5000000 for 5 ADA (required)
      --title string          Task title (required)
      --token stringArray     Native asset token (repeatable, format: "policy_id,asset_name,quantity"). asset_name is auto-hex-encoded if not already hex
```

### Options inherited from parent commands

```
  -o, --output string   Output format: text, json, csv, markdown (default "text")
```

### SEE ALSO

* [andamio project task](andamio_project_task.md)	 - Manage project tasks (manager role)

