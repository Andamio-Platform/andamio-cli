---
title: "feat: project manager qualified-contributors command"
type: feat
status: active
date: 2026-04-24
origin: https://github.com/Andamio-Platform/andamio-cli/issues/70
---

# feat: project manager qualified-contributors command

## Overview

Add `andamio project manager qualified-contributors --project-id <id>`, a CLI wrapper around the gateway's `GET /api/v2/project/manager/contributors/get-qualified?project_id={id}` (andamio-api v2.3, PR #380). The endpoint returns the aliases qualified — by credential intersection — to commit to a managed project's tasks. Today only direct HTTP hits this surface; every sibling `/project/manager/*` endpoint has a CLI counterpart, so this closes the gap and brings the endpoint inside the existing composable CLI flow.

## Problem Frame

CI pipelines, scripts, and agents following `/cli-guide` cannot reach `get-qualified` without hand-rolling curl against staging with raw API keys. That breaks the product's "agents and scripts use the same CLI humans do" property. The asymmetry also complicates future work such as pre-assign flows and project staffing agents, which should shell out to one CLI invocation rather than composing auth, query-string encoding, and error mapping themselves.

## Requirements Trace

- R1. Command `andamio project manager qualified-contributors --project-id <id>` exists under the existing `project manager` group in `cmd/andamio/project_manager_ops.go`.
- R2. Reuses existing auth (`jwtAuthPreRunE` via the parent `projectManagerCmd`); no new auth flow introduced.
- R3. Supports text and JSON output formats via the global `--output` flag.
- R4. Default text mode prints one alias per line on stdout; JSON mode emits the raw gateway response.
- R5. `truncated: true` is surfaced — passed through in JSON mode; printed as a warning line to stderr in text mode.
- R6. Error mapping: 403 → `"not a manager of project <id>"`, 404 → `"project not found"`, 502 → `"scan temporarily unavailable, retry later"`.
- R7. `--help` explains the intersection semantics (alias must hold every declared `(course_id, slt_hash)` prerequisite pair) and the 500-alias cap.
- R8. Handler unit tests cover happy path, empty result, `truncated=true`, and each mapped status code.

## Scope Boundaries

- Not adding a new helper to `internal/client/` for query-parameter GETs — inline `url.Values` build is sufficient here. (See Key Technical Decisions.)
- Not adding pagination; v1 of the endpoint has a hard 500-alias cap and no cursor surface.
- Not wiring a dedicated integration test against a seeded preprod project in this plan. Issue #70's acceptance criterion for that is treated as a manual verification step (documented in Verification) — the automated test story is `httptest` coverage in `cmd/andamio/`.
- Not handling 504 (circuit breaker open) with a custom message. 504 is rare and flows through the generic server-error path unchanged; revisit if users report confusion.
- Not adding a `--limit` flag; the 500 cap is server-side.

## Context & Research

### Relevant Code and Patterns

- `cmd/andamio/project_manager_ops.go:1-43` — the parent `projectManagerCmd` (uses `PersistentPreRunE: jwtAuthPreRunE`) and the sibling `project manager commitments` command. The new command registers in the same `init()` and lives in the same file.
- `cmd/andamio/helpers.go:40-52` — `getJSON(ctx, path)` pattern for GET endpoints whose body wraps in `{"data": [...]}`. Does **not** fit the qualified-contributors response shape (top-level `aliases`/`totalCount`/`truncated`), so the handler constructs a client and decodes a typed struct instead. `getJSONWithHint` (lines 71-82) shows the existing 404-rewrite pattern but is single-status — this command needs multi-status mapping so inline `errors.As` is the right tool.
- `internal/client/client.go:58-83` — `(*Client).Get(ctx, path, result)`. Path carries the query string; no `GetWithParams` helper exists. Errors are typed via `statusError` in `client.go:178-204`.
- `internal/apierr/errors.go` — `AuthError` (401/403), `NotFoundError` (404), `ServerError` (5xx). `errors.As` is the standard extractor; see `isModuleAlreadyExistsError` in `cmd/andamio/course_teacher_ops.go` for an idiomatic multi-branch type-switch example.
- `internal/output/output.go:42-45` — `output.GetFormat()` returns the current format. Existing commands branch on `output.FormatJSON` to decide between `PrintJSON` and human-readable output.
- `cmd/andamio/course_teacher_ops_test.go:268-297` — reference pattern for handler tests using `httptest.NewServer` + `client.New(&config.Config{BaseURL: srv.URL})`. This is the shape the new tests mirror.

