# Concepts

Shared domain vocabulary for this project — entities, named processes, and status concepts with project-specific meaning. Seeded with core domain vocabulary, then accretes as ce-compound and ce-compound-refresh process learnings; direct edits are fine. Glossary only, not a spec or catch-all.

## Course

### SLT
A Student Learning Target: one statement of what a learner will be able to do after completing a Course Module. SLTs are the assessment criteria a Teacher judges an Assignment against, and they are also the Course Module's on-chain identity — the ordered list of SLT texts is the sole input to the module's SLT Hash.

Order and exact wording are load-bearing. Editing an SLT's text, or reordering the list, produces a different Course Module identity even though a human reads it as the same module.

### Course Module
A unit of a Course: an ordered list of SLTs plus its teaching content (an introduction, one lesson per SLT, and an assignment). A module exists first as an off-chain draft record and later as an on-chain entry; the two are bound by SLT Hash equality rather than by any shared identifier.

### Module Status
Where a Course Module sits between authoring and on-chain publication: DRAFT, APPROVED, PENDING_TX, ON_CHAIN.

Only a DRAFT accepts SLT edits — once a module leaves DRAFT its SLTs are locked, because changing them would change the identity the chain has already recorded or is about to. Forward advancement is earned rather than declared: the DRAFT-to-APPROVED step is gated on SLT Hash equality, so a module cannot advance on content the chain did not receive. Backward resets are a supported recovery path, not a violation — an operator moves a module back to APPROVED when a transaction fails to build or submit, or back to DRAFT to correct its content and re-register.

### SLT Hash
The content-derived identity of a Course Module: a digest computed over the ordered list of its SLT texts and nothing else — not the module code, not the course, not the title.

Both the CLI and the chain compute it independently from the same content, which is what makes it a linkage key rather than a checksum: two records agree that they describe the same module precisely when their SLT Hashes match. A mismatch is therefore not corruption to repair but a statement that these are different modules.

### Quiz Envelope
An assignment whose `content_json` is a `{"type": "quiz", "version": 1, ...}` object instead of a Tiptap `doc`. The gateway and db-api store it as opaque JSON; only the Andamio app's render layer interprets it, grading client-side and storing a self-contained evidence snapshot on commit. Its validity rules are owned by the app (`src/lib/quiz/quiz-envelope.ts` in fcb-fan-engagement-app); the CLI mirrors them so an envelope it publishes is one the app can render.

On disk a quiz assignment is `assignment.quiz.json`, never `assignment.md`: converting the envelope to Markdown loses it, so export and import carry it verbatim.

## Project

### Task
A unit of contributable work in a Project, carrying its own reward and deadline. Like a Course Module, a Task exists as an off-chain draft before it is minted on-chain, and its on-chain identity is content-derived rather than assigned.

### Task Hash
The content-derived identity of a Task: a digest over the Task's content, deadline, reward in lovelace, and any native assets attached as reward.

Reward assets are inside the identity, not metadata alongside it — changing what a Task pays changes which Task it is. This is the direct analogue of SLT Hash on the course side, and it plays the same linkage role.

## Cross-cutting

### Instance
A Course or Project as a distinct on-chain deployment, created before any of its modules or tasks exist. Instance creation is the one place in the domain where an off-chain record is bound to its on-chain counterpart by carrying a transaction hash forward as an explicit link, rather than by content-derived identity.

## Flagged ambiguities

- "Hash" is used for two structurally different things: a content-derived *identity* that links two records (SLT Hash, Task Hash) and a transaction hash used as a *correlation identifier* at Instance creation. These are not interchangeable — see `docs/solutions/architecture/content-hash-vs-pending-tx-hash-linking-mechanisms.md`.
