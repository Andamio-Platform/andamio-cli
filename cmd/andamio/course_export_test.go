package main

import (
	"strings"
	"testing"
)

func TestTiptapToMarkdown(t *testing.T) {
	tests := []struct {
		name     string
		input    map[string]interface{}
		wantMD   string
		wantURLs []string
	}{
		{
			name: "simple paragraph",
			input: map[string]interface{}{
				"type": "doc",
				"content": []interface{}{
					map[string]interface{}{
						"type": "paragraph",
						"content": []interface{}{
							map[string]interface{}{
								"type": "text",
								"text": "Hello world",
							},
						},
					},
				},
			},
			wantMD:   "Hello world\n\n",
			wantURLs: nil,
		},
		{
			name: "heading level 2",
			input: map[string]interface{}{
				"type": "doc",
				"content": []interface{}{
					map[string]interface{}{
						"type": "heading",
						"attrs": map[string]interface{}{
							"level": float64(2),
						},
						"content": []interface{}{
							map[string]interface{}{
								"type": "text",
								"text": "Section Title",
							},
						},
					},
				},
			},
			wantMD:   "## Section Title\n\n",
			wantURLs: nil,
		},
		{
			name: "bold text",
			input: map[string]interface{}{
				"type": "doc",
				"content": []interface{}{
					map[string]interface{}{
						"type": "paragraph",
						"content": []interface{}{
							map[string]interface{}{
								"type": "text",
								"text": "bold text",
								"marks": []interface{}{
									map[string]interface{}{
										"type": "bold",
									},
								},
							},
						},
					},
				},
			},
			wantMD:   "**bold text**\n\n",
			wantURLs: nil,
		},
		{
			name: "italic text",
			input: map[string]interface{}{
				"type": "doc",
				"content": []interface{}{
					map[string]interface{}{
						"type": "paragraph",
						"content": []interface{}{
							map[string]interface{}{
								"type": "text",
								"text": "italic text",
								"marks": []interface{}{
									map[string]interface{}{
										"type": "italic",
									},
								},
							},
						},
					},
				},
			},
			wantMD:   "*italic text*\n\n",
			wantURLs: nil,
		},
		{
			name: "code span",
			input: map[string]interface{}{
				"type": "doc",
				"content": []interface{}{
					map[string]interface{}{
						"type": "paragraph",
						"content": []interface{}{
							map[string]interface{}{
								"type": "text",
								"text": "code",
								"marks": []interface{}{
									map[string]interface{}{
										"type": "code",
									},
								},
							},
						},
					},
				},
			},
			wantMD:   "`code`\n\n",
			wantURLs: nil,
		},
		{
			name: "link",
			input: map[string]interface{}{
				"type": "doc",
				"content": []interface{}{
					map[string]interface{}{
						"type": "paragraph",
						"content": []interface{}{
							map[string]interface{}{
								"type": "text",
								"text": "click here",
								"marks": []interface{}{
									map[string]interface{}{
										"type": "link",
										"attrs": map[string]interface{}{
											"href": "https://example.com",
										},
									},
								},
							},
						},
					},
				},
			},
			wantMD:   "[click here](https://example.com)\n\n",
			wantURLs: nil,
		},
		{
			name: "strikethrough",
			input: map[string]interface{}{
				"type": "doc",
				"content": []interface{}{
					map[string]interface{}{
						"type": "paragraph",
						"content": []interface{}{
							map[string]interface{}{
								"type": "text",
								"text": "deleted",
								"marks": []interface{}{
									map[string]interface{}{
										"type": "strike",
									},
								},
							},
						},
					},
				},
			},
			wantMD:   "~~deleted~~\n\n",
			wantURLs: nil,
		},
		{
			name: "bullet list",
			input: map[string]interface{}{
				"type": "doc",
				"content": []interface{}{
					map[string]interface{}{
						"type": "bulletList",
						"content": []interface{}{
							map[string]interface{}{
								"type": "listItem",
								"content": []interface{}{
									map[string]interface{}{
										"type": "paragraph",
										"content": []interface{}{
											map[string]interface{}{
												"type": "text",
												"text": "Item 1",
											},
										},
									},
								},
							},
							map[string]interface{}{
								"type": "listItem",
								"content": []interface{}{
									map[string]interface{}{
										"type": "paragraph",
										"content": []interface{}{
											map[string]interface{}{
												"type": "text",
												"text": "Item 2",
											},
										},
									},
								},
							},
						},
					},
				},
			},
			wantMD:   "- Item 1\n- Item 2\n\n",
			wantURLs: nil,
		},
		{
			name: "ordered list",
			input: map[string]interface{}{
				"type": "doc",
				"content": []interface{}{
					map[string]interface{}{
						"type": "orderedList",
						"content": []interface{}{
							map[string]interface{}{
								"type": "listItem",
								"content": []interface{}{
									map[string]interface{}{
										"type": "paragraph",
										"content": []interface{}{
											map[string]interface{}{
												"type": "text",
												"text": "First",
											},
										},
									},
								},
							},
							map[string]interface{}{
								"type": "listItem",
								"content": []interface{}{
									map[string]interface{}{
										"type": "paragraph",
										"content": []interface{}{
											map[string]interface{}{
												"type": "text",
												"text": "Second",
											},
										},
									},
								},
							},
						},
					},
				},
			},
			wantMD:   "1. First\n2. Second\n\n",
			wantURLs: nil,
		},
		{
			name: "code block",
			input: map[string]interface{}{
				"type": "doc",
				"content": []interface{}{
					map[string]interface{}{
						"type": "codeBlock",
						"attrs": map[string]interface{}{
							"language": "go",
						},
						"content": []interface{}{
							map[string]interface{}{
								"type": "text",
								"text": "func main() {}",
							},
						},
					},
				},
			},
			wantMD:   "```go\nfunc main() {}\n```\n\n",
			wantURLs: nil,
		},
		{
			name: "blockquote",
			input: map[string]interface{}{
				"type": "doc",
				"content": []interface{}{
					map[string]interface{}{
						"type": "blockquote",
						"content": []interface{}{
							map[string]interface{}{
								"type": "paragraph",
								"content": []interface{}{
									map[string]interface{}{
										"type": "text",
										"text": "Quote text",
									},
								},
							},
						},
					},
				},
			},
			wantMD:   "> Quote text\n\n",
			wantURLs: nil,
		},
		{
			name: "horizontal rule",
			input: map[string]interface{}{
				"type": "doc",
				"content": []interface{}{
					map[string]interface{}{
						"type": "horizontalRule",
					},
				},
			},
			wantMD:   "---\n\n",
			wantURLs: nil,
		},
		{
			name: "image",
			input: map[string]interface{}{
				"type": "doc",
				"content": []interface{}{
					map[string]interface{}{
						"type": "image",
						"attrs": map[string]interface{}{
							"src": "https://cdn.example.com/image.png",
							"alt": "My image",
						},
					},
				},
			},
			wantMD:   "![My image](assets/image.png)\n\n",
			wantURLs: []string{"https://cdn.example.com/image.png"},
		},
		{
			name: "hard break",
			input: map[string]interface{}{
				"type": "doc",
				"content": []interface{}{
					map[string]interface{}{
						"type": "paragraph",
						"content": []interface{}{
							map[string]interface{}{
								"type": "text",
								"text": "Line 1",
							},
							map[string]interface{}{
								"type": "hardBreak",
							},
							map[string]interface{}{
								"type": "text",
								"text": "Line 2",
							},
						},
					},
				},
			},
			wantMD:   "Line 1  \nLine 2\n\n",
			wantURLs: nil,
		},
		{
			name: "mixed content",
			input: map[string]interface{}{
				"type": "doc",
				"content": []interface{}{
					map[string]interface{}{
						"type": "heading",
						"attrs": map[string]interface{}{
							"level": float64(1),
						},
						"content": []interface{}{
							map[string]interface{}{
								"type": "text",
								"text": "Title",
							},
						},
					},
					map[string]interface{}{
						"type": "paragraph",
						"content": []interface{}{
							map[string]interface{}{
								"type": "text",
								"text": "Some ",
							},
							map[string]interface{}{
								"type": "text",
								"text": "bold",
								"marks": []interface{}{
									map[string]interface{}{
										"type": "bold",
									},
								},
							},
							map[string]interface{}{
								"type": "text",
								"text": " text.",
							},
						},
					},
				},
			},
			wantMD:   "# Title\n\nSome **bold** text.\n\n",
			wantURLs: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotMD, gotURLs := tiptapToMarkdown(tt.input)
			if gotMD != tt.wantMD {
				t.Errorf("tiptapToMarkdown() markdown = %q, want %q", gotMD, tt.wantMD)
			}
			if len(gotURLs) != len(tt.wantURLs) {
				t.Errorf("tiptapToMarkdown() urls = %v, want %v", gotURLs, tt.wantURLs)
			} else {
				for i, url := range gotURLs {
					if url != tt.wantURLs[i] {
						t.Errorf("tiptapToMarkdown() url[%d] = %q, want %q", i, url, tt.wantURLs[i])
					}
				}
			}
		})
	}
}