### Institutional Learnings

- `docs/solutions/feature-implementations/cli-api-coverage-completion-phases-3-7.md` — documents the role-based-group convention (`project manager`, `project contributor`, `course teacher`, `course student`). Each group puts its parent `cobra.Command` with `PersistentPreRunE: jwtAuthPreRunE` at the top of the file and registers children in `init()`. New command must fit this convention.
- `docs/solutions/security-issues/cli-security-hardening-input-validation.md` — reinforces: never echo raw API response body in error messages; trust the typed error + inject a curated hint string.
- `CLAUDE.md` ("Composability Rules") — No interactive pickers, progress to stderr, structured data to stdout. This command is a pure read; the only stderr writes are the `truncated` warning and the conventional "not authenticated" preRunE error.

### External References

Not needed. All patterns are local and recent (sibling command merged 2026-03-21, typed errors 2026-04-22).

## Key Technical Decisions

- **Custom handler, not `printList`/`printListPost`**. The gateway response is `{projectId, aliases: [...], totalCount, truncated}` — no `data` wrapper. Forcing it into the list helpers would require wrapping/unwrapping. A purpose-built handler is clearer and keeps the `truncated` surface first-class. Rationale: the list helpers specialize on a `{"data": []}` contract; synthesizing a wrapper hides the response shape from the reader.
- **Typed response struct**. Decode into a typed Go struct (`qualifiedContributorsResponse`) rather than `map[string]interface{}`. Eliminates the `.(string)` type-assertion dance and makes the `truncated` branching provably correct at compile time.
- **Query-string built via `net/url.Values`**, not inlined. Already the norm elsewhere (`cmd/andamio/user.go:169-171`) and robust against future flag additions.
- **Inline error wrap with `errors.As`** rather than extending `getJSONWithHint`. The existing helper only handles 404; generalizing it to a multi-status map is broader than this issue warrants (3 lines inline vs refactor + callers). Revisit if a third command needs the same map.
- **Text-mode warning goes to stderr**, aliases go to stdout. Preserves the "pipe-able" property (`andamio project manager qualified-contributors --project-id P | xargs -n1 process-alias` works regardless of `truncated`).
- **Text-mode alias ordering** follows the server response. The gateway orders deterministically (`ORDER BY alias` in the scan SQL, then intersection preserves order); the CLI does not re-sort.
- **`--output json` passes through `totalCount` verbatim.** Callers that care about it read it directly; the CLI does not recompute `len(aliases)`.
- **Empty result (zero qualified aliases) is a success**, not an error. Text mode prints nothing to stdout and a single informational line to stderr (`No qualified contributors found.`), mirroring the `printList` "No X found" convention. JSON mode emits the full empty-aliases envelope.

## Open Questions

### Resolved During Planning

