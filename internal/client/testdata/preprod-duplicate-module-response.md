# Gateway response contract: duplicate `course_module_code` on register-module

## Source

Verified from `andamio-api/internal/handlers/v2/merged_handlers/merged_handlers.go:881-892`
(see also `internal/orchestration/course_orchestrator.go:2280` which is where
`RegisterModuleError{Code: "DUPLICATE_CODE"}` is constructed).

Endpoint: `POST /api/v2/course/teacher/course-module/register`
Trigger:  request carrying a `course_module_code` that already exists in the
          target course's draft/on-chain set.

## Response (actual)

```http
HTTP/1.1 400 Bad Request
Content-Type: application/json

{"error":"course_module_code already exists in this course"}
```

Expand of the gateway error envelope (via `writeErrorEnvelope`):

```json
{
  "error": "course_module_code already exists in this course"
}
```

## Response (originally assumed by CLI / todo #021)

```http
HTTP/1.1 409 Conflict
Content-Type: application/json

{"error":"course_module_code already exists in this course"}
```

## Divergence and CLI handling

The gateway maps `DUPLICATE_CODE` to `fiber.StatusBadRequest` (400), not
`StatusConflict` (409). `internal/client/client.go`'s `statusError` does not
construct `*apierr.ConflictError` for 400 — it returns a plain `errors.New`.
As a result, the original strict-only `isModuleAlreadyExistsError` (pre
todo #021 resolution) never fired in production, silently breaking
register-module's idempotency recovery.

The fix in `cmd/andamio/course_teacher_ops.go` adds a body-token fallback
path that accepts non-`ConflictError` types when both `"already exists"` and
`"course_module_code"` are present in `err.Error()`. When the fallback fires,
a stderr warning is printed so the operator/user can report the drift.

When the gateway is fixed to return 409 (preferred long-term contract), the
strict path will fire first and the fallback's warning will stop appearing —
no CLI change required on that day.

## Follow-up

File an issue against andamio-api to map `RegisterModuleError{Code:
"DUPLICATE_CODE"}` to `fiber.StatusConflict` (409), mirroring the standard
REST semantic of "resource already exists, retry will not succeed as-is."
