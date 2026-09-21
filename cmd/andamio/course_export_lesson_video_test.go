package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func lessonDoc(text string) map[string]interface{} {
	return map[string]interface{}{
		"type": "doc",
		"content": []interface{}{
			map[string]interface{}{
				"type":    "paragraph",
				"content": []interface{}{map[string]interface{}{"type": "text", "text": text}},
			},
		},
	}
}

func exportLessonModule(lesson map[string]interface{}) *ModuleData {
	data := exportModuleData(nil)
	data.SLTs[0].Lesson = lesson
	return data
}

func exportedLesson(t *testing.T, lesson map[string]interface{}) (dir, content string) {
	t.Helper()
	dir = t.TempDir()
	if _, err := writeCompiledModule(dir, exportLessonModule(lesson)); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(dir, "lesson-1.md"))
	if err != nil {
		t.Fatal(err)
	}
	return dir, string(b)
}

// R7: a lesson with a video gets a frontmatter block ahead of its H1.
func TestExportLesson_WritesVideoURLFrontmatter(t *testing.T) {
	_, got := exportedLesson(t, map[string]interface{}{
		"title": "Intro", "content_json": lessonDoc("Hello."), "video_url": lessonVideoNew,
	})
	want := "---\nvideo_url: \"" + lessonVideoNew + "\"\n---\n\n# Intro\n\n"
	if !strings.HasPrefix(got, want) {
		t.Errorf("lesson-1.md = %q, want prefix %q", got, want)
	}
}

// R7: without a video the file is what it was before the feature.
func TestExportLesson_NoVideoURLIsUnchanged(t *testing.T) {
	_, plain := exportedLesson(t, map[string]interface{}{"title": "Intro", "content_json": lessonDoc("Hello.")})
	if strings.Contains(plain, "---") {
		t.Fatalf("lesson-1.md = %q, want no frontmatter", plain)
	}
	for name, v := range map[string]interface{}{"empty": "", "null": nil, "whitespace": "  "} {
		_, got := exportedLesson(t, map[string]interface{}{"title": "Intro", "content_json": lessonDoc("Hello."), "video_url": v})
		if got != plain {
			t.Errorf("%s video_url: lesson-1.md = %q, want %q", name, got, plain)
		}
	}
}

// A lesson that has a video and no body still shows the video on disk.
func TestExportLesson_VideoURLWithoutContent(t *testing.T) {
	dir, got := exportedLesson(t, map[string]interface{}{"title": "", "content_json": nil, "video_url": lessonVideoNew})
	if !strings.Contains(got, "video_url: \""+lessonVideoNew+"\"") {
		t.Errorf("lesson-1.md = %q, want the frontmatter block", got)
	}
	data, err := readCompiledModule(dir)
	if err != nil {
		t.Fatalf("re-import failed: %v", err)
	}
	if l := data.Lessons[0]; !l.VideoURLSet || l.VideoURL != lessonVideoNew || l.Title != "" {
		t.Errorf("re-imported lesson = %+v, want the video and no title", l)
	}
}

// fetchModuleData must carry video_url from the gateway's slt.lesson object
// through to the file on disk.
func TestFetchModuleData_CarriesLessonVideoURL(t *testing.T) {
	content := map[string]interface{}{
		"course_module_code": "101",
		"title":              "Module 101",
		"module_status":      "DRAFT",
		"slts": []interface{}{
			map[string]interface{}{
				"slt_text": "Do a thing",
				"lesson": map[string]interface{}{
					"title": "Intro", "content_json": lessonDoc("Hello."), "video_url": lessonVideoNew,
				},
			},
			map[string]interface{}{"slt_text": "Do another", "lesson": map[string]interface{}{"title": "Two", "content_json": lessonDoc("Hi.")}},
		},
	}
	body, err := json.Marshal(map[string]interface{}{"data": []interface{}{map[string]interface{}{"content": content}}})
	if err != nil {
		t.Fatal(err)
	}
	stub := &assignmentStub{listBodies: []string{string(body)}}
	c, _ := stub.serve(t)

	data, err := fetchModuleData(context.Background(), c, "course-1", "101")
	if err != nil {
		t.Fatalf("fetchModuleData: %v", err)
	}
	dir := t.TempDir()
	if _, err := writeCompiledModule(dir, data); err != nil {
		t.Fatal(err)
	}
	one, _ := os.ReadFile(filepath.Join(dir, "lesson-1.md"))
	two, _ := os.ReadFile(filepath.Join(dir, "lesson-2.md"))
	if !strings.HasPrefix(string(one), "---\nvideo_url: \""+lessonVideoNew+"\"\n---\n\n# Intro") {
		t.Errorf("lesson-1.md = %q, want the video frontmatter", one)
	}
	if strings.Contains(string(two), "video_url") {
		t.Errorf("lesson-2.md = %q, want no frontmatter", two)
	}
}

// R8: export then import sends the video the gateway already holds, for
// values that need YAML quoting too.
func TestExportImportRoundTrip_LessonVideoURL(t *testing.T) {
	for _, videoURL := range []string{
		lessonVideoNew,
		"https://www.youtube.com/watch?v=dQw4w9WgXcQ&t=10#frag",
		"https://example.com/a: b\"c'd #e",
	} {
		t.Run(videoURL, func(t *testing.T) {
			dir, _ := exportedLesson(t, map[string]interface{}{
				"title": "Intro", "content_json": lessonDoc("Hello."), "video_url": videoURL,
			})
			data, err := readCompiledModule(dir)
			if err != nil {
				t.Fatalf("readCompiledModule after export: %v", err)
			}
			existing := existingWithLessonVideo(videoURL)
			resp, err := updateModuleContent(context.Background(), nil, "course-1", data, existing, true, true, false)
			if err != nil {
				t.Fatal(err)
			}
			lesson := resp["payload"].(map[string]interface{})["lessons"].([]map[string]interface{})[0]
			if lesson["video_url"] != videoURL {
				t.Errorf("video_url = %v, want %s", lesson["video_url"], videoURL)
			}
			if lesson["title"] != "Intro" {
				t.Errorf("title = %v, want Intro", lesson["title"])
			}
			want, _ := markdownToTiptap("Hello.", nil)
			if !reflect.DeepEqual(lesson["content_json"], want) {
				t.Errorf("content_json = %v, want %v", lesson["content_json"], want)
			}
		})
	}
}
