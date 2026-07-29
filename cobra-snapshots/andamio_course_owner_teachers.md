## andamio course owner teachers

Update the teacher list for a course

### Synopsis

Add or remove teachers from a course. Use --add and --remove flags with user aliases.

Examples:
  andamio course owner teachers --course-id <id> --add alice --add bob
  andamio course owner teachers --course-id <id> --remove charlie
  andamio course owner teachers --course-id <id> --add alice --remove charlie

```
andamio course owner teachers [flags]
```

### Options

```
      --add stringArray      Teacher alias to add (repeatable)
      --course-id string     Course ID (required)
  -h, --help                 help for teachers
      --remove stringArray   Teacher alias to remove (repeatable)
```

### Options inherited from parent commands

```
  -o, --output string   Output format: text, json, csv, markdown (default "text")
```

### SEE ALSO

* [andamio course owner](andamio_course_owner.md)	 - Course owner operations (requires user login)

