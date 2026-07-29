## andamio project task compute-hash

Compute task hash from task data fields

### Synopsis

Compute the Blake2b-256 hash of task data, matching the on-chain Plutus validator.

This is the same hash used for task verification on-chain. Use it to
pre-compute the task_hash before minting a task.

Provide task data either as individual flags or via --file pointing to a
task markdown file with frontmatter (same format as 'project task export').

No authentication required — this is a purely local computation.

Examples:
  andamio project task compute-hash --content "Build API" --lovelace 5000000 --expiration 2026-12-31
  andamio project task compute-hash --content "Earn XP" --lovelace 5000000 --expiration 2026-12-31 --token "policyid...,XP,50"
  andamio project task compute-hash --file tasks/my-project/001-build-api.md --output json

```
andamio project task compute-hash [flags]
```

### Options

```
      --content string      Task content text (max 140 chars)
      --expiration string   Expiration time in ISO 8601 format, e.g. 2026-12-31
      --file string         Path to task markdown file with frontmatter
  -h, --help                help for compute-hash
      --lovelace string     Lovelace reward amount, e.g. 5000000
      --token stringArray   Native asset token (repeatable, format: "policy_id,asset_name,quantity")
```

### Options inherited from parent commands

```
  -o, --output string   Output format: text, json, csv, markdown (default "text")
```

### SEE ALSO

* [andamio project task](andamio_project_task.md)	 - Manage project tasks (manager role)

