## andamio course import-all

Import all modules in a compiled course directory

### Synopsis

Import all module subdirectories in a compiled course directory.

Scans for subdirectories containing outline.md and imports each one.

Examples:
  andamio course import-all ./compiled/my-course --course-id <id>
  andamio course import-all ./compiled/my-course --course-id <id> --create
  andamio course import-all ./compiled/my-course --course "Intro to Cardano"

Requires user authentication via 'andamio user login'.

```
andamio course import-all <dir> [flags]
```

### Options

```
      --continue-on-error      Continue past failures
      --course string          Course name or substring (alternative to --course-id)
      --course-id string       Course ID to import into
      --create                 Create modules that don't exist
      --dry-run                Show what would be imported without sending. Summary-only; use --show-payload to also emit full API payloads per module
  -h, --help                   help for import-all
      --show-payload           Also print full API payloads on --dry-run. Noisy across many modules
      --sort-order-start int   Starting sort order for --create (increments per module)
```

### Options inherited from parent commands

```
  -o, --output string   Output format: text, json, csv, markdown (default "text")
```

### SEE ALSO

* [andamio course](andamio_course.md)	 - Manage courses

