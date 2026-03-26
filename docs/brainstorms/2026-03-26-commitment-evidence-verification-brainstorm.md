# Commitment Evidence Verification

**Date:** 2026-03-26
**Status:** Ready for planning

## What We're Building

Commands for teachers and project managers to verify that commitment evidence stored in the database matches the evidence_hash recorded on-chain. This confirms DB-to-chain integrity for submitted work.

Two new commands:

```
andamio course teacher verify-evidence \
  --course-id <id> --module-code <code> \
  --participant-alias <alias>

andamio project manager verify-evidence \
  --project-id <id> --task-index <idx> \
  --participant-alias <alias>
```

Each command:
1. Fetches the commitment from the API (evidence JSON + evidence_hash)
2. Re-normalizes the evidence JSON locally (sort keys, trim strings)
3. Serializes to JSON, computes Blake2b-256
4. Compares computed hash against the API's evidence_hash
5. Reports match/mismatch

## Why This Approach

### Verification is a reviewer action
Teachers review course commitments, managers review project commitments. They need confidence that the evidence they're judging is what was actually committed on-chain. This is not a submitter concern or a batch audit — it's part of the review workflow.

### Evidence is decoupled from storage
Today evidence lives in the Andamio DB. In the future, evidence could live in private data stores, other databases, or be served through different gateways (dbapi, etc.). The on-chain hash is the **anchor** — it doesn't care where the evidence is stored. Wherever the evidence comes from, the hash must match.

### Phase 1: API-sourced, Phase 2: flexible input
For now, the CLI fetches evidence from the Andamio API. Later, we add the ability to accept evidence from any source (file, URL, pipe) and verify against a hash from any source (API, chain, manual `--evidence-hash` flag). The architecture should make phase 2 easy without reworking phase 1.

## Key Decisions

1. **Single commitment verification** — one commitment per invocation, identified by course/project + module/task + participant alias. Not batch.

2. **Teacher/manager command paths** — `course teacher verify-evidence` and `project manager verify-evidence`. Verification is a reviewer concern.

3. **andamio-core is canonical (for now)** — the Go normalization must produce identical hashes to andamio-core's `computeCommitmentHash()`. Cross-platform parity matters here because evidence may have been submitted from the web app. Goal: CLI becomes canonical source over time.

4. **API provides both evidence and hash** — the teacher/manager commitment endpoints should return the full evidence JSON body and the evidence_hash. The command confirms these are consistent.

5. **Same normalization as wrapEvidence()** — reuses the existing `normalizeForHashing()` from helpers.go. No new hashing code needed — just a new command that fetches and re-verifies instead of computing at submit time.

## How It Fits With Existing Verify Commands

| Command | What it verifies | Hash type |
|---------|-----------------|-----------|
| `project task verify-hash` | Task definition integrity (API vs computed Plutus CBOR) | task_hash |
| `course credential verify-hash` | SLT set integrity (API vs computed Plutus CBOR) | slt_hash |
| `course teacher verify-evidence` | Evidence integrity (DB evidence vs on-chain commitment hash) | evidence_hash |
| `project manager verify-evidence` | Evidence integrity (DB evidence vs on-chain commitment hash) | evidence_hash |

The first two verify **structural identity** (Plutus Data CBOR encoding). The evidence commands verify **content integrity** (normalized JSON encoding).

## Open Questions

1. **API response shape** — Do the teacher/manager commitment endpoints currently return both `evidence` (full JSON) and `evidence_hash`? Need to test against live API to confirm field names and nesting.

2. **Cross-platform hash parity** — The CLI's `normalizeForHashing()` and andamio-core's `normalizeForHashing()` are documented as "cross-platform parity is a non-goal." We need to verify they produce identical output for evidence submitted from the web app before shipping. Test with real commitments from preprod.

3. **Participant identification** — Is `--participant-alias` the right identifier, or do some endpoints use a different key (e.g., student_alias, contributor_alias, wallet address)?
