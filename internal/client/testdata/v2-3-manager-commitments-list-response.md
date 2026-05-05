# Gateway response contract: `POST /v2/project/manager/commitments/list` (v2.3)

## Source

Verified from `andamio-api`:

- Handler: `internal/handlers/v2/merged_handlers/merged_handlers.go:1619`
  (`ListManagerCommitments`).
- Orchestrator: `internal/orchestration/project_orchestrator.go:1908`
  (`ListManagerCommitments`).
- Item type: `internal/orchestration/types.go:552` (`ManagerCommitmentItem`)
  + `types.go:442` (`TaskCommitmentContent`).
- Canonical orchestrator test:
  `internal/orchestration/project_orchestrator_test.go:1234`
  (`TestListManagerCommitments_IncludesAssessedWithEvidence`).

Endpoint: `POST /api/v2/project/manager/commitments/list`
Trigger:  any authenticated manager listing commitments for `project_id`.

## Semantics

Pre-v2.3: returns task commitments **pending review** (optional
`project_id` filter).

v2.3 (andamio-api#373 / #404): returns **all** task commitments — pending
**and** assessed — for the manager's project. `project_id` is **required**;
the gateway emits `400 project_id is required` when missing or blank.

## Response (representative — both rows)

The body below is also kept in
`v2-3-manager-commitments-list-response.json` so tests can read the
canonical bytes directly. Keep the two in sync.

```http
HTTP/1.1 200 OK
Content-Type: application/json

{
  "data": [
    {
      "project_id": "projA",
      "task_hash": "hashRewarded",
      "submitted_by": "alice",
      "source": "db_only",
      "content": {
        "evidence": {
          "type": "doc",
          "content": [
            { "type": "paragraph", "text": "completed work evidence" }
          ]
        },
        "task_evidence_hash": "evidhash-alice",
        "commitment_status": "REWARDED",
        "assessed_by": "managerBob",
        "task_outcome": "accept"
      }
    },
    {
      "project_id": "projA",
      "task_hash": "hashCommitted",
      "submitted_by": "charlie",
      "submission_tx": "tx-submit-charlie",
      "on_chain_content": "on-chain-content-charlie",
      "source": "merged",
      "task": {
        "expiration": "2026-12-31T23:59:59Z",
        "expiration_posix": 1798761599,
        "lovelace_amount": 5000000
      },
      "content": {
        "evidence": {
          "type": "doc",
          "content": [
            { "type": "paragraph", "text": "pending work evidence" }
          ]
        },
        "task_evidence_hash": "evidhash-charlie",
        "commitment_status": "COMMITTED"
      }
    }
  ]
}
```

## Field guide

Top-level `ManagerCommitmentItem` fields (all rows):

| Field              | Required | Notes                                                    |
|--------------------|----------|----------------------------------------------------------|
| `project_id`       | yes      | Echoes the request `project_id`.                         |
| `task_hash`        | yes      | Stable identifier for the task.                          |
| `submitted_by`     | yes      | Contributor alias.                                       |
| `submission_tx`    | no       | Submit-tx hash (db-api or chain).                        |
| `on_chain_content` | no       | Hex-encoded on-chain evidence pointer (when present).    |
| `task`             | no       | Embedded task info: expiration, lovelace, assets.        |
| `content`          | no       | Off-chain commitment content (see below).                |
| `source`           | yes      | `"merged"` / `"db_only"` / `"chain_only"`.               |

Off-chain `content` (`TaskCommitmentContent`):

| Field                | Notes                                                                    |
|----------------------|--------------------------------------------------------------------------|
| `evidence`           | Tiptap JSON document — arbitrary nested structure. Omitted when nil.     |
| `task_evidence_hash` | Hash for on-chain verification.                                          |
| `commitment_status`  | `DRAFT`/`COMMITTED`/`ACCEPTED`/`REFUSED`/`DENIED`/`REWARDED`/`ABANDONED`/`PENDING_TX_*`. |
| `assessed_by`        | Manager alias (set on assessed rows).                                    |
| `task_outcome`       | `"accept"` / `"refuse"` / `"deny"` (lowercase, db-api enum).             |

## CLI handling

`andamio project manager commitments --project-id <id>`:

- The CLI already enforces `--project-id` via `MarkFlagRequired`, so the
  v2.3 hard-required wire-level change is a no-op.
- `--output json` passes the gateway envelope through verbatim. The
  `evidence` Tiptap document round-trips intact (deserialized into
  `map[string]interface{}` and re-marshalled via `output.PrintJSON`).
- Text mode renders columns `submitted_by` (title) + `task_hash` (id),
  populated on every row regardless of pending/assessed/source.

## Follow-up

A `--status pending|assessed|all` filter at the CLI layer is intentionally
deferred. v2.3 callers can already filter on the JSON envelope. Prefer
filtering on `task_outcome` (presence/absence of the manager's decision)
over `commitment_status` enum-string-match — the enum can grow with new
non-terminal states like `PENDING_TX_*`, but `task_outcome == null`
durably captures "no decision yet":

```sh
andamio project manager commitments --project-id <id> --output json \
  | jq '.data[] | select(.content.task_outcome == null)'
```

If a CLI-side filter lands later, decide whether `pending` means "any
non-terminal status" (covers `COMMITTED`, `PENDING_TX_*`, etc.) or
something narrower.
