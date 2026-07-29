## andamio project contributor update

Update task commitment evidence

### Synopsis

Update the evidence for your task commitment.

Examples:
  andamio project contributor update --project-id <id> --task-index 3 --evidence "https://github.com/..."

```
andamio project contributor update [flags]
```

### Options

```
      --evidence string        Evidence text or URL (Markdown supported)
      --evidence-file string   Path to evidence file (Markdown)
  -h, --help                   help for update
      --project-id string      Project ID (required)
      --task-hash string       Task hash (use instead of --task-index for chain-only tasks)
      --task-index string      Task index (use --task-hash for chain-only tasks)
```

### Options inherited from parent commands

```
  -o, --output string   Output format: text, json, csv, markdown (default "text")
```

### SEE ALSO

* [andamio project contributor](andamio_project_contributor.md)	 - Project contributor operations (requires user login)

