// Package apierr carries the CLI's typed error taxonomy.
//
// Every error the CLI surfaces maps to exactly one Kind and exactly one exit
// code. That mapping is the contract a non-human caller relies on: a person
// reading an error infers a lot from context, but a program takes the wrong
// branch silently. "Nothing found", "could not reach the service" and "not
// permitted" are three different situations, and a caller has to act on each
// of them differently.
//
// The exit-code mapping lives in cmd/andamio/main.go; Kind below is the
// machine-readable name that rides in the --output json error envelope.
package apierr

import (
	"context"
	"errors"
)

// Kind values are the stable, machine-readable failure names emitted as the
// "kind" field of the --output json error envelope. They are part of the
// CLI's public contract: scripts branch on them, so values are added rather
// than renamed.
const (
	KindNotFound       = "not_found"
	KindAuth           = "auth"
	KindConflict       = "conflict"
	KindServer         = "server"
	KindBackpressure   = "backpressure"
	KindRemovedCommand = "removed_command"
	KindUnreachable    = "unreachable"
	KindCanceled       = "canceled"
	KindError          = "error"
)

// Kind classifies err into one of the Kind constants.
//
// Unwraps through fmt.Errorf("%w") and ReportedError, which matters more than
// it might appear: the command layer wraps liberally ("failed to get
// commitment: %w"), so a mapper that only type-asserted the top-level error
// would classify almost everything as the generic kind and quietly defeat the
// whole point.
//
// Order matters where a single error could satisfy two checks. Context
// cancellation is tested before anything else because an interrupted or
// timed-out request is a distinct outcome from an unreachable service, and the
// transport layer wraps the former in the latter's territory.
func Kind(err error) string {
	if err == nil {
		return ""
	}

	switch {
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return KindCanceled
	}

	var (
		notFound     *NotFoundError
		auth         *AuthError
		conflict     *ConflictError
		server       *ServerError
		backpressure *BackpressureError
		removed      *RemovedCommandError
		network      *NetworkError
	)

	switch {
	case errors.As(err, &removed):
		return KindRemovedCommand
	case errors.As(err, &notFound):
		return KindNotFound
	case errors.As(err, &auth):
		return KindAuth
	case errors.As(err, &conflict):
		return KindConflict
	case errors.As(err, &backpressure):
		return KindBackpressure
	case errors.As(err, &server):
		return KindServer
	case errors.As(err, &network):
		return KindUnreachable
	}
	return KindError
}

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

// NetworkError is returned when a request never reached the service — DNS
// failure, connection refused, TLS failure, a dropped connection mid-flight.
// main.go maps this to exit code 5.
//
// This exists because "could not reach the service" was previously
// indistinguishable from every other unclassified failure: the transport error
// from http.Client.Do travelled to main.go untyped and exited 1, exactly like a
// malformed flag or a JSON decode failure. A person retries or asks a
// colleague; a program takes the wrong branch.
//
// Deliberately NOT used for context cancellation or deadline expiry. An
// operator pressing Ctrl-C and a service being down are different outcomes, and
// collapsing them would reintroduce the ambiguity this type exists to remove —
// see wrapTransportError in internal/client.
type NetworkError struct {
	Message string
	Err     error
}

func (e *NetworkError) Error() string { return e.Message }
func (e *NetworkError) Unwrap() error { return e.Err }

// RemovedCommandError is returned when a caller invokes a command that was
// retired in Andamio CLI 1.0. main.go maps this to exit code 4.
//
// This is not an "unknown command" — the CLI knows exactly what was asked for
// and is declining to do it because the operation moved elsewhere. Command
// carries the full retired path (e.g. "course student submit") and Guidance
// names where the operation lives now. Both are populated from the single
// registry in cmd/andamio/retired.go so every retired command explains itself
// the same way.
type RemovedCommandError struct {
	Command  string
	Guidance string
}

func (e *RemovedCommandError) Error() string {
	return "'andamio " + e.Command + "' was removed in Andamio CLI 1.0.\n" + e.Guidance
}

// ReportedError wraps an error whose output has already been printed to stdout
// (e.g., a structured JSON result). main.go should set the exit code from the
// wrapped error but skip printing a second error message.
type ReportedError struct{ Err error }

func (e *ReportedError) Error() string { return e.Err.Error() }
func (e *ReportedError) Unwrap() error { return e.Err }
