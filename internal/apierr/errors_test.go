package apierr

import (
	"context"
	"errors"
	"fmt"
	"testing"
)

func TestKind_ClassifiesEachTypedError(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want string
	}{
		{"not found", &NotFoundError{Message: "gone"}, KindNotFound},
		{"auth", &AuthError{HTTPStatus: 401, Message: "nope"}, KindAuth},
		{"conflict", &ConflictError{Message: "clash"}, KindConflict},
		{"server", &ServerError{Status: 503, Message: "down"}, KindServer},
		{"backpressure", &BackpressureError{Status: 429, Message: "slow down"}, KindBackpressure},
		{"tier limit", &TierLimitError{Status: 429, Code: "tier_limit_exceeded", Message: "cap"}, KindTierLimit},
		{"removed command", &RemovedCommandError{Command: "course student", Guidance: "use the app"}, KindRemovedCommand},
		{"network", &NetworkError{Message: "unreachable"}, KindUnreachable},
		{"verify", &VerifyError{Path: "assignment.content_json", Message: "did not read back identical"}, KindVerify},
		{"canceled", context.Canceled, KindCanceled},
		{"deadline exceeded", context.DeadlineExceeded, KindCanceled},
		{"plain error", errors.New("something"), KindError},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Kind(tc.err); got != tc.want {
				t.Errorf("Kind() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestKind_Nil(t *testing.T) {
	if got := Kind(nil); got != "" {
		t.Errorf("Kind(nil) = %q, want empty string", got)
	}
}

// The command layer wraps liberally — "failed to get commitment: %w" and the
// like. A mapper that only type-asserted the top-level error would classify
// nearly everything as generic and quietly defeat the entire feature, so
// unwrapping is the load-bearing behavior here rather than a nicety.
func TestKind_UnwrapsThroughErrorfWrapping(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want string
	}{
		{
			"single wrap",
			fmt.Errorf("failed to get commitment: %w", &NotFoundError{Message: "404"}),
			KindNotFound,
		},
		{
			"double wrap",
			fmt.Errorf("outer: %w", fmt.Errorf("inner: %w", &AuthError{Message: "401"})),
			KindAuth,
		},
		{
			"wrapped network error",
			fmt.Errorf("failed to list tasks: %w", &NetworkError{Message: "unreachable"}),
			KindUnreachable,
		},
		{
			"wrapped tier limit",
			fmt.Errorf("create developer key failed: %w", &TierLimitError{Status: 429, Code: "tier_limit_exceeded", Message: "cap"}),
			KindTierLimit,
		},
		{
			"wrapped cancellation",
			fmt.Errorf("aborted: %w", context.Canceled),
			KindCanceled,
		},
		{
			"verify inside ReportedError",
			&ReportedError{Err: fmt.Errorf("import-assignment: %w", &VerifyError{Path: "assignment.content_json", Message: "mismatch"})},
			KindVerify,
		},
		{
			// A failed read-back after an accepted write is verify, not the
			// cause's kind: the module was modified, and "unreachable" would
			// tell a script the request never happened.
			"verify wrapping a transport failure stays verify",
			&VerifyError{Path: "assignment.content_json", Message: "read-back failed", Err: &NetworkError{Message: "connection reset"}},
			KindVerify,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Kind(tc.err); got != tc.want {
				t.Errorf("Kind() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestKind_UnwrapsThroughReportedError(t *testing.T) {
	err := &ReportedError{Err: &NotFoundError{Message: "404"}}
	if got := Kind(err); got != KindNotFound {
		t.Errorf("Kind() = %q, want %q", got, KindNotFound)
	}

	nested := &ReportedError{Err: fmt.Errorf("wrapped: %w", &ConflictError{Message: "409"})}
	if got := Kind(nested); got != KindConflict {
		t.Errorf("Kind() through ReportedError+wrap = %q, want %q", got, KindConflict)
	}
}

// Cancellation must win over transport classification. An operator pressing
// Ctrl-C is not the service being down, and reporting "unreachable" would send
// a caller chasing an outage that never happened.
func TestKind_CancellationOutranksNetwork(t *testing.T) {
	err := &NetworkError{Message: "could not reach GET /x", Err: context.Canceled}
	if got := Kind(err); got != KindCanceled {
		t.Errorf("Kind() = %q, want %q — cancellation must outrank transport failure", got, KindCanceled)
	}
}

func TestNetworkError_UnwrapsToCause(t *testing.T) {
	cause := errors.New("connection refused")
	err := &NetworkError{Message: "could not reach GET /x: connection refused", Err: cause}

	if !errors.Is(err, cause) {
		t.Error("NetworkError does not unwrap to its cause; the retry classifier relies on reaching the underlying net error")
	}
	if err.Error() != "could not reach GET /x: connection refused" {
		t.Errorf("Error() = %q", err.Error())
	}
}

func TestKind_TierLimitThroughReportedError(t *testing.T) {
	err := &ReportedError{Err: fmt.Errorf("wrapped: %w", &TierLimitError{Status: 403, Code: "tier_limit_exceeded", Message: "cap"})}
	if got := Kind(err); got != KindTierLimit {
		t.Errorf("Kind() = %q, want %q", got, KindTierLimit)
	}
}

// The stable gateway code stays in the text (scripts match on it), the
// gateway's own remedy sentence rides verbatim, and details are appended only
// when present — no dangling separator otherwise.
func TestTierLimitError_Message(t *testing.T) {
	e := &TierLimitError{Status: 429, Code: "tier_limit_exceeded", Message: "maximum API key limit (1) reached"}
	if got, want := e.Error(), "API error 429 (tier_limit_exceeded): maximum API key limit (1) reached"; got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
	e.Details = "pioneer tier"
	if got, want := e.Error(), "API error 429 (tier_limit_exceeded): maximum API key limit (1) reached: pioneer tier"; got != want {
		t.Errorf("Error() with details = %q, want %q", got, want)
	}
}

func TestRemovedCommandError_Message(t *testing.T) {
	err := &RemovedCommandError{Command: "course student submit", Guidance: "Use the app."}
	want := "'andamio course student submit' was removed in Andamio CLI 1.0.\nUse the app."
	if err.Error() != want {
		t.Errorf("Error() = %q, want %q", err.Error(), want)
	}
}
