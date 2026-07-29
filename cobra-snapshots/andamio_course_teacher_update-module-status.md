## andamio course teacher update-module-status

Update a course module's status

### Synopsis

Update the status of a course module.

Examples:
  andamio course teacher update-module-status --course-id <id> --module-code 101 --status DRAFT

```
andamio course teacher update-module-status [flags]
```

### Options

```
      --course-id string     Course ID (required)
  -h, --help                 help for update-module-status
      --module-code string   Module code (required)
      --slt-hash string      SLT hash (required when status is APPROVED)
      --status string        New status (required)
```

### Options inherited from parent commands

```
  -o, --output string   Output format: text, json, csv, markdown (default "text")
```

### SEE ALSO

* [andamio course teacher](andamio_course_teacher.md)	 - Course teacher operations (requires user login)

