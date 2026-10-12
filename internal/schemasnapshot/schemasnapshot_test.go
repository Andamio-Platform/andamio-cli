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
	got, err := Generate([]string{"testdata/fixture"})
	if err != nil {
		t.Fatalf("Generate: %v", err)
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

// TestGenerate covers the shapes the fixture doesn't: each case writes its
// own sources to a temp dir, which also keeps the deliberately broken file
// out of the repo (and out of CI's gofmt step).
func TestGenerate(t *testing.T) {
	tests := []struct {
		name    string
		files   map[string]string // path relative to the temp root -> source
		dirs    []string          // srcDirs, relative to the temp root
		want    []string
		wantErr string
	}{
		{
			name: "embedded fields",
			files: map[string]string{"a.go": `package a

type Base struct {
	ID string ` + "`json:\"id\"`" + `
}

type inner struct {
	X string ` + "`json:\"x\"`" + `
}

type WithEmbedded struct {
	Base
	inner
	Name string ` + "`json:\"name\"`" + `
}
`},
			dirs: []string{"."},
			want: []string{
				`Base.ID string json:"id"`,
				// Embedded fields are keyed by their type name. The
				// unexported one is kept: encoding/json still promotes its
				// exported fields into the parent's output.
				`WithEmbedded.Base Base json:""`,
				`WithEmbedded.inner inner json:""`,
				`WithEmbedded.Name string json:"name"`,
				`inner.X string json:"x"`,
			},
		},
		{
			name: `json:"-" fields`,
			files: map[string]string{"a.go": `package a

type Creds struct {
	User  string ` + "`json:\"user\"`" + `
	Token string ` + "`json:\"-\"`" + `
}

type OnlyDash struct {
	Hidden string ` + "`json:\"-\"`" + `
}
`},
			dirs: []string{"."},
			want: []string{
				// Excluding a field is part of the JSON shape, so it's
				// recorded, and a "-" tag alone still counts as tagged.
				`Creds.User string json:"user"`,
				`Creds.Token string json:"-"`,
				`OnlyDash.Hidden string json:"-"`,
			},
		},
		{
			name: "same struct name in different files",
			files: map[string]string{
				"pkga/result.go": "package pkga\n\ntype Result struct {\n\tA string `json:\"a\"`\n}\n",
				"pkgb/result.go": "package pkgb\n\ntype Result struct {\n\tB string `json:\"b\"`\n}\n",
			},
			// Walk order is pkgb first; the sort on Source must still put
			// pkga first, or the golden would churn with srcDirs order.
			dirs: []string{"pkgb", "pkga"},
			want: []string{
				`Result.A string json:"a"`,
				`Result.B string json:"b"`,
			},
		},
		{
			name: "same label twice in one file keeps source order",
			files: map[string]string{"a.go": `package a

func run(ok bool) {
	if ok {
		resp := struct {
			Z string ` + "`json:\"z\"`" + `
		}{}
		_ = resp
	} else {
		resp := struct {
			A string ` + "`json:\"a\"`" + `
		}{}
		_ = resp
	}
}
`},
			dirs: []string{"."},
			// Both are run.resp in the same file: a full tie on the sort
			// key, so Z must stay ahead of A as in the source.
			want: []string{
				`run.resp.Z string json:"z"`,
				`run.resp.A string json:"a"`,
			},
		},
		{
			name:    "file that fails to parse",
			files:   map[string]string{"broken.go": "package a\n\ntype X struct {\n"},
			dirs:    []string{"."},
			wantErr: "could not parse file",
		},
		{
			name:    "missing source directory",
			dirs:    []string{"does-not-exist"},
			wantErr: "walking",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			for rel, src := range tt.files {
				path := filepath.Join(root, rel)
				if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			var dirs []string
			for _, d := range tt.dirs {
				dirs = append(dirs, filepath.Join(root, d))
			}

			got, err := Generate(dirs)

			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want one containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Generate: %v", err)
			}
			want := strings.Join(tt.want, "\n") + "\n"
			if string(got) != want {
				t.Errorf("snapshot mismatch\n--- got ---\n%s--- want ---\n%s", got, want)
			}
		})
	}
}
