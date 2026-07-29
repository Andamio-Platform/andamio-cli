## andamio course teacher review

Review a student assignment commitment

### Synopsis

Review a student's assignment submission. Accept or refuse.

Examples:
  andamio course teacher review --course-id <id> --module-code 101 --participant-alias student1 --decision accept
  andamio course teacher review --course-id <id> --module-code 101 --participant-alias student1 --decision refuse

```
andamio course teacher review [flags]
```

### Options

```
      --course-id string           Course ID (required)
      --decision string            Review decision: accept or refuse (required)
  -h, --help                       help for review
      --module-code string         Module code (required)
      --participant-alias string   Student alias (required)
```

### Options inherited from parent commands

```
  -o, --output string   Output format: text, json, csv, markdown (default "text")
```

### SEE ALSO

* [andamio course teacher](andamio_course_teacher.md)	 - Course teacher operations (requires user login)