- **Should the auth flow require JWT or accept API-key-only?** — Inherited. The parent `projectManagerCmd` already declares `PersistentPreRunE: jwtAuthPreRunE`, which enforces **JWT-only** authentication (the issue's phrase "reuses the existing API-key + wallet-JWT flow from sibling commands" refers to the client still transmitting both headers when both are present — it does **not** mean the command is reachable with an API key alone). This matches the sibling `project manager commitments` and every other manager-group command that requires edit access.
- **Should the command accept `--project-id` as a flag or as a positional arg?** — Flag. Matches the sibling `project manager commitments --project-id <id>` exactly; positional would break group symmetry.
- **Truncation surfacing in text mode?** — Stderr warning line after the aliases stream. Keeps stdout clean for piping; the warning is still visible to a human.

### Deferred to Implementation

- **Exact wording of the three error hints.** Plan locks in the strings from issue #70; implementation may tune for grammar/capitalization consistency with other CLI error hints (e.g., trailing punctuation, inclusion of the project ID).
- **Whether to show the `totalCount` in text mode as a trailing stderr summary.** Default in this plan: no — `wc -l` on stdout is adequate. Revisit only if a reviewer surfaces a clear UX benefit.
- **Auth-bypass edge case**: what the command reports if the JWT is present but expired and the gateway returns 401. Expected behavior: `AuthError` flows through with the gateway's message. Document during implementation if the string turns out to be user-hostile.

## Implementation Units

- [ ] **Unit 0: Add `Status` field to `apierr.AuthError`**

**Goal:** Give `*apierr.AuthError` an `HTTPStatus int` field populated by `statusError`, so callers can distinguish 401 from 403 without substring-matching the message. Symmetric with the existing `ServerError{Status: int}` pattern.

**Requirements:** Prerequisite for R6 (403 mapping must not catch 401).

**Dependencies:** None. Pure additive typed-struct change; no caller currently reads a field that does not yet exist.

**Files:**
- Modify: `internal/apierr/errors.go` — add `HTTPStatus int` field to `AuthError`.
- Modify: `internal/client/client.go` — populate `HTTPStatus` in `statusError` for 401/403 branches.
- Modify: `internal/client/client_test.go` — one assertion that 401 vs 403 produce distinct `HTTPStatus` values.

**Approach:**
- `AuthError` becomes `struct { HTTPStatus int; Message string }`. `Error()` still returns `Message`.
- In `statusError` (`client.go:178-204`), the 401/403 branch sets `HTTPStatus: status` when constructing the error.
- Zero caller changes required: no existing callsite reads `AuthError.HTTPStatus`; the field defaults to zero for unit-constructed errors and that remains acceptable (existing tests build `&apierr.AuthError{Message: "..."}` literals — they continue to compile).

**Patterns to follow:**
- `apierr.ServerError{Status: int; Message: string}` — the existing template this mirrors.
- Existing `statusError` test table in `internal/client/client_test.go:21-88` — same shape, one new assertion.

**Test scenarios:**
- *Happy path:* `statusError(401, body)` returns `*apierr.AuthError` whose `HTTPStatus == 401`.
- *Happy path:* `statusError(403, body)` returns `*apierr.AuthError` whose `HTTPStatus == 403`.
- *Happy path:* Constructing `&apierr.AuthError{Message: "x"}` still works (zero-value `HTTPStatus` is acceptable for hand-built errors). Assert via compile — no runtime assertion needed.

**Verification:**
- `go test ./internal/...` passes including the new assertions.
- `grep -rn "apierr.AuthError{Message" cmd/ internal/` still compiles (existing literals untouched).

---

- [ ] **Unit 1: Add command, flag, registration, and help text**

**Goal:** Wire the Cobra command into the existing `project manager` group with its flag and a help string that explains intersection semantics and the 500 cap.

**Requirements:** R1, R2, R7

**Dependencies:** Unit 0 (Unit 1 itself does not read `HTTPStatus`, but staging Unit 0 first keeps the tree compiling during Unit 2).

**Files:**
- Modify: `cmd/andamio/project_manager_ops.go`

**Approach:**
- Add `projectManagerQualifiedContributorsCmd` alongside `projectManagerCommitmentsCmd`.
- Register in the existing `init()` via `projectManagerCmd.AddCommand(...)`.
- Declare `String("project-id", "", "Project ID (required)")` and `MarkFlagRequired("project-id")`.
- Write a `Long` help string that explains: "An alias is qualified iff they hold every `(course_id, slt_hash)` prerequisite declared in the project's current on-chain state" + "Results are capped at 500 aliases; `truncated: true` indicates the cap was reached." + a `Find your project IDs with: andamio project list --output json` hint.
- `RunE: runProjectManagerQualifiedContributors` (defined in Unit 2).

**Patterns to follow:**
- Mirror `projectManagerCommitmentsCmd` in the same file, including `Use`, `Short`, `Long` structure.
- Mirror help-text voice from `course teacher register-module` for technical explanations of semantics.

**Test scenarios:**
- *Happy path:* `andamio project manager qualified-contributors --help` prints the intersection-semantics paragraph and the 500-cap note. Verify by substring match in a CLI integration test.
- *Edge case:* Missing `--project-id` returns Cobra's "required flag(s) \"project-id\" not set" error. Existing Cobra behavior; one regression assertion to prevent accidental `MarkFlagRequired` removal.

**Verification:**
- `andamio project manager qualified-contributors --help` prints a multi-paragraph description including "hold every" and "500".
- `andamio project manager qualified-contributors` (no flag) exits non-zero with a flag-required error.

---

- [ ] **Unit 2: Handler — fetch, decode, format, error-map**

**Goal:** Implement `runProjectManagerQualifiedContributors` — the core read, decode, output-branch, and error-hint logic.

**Requirements:** R3, R4, R5, R6

**Dependencies:** Units 0 and 1.

**Files:**
- Modify: `cmd/andamio/project_manager_ops.go`
- Test: `cmd/andamio/project_manager_ops_test.go` (new)

**Approach:**
- Load config and build `client.New(cfg)`.
- Build the path as the literal string `/api/v2/project/manager/contributors/get-qualified?` concatenated with `url.Values{"project_id": {projectID}}.Encode()` (e.g., for project ID `P` the assembled path is `/api/v2/project/manager/contributors/get-qualified?project_id=P`). `Encode()` does not include a leading `?`, so the `?` belongs in the base path.
- Decode into a typed struct:
  ```
  type qualifiedContributorsResponse struct {
      ProjectID  string   `json:"projectId"`
      Aliases    []string `json:"aliases"`
      TotalCount int      `json:"totalCount"`
      Truncated  bool     `json:"truncated"`
  }
  ```
  *Directional — field names follow the gateway contract; final spelling confirmed during implementation via a live call against preprod.*
- On client error, use `errors.As` to route:
  - `*apierr.AuthError` with `HTTPStatus == 403` (the field added in Unit 0) → return `&apierr.AuthError{HTTPStatus: 403, Message: "not a manager of project " + projectID}`. `HTTPStatus != 403` (e.g., a 401 from an expired JWT) bubbles unchanged.
  - `*apierr.NotFoundError` → return `*apierr.NotFoundError{Message: "project " + projectID + " not found"}`.
  - `*apierr.ServerError` with `Status == 502` → return `&apierr.ServerError{Status: 502, Message: "scan temporarily unavailable, retry later"}`. `Status != 502` (500, 503, 504) bubbles unchanged.
  - Any other error → return as-is. This intentionally includes transport errors (DNS, TCP, `context.DeadlineExceeded` from the 30s client timeout) — they surface with the underlying `net`/`context` message, which is adequate for a read-only command.
- Branch on `output.GetFormat()`:
  - `FormatJSON` → `output.PrintJSON(resp)` (pass through the full envelope).
  - Other → for each alias, `fmt.Println(alias)` on stdout. The empty-list notice and the truncated warning are **not** mutually exclusive: if `len(Aliases)==0` emit `"No qualified contributors found."` to stderr first; separately, if `resp.Truncated` emit `"warning: result truncated at 500 aliases"` to stderr. When both conditions hold (empty list + truncated=true — rare, but representable by the gateway contract if every intersection step truncated independently) both stderr lines are emitted in that order. stdout is unaffected by either stderr line.

**Technical design:** *Directional guidance, not implementation specification.*

```
runProjectManagerQualifiedContributors:
    projectID <- flag
    cfg, c    <- config.Load, client.New
    path      <- "/api/v2/project/manager/contributors/get-qualified?" + url.Values{project_id:=id}.Encode()
    resp      <- c.Get(ctx, path, &qualifiedContributorsResponse)
    on err:
        switch errors.As:
            AuthError with HTTPStatus==403  -> rewrite with manager-of hint
            NotFoundError                    -> rewrite with project-not-found hint
            ServerError with Status==502     -> rewrite with scan-unavailable hint
            default                          -> bubble (covers 401, 500, 503, 504,
                                                        transport errors, etc.)
    on ok:
        if FormatJSON: PrintJSON(resp)
        else:
            stdout: each alias line
            stderr: empty-result notice if no aliases    # independent of truncated
            stderr: truncated warning if resp.Truncated  # independent of empty
```

**Patterns to follow:**
- `cmd/andamio/project_contributor.go:runProjectContributorCommitment` — typed-struct decode shape and client construction.
- `cmd/andamio/helpers.go:getJSONWithHint` — `errors.As` single-branch rewrite; extend to three branches inline.
- `cmd/andamio/course_teacher_ops_test.go:268-297` — `httptest.NewServer` + `client.New(&config.Config{BaseURL: srv.URL})` for handler tests.

**Test scenarios:**
- *Happy path:* Stub server returns `{"projectId":"P","aliases":["ada","alan"],"totalCount":2,"truncated":false}`. Handler with `output.FormatText` writes `"ada\nalan\n"` to stdout and nothing to stderr. Handler with `output.FormatJSON` writes the full envelope to stdout.
- *Happy path:* Stub returns non-empty aliases with `truncated: true`. Text mode writes all aliases to stdout, writes a `truncated` warning line to stderr.
- *Edge case:* Stub returns `{"aliases":[],"totalCount":0,"truncated":false}`. Text mode writes nothing to stdout and "No qualified contributors found." to stderr; JSON mode writes the empty envelope to stdout.
- *Edge case:* Stub returns `{"aliases":[],"totalCount":0,"truncated":true}` (degenerate empty+truncated). Text mode writes nothing to stdout and emits both the "No qualified contributors found." line and the truncated warning to stderr, in that order. JSON mode passes the envelope through unchanged.
- *Edge case:* Query string contains a `project_id` with URL-unsafe characters (`+`, space). Handler encodes correctly; server receives the literal value.
- *Error path:* Stub returns 403 with any body. Handler returns `*apierr.AuthError` whose message contains `"not a manager of project P"`.
- *Error path:* Stub returns 404. Handler returns `*apierr.NotFoundError` whose message contains `"project P not found"`.
- *Error path:* Stub returns 502. Handler returns `*apierr.ServerError{Status: 502}` whose message contains `"scan temporarily unavailable"`.
- *Error path:* Stub returns 401 (not mapped by issue #70). Handler bubbles the existing `AuthError` unchanged — assert the message does **not** contain the 403 hint. This guards against the manager-of rewrite catching every `AuthError`.
- *Error path:* Stub returns 500 (generic server error). Handler bubbles the existing `ServerError` unchanged — assert the message does **not** contain the 502 hint. Guards the 502 branch against catching every 5xx.
- *Integration scenario:* Assert the outbound request path is exactly `/api/v2/project/manager/contributors/get-qualified?project_id=P` and the `X-API-Key`/`Authorization` headers are propagated (delegates to existing client behavior — one assertion that both headers land on the stub).

**Verification:**
- All unit tests pass with the race detector (`go test -race ./cmd/andamio/...`).
- A manual call against preprod — with a known-managed project, a non-managed project, and an unknown project ID — produces the expected output for each of text and JSON modes.

---

- [ ] **Unit 3: Documentation updates**

**Goal:** Reflect the new command in the CLI reference surfaces so the `/cli-guide` agent path and human docs pick it up.

**Requirements:** R1 (discoverability)

**Dependencies:** Unit 2 complete (copy the final `--help` wording).

**Files:**
- Modify: `CLAUDE.md` — add a row to the `project manager` table in the "Complete Command Reference".
- Modify: `README.md` if the README enumerates manager-group commands (verify during implementation; skip the edit if README only links to CLAUDE.md).
- Modify: `docs/PROJECT-LIFECYCLE.md` — add a short "Finding qualified contributors" subsection if the doc has a pre-assign or staffing section; otherwise append a reference in the manager-operations section.
- Modify: `CHANGELOG.md` — add a bullet under `## [Unreleased]` (per `CLAUDE.md:Release`).

**Approach:**
- Add one row to the manager table in `CLAUDE.md`:

  ```
  | `project manager qualified-contributors --project-id <id>` | `/v2/project/manager/contributors/get-qualified` | jwt | List aliases qualified to commit (holds every prerequisite SLT). Capped at 500; JSON surfaces `truncated`. |
  ```

- In `CHANGELOG.md` under `[Unreleased] / ### Added`, write one line: "`project manager qualified-contributors` — list aliases qualified to commit to a managed project (gateway v2.3)."

**Patterns to follow:**
- Existing `project manager` rows in `CLAUDE.md`.
- Existing `[Unreleased]` bullet formatting in `CHANGELOG.md` (imperative voice, one-line, ending period).

**Test scenarios:**
- Test expectation: none — documentation-only unit. Spot-check by rendering `CLAUDE.md` and verifying the new row aligns with neighbors.

**Verification:**
- `grep -n "qualified-contributors" CLAUDE.md CHANGELOG.md` returns one match per file.
- `docs/PROJECT-LIFECYCLE.md` either updated or explicitly not-applicable (reviewer notes the choice).

## System-Wide Impact

- **Interaction graph:** The new command is read-only; no callbacks, side effects, or state mutation. It composes with existing `project list` for discovery and with shell tools (`wc -l`, `xargs`) for counting and fan-out.
- **Error propagation:** Typed errors (`AuthError`, `NotFoundError`, `ServerError`) continue to drive exit codes through `main.go`'s error handler. No new exit-code paths.
- **State lifecycle risks:** None — pure read.
- **API surface parity:** Brings CLI coverage to parity with gateway v2.3. No other role-group needs the same change yet; future "who can commit to this task?" flows may want a `project contributor eligible` variant, but that's out of scope here.
- **Integration coverage:** Unit 2's `httptest`-backed tests prove the wire contract; a manual call against preprod proves the happy path end-to-end. No cross-layer callback coverage is needed because the handler is stateless.
- **Unchanged invariants:** The existing `printList`/`printListPost` helpers are untouched. The client's retry/timeout behavior is untouched. Other `project manager` commands continue to send POSTs with `{"project_id": ...}` bodies; only this command uses a GET with a query string, and that difference is isolated to the handler.

## Risks & Dependencies

| Risk | Mitigation |
|------|------------|
| Gateway response field names drift between plan and merged code (e.g., `project_id` vs `projectId`). | During implementation, run one live call against preprod before finalizing the typed struct. If field casing differs from the plan's snake/camel assumption, correct the struct tags and add a regression comment in the test. |
| 403 rewrite catches genuine 401s (both currently map to `AuthError`). | Resolved by Unit 0: `AuthError.HTTPStatus` gives Unit 2 a structured field to branch on, eliminating substring-matching on gateway-authored messages. Covered by the 401-passthrough test in Unit 2. |
| 502 rewrite catches every 5xx, hiding real errors. | Branch on `ServerError.Status == 502` explicitly; 500/503/504 bubble unchanged. Covered by the 500-passthrough test in Unit 2. |
| Gateway endpoint not yet deployed to preprod when CLI ships. | Unit-test coverage is `httptest`-based and doesn't require the live endpoint. Ship behind normal release flow; the command returns a clear 404/502 if the endpoint is missing, matching other CLI-commands-ahead-of-gateway cases. |

## Documentation / Operational Notes

- No rollout or feature-flag concerns — read-only, additive command.
- No monitoring implications beyond existing client-level telemetry (retry counts, typed errors).
- `CHANGELOG.md` entry goes under `[Unreleased]`; the next `./scripts/release.sh` run promotes it to the new version heading.

## Sources & References

- **Issue:** https://github.com/Andamio-Platform/andamio-cli/issues/70
- **Gateway plan:** `~/projects/01-projects/andamio-dev-kit-internal/plans/EligibleContributorsEndpointPlan.md` (response shape, status codes, 500-alias cap)
- **Related code:**
  - `cmd/andamio/project_manager_ops.go` — sibling command + registration site
  - `cmd/andamio/helpers.go` — `getJSONWithHint`, `jwtAuthPreRunE`
  - `internal/client/client.go` — `(*Client).Get`, `statusError`
  - `internal/apierr/errors.go` — typed errors used by the mapping
  - `cmd/andamio/course_teacher_ops_test.go:268-297` — handler-test pattern
- **Related solutions:** `docs/solutions/feature-implementations/cli-api-coverage-completion-phases-3-7.md` (role-group convention)
