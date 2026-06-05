---
title: "fix: Image upload 404 on mainnet — unify app-URL host derivation"
status: completed
date: 2026-06-05
type: fix
origin: "GitHub issue #117"
depth: lightweight
---

# fix: Image upload 404 on mainnet — unify app-URL host derivation

## Summary

`course import` image uploads 404 on mainnet because `uploadImage` derives the
app host with a single `.api.` → `.app.` string replace. The mainnet base URL
`https://api.andamio.io` contains `//api.`, not `.api.`, so the replace no-ops
and the CLI POSTs images to the API gateway (`api.andamio.io/api/upload` → 404)
instead of the app (`app.andamio.io/api/upload`). Preprod works only because
`preprod.api.andamio.io` happens to contain `.api.`.

The correct dual-host logic already lives inline in `buildAuthURL`
(`cmd/andamio/user.go:644-648`). Two other sites copy the broken single-replace:
`uploadImage` (the reported bug) and `deriveAppOrigin` (`cmd/andamio/dev.go:51`,
a latent mainnet bug in the dev browser-login Origin allow-list).

This plan extracts a single shared `appURLFromBase(baseURL string) string` helper
and routes all three call sites through it — fixing the reported 404, fixing the
latent dev-login bug, and removing the duplicated derivation so the next mainnet
host shape can't silently break one site but not another.

---

## Problem Frame

App URLs are derived from the configured API base URL by swapping the `api`
host segment for `app`. The hostname has two shapes in production use:

| Base URL | Correct app URL | `.api.`→`.app.` (single) | `//api.`→`//app.` |
|----------|-----------------|--------------------------|-------------------|
| `https://preprod.api.andamio.io` | `https://preprod.app.andamio.io` | ✅ works | ✗ no-op |
| `https://api.andamio.io` | `https://app.andamio.io` | ✗ **no-op (bug)** | ✅ works |

No single replace covers both. `buildAuthURL` already branches correctly; the
other two sites do not. The fix is to make one correct implementation and reuse
it.

**Affected sites (all in `cmd/andamio/`):**

