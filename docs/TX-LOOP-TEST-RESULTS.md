# TX Loop Test Results

**Date:** 2026-04-06
**Environment:** Local devkit (localhost:8082)
**CLI:** andamio dev (commit 4ccd87e + draft-before-mint fix)
**Wallets:** andamio-preprod-001 (owner/teacher/manager), andamio-preprod-002 (student/contributor)

## Summary

| Loop | Name | TX Steps | On-Chain | DB Updated | Verdict |
|------|------|----------|----------|------------|---------|
| 0 | access_token_mint | 2/2 | 2/2 | 2/2 | PASS |
| 1 | course.setup | 2/2 | 2/2 | 1/2 | FAIL (modules_manage DB) |
| 3 | course.credential | 3/3 | 3/3 | 3/3 | PASS |
| 2 | project.setup | 2/2 | 2/2 | 2/2 | PASS |
| 4 | project.credential | 3/3 | 3/3 | 1/3 | FAIL (project_join + credential_claim DB) |

**On-chain success rate:** 12/12 (100%)
**DB update success rate:** 10/12 (83%)

## TX Details

### Loop 0: access_token_mint
| Step | TX Hash | State | Notes |
|------|---------|-------|-------|
| Mint 001 | `d68895ff...` | updated | |
| Mint 002 | `156bcf55...` | updated | |

### Loop 1: course.setup
| Step | TX Hash | State | Error |
|------|---------|-------|-------|
| course_create | `261bb4e8...` | updated | |
| modules_manage | `e00a7242...` | **failed** | "Module not found" — DB record didn't exist at confirm time |

**Root cause:** The batch confirm endpoint requires the module to be in `PENDING_TX` status. The CLI had no way to create a DRAFT module with SLTs before minting. Even after adding `create-module --slt --approve`, the `APPROVED → PENDING_TX` transition is blocked by the DB API (only accepts DRAFT/APPROVED via teacher endpoint).

**Course 2 (clean retry):**
| Step | TX Hash | State | Notes |
|------|---------|-------|-------|
| course_create | `0c880f13...` | updated | |
| create-module --slt --approve | n/a | APPROVED | New CLI feature worked |
| PENDING_TX (manual DB) | n/a | PENDING_TX | Had to set via direct postgres |
| modules_manage | `d9719ab4...` | failed (4 retries) | Module was APPROVED, not PENDING_TX at time of confirm |
| confirm-tx (manual) | n/a | ON_CHAIN | Manual confirm after DB fix |

### Loop 3: course.credential (using Course 2)
| Step | TX Hash | State | FSM Transition |
|------|---------|-------|----------------|
| assignment_submit | `8430c52e...` | updated | AWAITING_SUBMISSION → SUBMITTED |
| assessment_assess | `7e83ee1f...` | updated | SUBMITTED → ACCEPTED |
| credential_claim | `346a2687...` | updated | On-chain NFT claimed |

### Loop 2: project.setup
| Step | TX Hash | State | Notes |
|------|---------|-------|-------|
| project_create | `b1ec01d3...` | updated | |
| task create (off-chain) | n/a | DRAFT | `project task create` |
| treasury_fund | `7121be8f...` | updated | Needed to add 10 ADA |
| tasks_manage | `50263d95...` | updated | Task confirm used index fallback |

### Loop 4: project.credential
| Step | TX Hash | State | Error |
|------|---------|-------|-------|
| project_join | `87274604...` | **failed** | "required 'task_hash' not found" in event/metadata |
| task_assess | `6c3b4595...` | updated | Accepted |
| project_credential_claim | `b660c2e1...` | **failed** | "required metadata 'task_hash' not found" |

**Root cause:** The gateway's confirm logic for `project_join` and `project_credential_claim` requires `task_hash` in the andamioscan event response or TX registration metadata. Neither source provides it automatically. The CLI's `tx run` doesn't inject `task_hash` into metadata for project TXs.

## Required Fixes

### 1. Module PENDING_TX transition (DB API)
**Priority: Critical**
The `update-module-status` endpoint must accept `PENDING_TX` as a valid status when the module is `APPROVED`. Without this, the CLI cannot drive the full module lifecycle. The app does this client-side before submitting the TX.

**Workaround:** Direct DB update or manual `confirm-tx` call after setting status via postgres.

### 2. Project TX task_hash metadata (Gateway or CLI)
**Priority: Critical**
The gateway's confirm logic for `project_join` and `project_credential_claim` requires `task_hash`. Options:
- **Gateway fix:** Extract `task_hash` from the on-chain datum (andamioscan already has it)
- **CLI fix:** `tx run` could auto-inject `task_hash` into registration metadata for project TXs when the body contains it
- **Immediate workaround:** Pass `--metadata task_hash=<hash>` explicitly

### 3. Student commitment pre-creation (Documentation)
**Priority: Medium**
The off-chain commitment must be created (`course student create`) before the `assignment_submit` TX. SYSTEM_REFERENCE mentions "gateway shortcuts" for direct transitions but the DB API's confirm endpoint returns 404 when no commitment record exists. Document that `course student create` is a mandatory pre-step.

### 4. Treasury funding for project tasks (Documentation)
**Priority: Low**
The `project_create` TX doesn't include enough treasury for tasks by default. A `treasury_fund` step is needed between project creation and contributor join. tx-loops.yaml should include this as an explicit step.

## SYSTEM_REFERENCE Ambiguities Found

| Section | Issue | Type |
|---------|-------|------|
| §4.1 | `initiator_data` structure undocumented — it's a plain bech32 address string, not a JSON object | Gap |
| §5.2 | "Module not found" error message is misleading — the real issue may be assignment commitment not found, not module | Misleading |
| §6.2 | `contributor_state_id` discovery not documented — found via `project get` response | Gap |
| §9.3 | No mention that PENDING_TX cannot be set via any public API endpoint | Critical gap |
| §10.2 | Task confirm fallback to `(project_state_id, task_index)` works but project confirm has no equivalent fallback for `task_hash` | Inconsistency |
| §11 | Service fees say 100 ADA base but tx-loops.yaml says ~50 ADA — needs reconciliation | Conflict |

## tx-loops.yaml Issues

| Loop | Issue |
|------|-------|
| All | No mention of `initiator_data` requirement for access_token_mint |
| 1 | Missing step: "Create module in DB before minting (DRAFT → APPROVED → PENDING_TX)" |
| 2 | Missing step: "Fund treasury before contributor can join" |
| 2 | Missing step: "Create task in DB (DRAFT) before tasks_manage TX" |
| 4 | Missing: `task_hash` must be passed as metadata for project_join and project_credential_claim |
| 15 | Status is `stubbed` but is a prerequisite for loops 1-4 which are `validate-today` |
| All | Endpoints in YAML don't match actual build endpoints (e.g., `/api/v2/course/owner/create` vs `/v2/tx/instance/owner/course/create`) |

## CLI Changes Made

1. **`course create-module`** — Added `--slt` (repeatable) and `--approve` flags. When both are provided, creates module with SLTs and auto-approves with computed `slt_hash` in a single command.

2. **`course teacher update-module-status`** — Added `--slt-hash` flag for the APPROVED transition (required by the API).
