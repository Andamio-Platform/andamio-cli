---
name: assess-assignment
description: Evaluate student submissions against SLTs and build assessment transactions for human signing. Agent reads, human judges.
argument-hint: "<course-id> <module-code> [--demo]"
---

# Assess Assignment

**Purpose:** Agent reads student submissions for a course module, evaluates each against the module's Student Learning Targets, produces an assessment report with evidence-based reasoning, and builds credential transactions — but only the human signs. The agent does the reading; the human does the judging.

## Usage

```
/assess-assignment <course-id> <module-code>
/assess-assignment <course-id> <module-code> --demo
```

`--demo` uses preloaded submissions for the Go course example (CF Office Hours demo).

---

## Execution Flow

### Phase 1: Load SLTs and Assignment

Fetch the module's Student Learning Targets — these are the assessment criteria:

```bash
andamio course slts <course-id> <module-code> --output json
```

Fetch the assignment prompt so we know what was asked:

```bash
andamio course intro <course-id> <module-code> --output json
```

Display the SLTs and assignment to the user:

```
## Module <code>: <title>

### Assignment
<assignment prompt>

### Student Learning Targets
1. <SLT text>
2. <SLT text>
```

If no SLTs found, stop: "This module has no SLTs — nothing to assess against."

### Phase 2: Load Submissions

**Production mode:** Fetch pending assignment commitments for this course:

```bash
andamio teacher assignments list --course <course-id> --output json
```