func TestApplyMarks(t *testing.T) {
	tests := []struct {
		name  string
		text  string
		node  map[string]interface{}
		want  string
	}{
		{
			name: "no marks",
			text: "plain",
			node: map[string]interface{}{},
			want: "plain",
		},
		{
			name: "bold",
			text: "text",
			node: map[string]interface{}{
				"marks": []interface{}{
					map[string]interface{}{"type": "bold"},
				},
			},
			want: "**text**",
		},
		{
			name: "bold and italic",
			text: "text",
			node: map[string]interface{}{
				"marks": []interface{}{
					map[string]interface{}{"type": "bold"},
					map[string]interface{}{"type": "italic"},
				},
			},
			want: "***text***",
		},
		{
			name: "link with href",
			text: "click",
			node: map[string]interface{}{
				"marks": []interface{}{
					map[string]interface{}{
						"type": "link",
						"attrs": map[string]interface{}{
							"href": "https://example.com",
						},
					},
				},
			},
			want: "[click](https://example.com)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := applyMarks(tt.text, tt.node)
			if got != tt.want {
				t.Errorf("applyMarks() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSlugify(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"Hello World", "hello-world"},
		{"My Course Title!", "my-course-title"},
		{"Test 123", "test-123"},
		{"  Spaces  ", "spaces"},
		{"CamelCase", "camelcase"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := slugify(tt.input)
			if got != tt.want {
				t.Errorf("slugify(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestGenerateOutline(t *testing.T) {
	data := &ModuleData{
		ModuleCode: "101",
		Title:      "Getting Started",
		SLTs: []SLTData{
			{Index: 1, Text: "Understand the basics"},
			{Index: 2, Text: "Apply the concepts"},
		},
	}

	got := generateOutline(data)

	// Check YAML frontmatter
	if !strings.Contains(got, "---\ntitle: Getting Started") {
		t.Error("Missing title in frontmatter")
	}
	if !strings.Contains(got, "code: 101") {
		t.Error("Missing code in frontmatter")
	}

	// Check content
	if !strings.Contains(got, "# Getting Started") {
		t.Error("Missing title heading")
	}
	if !strings.Contains(got, "## SLTs") {
		t.Error("Missing SLTs heading")
	}
	if !strings.Contains(got, "1. Understand the basics") {
		t.Error("Missing SLT 1")
	}
	if !strings.Contains(got, "2. Apply the concepts") {
		t.Error("Missing SLT 2")
	}
}
