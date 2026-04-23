package apierr

// NotFoundError is returned when a requested resource does not exist (HTTP 404).
// main.go maps this to exit code 2.
type NotFoundError struct{ Message string }

func (e *NotFoundError) Error() string { return e.Message }

// AuthError is returned when a request lacks valid credentials (HTTP 401/403).
// main.go maps this to exit code 3.
type AuthError struct{ Message string }

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

// ReportedError wraps an error whose output has already been printed to stdout
// (e.g., a structured JSON result). main.go should set the exit code from the
// wrapped error but skip printing a second error message.
type ReportedError struct{ Err error }

func (e *ReportedError) Error() string { return e.Err.Error() }
func (e *ReportedError) Unwrap() error { return e.Err }
