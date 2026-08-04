## andamio teacher assignments get

Get a specific assignment commitment for review

### Synopsis

Get full details for a specific student's assignment commitment.

Emits the matched row from 'teacher assignments list', including
content.evidence_text — the submission rendered as Markdown alongside the
raw Tiptap document in content.evidence. See 'teacher assignments list --help'
for the full output contract.

Read one submission:
  andamio teacher assignments get <course-id> <module-code> <student-alias> \
    --output json | jq -r '.content.evidence_text'

Examples:
  andamio teacher assignments get <course-id> <module-code> <student-alias>
  andamio teacher assignments get <course-id> <module-code> <student-alias> --output json

```
andamio teacher assignments get <course-id> <module-code> <student-alias> [flags]
```

### Options

```
  -h, --help   help for get
```

### Options inherited from parent commands

```
  -o, --output string   Output format: text, json, csv, markdown (default "text")
```

### SEE ALSO

* [andamio teacher assignments](andamio_teacher_assignments.md)	 - Manage assignment reviews (teacher role)

