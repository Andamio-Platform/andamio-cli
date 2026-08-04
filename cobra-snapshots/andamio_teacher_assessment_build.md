## andamio teacher assessment build

Build an assessment transaction without signing or submitting it

### Synopsis

Build a course assignment assessment transaction and stop.

Nothing is signed and nothing is submitted. The transaction is returned
unsigned so a person can review the decisions it carries before approving
them — then sign and submit as separate steps:

  andamio tx sign --tx <unsigned-tx> --skey <path>
  andamio tx submit --tx <signed-tx>

'andamio tx run' remains available for anyone who wants build, sign, submit
and confirm in a single step.

One transaction carries every decision. Include accepts AND refuses: the
transaction records the teacher's decision for every submission assessed,
not only the passing ones.

Decisions are given as <student-alias>=<accept|refuse>, repeatable, or as a
JSON file of [{"alias": "...", "outcome": "accept"}, ...].

WHAT --output json RETURNS

  .unsigned_tx     CBOR hex, ready for 'andamio tx sign'
  .course_id       course the assessment applies to
  .teacher_alias   alias the assessment is signed on behalf of
  .decisions[]     the decision set, in the order given
  .decision_count  length of .decisions

The decisions echoed back are the request this command sent — not a decode
of the returned transaction. They let a reviewer check the decision set and
the transaction in one place, from one command, instead of trusting a
separate summary. They do not prove what the gateway built.

Examples:
  andamio teacher assessment build --course-id <id> --alias teacher-01 \
    --decision student-01=accept --decision student-02=refuse

  andamio teacher assessment build --course-id <id> --alias teacher-01 \
    --decisions-file decisions.json --output json

```
andamio teacher assessment build [flags]
```

### Options

```
      --alias string            Your on-chain teacher alias (required)
      --course-id string        Course ID (required)
      --decision stringArray    Decision as <student-alias>=<accept|refuse> (repeatable)
      --decisions-file string   Path to a JSON file of [{"alias":"...","outcome":"accept"}]
  -h, --help                    help for build
```

### Options inherited from parent commands

```
  -o, --output string   Output format: text, json, csv, markdown (default "text")
```

### SEE ALSO

* [andamio teacher assessment](andamio_teacher_assessment.md)	 - Build assessment transactions for review (teacher role)

