## andamio project task verify-hash

Verify task hashes match computed hashes

### Synopsis

Compute task hashes locally and compare against API-returned hashes.

Fetches all tasks for a project, computes the Blake2b-256 hash from
the task data (content, expiration, lovelace, assets), and reports
any mismatches. Useful for diagnosing hash issues with on-chain tasks.

Requires an API key or user authentication.

Examples:
  andamio project task verify-hash <project-id>
  andamio project task verify-hash <project-id> --output json

```
andamio project task verify-hash <project-id> [flags]
```

### Options

```
  -h, --help   help for verify-hash
```

### Options inherited from parent commands

```
  -o, --output string   Output format: text, json, csv, markdown (default "text")
```

### SEE ALSO

* [andamio project task](andamio_project_task.md)	 - Manage project tasks (manager role)

