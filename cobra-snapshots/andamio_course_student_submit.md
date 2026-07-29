## andamio course student submit

Submit assignment evidence

### Synopsis

Submit evidence for your assignment commitment.

Examples:
  andamio course student submit --course-id <id> --module-code 101 --evidence "https://github.com/..."

```
andamio course student submit [flags]
```

### Options

```
      --course-id string       Course ID (required)
      --evidence string        Evidence text or URL (Markdown supported)
      --evidence-file string   Path to evidence file (Markdown)
  -h, --help                   help for submit
      --module-code string     Module code (use --slt-hash for chain-only modules)
      --slt-hash string        SLT hash (use instead of --module-code for chain-only modules)
```

### Options inherited from parent commands

```
  -o, --output string   Output format: text, json, csv, markdown (default "text")
```

### SEE ALSO

* [andamio course student](andamio_course_student.md)	 - Course student operations (requires user login)

