# GET /api/v2/project/manager/contributors/get-qualified — v2.5 wire shape

Captured shape of the gateway envelope (andamio-api
`merged_handlers.QualifiedContributorsResponseEnvelope` wrapping
`orchestration.QualifiedContributorsResponse`). Fields are **snake_case** and
the payload is wrapped in `data`.

Pinned because the first CLI decoder (v0.12.1, #70) used camelCase tags and
decoded the inner object without unwrapping `data`, so every field silently
stayed at its zero value (#90 item 6). `cmd/andamio/project_manager_ops_test.go`
reads this file and asserts `total_count > 0` round-trips.

`status` was added by the gateway after #70: `"ok"` when the prerequisite
intersection was computed, `"no_prerequisites_configured"` when the project
has no prerequisites on-chain and every alias is vacuously qualified.
