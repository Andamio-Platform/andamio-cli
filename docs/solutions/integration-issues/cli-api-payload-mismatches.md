---
title: Fix CLI command payloads to match current gateway API contracts
date: "2026-03-31"
category: integration-issues
module: course_owner, course_student, course_teacher_ops, project_owner
problem_type: integration_issue
component: tooling
symptoms:
  - "course/project owner create fails — API requires course_id + pending_tx_hash but CLI only sent title"
  - "course/project owner register silently accepts --title '' and omits title from payload"
  - "course owner teachers sends flat {teachers: [...]} instead of {add: [...], remove: [...]}"
  - "course teacher register-module fails — API requires slt_hash but CLI omitted it"
  - "course student commitment-get fails — API requires slt_hash but CLI sent module_code"
  - "course student leave/claim fails — API requires pending_tx_hash but CLI did not send it"
root_cause: wrong_api
resolution_type: code_fix
severity: high
tags:
  - api-contract
  - payload-mismatch
  - cobra-flags
  - cli
  - go
  - gateway-api
  - field-validation
---

# Fix CLI command payloads to match current gateway API contracts

## Problem

The Andamio CLI sent payloads that did not match the gateway API's actual request schemas — wrong required fields, wrong JSON keys, and wrong payload shapes — causing API errors across course/project owner, teacher, and student commands. This is the third instance of the same class of bug (payload drift from API spec) documented in this codebase.

## Symptoms

- `course owner create` and `project owner create` failed because the CLI required only `--title`, but the API requires `course_id` + `pending_tx_hash` with title optional.
- `course owner teachers` replaced the entire teacher list instead of doing incremental updates (CLI sent `{"teachers": [...]}` instead of `{"add": [...], "remove": [...]}`).
- `course teacher register-module` failed because the CLI omitted the required `slt_hash` field.
- `course student commitment` lookups failed because the CLI used `module_code` as the primary key, but the API uses `slt_hash`.
- `course student leave` and `course student claim` failed because the CLI never sent `pending_tx_hash`, which the API requires.
- `course owner register` silently accepted `--title ""` (Cobra's `MarkFlagRequired` only checks presence, not non-empty), producing a payload with no title field.

## What Didn't Work

The original implementation inferred API payload fields from CLI flag names without consulting the OpenAPI spec. As the API evolved, the CLI drifted from the actual contracts. The recurring pattern: a developer looks at the endpoint path, guesses the payload shape from the flag names, and never runs `andamio spec fetch` to verify against the actual `requestBody` definition.

## Solution

### 1. Owner create commands: new required fields

Before: `--title` required, no `--pending-tx-hash`.

After: `--course-id`/`--project-id` and `--pending-tx-hash` required, `--title` optional.

### 2. Teachers: replace-all changed to incremental add/remove

Before:
```go
// --teacher flag (repeatable), sent as full replacement
payload := map[string]interface{}{"course_id": courseID, "teachers": teachers}
```

After:
```go
// --add and --remove flags, filtered for empty strings
addTeachers = filterEmpty(addTeachers)
removeTeachers = filterEmpty(removeTeachers)
if len(addTeachers) == 0 && len(removeTeachers) == 0 {
    return fmt.Errorf("specify at least one of --add or --remove. Use 'andamio user exists <alias>' to verify aliases")
}
payload := map[string]interface{}{"course_id": courseID}
if len(addTeachers) > 0 { payload["add"] = addTeachers }
if len(removeTeachers) > 0 { payload["remove"] = removeTeachers }
```

### 3. Register-module: added missing slt_hash

```go
payload := map[string]interface{}{
    "course_id":          courseID,
    "course_module_code": moduleCode,
    "slt_hash":           sltHash,  // was missing — API requires it
}
```

### 4. Student commitment-get: slt_hash as primary key

```go
// Before: map[string]string with course_module_code required
// After: map[string]interface{} with slt_hash required, course_module_code optional
payload := map[string]interface{}{
    "course_id": courseID,
    "slt_hash":  sltHash,
}
if moduleCode != "" {
    payload["course_module_code"] = moduleCode
}
```

### 5. Student leave/claim: added pending_tx_hash

New `runCourseStudentTxAction` handler:
```go
payload := map[string]interface{}{
    "course_id":          courseID,
    "course_module_code": moduleCode,
    "pending_tx_hash":    pendingTxHash,  // was missing — API requires it
}
```

Note: leave/claim use `course_module_code` (not `slt_hash`) per the OpenAPI spec — verified against the live spec.

### 6. Empty-string guard on register title

Cobra's `MarkFlagRequired` accepts `--title ""` as valid. Added explicit validation:
```go
if title == "" {
    return fmt.Errorf("--title must not be empty")
}
payload["title"] = title  // always include, not conditionally
```

### 7. Documentation fixes

- **andamio-docs**: Fixed `course teacher review` examples — docs used `--commitment-id` and `approve/reject`, but CLI uses `--module-code` + `--participant-alias` and `accept/refuse`.
- **CLAUDE.md**: Made `--pending-tx-hash` visible as required in command column for create commands.

## Why This Works

Each fix aligns the CLI's outbound payload with the actual OpenAPI `requestBody` schema. The key insight: Cobra flag names and API JSON keys are independent contracts. The CLI must explicitly map between them, not assume they match. The `filterEmpty` helper and empty-string validation add defense-in-depth against Cobra's lenient flag parsing, which only checks flag presence, not semantic validity.

## Prevention

1. **Spec-first development**: Always run `andamio spec fetch` and check `requestBody` schema before adding or modifying POST/PUT commands:
   ```bash
   andamio spec fetch
   python3 -c "import json; d=json.load(open('openapi.json')); print(json.dumps(d['definitions']['CreateCourseV2Request'], indent=2))"
   ```

2. **Validate non-empty required strings**: Cobra's `MarkFlagRequired` only checks presence. Add `if val == "" { return error }` for fields the API requires non-empty.

3. **Use `map[string]interface{}` consistently**: Avoid `map[string]string` for POST payloads — it prevents sending non-string fields and creates type inconsistency.

4. **Keep docs in sync**: When changing flags, update CLAUDE.md, andamio-docs CLI pages, and command Long descriptions in the same commit.

5. **Check existing solutions first**: This is the third fix for "CLI payload drifted from API spec." Before modifying CLI commands, search `docs/solutions/integration-issues/` for prior instances of this pattern.

## Related Issues

- `docs/solutions/integration-issues/evidence-submission-payload-format-and-field-alignment.md` — Same class of bug, different commands (evidence submission fields). Root cause identical.
- `docs/solutions/integration-issues/cli-course-import-app-parity-and-payload-alignment.md` — Same API contract mismatch pattern in export/import commands.
- `docs/solutions/feature-implementations/cli-api-coverage-completion-phases-3-7.md` — Original implementation of the commands fixed here. Now partially stale (register-module factory, review decision values).
- GitHub #36 — Evidence submission payload format mismatch (same bug class).
- GitHub #42 — Student commands fail for chain-only modules (relates to slt_hash requirement).
