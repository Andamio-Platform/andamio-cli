---
status: complete
priority: p2
issue_id: "016"
tags: [code-review, composability, json, spec, agent-native]
dependencies: []
---

# `spec fetch --output json` Emits Nothing — Agents Can't Confirm Success

## Problem Statement

`spec fetch` in JSON mode suppresses all output:

```go
if !isJSON {
    fmt.Fprintf(os.Stderr, "Saved to %s\n", outPath)
    fmt.Fprintf(os.Stderr, "API: %s v%s\n", title, version)
}
```

When `--output json` is set, the command returns exit code 0 but emits nothing to stdout. An agent piping the output gets an empty string and cannot:
- Confirm the file was written
- Learn which API version was fetched
- Know the output path

This violates the principle that `--output json` is the scripting surface. A successful side-effecting command should emit a confirmation JSON object.

## Findings

- **Source**: Agent-native reviewer (P2 #6)
- **Location**: `cmd/andamio/spec.go:72-83`

## Proposed Solutions

### Option A: Emit confirmation JSON on success (Recommended)

```go
if isJSON {
    return output.PrintJSON(map[string]interface{}{
        "path":        outPath,
        "api_version": fmt.Sprintf("%v", version),
        "api_title":   fmt.Sprintf("%v", title),
    })
}
fmt.Fprintf(os.Stderr, "Saved to %s\n", outPath)
fmt.Fprintf(os.Stderr, "API: %s v%s\n", title, version)
```

**Pros:** Consistent with how other commands emit results in JSON mode. Agents can confirm success.
**Cons:** Adds a small JSON output to a command that was previously "fire and forget."
**Effort:** Small
**Risk:** None

### Option B: Always write confirmation to stderr, emit nothing to stdout

Keep current behavior — just document that `spec fetch` has no stdout output.

**Pros:** Simpler.
**Cons:** Breaks the scripting contract. Agents cannot programmatically confirm success.
**Effort:** None
**Risk:** None to code; breaks composability contract

## Recommended Action

Option A. One small JSON object confirming the fetch is the correct composable behavior for a side-effecting command.

## Technical Details

- **Affected files**: `cmd/andamio/spec.go:72-83`
- **PR**: #15 fix/composability-gaps

## Acceptance Criteria

- [ ] `andamio spec fetch --output json | jq .path` returns `"openapi.json"`
- [ ] `andamio spec fetch --output json | jq .api_version` returns a version string
- [ ] Exit code 0 on success, non-zero on failure

## Work Log

- 2026-03-18: Flagged by agent-native reviewer during PR #15 review
