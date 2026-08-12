---
status: pending
priority: p2
issue_id: "032"
tags: [ci, breaking-changes, release-process]
dependencies: []
---

# Add Tag-Pinned Release Gate for Command Surface Check

## Problem Statement

`cmd/andamio/surface_test.go`'s golden test compares the current branch
against whatever's committed in `testdata/golden/` — effectively pinned to
`main`. That's the right check for day-to-day PRs (it catches accidental
regressions against already-agreed-on `main` state), but it doesn't answer
"did we break a contract our published users actually depend on." Right now
`main` carries the unreleased 1.0 removals (`course student` / `project
contributor` retired, etc.), which are deliberate breaks against the last
published release (`v0.13.3`). A tag-pinned comparison run per-PR would flag
every one of those intentional removals as a failure throughout the whole
1.0 development window — fighting the very branch doing that work.

## Findings

- Raised in review of #141 (james): "It's worth deciding whether the gate
  compares against main or against the last released tag. Those answer
  different questions, and 'did we break a published contract?' is the one
  that protects users."
- `main` and the last released tag (`v0.13.3`) diverge specifically during
  pre-release windows like this one (1.0 unreleased, sitting on `main`).
  Once `v1.0.0` tags, they converge again and the divergence disappears
  until the next release cycle starts it over.

## Proposed Solutions

### Option A: Add a release-time gate, not a per-PR gate
A separate check (CI step or manual release-process step) that runs only
when cutting a release — compares the tag about to be created against the
previous release tag, using `git worktree add` to check out the prior tag
into an isolated directory (not the main working tree), so it can't
interfere with the build/vet/test steps that operate on the actual PR code.
Fails the release if unplanned surface changes snuck in beyond what's
documented in `CHANGELOG.md`.

- **Pros**: Directly answers "did we break published users" without
  fighting in-progress PRs mid-development.
- **Cons**: New CI machinery not yet built — worktree isolation, guaranteed
  tag fetching (`fetch-tags: true`), a Go/shell step that shells out to git.
- **Effort**: Medium
- **Risk**: Low if isolated via `git worktree`; high if implemented as a
  naive `git checkout <tag>` that mutates the working tree other steps rely on.

### Option B: Do nothing until v1.0.0 ships, then decide
`main` and the last tag become identical the moment `v1.0.0` tags, so the
divergence this todo describes resolves on its own. Revisit whether a
release gate is worth building only if it becomes a recurring problem
across future release cycles.

- **Pros**: Zero effort now; the existing `main`-comparison golden test
  already protects day-to-day development either way.
- **Cons**: No protection against a surface mistake during whatever the
  *next* release cycle after 1.0 turns out to be.
- **Effort**: None
- **Risk**: Low

## Recommended Action

Option B for now — revisit once `v1.0.0` has shipped and there's a second
release cycle to actually protect.

## Technical Details

Relevant: `cmd/andamio/surface_test.go` (the golden test),
`testdata/golden/` (the committed baseline it compares against).

## Acceptance Criteria

- [ ] Decide whether a release-time gate is worth building (Option A) or
      deferred indefinitely (Option B)
- [ ] If A: implement using `git worktree` isolation, not a
      working-tree-mutating checkout

## Work Log

| Date | Action | Learnings |
|------|--------|-----------|
| 2026-08-05 | Raised while addressing james's review of #141 | main-vs-tag tension is specific to the pre-1.0-release window; converges naturally once v1.0.0 tags |

## Resources

- Review discussion on #141
- Related: `todos/031-pending-p1-no-typed-api-contract-coupling.md`
