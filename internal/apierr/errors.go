package apierr

// NotFoundError is returned when a requested resource does not exist (HTTP 404).
// main.go maps this to exit code 2.
type NotFoundError struct{ Message string }

func (e *NotFoundError) Error() string { return e.Message }

// AuthError is returned when a request lacks valid credentials (HTTP 401/403).
// main.go maps this to exit code 3. HTTPStatus carries the originating status
// code so callers can distinguish "not authenticated" (401) from "not
// authorized for this resource" (403) without substring-matching Message.
// HTTPStatus is 0 for hand-built errors that don't come from statusError.
type AuthError struct {
	HTTPStatus int
	Message    string
}

func (e *AuthError) Error() string { return e.Message }

// ConflictError is returned when a request conflicts with existing state (HTTP 409).
type ConflictError struct{ Message string }

func (e *ConflictError) Error() string { return e.Message }

// ServerError is returned when a request fails with a 5xx response (500-599).
// Callers can use errors.As to detect server-side failures — e.g., the retry
// helper in internal/client uses this to decide whether to retry.
type ServerError struct {
	Status  int
	Message string
}

func (e *ServerError) Error() string { return e.Message }

// BackpressureError is returned when a request hits a transient backpressure
// status — HTTP 408 (Request Timeout), 425 (Too Early), or 429 (Too Many
// Requests). These are 4xx responses that represent "try again later" rather
// than semantic failures, so the retry helper treats them as retryable via
// errors.As rather than string-matching gateway error bodies.
//
// RetryAfterSeconds carries a parsed Retry-After hint when present (0 means
// "not supplied or unparseable"; callers fall through to exponential backoff).
// Only integer-second form is parsed — HTTP-date form is unsupported.
type BackpressureError struct {
	Status            int
	Message           string
	RetryAfterSeconds int
}

func (e *BackpressureError) Error() string { return e.Message }

// ReportedError wraps an error whose output has already been printed to stdout
// (e.g., a structured JSON result). main.go should set the exit code from the
// wrapped error but skip printing a second error message.
type ReportedError struct{ Err error }

func (e *ReportedError) Error() string { return e.Err.Error() }
func (e *ReportedError) Unwrap() error { return e.Err }
