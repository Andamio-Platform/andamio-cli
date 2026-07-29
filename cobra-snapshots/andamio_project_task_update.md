## andamio project task update

Update a task by index

### Synopsis

Update a task's fields by its index.

Requires --project-id flag. Only specified flags are updated.

Requires user authentication via 'andamio user login'.

```
andamio project task update <index> [flags]
```

### Options

```
      --content string        New plain text description
      --content-file string   Markdown file for rich task content (converted to Tiptap JSON)
      --expiration string     New expiration time (ISO 8601)
  -h, --help                  help for update
      --lovelace string       New lovelace reward amount
      --project-id string     Project ID (required)
      --title string          New task title
      --token stringArray     Native asset token (repeatable, format: "policy_id,asset_name,quantity"). asset_name is auto-hex-encoded if not already hex
```

### Options inherited from parent commands

```
  -o, --output string   Output format: text, json, csv, markdown (default "text")
```

### SEE ALSO

* [andamio project task](andamio_project_task.md)	 - Manage project tasks (manager role)

