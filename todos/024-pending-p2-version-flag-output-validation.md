---
status: pending
priority: p2
issue_id: "024"
tags: [code-review, cli, version, output-validation, pr-69-followup]
dependencies: []
---

# `--version --output <value>` silently coerces invalid/uppercase values to text

## Problem Statement

Every command in the CLI except `--version` rejects `--output BOGUS` with `unsupported format: BOGUS` (exit 1). The `--version` path diverges: it silently falls through to text output with exit 0 on any value that isn't literal lowercase `json`.

**Symptoms (verified against built binary):**
```
andamio --version --output JSON     # text output (wrong — other commands accept case-insensitive)
andamio --version --output xml      # text output, exit 0 (wrong — other commands exit 1)
andamio --version --output csv      # text output, exit 0 (undocumented fallback)
andamio --version --output markdown # text output, exit 0 (undocumented fallback)
```

**Root cause** (`cmd/andamio/main.go:43-58`): Cobra's `--version` path bypasses `PersistentPreRunE`, so `output.SetFormat()` — which lowercases input and validates against the known format enum — never runs. `buildVersionOutput` checks `output.Format(outputFormat) == output.FormatJSON` via a direct typed-string comparison, which is case-sensitive and accepts anything.

Found during ce:review autofix of PR #69 — cross-reviewer consensus (correctness 0.95 + testing 0.82 + agent-native + api-contract 0.85, merged confidence 1.00).

## Affected Files

- `cmd/andamio/main.go:43-58` — `buildVersionOutput`
- `internal/output/output.go:26-45` — `SetFormat` (the validation path that's skipped)
- `cmd/andamio/main.go:35-37` — `PersistentPreRunE` (where `SetFormat` normally runs)

## Options

### Option A (minimal — case-insensitive match, silent fallback preserved)

```go
if strings.EqualFold(outputFormat, "json") {
    // emit JSON
}
```

Accepts `JSON`, `Json`, `json` identically. `xml`, `csv`, etc. continue to silently fall through to text. **Partial fix** — solves the uppercase problem but preserves the "invalid value = text" divergence from other commands.

### Option B (consistent — validate and route through SetFormat)

Call `output.SetFormat(outputFormat)` at the top of `buildVersionOutput`; if it returns an error, emit the error text and set a package-level flag (or panic — `--version` short-circuits anyway) to cause non-zero exit. Then read `output.GetFormat() == output.FormatJSON`.

Challenge: `buildVersionOutput` returns a string for Cobra's template system; there's no clean error-return channel. Would need to emit the error message inline (e.g., prefix stdout/stderr) and use a side channel to signal exit code.

**Full fix** — brings `--version` into parity with other commands' `--output` handling.

### Option C (route through a custom Run handler)

Replace `SetVersionTemplate` + template function with a custom subcommand (`andamio version`) that goes through normal Cobra RunE. Handles `--output` validation via `PersistentPreRunE` like any other command. `--version` flag stays with the current template for backwards compat, but accepts only text (gives an informational message for `--output json`: "use `andamio version --output json` for structured output").

**Re-architect** — cleanest contract but larger scope.

**Preferred:** Option A as an immediate follow-up (one-line fix, handles the most likely user error). Option B or C if the team wants strict parity with other commands' `--output` handling.

## Acceptance

- [ ] Decision on Option A/B/C documented (either in commit message or a short RFC).
- [ ] `andamio --version --output JSON` produces valid JSON (case-insensitive match), OR produces a clear error.
- [ ] `andamio --version --output <invalid>` behavior is explicitly tested — either the test asserts error+exit 1 (Option B/C), or explicitly documents the silent-text-fallback (Option A) with a comment in `main_test.go`.
- [ ] New test case in `main_test.go` covering case-insensitive `JSON`/`Json`/`json` (Option A) or invalid-value rejection (Option B/C).

## Context

- **ce:review run artifact:** `.context/compound-engineering/ce-review/2026-04-22-pr69-release-hygiene/findings.md`
- **Origin PR:** https://github.com/Andamio-Platform/andamio-cli/pull/69
- **Related:** pattern established by `docs/solutions/architecture/cli-composability-audit-and-fix.md` — `--output` is the scripting surface, consumers expect consistent validation.
