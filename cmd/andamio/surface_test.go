package main

import (
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/Andamio-Platform/andamio-cli/internal/schemasnapshot"
)

var update = flag.Bool("update", false, "update golden files instead of comparing against them")

// Everything under internal/ is scanned, so a new package is covered the
// moment it declares a json-tagged struct; there is no list to keep in sync.
// Packages with no tagged structs (apierr, output, submit, schemasnapshot)
// simply contribute no lines; the apierr kind contract is guarded separately
// by exitcode_test.go. Notable contracts pinned this way:
// cardano.SignResult (`tx sign --output json`), cardano.NativeAsset
// (`project task verify-hash` assets), client.gatewayError (its `code` drives
// exit-code classification, e.g. tier_limit_exceeded -> exit 7), quiz.Summary
// (`assignment_quiz` / `assignment` in import output) and prompts.Answer
// (`content.evidence_answers` in `teacher assignments`).
var schemaSrcDirs = []string{
	".",
	"../../internal",
}

// compareOrUpdateGolden either overwrites goldenPath with actual (-update)
// or fails the test if goldenPath's contents don't match actual.
func compareOrUpdateGolden(t *testing.T, goldenPath string, actual []byte) {
	t.Helper()

	if *update {
		if err := os.MkdirAll(filepath.Dir(goldenPath), 0755); err != nil {
			t.Fatalf("creating golden dir: %v", err)
		}
		if err := os.WriteFile(goldenPath, actual, 0644); err != nil {
			t.Fatalf("writing golden file %s: %v", goldenPath, err)
		}
		return
	}

	want, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("reading golden file %s: %v (run with -update if this is a new golden file)", goldenPath, err)
	}

	if string(want) != string(actual) {
		t.Errorf("%s does not match generated output; run `go test ./cmd/andamio -run %s -update` if this change is intentional", goldenPath, t.Name())
	}
}

func TestCommandSurfaceGolden(t *testing.T) {
	actual := CommandSurface(rootCmd)
	compareOrUpdateGolden(t, filepath.Join("testdata", "golden", "commands.golden"), []byte(actual))
}

func TestSchemaSurfaceGolden(t *testing.T) {
	tmp := filepath.Join(t.TempDir(), "schema.txt")

	if err := schemasnapshot.Generate(schemaSrcDirs, tmp); err != nil {
		t.Fatalf("generating schema snapshot: %v", err)
	}

	actual, err := os.ReadFile(tmp)
	if err != nil {
		t.Fatalf("reading generated schema: %v", err)
	}

	compareOrUpdateGolden(t, filepath.Join("testdata", "golden", "schema.golden"), actual)
}
