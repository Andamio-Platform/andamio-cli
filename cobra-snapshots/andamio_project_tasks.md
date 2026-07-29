## andamio project tasks

List tasks for a project (public view)

### Synopsis

List tasks for a project. Unlike 'project task list' (manager endpoint),
this uses the public user endpoint and does not require manager role.

Examples:
  andamio project tasks <project-id>
  andamio project tasks <project-id> --output json

```
andamio project tasks <project-id> [flags]
```

### Options

```
  -h, --help   help for tasks
```

### Options inherited from parent commands

```
  -o, --output string   Output format: text, json, csv, markdown (default "text")
```

### SEE ALSO

* [andamio project](andamio_project.md)	 - Manage projects

