# Assess Assignment

Reads student submissions for a course module, evaluates each against the module's Student Learning Targets, and produces an assessment report. The human reviews the report and decides whether to sign the credential transaction.

## Usage

```
/assess-assignment <course-id> <module-code>
```

## Process

### 1. Load the SLTs

Fetch the module's Student Learning Targets:

```bash
andamio course slts <course-id> <module-code> --output json
```

Extract each SLT text. These are the criteria.

### 2. Load the assignment

Fetch the assignment prompt so we know what was asked:

```bash
andamio course assignment <course-id> <module-code>
```

### 3. Read submissions

Fetch committed assignments for this module. Each submission is a student's answer to the assignment prompt.

For the demo, submissions are preloaded. In production, these come from the API via project task commitments.

### Demo submissions (Go course, module 100)

| Student | Submission |
|---------|-----------|
| student-01 | "This course is about the programming language, Go." |
| student-02 | "This course is about a game called Go." |
| student-03 | "This is what I yell at my children when we have to hurry." |
| student-04 | "Golang" |

### 4. Evaluate each submission against SLTs

For each submission, assess against each SLT:

**SLT 1: "I know that this course is about Go, the programming language."**
- Does the submission demonstrate awareness that the course is about the programming language?

**SLT 2: "I can distinguish between Go the programming language and the game of Go."**
- Does the submission show the student can tell them apart?

Score each as: **Met** / **Not Met** / **Insufficient Evidence**

### 5. Produce assessment report

For each student, output:

```
Student: student-01
Submission: "This course is about the programming language, Go."

  SLT 1 (I know this is about Go the language): MET
    Evidence: Explicitly names "the programming language, Go"

  SLT 2 (I can distinguish language from game): MET
    Evidence: Specifying "the programming language" implies distinction

  Result: PASS — both SLTs met
```

### 6. Summary for human reviewer

Present the full results as a table:

| Student | SLT 1 | SLT 2 | Result |
|---------|-------|-------|--------|
| student-01 | Met | Met | PASS |
| student-02 | Not Met | Not Met | FAIL |
| student-03 | Not Met | Not Met | FAIL |
| student-04 | Met | Insufficient | BORDERLINE |

Then ask: "Review the assessments above. For each PASS, I can build a credential transaction for you to sign. Proceed?"

### 7. Build transactions (on human approval)

For each approved assessment:

```bash
andamio tx build /v2/tx/course/teacher/assignments/assess \
  --body '{"course_id":"<id>","student_alias":"<alias>","slt_hash":"<hash>","result":"pass"}' \
  --output json
```

The human signs. The credential lands on-chain.

## The Point

The agent does the reading and reasoning. The human does the judging and signing. The credential is the artifact — on-chain, verifiable, issued by a human who reviewed an agent's work.

This is what "infrastructure is the curriculum" looks like in practice.

## Rules

- Never auto-sign. Always present the report and wait for human approval.
- Be honest about borderline cases. "Golang" demonstrates SLT 1 but is ambiguous on SLT 2.
- Show your reasoning. The human needs to understand WHY you scored each SLT.
- The assessment is the demo. Make it clear, readable, and fun.
