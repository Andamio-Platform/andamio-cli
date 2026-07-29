## andamio teacher assignments list

List pending assignment commitments for review

### Synopsis

List assignment commitments pending teacher review.

Without --course, returns a lightweight summary across all courses. The
on-chain-only summary has no nested content.commitment_status field, so
the Status column renders "—" in text mode and the field is absent from
the JSON envelope. To get DB statuses, re-run with --course <id>.

With --course, returns full merged history (on-chain + DB) for that course,
with the Status column populated from content.commitment_status (raw API
enum, displayed verbatim). For scripting, use:
  andamio teacher assignments list --course <id> --output json \
    | jq '.data[].content.commitment_status'

Known commitment_status values: AWAITING_SUBMISSION, SUBMITTED, ACCEPTED,
REFUSED, CREDENTIAL_CLAIMED, LEFT, PENDING_TX_* (transient). The CLI does
not validate or alias — whatever string the gateway returns is what you see.

Examples:
  andamio teacher assignments list
  andamio teacher assignments list --course <course-id>
  andamio teacher assignments list --course <course-id> --output json

```
andamio teacher assignments list [flags]
```

### Options

```
      --course string   Filter by course ID
  -h, --help            help for list
```

### Options inherited from parent commands

```
  -o, --output string   Output format: text, json, csv, markdown (default "text")
```

### SEE ALSO

* [andamio teacher assignments](andamio_teacher_assignments.md)	 - Manage assignment reviews (teacher role)

