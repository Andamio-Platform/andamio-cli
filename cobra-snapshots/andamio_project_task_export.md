## andamio project task export

Export tasks to local Markdown files

### Synopsis

Export all tasks for a project to local Markdown files with YAML frontmatter.

Files are written to tasks/<project-slug>/ with one file per task.
Each file contains YAML frontmatter (title, lovelace, expiration, tokens, etc.)
and the task content as Markdown.

Find your project IDs with: andamio project list --output json

Requires user authentication via 'andamio user login'.

```
andamio project task export <project-id> [flags]
```

### Options

```
  -h, --help   help for export
```

### Options inherited from parent commands

```
  -o, --output string   Output format: text, json, csv, markdown (default "text")
```

### SEE ALSO

* [andamio project task](andamio_project_task.md)	 - Manage project tasks (manager role)

