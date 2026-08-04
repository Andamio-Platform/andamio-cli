# Contributing to Andamio CLI

## Before you start

Read [`CLAUDE.md`](CLAUDE.md). It is the architecture document for this repo and is kept current — the command reference, the failure contract, the auth flows, and the export/import invariants all live there.

Two sections are worth reading before writing any code: **The 1.0 Surface** (what was removed and why the stubs must stay hidden) and **Failure Contract** (exit codes and `kind` strings, which are a public API).

## How change lands

Branch → commit → push → open a PR against `main`. Never push directly to `main`.

Nothing is merged by the author. CI must be green.

## What CI runs

`.github/workflows/ci.yml` runs on every PR: `gofmt`, `go vet ./...`, `go build ./...`, and `go test -count=1 ./...`.

Run all four locally before pushing:

```bash
go vet ./... && go build ./... && go test -count=1 ./...
gofmt -l $(git diff --name-only --diff-filter=d origin/main HEAD -- '*.go')
```

**Check `gofmt` only on the files your branch touches, as CI does.** The tree carries pre-existing alignment drift from a toolchain bump, so `gofmt -l .` across the whole repo prints ~9 files that have nothing to do with your change. `ci.yml` scopes the check to your diff for exactly this reason.

`gofmt -l` printing a filename is a failure even though it exits 0 — so it goes on its own line above, not in an `&&` chain where a non-empty result would be silently swallowed.

## The test suite is the contract

**~375 test functions, and most of this repo's documented guarantees are enforced by a specific one.** Before changing behavior in a guarded area, read the guard. `CLAUDE.md`'s Testing section maps invariants to files; the load-bearing ones are:

- `cmd/andamio/exitcode_test.go` — the exit-code / `kind` table
- `cmd/andamio/retired_test.go` — retired 1.0 commands (exit 4, hidden, message text, every invocation shape)
- `cmd/andamio/unknown_test.go` — unknown commands exit non-zero with clean stdout
- `cmd/andamio/expired_jwt_test.go` — local JWT expiry handling

**If you are adding a rule that must not regress, add a case to the relevant file above.** Prefer extending the existing suite over introducing a parallel mechanism — a check that lives outside `go test` will not be run by contributors before pushing, and tends to be blind to things the suite already covers (hidden commands, anything under `internal/`).

Golden-file style tests belong in the same package as what they cover, with an `-update` flag to regenerate.

## CHANGELOG policy

`CHANGELOG.md` is the source of truth for user-facing release notes — `.github/workflows/release.yml` extracts the `## [VERSION]` section verbatim into the GitHub release body.

**It is not a per-PR requirement.** Add an entry when your change is user-visible: a new or removed command, a flag change, a change to `--output json` shape, a behavior change, an exit-code change. Put it under `## [Unreleased]`.

Do **not** add an entry for docs, CI, refactors with no behavior change, test-only changes, or `todos/` notes. A changelog padded with those is worse than one with gaps.

At release, a maintainer moves `## [Unreleased]` content under a new version heading. `scripts/release.sh` verifies the section is *extractable*, not merely that the heading exists.

## Adding a command

`CLAUDE.md` has the pattern under **Adding Endpoints**. Beyond that:

- **Every command must work without a TTY.** Never read from stdin in a handler. See the Composability Rules in `CLAUDE.md` — they are not optional, and `--output json` is the scripting surface that agents and pipes consume.
- **Data to stdout, progress to stderr.** Gate progress messages on `output.GetFormat()` so `--output json` stays parseable.
- **Required arguments are required.** Use `cobra.ExactArgs(N)` and `MarkFlagRequired`. If an argument is missing, return an error that says how to discover valid values — no interactive pickers.
- **Update the command reference in `CLAUDE.md`** in the same PR. It is meant to be complete; drift there is how contributors end up building on wrong assumptions.

**Repo tooling does not belong on `rootCmd`.** Anything that only makes sense from the repo root — code generation, snapshot regeneration, CI checks — belongs in `go test`, `scripts/`, or a separate `tools/` main. Commands registered on `rootCmd` ship in the binary users install, and `Hidden: true` does not make them unreachable.

## `todos/`

Findings that are real but not being fixed now go in `todos/` as `NNN-<status>-<priority>-<slug>.md` with YAML frontmatter (`status`, `priority`, `issue_id`, `tags`, `dependencies`). Take the next free number. Cross-reference related todos by filename.

`docs/solutions/` is the counterpart for problems already solved — a documented solution, not an open item.

## API version coupling

**1.0 requires Andamio API 2.5 or later**, by deliberate choice — supporting 2.4 and 2.5 from one binary was considered and rejected. See the `## [1.0.0]` CHANGELOG entry.

The CLI decodes most gateway responses into `map[string]interface{}`, so a renamed upstream field usually produces a *wrong* result rather than an error. If you are touching response handling, prefer a typed struct (`devKeyListItem` in `cmd/andamio/dev_keys.go` is the reference pattern) and check that a missing key fails loudly. Context and known instances: `todos/031-pending-p3-no-typed-api-contract-coupling.md`.
