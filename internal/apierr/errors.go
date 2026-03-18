package apierr

// NotFoundError is returned when a requested resource does not exist (HTTP 404).
// main.go maps this to exit code 2.
type NotFoundError struct{ Message string }

func (e *NotFoundError) Error() string { return e.Message }

// AuthError is returned when a request lacks valid credentials (HTTP 401/403).
// main.go maps this to exit code 3.
type AuthError struct{ Message string }

func (e *AuthError) Error() string { return e.Message }
