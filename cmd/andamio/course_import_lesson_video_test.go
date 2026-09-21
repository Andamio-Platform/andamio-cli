package main

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Andamio-Platform/andamio-cli/internal/output"
)

const (
	lessonVideoNew = "https://youtu.be/dQw4w9WgXcQ"
	lessonVideoOld = "https://old.example/v"
)

// writeLessonModuleDir builds a one-SLT compiled module directory whose
// lesson-1.md holds the given text.
func writeLessonModuleDir(t *testing.T, lessonMD string) string {
	t.Helper()
	dir := t.TempDir()
	for name, body := range map[string]string{"outline.md": quizOutlineMD, "lesson-1.md": lessonMD} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func existingWithLessonVideo(videoURL string) *ExistingModuleData {
	lesson := map[string]interface{}{
		"title":       "Old title",
		"description": "About the lesson",
		"image_url":   "https://cdn/x.png",
	}
	if videoURL != "" {
		lesson["video_url"] = videoURL
	}
	return &ExistingModuleData{
		Status:   "ON_CHAIN",
		SLTCount: 1,
		Lessons:  map[int]map[string]interface{}{1: lesson},
	}
}

// payloadLesson returns the dry-run payload's only lesson.
func payloadLesson(t *testing.T, lessonMD string, existing *ExistingModuleData) map[string]interface{} {
	t.Helper()
	data, err := readCompiledModule(writeLessonModuleDir(t, lessonMD))
	if err != nil {
		t.Fatalf("readCompiledModule: %v", err)
	}
	resp, err := updateModuleContent(context.Background(), nil, "course-1", data, existing, true, true, false)
	if err != nil {
		t.Fatalf("updateModuleContent: %v", err)
	}
	lessons := resp["payload"].(map[string]interface{})["lessons"].([]map[string]interface{})
	if len(lessons) != 1 {
		t.Fatalf("payload has %d lessons, want 1", len(lessons))
	}
	return lessons[0]
}

// AE1: frontmatter sets the video, and the H1 and body come from the text
// after the block.
func TestLessonFrontmatter_SetsVideoURL(t *testing.T) {
	md := "---\nvideo_url: \"" + lessonVideoNew + "\"\n---\n\n# Intro\n\nHello.\n"
	lesson := payloadLesson(t, md, existingWithLessonVideo(lessonVideoOld))

	if lesson["video_url"] != lessonVideoNew {
		t.Errorf("video_url = %v, want %s", lesson["video_url"], lessonVideoNew)
	}
	if lesson["title"] != "Intro" {
		t.Errorf("title = %v, want Intro", lesson["title"])
	}
	want, err := markdownToTiptap("Hello.", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(lesson["content_json"], want) {
		t.Errorf("content_json carries frontmatter text\n got: %v\nwant: %v", lesson["content_json"], want)
	}
	if lesson["description"] != "About the lesson" || lesson["image_url"] != "https://cdn/x.png" {
		t.Errorf("existing metadata not preserved: %v", lesson)
	}
}

// AE2 and the absent-key case: no video_url key means the stored value stays.
func TestLessonFrontmatter_AbsentKeyPreservesExisting(t *testing.T) {
	for name, md := range map[string]string{
		"no frontmatter": "# Intro\n\nHello.\n",
		"empty block":    "---\n---\n\n# Intro\n\nHello.\n",
	} {
		t.Run(name, func(t *testing.T) {
			lesson := payloadLesson(t, md, existingWithLessonVideo(lessonVideoOld))
			if lesson["video_url"] != lessonVideoOld {
				t.Errorf("video_url = %v, want the existing %s", lesson["video_url"], lessonVideoOld)
			}
		})
	}
}

// AE3: an explicit empty or null value clears the video and nothing else.
func TestLessonFrontmatter_EmptyValueClears(t *testing.T) {
	for name, block := range map[string]string{
		"empty string": "video_url: \"\"",
		"null":         "video_url:",
	} {
		t.Run(name, func(t *testing.T) {
			md := "---\n" + block + "\n---\n\n# Intro\n\nHello.\n"
			lesson := payloadLesson(t, md, existingWithLessonVideo(lessonVideoOld))
			if v, ok := lesson["video_url"]; ok {
				t.Errorf("video_url = %v, want the key absent", v)
			}
			if lesson["description"] != "About the lesson" || lesson["image_url"] != "https://cdn/x.png" {
				t.Errorf("clearing the video dropped other metadata: %v", lesson)
			}
		})
	}
}

func TestLessonFrontmatter_NewModuleCarriesVideoURL(t *testing.T) {
	md := "---\nvideo_url: " + lessonVideoNew + "\n---\n\n# Intro\n\nHello.\n"
	existing := &ExistingModuleData{Status: "DRAFT", Lessons: map[int]map[string]interface{}{}}
	lesson := payloadLesson(t, md, existing)
	if lesson["video_url"] != lessonVideoNew {
		t.Errorf("video_url = %v, want %s", lesson["video_url"], lessonVideoNew)
	}
}

// AE4 and the other parse-time refusals: each fails before any request and
// names the file.
func TestLessonFrontmatter_Errors(t *testing.T) {
	tests := []struct {
		name  string
		block string
		want  []string
	}{
		{"unknown key", "video-url: " + lessonVideoNew, []string{"lesson-1.md", "video-url", "video_url", "***"}},
		{"extra key", "video_url: " + lessonVideoNew + "\nimage_url: https://cdn/x.png", []string{"lesson-1.md", "image_url"}},
		{"not a url", "video_url: not a url", []string{"lesson-1.md", "video_url"}},
		{"ftp scheme", "video_url: ftp://host/v.mp4", []string{"lesson-1.md", "http"}},
		{"no host", "video_url: \"https://\"", []string{"lesson-1.md", "video_url"}},
		{"relative", "video_url: /relative/path", []string{"lesson-1.md", "video_url"}},
		{"non-string", "video_url: [a, b]", []string{"lesson-1.md", "video_url"}},
		{"malformed yaml", "video_url: \"https://unclosed", []string{"lesson-1.md", "frontmatter"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			md := "---\n" + tt.block + "\n---\n\n# Intro\n\nHello.\n"
			_, err := readCompiledModule(writeLessonModuleDir(t, md))
			if err == nil {
				t.Fatal("readCompiledModule succeeded, want an error")
			}
			for _, s := range tt.want {
				if !strings.Contains(err.Error(), s) {
					t.Errorf("err = %q, want it to mention %q", err, s)
				}
			}
		})
	}
}

// R5: a file that is not frontmatter imports exactly as before, including
// one that opens with a thematic break (KTD6).
func TestLessonFrontmatter_NonFrontmatterFilesUnchanged(t *testing.T) {
	for name, md := range map[string]string{
		"plain":               "# Intro\n\nHello.\n",
		"unclosed rule":       "---\n\n# Intro\n\nHello.\n",
		"two rules and prose": "---\n\nFirst para.\n\n---\n\nSecond para.\n",
		"blank lines first":   "\n\n# Intro\n\nHello.\n",
		"empty block":         "---\n---\n\n# Intro\n\nHello.\n",
		"heading in block":    "---\n# Intro\n---\n\nHello.\n",
	} {
		t.Run(name, func(t *testing.T) {
			data, err := readCompiledModule(writeLessonModuleDir(t, md))
			if err != nil {
				t.Fatalf("readCompiledModule: %v", err)
			}
			wantTitle, body := extractH1Title(md)
			want, err := markdownToTiptap(body, nil)
			if err != nil {
				t.Fatal(err)
			}
			got := data.Lessons[0]
			if got.Title != wantTitle {
				t.Errorf("title = %q, want %q", got.Title, wantTitle)
			}
			if !reflect.DeepEqual(got.TiptapJSON, want) {
				t.Errorf("content changed\n got: %v\nwant: %v", got.TiptapJSON, want)
			}
			if got.VideoURLSet {
				t.Error("VideoURLSet = true for a file with no frontmatter")
			}
		})
	}
}

// R9: a line only when the stored value changes, and never in JSON mode.
func TestLessonFrontmatter_ReportsOnlyChanges(t *testing.T) {
	set := "---\nvideo_url: " + lessonVideoNew + "\n---\n\n# Intro\n\nHello.\n"
	cleared := "---\nvideo_url: \"\"\n---\n\n# Intro\n\nHello.\n"
	tests := []struct {
		name     string
		md       string
		existing string
		want     string
	}{
		{"same value", set, lessonVideoNew, ""},
		{"different value", set, lessonVideoOld, "video set"},
		{"no existing value", set, "", "video set"},
		{"clear existing", cleared, lessonVideoOld, "video cleared"},
		{"clear nothing", cleared, "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stderr := captureStderr(t, func() { payloadLesson(t, tt.md, existingWithLessonVideo(tt.existing)) })
			reported := strings.Contains(stderr, "video set") || strings.Contains(stderr, "video cleared")
			if tt.want == "" && reported {
				t.Errorf("stderr = %q, want no video line", stderr)
			}
			if tt.want != "" && !strings.Contains(stderr, "lesson-1.md: "+tt.want) {
				t.Errorf("stderr = %q, want %q", stderr, tt.want)
			}
		})
	}

	t.Run("json mode is silent", func(t *testing.T) {
		old := output.GetFormat()
		_ = output.SetFormat(string(output.FormatJSON))
		t.Cleanup(func() { _ = output.SetFormat(string(old)) })
		stderr := captureStderr(t, func() { payloadLesson(t, set, existingWithLessonVideo(lessonVideoOld)) })
		if strings.Contains(stderr, "video set") {
			t.Errorf("stderr = %q, want no video line in JSON mode", stderr)
		}
	})
}

// R11: a URL the app cannot embed is sent, and recorded as a warning.
func TestLessonFrontmatter_WarnsOnNonYouTubeURL(t *testing.T) {
	tests := []struct {
		url  string
		warn bool
	}{
		{"https://vimeo.com/123", true},
		{"https://youtu.be/abc", true},
		{"https://youtube.com.evil.com/watch?v=dQw4w9WgXcQ", true},
		{"https://www.youtube.com/embed/videoseries", true},
		{"https://www.youtube.com/watch?v=dQw4w9WgXcQ", false},
		{"https://youtu.be/dQw4w9WgXcQ", false},
		{"https://www.youtube.com/shorts/dQw4w9WgXcQ", false},
		{"https://m.youtube.com/watch?v=dQw4w9WgXcQ&t=10", false},
		{"https://www.youtube-nocookie.com/embed/dQw4w9WgXcQ", false},
	}
	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			md := "---\nvideo_url: \"" + tt.url + "\"\n---\n\n# Intro\n\nHello.\n"
			data, err := readCompiledModule(writeLessonModuleDir(t, md))
			if err != nil {
				t.Fatalf("readCompiledModule: %v", err)
			}
			if got := data.Lessons[0].VideoURL; got != tt.url {
				t.Errorf("VideoURL = %q, want it kept as %s", got, tt.url)
			}
			warned := len(data.VideoWarnings) == 1 &&
				strings.Contains(data.VideoWarnings[0], "lesson-1.md") && strings.Contains(data.VideoWarnings[0], "YouTube")
			if warned != tt.warn {
				t.Errorf("warned = %v, want %v; warnings %q", warned, tt.warn, data.VideoWarnings)
			}
		})
	}
}

// R11: importModule prints the warning exactly once, in every output mode.
func TestImportModule_PrintsVideoWarningOnce(t *testing.T) {
	md := "---\nvideo_url: https://vimeo.com/123\n---\n\n# Intro\n\nHello.\n"
	for _, format := range []output.Format{output.FormatText, output.FormatJSON} {
		t.Run(string(format), func(t *testing.T) {
			old := output.GetFormat()
			_ = output.SetFormat(string(format))
			t.Cleanup(func() { _ = output.SetFormat(string(old)) })

			stub := &assignmentStub{listBodies: []string{listBody(t, nil, "")}}
			c, _ := stub.serve(t)
			var stderr string
			stdout := captureStdout(t, func() {
				stderr = captureStderr(t, func() {
					_, err := importModule(ImportParams{
						Ctx: context.Background(), Client: c, CourseID: "course-1",
						ModuleDir: writeLessonModuleDir(t, md), DryRun: true, Quiet: true,
					})
					if err != nil {
						t.Errorf("importModule: %v", err)
					}
				})
			})
			_ = stdout
			if n := strings.Count(stderr, "Warning: lesson-1.md"); n != 1 {
				t.Errorf("warning printed %d times, want 1; stderr %q", n, stderr)
			}
		})
	}
}