1. `course_import.go:1416` `uploadImage` — single replace → **mainnet 404** (issue #117).
2. `dev.go:51` `deriveAppOrigin` — single replace → on mainnet returns
   `https://api.andamio.io` as the allowed Origin, so the browser POST from
   `https://app.andamio.io` fails the exact-string Origin allow-list and dev
   browser-login breaks. Latent today (preprod-only testing has masked it).
3. `user.go:644-648` `buildAuthURL` — already correct; refactor to call the
   shared helper so there is exactly one implementation.

---

## Requirements

- **R1.** `uploadImage` POSTs to the app host (not the API host) for both
  preprod and mainnet base URLs. (issue #117 root requirement)
- **R2.** `deriveAppOrigin` returns the app scheme+host for both base-URL shapes,
  preserving its existing empty-string-on-unparseable contract.
- **R3.** A single shared helper is the only place the `api`→`app` host swap
  lives; all three call sites use it.
- **R4.** `buildAuthURL` output is unchanged for both base-URL shapes (no
  regression to the existing user/dev browser flows).

---

## Key Technical Decisions

**KTD1 — One helper, dual-host logic.** Add
`appURLFromBase(baseURL string) string` implementing the proven branch from
`buildAuthURL`: if the URL contains `.api.`, replace `.api.` → `.app.`; else
replace `//api.` → `//app.`. This is a pure string transform returning the full
app base URL (scheme + host + any path), matching what `buildAuthURL` and
`uploadImage` need. `deriveAppOrigin` keeps its own `url.Parse` + empty-string
guard, but sources its `appURL` from this helper instead of the inline replace.

**KTD2 — Home for the helper: `cmd/andamio/user.go`.** Place it adjacent to
`buildAuthURL` (its origin logic) rather than introducing a new file. `user.go`,
`dev.go`, and `course_import.go` are all `package main`, so the unexported
helper is visible to all three with no new file or import churn. (A dedicated
`apphost.go` is a reasonable alternative but adds a file for one small function;
keep it co-located with the existing correct implementation.)

**KTD3 — Preserve `deriveAppOrigin`'s safe-failure contract.** The helper does
not parse or validate; `deriveAppOrigin` still does `url.Parse` and returns `""`
on unparseable/empty input so the dev-login Origin allow-list fails closed.
Empty `baseURL` must still flow to `""` — the helper returns `""` unchanged for
`""` input (no `//api.` or `.api.` to match), and `deriveAppOrigin`'s existing
`if baseURL == ""` guard stays.

---

## Implementation Units

### U1. Add shared `appURLFromBase` helper and adopt it in `buildAuthURL`

**Goal:** Single source of truth for the `api`→`app` host swap; prove parity
with the existing correct behavior.

**Requirements:** R3, R4

**Dependencies:** none

**Files:**
- `cmd/andamio/user.go` — add `appURLFromBase`; replace the inline 4-line branch
  in `buildAuthURL` (lines 644-648) with a call to it.
- `cmd/andamio/user_test.go` — extend `TestBuildAuthURL` coverage / add
  `TestAppURLFromBase`.

**Approach:** Lift the exact branch from `buildAuthURL` into
`appURLFromBase(baseURL string) string`. `buildAuthURL` then does
`appURL := appURLFromBase(baseURL)`. No behavior change — `TestBuildAuthURL`
(production + preprod cases at `user_test.go:45-46`) must still pass byte-for-byte.

**Patterns to follow:** Existing dual-host branch and doc comment at
`user.go:638-649`; table-driven test style in `TestBuildAuthURL`
(`user_test.go:39-56`).

**Test scenarios:**
- `appURLFromBase("https://preprod.api.andamio.io")` → `https://preprod.app.andamio.io`.
- `appURLFromBase("https://api.andamio.io")` → `https://app.andamio.io` (the bug case).
- `appURLFromBase("https://mainnet.api.andamio.io")` → `https://mainnet.app.andamio.io`.
- `appURLFromBase("")` → `""` (no panic, no partial transform).
- Existing `TestBuildAuthURL` production + preprod cases still produce identical URLs (regression guard for R4).

**Verification:** `go test ./cmd/andamio/ -run 'TestBuildAuthURL|TestAppURLFromBase'` passes; no diff in `buildAuthURL` output for known inputs.

### U2. Route `uploadImage` through the helper (fixes the reported 404)

**Goal:** `course import` uploads land on the app host for mainnet and preprod.

**Requirements:** R1

**Dependencies:** U1

**Files:**
- `cmd/andamio/course_import.go` — replace line 1416
  (`appURL := strings.Replace(cfg.BaseURL, ".api.", ".app.", 1)`) with
  `appURL := appURLFromBase(cfg.BaseURL)`.
- `cmd/andamio/course_import_test.go` — add a focused test asserting the upload
  URL derivation for mainnet vs preprod (see scenarios).

**Approach:** One-line swap at the derivation; `uploadURL := appURL + "/api/upload"`
is unchanged. If `strings` becomes unused in the file after the swap, drop the
import (verify — the file almost certainly still uses `strings` elsewhere for
MIME/extension handling, so likely no import change). Confirm whether
`uploadImage`'s URL assembly is testable in isolation; if it is not currently
factored for a unit test, assert on `appURLFromBase(cfg.BaseURL)+"/api/upload"`
directly rather than refactoring `uploadImage`'s HTTP path in this fix.

**Patterns to follow:** `uploadImage` signature and surrounding logic at
`course_import.go:1414-1417`.

**Test scenarios:**
- Derived upload URL for `cfg.BaseURL = "https://api.andamio.io"` is
  `https://app.andamio.io/api/upload` (mainnet — the reported 404 path). Covers issue #117.
- Derived upload URL for `cfg.BaseURL = "https://preprod.api.andamio.io"` is
  `https://preprod.app.andamio.io/api/upload` (preprod regression guard — must keep working).

**Verification:** `go test ./cmd/andamio/` green; manual confidence check —
trace that a mainnet config now targets `app.andamio.io`.

### U3. Route `deriveAppOrigin` through the helper (fixes latent dev-login bug)

**Goal:** Dev browser-login Origin allow-list is correct on mainnet; helper is
the only derivation site.

**Requirements:** R2, R3

**Dependencies:** U1

**Files:**
- `cmd/andamio/dev.go` — replace line 51
  (`appURL := strings.Replace(baseURL, ".api.", ".app.", 1)`) with
  `appURL := appURLFromBase(baseURL)`. Keep the `if baseURL == ""` guard and the
  `url.Parse` + empty-on-error return intact.
- `cmd/andamio/dev_test.go` — add/extend `deriveAppOrigin` coverage for the
  mainnet host shape and the empty/unparseable contract.

**Approach:** Swap only the derivation line. The parse-and-validate guard around
it is unchanged, so the fail-closed behavior (empty Origin ⇒ no browser POST
allowed) is preserved. Update the doc comment at `dev.go:38-46` if it still
claims a single `.api.` substitution.

**Patterns to follow:** Existing `deriveAppOrigin` structure at `dev.go:47-57`;
the test at `dev_test.go:~1353` that asserts the preprod-callback Origin.

**Test scenarios:**
- `deriveAppOrigin("https://api.andamio.io")` → `https://app.andamio.io`
  (mainnet — the latent bug case; previously returned `https://api.andamio.io`).
- `deriveAppOrigin("https://preprod.api.andamio.io")` → `https://preprod.app.andamio.io` (regression guard).
- `deriveAppOrigin("")` → `""` (empty contract preserved).
- `deriveAppOrigin("://bad")` or other unparseable → `""` (fail-closed contract preserved).

**Verification:** `go test ./cmd/andamio/` green, including any existing dev
browser-login Origin tests; the helper is now referenced by all three sites
(`grep -n 'strings.Replace.*api' cmd/andamio/*.go` returns nothing).

---

## Scope Boundaries

**In scope:** The three app-URL derivation sites in `cmd/andamio/`; one shared
helper; tests proving mainnet + preprod for each.

### Deferred to Follow-Up Work
- **BaseURL validation** — issue #108 / `dev.go:44` already track validating
  malformed base URLs at config time. This fix does not add validation; it only
  makes derivation correct for the two real host shapes.
- **Re-import of the 11 broken PBL images** (course `b7795c1b…a286f210`,
  modules 099/100/102/202) — an operational follow-up once the binary ships, not
  a code change. Re-running `course import` will pick up the images.

**Out of scope:** Any change to the `/api/upload` endpoint, the upload payload,
the CORS/PNA preflight logic itself, or the user-login GET wire format.

---

## Risks & Verification

- **Risk: silently breaking the preprod path while fixing mainnet.** Mitigated
  by keeping a preprod regression assertion in every unit's test scenarios — the
  helper must satisfy both shapes, and the existing `TestBuildAuthURL` preprod
  case is an additional guard.
- **Risk: regressing `deriveAppOrigin`'s fail-closed contract.** Mitigated by
  leaving the `url.Parse`/empty-return guard untouched and explicitly testing the
  empty + unparseable cases (U3).
- **End-to-end check (post-merge, optional):** with a mainnet config, run
  `course import` on a module containing an `assets/*.png` reference and confirm
  the upload returns a CDN URL rather than 404, and the lesson stores the CDN URL
  rather than `assets/...`.

---

## Sources & Research

- GitHub issue #117 — root-cause analysis, reproduction, suggested fix (shared
  helper across the three sites).
- `cmd/andamio/user.go:637-657` — the existing correct dual-host implementation
  this plan generalizes.
- `cmd/andamio/dev.go:47-57`, `cmd/andamio/course_import.go:1414-1417` — the two
  single-replace copies being fixed.
