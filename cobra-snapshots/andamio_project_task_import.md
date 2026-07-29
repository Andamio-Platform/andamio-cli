## andamio project task import

Import tasks from local Markdown files

### Synopsis

Import tasks from local Markdown files with YAML frontmatter.

Reads .md files from tasks/<project-slug>/ directory.
Files without an 'index' in frontmatter create new tasks.
Files with an 'index' update existing tasks.

Non-DRAFT tasks are skipped with a warning.

Find your project IDs with: andamio project list --output json

Requires user authentication via 'andamio user login'.

```
andamio project task import <project-id> [flags]
```

### Options

```
      --dry-run   Preview API payloads without sending
  -h, --help      help for import
```

### Options inherited from parent commands

```
  -o, --output string   Output format: text, json, csv, markdown (default "text")
```

### SEE ALSO

* [andamio project task](andamio_project_task.md)	 - Manage project tasks (manager role)

