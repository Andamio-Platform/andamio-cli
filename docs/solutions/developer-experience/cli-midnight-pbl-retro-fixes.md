---
title: "CLI PBL retro: silent failures, unpersisted config, undocumented workflow"
date: "2026-04-01"
category: developer-experience
module: andamio-cli
problem_type: developer_experience
component: tooling
symptoms:
  - "publish-module reports success even when no on-chain module exists to link"
  - "submit headers must be passed on every tx invocation, not persistable"
  - "tx build help shows wrong type for initiator_data field"
  - "course owner create description implies it creates a course, not off-chain metadata"
  - "8-step course creation workflow undocumented, every new user hits the same failures"
root_cause: incomplete_setup
resolution_type: tooling_addition
severity: high
tags:
  - cli
  - developer-experience
  - course-workflow
  - submit-headers
  - publish-module
  - help-text
  - onboarding
related_components:
  - documentation
  - development_workflow
---

# CLI PBL retro: silent failures, unpersisted config, undocumented workflow

## Problem

Six issues from the Midnight PBL course setup retro caused silent failures, wrong-first-attempt experiences, and an undocumented 8-step workflow. Every new user hit the same sequence of failures because the CLI gave misleading success signals, required unpersisted flags on every invocation, showed incorrect help examples, and left the course creation workflow as tribal knowledge.

## Symptoms

- `course teacher publish-module` prints "done" even when no on-chain module exists to link — false success signal
- Blockfrost users must pass `--submit-header "project_id: ..."` on every `tx submit`/`tx run` — forgotten-flag errors
- `tx build --help` shows `"initiator_data":"addr_test1..."` (string), but API expects `{"change_address":"...", "used_addresses":[...]}`
- `course owner create` Short says "Create a new course" — users run it first and wonder why nothing appears on-chain
- `register-module` sets APPROVED, but `import` requires DRAFT — users get "slt_index out of range" with no guidance

## What Didn't Work

- **Assuming the API would return errors for no-op publish**: The API returns 200 with a response body containing `source` field that distinguishes real merges from no-ops. The generic CLI handler discarded this distinction.
- **Relying on flags for recurring configuration**: Submit headers are environment-specific (Blockfrost project_id) but session-persistent. Per-invocation flags don't match this usage pattern.
- **Writing help text before API contracts were finalized**: The `initiator_data` example was speculative and never corrected when the API solidified the object format.
- **Treating commands as standalone**: Each command's help text described itself without situating it in the larger workflow. No way to discover command ordering.
- **Expecting users to discover status requirements through error messages**: `SLT_LOCKED` / "slt_index out of range" errors don't mention `update-module-status --status DRAFT` as the fix.

## Solution

### 1. publish-module: dedicated handler with response inspection

Replaced the generic `runCourseTeacherModuleAction` handler with a dedicated `runCourseTeacherPublishModule` that inspects the API response:

```go
// After successful POST, inspect response for linkage signals
source, hasSource := resp["source"]
linked := hasSource && source == "merged"

if !linked {
    fmt.Fprintf(os.Stderr, "Warning: module %s may not have been linked to an on-chain module.\n"+
        "Ensure the module exists on-chain first (use 'andamio tx run' with modules_manage).\n"+
        "Then link with: andamio course teacher register-module --course-id %s --module-code %s --slt-hash <hash>\n",
        moduleCode, courseID, moduleCode)
}
```

JSON output still returns the raw API response; warnings go to stderr regardless of output mode.

### 2. Persistable submit headers in config

Added `SubmitHeaders map[string]string` to Config struct with three-tier precedence: config < env var < flag.

```go
// Config struct addition
SubmitHeaders map[string]string `json:"submit_headers,omitempty"`

// New commands
// andamio config set-submit-header project_id preprodABC123
// andamio config remove-submit-header project_id

// Env var override: ANDAMIO_SUBMIT_HEADERS='{"project_id":"abc"}'
```

Merge helper in `tx_submit.go` handles case-insensitive key matching:

```go
func mergeSubmitHeaders(configHeaders map[string]string, flagHeaders []string) []string {
    merged := make(map[string]string)
    for k, v := range configHeaders {
        merged[strings.ToLower(k)] = k + ": " + v
    }
    for _, h := range flagHeaders {
        parts := strings.SplitN(h, ":", 2)
        if len(parts) == 2 {
            key := strings.TrimSpace(parts[0])
            merged[strings.ToLower(key)] = key + ": " + strings.TrimSpace(parts[1])
        }
    }
    // ...
}
```

Both `tx submit` and `tx run` (via `tx_lifecycle.go`) use the same merge.

### 3. Corrected help text

**tx build**: Replaced string `initiator_data` example with correct object format, led with `--body-file` example.

**course owner create**: Changed Short to "Create off-chain course record (after on-chain creation)", updated Long to explain auto-registration and point to `course owner update` as the common path.

### 4. Workflow documentation in README

Added "Course Creation Workflow" section with all 8 steps and "Common Gotchas" covering: register sets APPROVED (need DRAFT for import), module hash ordering is non-deterministic, publish-module vs tx run semantics.

### Review bugs caught

- `remove-submit-header` used `cfg.SubmitHeaders[key] == ""` which would fail for headers intentionally set to empty string. Fixed to `_, exists := cfg.SubmitHeaders[key]`.
- Self-referential help text: "see: andamio course owner create --help" inside that command's own help. Replaced with inline workflow.

## Why This Works

- **Silent success → inspected response**: The API communicates outcome via `source` field. The CLI now reads it and translates to actionable guidance. The check belongs in the CLI layer — the API correctly returns 200 for valid requests.
- **Ephemeral config → persistent config**: Submit headers are environment-specific but session-persistent. Storing them in `~/.andamio/config.json` with config < env < flag precedence matches the standard model (like git config: system < global < local < env < flag).
- **Stale examples → spec-aligned examples**: Help text drifted from the API contract. Direct correction with a prevention strategy matters more than the fix itself.
- **Tribal knowledge → explicit documentation**: The workflow was only discoverable by failing through it. README docs + per-command Long descriptions + gotchas section means users can read the happy path before starting.

## Prevention

1. **Response-aware handlers for state-changing commands**: Every POST command that can partially succeed should inspect the response body, not just the HTTP status. New command PR checklist item: "Does this command inspect response content or just check for 200?"

2. **Config-first for environment-specific values**: Any flag value that comes from the deployment environment (not the specific operation) should have a `config set-*` counterpart. Heuristic: if a user passes the same flag value on every invocation, it belongs in config.

3. **Help text review against OpenAPI spec**: Compare help text examples against `openapi.json` payload schemas. The `spec fetch` + `spec paths` commands already exist — a script could diff help text examples against spec payloads.

4. **Workflow-aware help text**: Each command's Long description should include a "Where this fits" line referencing the workflow. When users read `--help`, they see their position in the sequence.

5. **Post-retro review pass**: After any multi-step workflow retro, do a dedicated pass through every command involved, checking: (a) does help text match current behavior? (b) does success output distinguish partial from full success? (c) is the next step discoverable from this command's output?

## Related Issues

- `docs/solutions/integration-issues/cli-api-payload-mismatches.md` — Same bug class (CLI drift from API spec), third instance of this pattern
- `docs/solutions/feature-implementations/cli-api-coverage-completion-phases-3-7.md` — Original implementation of the `runCourseTeacherModuleAction` factory that publish-module used
- `docs/solutions/security-issues/tx-signing-code-review-witness-drop-url-validation.md` — Submit URL validation, `--submit-header` over plaintext HTTP
- GitHub #42 — Student commands fail for chain-only modules (same payload mismatch class)
