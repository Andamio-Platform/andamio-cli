package schemasnapshot

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestGenerateFixture pins the scanner's own behaviour against a hand-written
// expectation, rather than against a golden the scanner itself produced
// (which is all cmd/andamio's surface test can do).
func TestGenerateFixture(t *testing.T) {
	out := filepath.Join(t.TempDir(), "schema.txt")
	if err := Generate([]string{"testdata/fixture"}, out); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	got, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("reading output: %v", err)
	}

	want := strings.Join([]string{
		// method-local type, labelled Receiver.Method.type
		`Client.Do.response.Code string json:"code"`,
		// top-level type: bare name; unexported field omitted
		`TopLevel.ID string json:"id"`,
		`TopLevel.Nested struct{Value int} json:"nested"`,
		// anonymous struct nested in a field: Outer.Field
		`TopLevel.Nested.Value int json:"value"`,
		// struct literal passed straight to a call: nothing binds it
		`runCommand.anon#1.Inline string json:"inline"`,
		// function-local named type
		`runCommand.localResult.OK bool json:"ok"`,
		// x := struct{...}{...}
		`runCommand.payload.Version string json:"version"`,
		// x := &struct{...}{...}
		`runCommand.ptr.Key string json:"key"`,
		// x := []struct{...}{...}
		`runCommand.rows.Name string json:"name"`,
		// var x struct{...}
		`runCommand.session.Nonce string json:"nonce"`,
	}, "\n") + "\n"
	// Absent by design: NoTags (no json tags), fixture_test.go's testOnly
	// (_test file) and testdata/skipped.go's Skipped (nested testdata dir).

	if string(got) != want {
		t.Errorf("snapshot mismatch\n--- got ---\n%s--- want ---\n%s", got, want)
	}
}
