## andamio project contributor delete

Delete a task commitment

### Synopsis

Withdraw your commitment to a project task.

Examples:
  andamio project contributor delete --project-id <id> --task-index 3

```
andamio project contributor delete [flags]
```

### Options

```
  -h, --help                help for delete
      --project-id string   Project ID (required)
      --task-hash string    Task hash (use instead of --task-index for chain-only tasks)
      --task-index string   Task index (use --task-hash for chain-only tasks)
```

### Options inherited from parent commands

```
  -o, --output string   Output format: text, json, csv, markdown (default "text")
```

### SEE ALSO

* [andamio project contributor](andamio_project_contributor.md)	 - Project contributor operations (requires user login)

