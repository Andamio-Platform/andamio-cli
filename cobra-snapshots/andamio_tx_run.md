## andamio tx run

Build, sign, submit, register, and confirm a transaction in one command

### Synopsis

Execute the full Cardano transaction lifecycle in a single command.

Steps: build unsigned TX via API, sign with local .skey, submit to network,
register with Andamio state machine, and poll for DB confirmation.

All 5 existing tx commands (build, sign, submit, register, status) remain available
for advanced use and scripting. This command is a convenience layer on top.

Progress lines are printed to stderr. Use --output json for scripted consumption.
Use --no-wait to exit after registration without polling for confirmation.

Examples:
  andamio tx run /v2/tx/course/teacher/assignments/assess \
    --body '{"alias":"teacher-01","course_id":"abc123","assignment_decisions":[...]}' \
    --skey ./payment.skey \
    --tx-type assessment_assess

  andamio tx run /v2/tx/instance/owner/course/create \
    --body-file create-course.json \
    --skey ./payment.skey \
    --tx-type course_create \
    --instance-id abc123 \
    --no-wait

```
andamio tx run <endpoint> [flags]
```

### Options

```
      --body string                 Inline JSON request body
      --body-file string            Path to JSON file (mutually exclusive with --body)
  -h, --help                        help for run
      --instance-id string          Course or project ID for registration
      --metadata stringArray        Metadata for registration (repeatable, format: key=value)
      --no-wait                     Exit after registration without polling for confirmation
      --skey string                 Path to Cardano .skey file for signing
      --submit-header stringArray   Additional submit headers (repeatable, format: "Key: Value")
      --submit-url string           Override submit API URL (falls back to config)
      --timeout duration            Max time to wait for confirmation (default 10m0s)
      --tx-type string              Transaction type for registration (see 'andamio tx types')
```

### Options inherited from parent commands

```
  -o, --output string   Output format: text, json, csv, markdown (default "text")
```

### SEE ALSO

* [andamio tx](andamio_tx.md)	 - Transaction operations

