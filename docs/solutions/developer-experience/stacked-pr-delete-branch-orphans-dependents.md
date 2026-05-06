---
title: "gh pr merge --delete-branch orphans stacked dependents"
module: git-workflow
date: 2026-05-06
problem_type: workflow_issue
component: development_workflow
severity: medium
applies_when: "merging a PR that has another PR stacked on its branch (PR-B's base is PR-A's head)"
tags:
  - git
  - github
  - gh-cli
  - pull-requests
  - stacked-prs
  - workflow
related_components:
  - documentation
---

# `gh pr merge --delete-branch` orphans stacked dependents

## Context

Stacked PRs are a common pattern for landing a multi-step feature without forcing reviewers to read one giant diff. The shape: PR-A targets `main`, PR-B's base is PR-A's branch (`feat/80a` -> `main`, then `feat/80b` -> `feat/80a`). When PR-A merges, GitHub normally **auto-retargets** any open PR whose base was PR-A's branch so it now targets `main` directly. The dependent PR stays open and reviewable.

This auto-retargeting depends on PR-A's branch still existing momentarily after the merge — long enough for GitHub's webhook to update the dependent PR's base. If you delete PR-A's branch *as part of* the merge action, GitHub takes the wrong branch first and **closes** the dependent PR instead of retargeting it.

## Guidance

When merging a PR that has stacked dependents, **do not** pass `--delete-branch` to `gh pr merge`. Merge first, let GitHub retarget the dependents, then delete the now-merged branch separately if needed.

```bash
# WRONG when there are stacked PRs depending on this branch:
gh pr merge 83 --merge --delete-branch
# → dependent PR #85 (base=feat/80a-dev-login) is auto-CLOSED, not retargeted

# RIGHT when there are stacked dependents:
gh pr merge 83 --merge
# → dependent PR #85 auto-retargets to main, stays OPEN
# → optionally clean up the merged branch later:
git push origin --delete feat/80a-dev-login
git branch -d feat/80a-dev-login

# RIGHT when there are NO stacked dependents (the common case):
gh pr merge 86 --merge --delete-branch
# → safe; nothing depends on this branch
```

Before invoking `gh pr merge --delete-branch`, check whether any open PRs use the merging branch as their base:

```bash
# What PRs depend on this branch?
gh pr list --base feat/80a-dev-login --state open
```

If the list is non-empty, drop `--delete-branch` from the merge command.

## Why This Matters

Recovering from the orphan is non-trivial. Once GitHub auto-closes a stacked PR with its base branch deleted:

1. **`gh pr reopen` fails** with `Could not open the pull request` because the base branch no longer exists.
2. **`gh pr edit --base main`** on a closed PR doesn't help either — GitHub won't reopen on retarget.
3. The recovery is to **create a fresh PR from the same head branch** targeting `main`. This loses the original PR's number, comments, and review history. You can reference the orphaned PR in the new one's body to preserve the audit trail, but reviewers who had the old PR loaded in their browser get a 404 or "closed" state.

The cost is real: ~10 minutes of cleanup, broken links in any external system that referenced the original PR, and (in CI environments) the loss of any draft state, review threads, or check history attached to the original PR.

## When to Apply

This applies whenever the merging PR's branch is the **base** of another open PR. The most common shapes:

- A two-PR feature slice (PR-A wires the plumbing, PR-B uses it) where PR-B was opened against PR-A's branch.
- A "stacked refactor" where each PR builds on the previous, all marked draft, merged in sequence.
- An emergency fix branched off a feature branch (less common but the same trap).

It does **not** apply when:

- The branch has no open dependents (the typical case — `--delete-branch` is fine and keeps the branch list clean).
- The dependents are all in `closed` state already.
- The merge is happening to `main` and the dependents target the same `main` (no stacking).

## Examples

**The trap, observed in this repo (2026-05-06):**

```
PR #83: feat/80a-dev-login -> main  (CIP-30 dev login)
PR #85: feat/80b-dev-keys -> feat/80a-dev-login  (dev keys, draft, stacked)

$ gh pr merge 83 --merge --delete-branch
# 504 timeout, retried, succeeded. Branch feat/80a-dev-login deleted.

$ gh pr view 85 --json state,baseRefName
{"baseRefName":"feat/80a-dev-login","state":"CLOSED"}
# auto-closed, not retargeted

$ gh pr reopen 85
API call failed: GraphQL: Could not open the pull request.
# can't reopen — base branch is gone
```

Recovery:

```
$ gh pr create --draft --base main --head feat/80b-dev-keys ...
https://github.com/.../pull/86  # new number, references #85 in body
```

**The fix, applied on the next merge in the same series:**

```
PR #86: feat/80b-dev-keys -> main  (no stacked dependents)

$ gh pr merge 86 --merge --delete-branch  # safe — nothing depends on feat/80b
# branch deleted, no orphans, working tree clean
```

## Prevention checklist

Add to your `gh pr merge` muscle memory:

- [ ] `gh pr list --base <branch-being-merged> --state open` — any results?
- [ ] If yes: drop `--delete-branch`. Merge, let dependents retarget, then delete the branch manually.
- [ ] If no: `--delete-branch` is safe.

A repo-level mitigation is to enable GitHub's "Automatically delete head branches" repo setting AND **not** pass `--delete-branch` in `gh` calls. Auto-deletion runs after retargeting completes, sidestepping the race entirely. The setting is at: Settings -> General -> Pull Requests -> "Automatically delete head branches".
