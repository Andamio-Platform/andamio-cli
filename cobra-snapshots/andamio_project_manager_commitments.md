## andamio project manager commitments

List task commitments — pending and assessed (with evidence)

### Synopsis

List task commitments for a managed project.

As of andamio-api v2.3, this returns ALL task commitments — pending review and
already-assessed (the latter with evidence and decision details). Pre-v2.3 the
endpoint returned only pending rows; that filter has been removed.

Each row carries:
  - top level:  project_id, task_hash, submitted_by, submission_tx,
                on_chain_content, source ("merged"/"db_only"/"chain_only")
  - task:       on_chain_content, expiration, lovelace_amount, assets
  - content:    evidence (Tiptap doc), task_evidence_hash, commitment_status,
                assessed_by, task_outcome ("accept"/"refuse"/"deny")

The --output json envelope passes the gateway response through verbatim, so
downstream filters can branch on commitment_status, source, or task_outcome.
For example, to surface only rows still awaiting assessment — defined as
"no decision yet recorded" rather than matching a specific commitment_status
value (which can grow as the gateway adds non-terminal states like
PENDING_TX_*):

  andamio project manager commitments --project-id <id> --output json \
    | jq '.data[] | select(.content.task_outcome == null)'

Find your project IDs with: andamio project list --output json

Examples:
  andamio project manager commitments --project-id <id>
  andamio project manager commitments --project-id <id> --output json

```
andamio project manager commitments [flags]
```

### Options

```
  -h, --help                help for commitments
      --project-id string   Project ID (required)
```

### Options inherited from parent commands

```
  -o, --output string   Output format: text, json, csv, markdown (default "text")
```

### SEE ALSO

* [andamio project manager](andamio_project_manager.md)	 - Project manager operations (requires user login)