Filter to:
- `course_module_code` matching the requested module
- `commitment_status` is `"SUBMITTED"` (skip `PENDING_TX_COMMIT` — those aren't on-chain yet)

Each commitment contains the student's submission in `content.evidence` (Tiptap JSON — extract the text). The student is identified by `student_alias`.

**Demo mode (`--demo`):** Use preloaded submissions:

| Student | Submission |
|---------|-----------|
| student-01 | "This course is about the programming language, Go." |
| student-02 | "This course is about a game called Go." |
| student-03 | "This is what I yell at my children when we have to hurry." |
| student-04 | "Golang" |

If no submissions found (production mode), report and exit: "No pending submissions for this module."

### Phase 3: Evaluate Each Submission

For each submission, assess against every SLT independently.

**Scoring:** Each SLT is scored as:
- **Met** — submission demonstrates the capability described in the SLT
- **Not Met** — submission contradicts or fails to demonstrate the capability
- **Insufficient Evidence** — submission is ambiguous; can't determine from this alone

**For each score, provide evidence.** Quote the specific part of the submission that supports the score. This is what the human reviewer needs to make their decision.

**Example assessment:**

```
### student-01
Submission: "This course is about the programming language, Go."

  SLT 1: "I know that this course is about Go, the programming language"
  Score: MET
  Evidence: Explicitly states "the programming language, Go" — directly demonstrates awareness.

  SLT 2: "I can distinguish between Go the programming language and the game of Go"
  Score: MET
  Evidence: By specifying "the programming language," the student implicitly distinguishes
  it from other meanings. Marginal — they didn't name alternatives, but the qualifier is clear.

  Result: PASS (2/2 SLTs met)
```

**Be honest about borderline cases.** "Golang" (student-04) demonstrates SLT 1 (it's a common name for the language) but is ambiguous on SLT 2 (doesn't explicitly distinguish). Say so. The human decides borderlines, not the agent.

### Phase 4: Present Summary for Human Review

**First, show only the summary table:**

```
## Assessment Summary

| Student | SLT 1 | SLT 2 | Result | Recommendation |
|---------|-------|-------|--------|----------------|
| student-01 | Met | Met | PASS | Accept |
| student-02 | Not Met | Not Met | FAIL | Refuse |
| student-03 | Not Met | Not Met | FAIL | Refuse |
| student-04 | Met | Insufficient | BORDERLINE | Your call |

Passing: 1  Failing: 2  Borderline: 1
```

**Then offer to show detailed reasoning:**

> Want to see the full reasoning? I can show the detailed assessment with evidence for each student before you decide.
>
> Otherwise, tell me your decisions:
> - **Accept** — issue the credential
> - **Refuse** — no credential
> - **Override** — change my assessment (you're the teacher)
>
> Or say "approve all passing" to batch-accept passes and refuse fails.
> For borderline cases, you must decide — the agent flags them, the human calls them.

**If the user asks to see reasoning**, show the detailed per-student assessments from Phase 3 (submission text, per-SLT scores with quoted evidence, result). Then re-ask for decisions.

**If the user gives decisions without asking for reasoning**, proceed directly to Phase 5. Don't force the detail on them — the summary table may be enough.

**Never auto-approve. Always wait for human judgment.**

### Phase 5: Build Transaction (on human decisions)

Build a **single transaction** containing ALL assignment decisions — both accepts and refuses. The API processes them as a batch.

**API schema (`POST /v2/tx/course/teacher/assignments/assess`):**

```json
{
  "alias": "<teacher-alias>",
  "course_id": "<course-id>",
  "assignment_decisions": [
    {"alias": "<student-alias>", "outcome": "accept"},
    {"alias": "<student-alias>", "outcome": "refuse"}
  ]
}
```

**Field reference:**
- `alias` (top level): The teacher's on-chain alias (e.g., `"manager-001"`)
- `course_id`: The course minting policy hash
- `assignment_decisions`: Array of `AssignmentOutcome` objects — one per student
- `assignment_decisions[].alias`: The student's on-chain alias (from `student_alias` in the commitment)
- `assignment_decisions[].outcome`: `"accept"` (issue credential) or `"refuse"` (deny credential)

**Note:** No `module_code` field — the protocol determines the module from the student's on-chain commitment.

```bash
andamio tx build /v2/tx/course/teacher/assignments/assess \
  --body '{
    "alias": "<teacher-alias>",
    "course_id": "<course-id>",
    "assignment_decisions": [
      {"alias": "student-01", "outcome": "accept"},
      {"alias": "student-02", "outcome": "refuse"},
      {"alias": "student-03", "outcome": "refuse"},
      {"alias": "student-04", "outcome": "accept"}
    ]
  }' \
  --output json
```

Capture the `unsigned_tx` CBOR from the response.

Present the transaction for signing:

```
## Transaction Ready

Decisions:
  - student-01: ACCEPT (credential will be issued)
  - student-02: REFUSE
  - student-03: REFUSE
  - student-04: ACCEPT (teacher override — borderline)

Unsigned TX: <cbor-hex>

To sign and submit:
  andamio tx sign --tx <cbor-hex> --skey <path-to-skey>
  andamio tx submit --tx <signed-cbor-hex>

Or I can sign if you provide your .skey path.
```

If the human provides their skey path:

```bash
andamio tx sign --tx <unsigned-cbor> --skey <path> --output json
andamio tx submit --tx <signed-cbor> --output json
```

### Phase 6: Report

After all transactions are processed:

```
## Assessment Complete

| Student | Agent Recommendation | Teacher Decision | Outcome |
|---------|---------------------|-----------------|---------|
| student-01 | PASS | Accept | Credential issued |
| student-02 | FAIL | Refuse | No credential |
| student-03 | FAIL | Refuse | No credential |
| student-04 | BORDERLINE | Accept (override) | Credential issued |

Accepted: 2  Refused: 2  TX: <tx-hash>
```

---

## The Demo (CF Office Hours, Mar 21)

The Go course example is designed to be funny but illustrative:

**Two SLTs:**
1. "I know that this course is about Go, the programming language"
2. "I can distinguish between Go, the programming language, and the game of Go"

**Assignment:** "What is this course about?"

**Four submissions — two pass, one fails, one is borderline:**
- "This course is about the programming language, Go." — clear pass
- "This course is about a game called Go." — clear fail (wrong Go)
- "This is what I yell at my children when we have to hurry." — clear fail (funny, wrong)
- "Golang" — borderline (correct language, but does it show distinction?)

**The punchline:** The agent reads, reasons, and recommends. The human reviews, overrides if needed, and signs. The credential lands on-chain — issued by a human who reviewed an agent's assessment. That's the model.

**Demo talking points:**
- The agent identified a borderline case ("Golang") and flagged it instead of deciding
- The human can override any assessment — the agent's recommendation is advisory
- The credential transaction requires a human signature — the protocol enforces the boundary
- SLTs define what "competence" means for this module — both humans and agents read the same criteria

---

## Rules

- **Never auto-sign.** Always present the report and wait for human approval.
- **Never auto-approve.** Even clear passes get presented for human review.
- **Be honest about borderlines.** If evidence is ambiguous, say "Insufficient Evidence" and explain why. Don't round up.
- **Show your reasoning.** The human needs to understand WHY each SLT was scored. Quote the submission.
- **One transaction for all decisions.** The API accepts a batch — include all accepts AND refuses in a single `assignment_decisions` array.
- **Include all students in the TX.** Passing students get `"outcome": "accept"`, failing students get `"outcome": "refuse"`. The transaction records the teacher's decision for every submission.
- **Only assess SUBMITTED commitments.** Skip `PENDING_TX_COMMIT` (not yet on-chain) and any other non-submitted status.
- **Use correct API terms.** The outcome field is `"outcome"` (not `"result"`), with values `"accept"` / `"refuse"` (not `"pass"` / `"fail"`). The student field in decisions is `"alias"` (not `"student_alias"`).
- **The assessment is the demo.** Make it clear, readable, and fun. The Go course is designed to get a laugh — lean into it.

---

## Integration Points

**Reads:**
- Course SLTs via `andamio course slts <course-id> <module-code>`
- Assignment prompt via `andamio course intro <course-id> <module-code>`
- Pending submissions via `andamio teacher assignments list --course <course-id>` (production)
- Preloaded submissions (demo mode)

**Writes:**
- Assessment transactions via `andamio tx build /v2/tx/course/teacher/assignments/assess`
- Signed/submitted transactions via `andamio tx sign` and `andamio tx submit`

**The architecture:**
- Agent: reads submissions, evaluates against SLTs, produces report
- Human: reviews report, approves/rejects/overrides, signs transaction
- Protocol: credential lands on-chain, verifiable, issued by the human who signed it
