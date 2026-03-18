---
title: "Fix silent witness dropping, URL validation bypass, and 4 additional issues in tx signing commands"
date: 2026-03-18
severity: high
component:
  - internal/cardano/sign.go
  - internal/submit/client.go
  - cmd/andamio/tx_build.go
  - cmd/andamio/tx_sign.go
  - cmd/andamio/tx_submit.go
category: code-review
tags:
  - security
  - cardano
  - transaction-signing
  - cbor
  - multi-sig
  - url-validation
  - input-validation
  - dos-prevention
status: resolved
---

# Fix silent witness dropping, URL validation bypass, and 4 additional issues in tx signing commands

## Problem

During code review of the new Cardano transaction signing feature (`feat/tx-signing` branch), 6 review agents identified 6 issues across the new `tx build`, `tx sign`, `tx submit`, and `tx register` commands. Two were critical (P1) with financial and security risk, four were important (P2).

## Findings

### P1-1: Silent Witness Dropping in `assembleSignedTx` (Financial Risk)

In `internal/cardano/sign.go`, existing VKey witnesses were decoded with the error silently discarded:

```go
// BEFORE — error discarded, existing witnesses silently lost
_ = cbor.Unmarshal(existing, &existingVKeyWitnesses)

for _, w := range existingVKeyWitnesses {
    encoded, err := cbor.Marshal(w)
    if err != nil {
        continue // silently drops the witness
    }
    allWitnesses = append(allWitnesses, encoded)
}
```

A multi-sig transaction could lose existing witnesses, appearing to succeed locally but failing on-chain. The user would waste transaction fees with no indication of what went wrong.

### P1-2: Submit URL Validation Bypass

In `cmd/andamio/tx_submit.go`, when `--submit-url` was passed as a flag (not from config), the URL went directly to `SubmitTransaction` without calling `ValidateSubmitURL`. API keys passed via `--submit-header` could be sent over plaintext HTTP.

### P2-3: Custom `bytesEqual` Duplicated Stdlib

A hand-rolled `bytesEqual()` function in `internal/cardano/sign.go` duplicated `bytes.Equal` from Go's standard library.

### P2-4: Unbounded Response Read from Untrusted Server

`internal/submit/client.go` used `io.ReadAll(resp.Body)` with no size limit. A malicious submit API could force unbounded memory allocation.

### P2-5: Text Output Panic on Short Hex

`tx_build.go` and `tx_sign.go` sliced hex strings with `s[:16]` without checking length, causing panics on short or malformed responses.

### P2-6: Empty Body Allowed on `tx build`

Neither `--body` nor `--body-file` was required, allowing empty POSTs that produced cryptic API errors instead of a clear "body required" message.

## Root Cause

The P1 issues share a common root: **validation at the wrong layer**. URL validation was applied at config-write time but not at use-time. CBOR decode errors were swallowed because the happy path worked in testing — the failure mode only manifests with pre-populated witness sets (multi-sig transactions).

## Solution

### P1-1: Return errors, preserve raw bytes

Changed witness decoding to use `[]cbor.RawMessage` (preserving raw CBOR bytes without re-encoding) and returned errors instead of swallowing them:

```go
// AFTER — errors returned, raw bytes preserved
if err := cbor.Unmarshal(existing, &existingVKeyWitnesses); err != nil {
    return nil, fmt.Errorf("failed to decode existing VKey witnesses: %w", err)
}

// existingVKeyWitnesses is []cbor.RawMessage — no re-encode needed
allWitnesses := append(existingVKeyWitnesses, cbor.RawMessage(newWitnessRaw))
```

### P1-2: Validate URL at point of use

Added `config.ValidateSubmitURL(submitURL)` after URL resolution, before calling `submit.SubmitTransaction`. Validation now runs regardless of whether the URL came from a flag or config.

### P2-3: Use `bytes.Equal`

Replaced `bytesEqual()` with `bytes.Equal()` from stdlib. Deleted the custom function and its test.

### P2-4: Limit response reads

```go
// BEFORE
body, err := io.ReadAll(resp.Body)

// AFTER
body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20)) // 1 MB limit
```

### P2-5: Guard string slicing

```go
if len(s) > 32 {
    fmt.Printf("TX: %s...%s\n", s[:16], s[len(s)-16:])
} else {
    fmt.Printf("TX: %s\n", s)
}
```

### P2-6: Require body input

```go
if bodyStr == "" && bodyFile == "" {
    return fmt.Errorf("either --body or --body-file is required")
}
```

## Prevention

| Finding | Prevention Rule | Automation |
|---------|----------------|------------|
| Silent error swallowing | Never discard errors from marshal/unmarshal or crypto ops | `errcheck` linter |
| URL validation bypass | Validate at point of use, not point of entry | Integration tests with flag-supplied invalid URLs |
| Reimplementing stdlib | Check `bytes`, `strings`, `slices` before writing utility functions | `gocritic` linter |
| Unbounded reads | Always wrap network `io.ReadAll` with `io.LimitReader` | `semgrep` rule for `io.ReadAll($RESP.Body)` |
| Unsafe slicing | Always check `len(s) >= N` before `s[:N]` | Fuzz tests, `gocritic` |
| Missing validation | Use `MarkFlagRequired` or explicit checks with actionable error messages | Table-driven test exercising every command with missing args |

## Related Documents

- [CLI Security Hardening](../security-issues/cli-security-hardening-input-validation.md) — HTTP client hardening, URL validation, file permissions
- [CLI Composability Audit](../architecture/cli-composability-audit-and-fix.md) — Error handling patterns, stderr/stdout separation, exit codes
- [Non-Interactive CLI](../architecture/non-interactive-cli-stdin-picker-removal.md) — No stdin reads, `ExactArgs`, composability rules
- [Export/Import Silent Data Loss](../logic-errors/export-import-round-trip-title-preservation.md) — Prior instance of silent error swallowing
- [API Response Structure Mismatch](../integration-issues/cli-export-api-response-structure-mismatch.md) — Prior instance of silent data loss from unvalidated assumptions
