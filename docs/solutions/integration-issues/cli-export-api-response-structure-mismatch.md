---
title: "Course export produces empty files due to API response structure mismatch"
date: 2026-03-16
category: integration-issues
tags:
  - api-parsing
  - response-structure
  - json-extraction
  - debugging
components:
  - cmd/andamio/course_export.go
symptoms:
  - "outline.md has empty title and no SLTs listed"
  - "lesson-N.md files are 0 bytes"
  - "No images downloaded to assets/ directory"
root_cause: "Code assumed API response structure that differed from actual structure - SLTs nested at data.slts, lessons embedded in SLT response, module title in separate endpoint"
severity: high
time_to_resolve: "2 hours"
---

# Course Export Empty Files - API Response Structure Mismatch

## Problem

After implementing the `course export` command, running it produced:
- `outline.md` with empty title and no SLTs
- `lesson-N.md` files that were 0 bytes
- No images downloaded

The code was making successful API calls but extracting data from wrong paths in the JSON response.

## Root Causes

| Assumption | Reality |
|------------|---------|
| SLTs at `response["data"]` (array) | SLTs at `response["data"]["slts"]` (nested) |
| Lessons fetched via separate API calls | Lessons embedded in each SLT object |
| Module title in SLT response | Module title requires separate modules endpoint |
| Only `image` node type | Also `imageBlock` type (inside list items) |

## Solution

### Fix 1: SLT Extraction Path

```go
// WRONG - assumes data is array
sltsData, ok := sltsResp["data"].([]interface{})

// RIGHT - data is a map containing slts array
dataWrapper, ok := sltsResp["data"].(map[string]interface{})
if !ok {
    return nil, fmt.Errorf("unexpected response format")
}
sltsData, ok := dataWrapper["slts"].([]interface{})
```

### Fix 2: Extract Embedded Lessons

Lessons are already in the SLT response - no parallel fetch needed:

```go
// Lessons embedded in each SLT
for i, sltItem := range sltsData {
    sltMap, _ := sltItem.(map[string]interface{})

    // Extract embedded lesson content
    var lessonContent map[string]interface{}
    if lesson, ok := sltMap["lesson"].(map[string]interface{}); ok {
        if contentJSON, ok := lesson["content_json"].(map[string]interface{}); ok {
            lessonContent = contentJSON
        }
    }

    data.SLTs[i] = SLTData{
        Index:  i + 1,
        Lesson: map[string]interface{}{"content_json": lessonContent},
    }
}
```

### Fix 3: Fetch Module Title Separately

```go
// Module title not in SLT response - fetch from modules endpoint
var modulesResp map[string]interface{}
c.Get("/api/v2/course/user/modules/"+url.PathEscape(courseID), &modulesResp)

if modules, ok := modulesResp["data"].([]interface{}); ok {
    for _, m := range modules {
        mod, _ := m.(map[string]interface{})
        if content, ok := mod["content"].(map[string]interface{}); ok {
            if code, _ := content["course_module_code"].(string); code == moduleCode {
                data.Title, _ = content["title"].(string)
                break
            }
        }
    }
}
```

### Fix 4: Handle imageBlock in List Items

```go
func renderListItem(node map[string]interface{}, prefix string) (string, []string) {
    // ... existing paragraph/list handling ...

    } else if childType == "image" || childType == "imageBlock" {
        if attrs, ok := childMap["attrs"].(map[string]interface{}); ok {
            src, _ := attrs["src"].(string)
            alt, _ := attrs["alt"].(string)
            if src != "" {
                imageURLs = append(imageURLs, src)
                filename := filepath.Base(src)
                buf.WriteString(fmt.Sprintf("![%s](assets/%s)\n", alt, filename))
            }
        }
    }
}
```

### Fix 5: Flexible Lesson Content Extraction

Support both embedded and nested structures:

```go
func convertLessonToMarkdown(lesson map[string]interface{}) (string, []string) {
    var contentJSON map[string]interface{}

    // Try direct content_json (embedded in SLT response)
    if cj, ok := lesson["content_json"].(map[string]interface{}); ok {
        contentJSON = cj
    } else if data, ok := lesson["data"].(map[string]interface{}); ok {
        // Try nested structure (from separate API call)
        if lessonData, ok := data["lesson"].(map[string]interface{}); ok {
            if cj, ok := lessonData["content_json"].(map[string]interface{}); ok {
                contentJSON = cj
            }
        }
    }

    if contentJSON == nil {
        return "", nil
    }
    return tiptapToMarkdown(contentJSON)
}
```

## Debugging Approach

1. **Check actual API response:**
   ```bash
   ./andamio course slts <course-id> <module-code> --output json | head -100
   ```

2. **Compare expected vs actual structure:**
   - Expected: `{"data": [...]}`
   - Actual: `{"data": {"slts": [...], "course_id": "..."}}`

3. **Trace data flow:** Follow where extracted values are used and why they're nil/empty

## Prevention

1. **Always inspect API responses** before writing extraction code:
   ```bash
   ./andamio <command> --output json | jq .
   ```

2. **Add debug logging** during development:
   ```go
   fmt.Printf("DEBUG: response keys: %v\n", reflect.ValueOf(resp).MapKeys())
   ```

3. **Write integration tests** that use real API responses (or recorded fixtures)

4. **Document expected response structure** in comments near extraction code

## Related

- [CLI Course Export Implementation](../feature-implementations/cli-course-export-tiptap-conversion.md)
- [CLI Auth Middleware Mismatch](cli-api-auth-middleware-mismatch.md) - similar API structure issues
