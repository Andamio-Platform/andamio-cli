## andamio course export

Export a course module to compiled/ format

### Synopsis

Export a course module to the compiled/ directory format used by lesson-coach.

This creates a directory structure that can be edited locally and re-imported:
  compiled/<course-slug>/<module-code>/
  ├── outline.md          # Module metadata and SLTs
  ├── introduction.md     # Module introduction (if present)
  ├── lesson-1.md         # Lesson for SLT 1
  ├── lesson-N.md         # Lesson for SLT N
  ├── assignment.md       # Module assignment (if present)
  └── assets/             # Downloaded images

The course can be specified by ID (first arg) or by name (--course flag):
  andamio course export <course-id> <module-code>
  andamio course export <module-code> --course "Intro to Cardano"

Requires user authentication via 'andamio user login'.

```
andamio course export [course-id] <module-code> [flags]
```

### Options

```
      --course string       Course name or substring (alternative to course-id arg)
      --force               Overwrite existing directory
  -h, --help                help for export
      --output-dir string   Output directory (default: ./compiled/<course-slug>/<module-code>/)
```

### Options inherited from parent commands

```
  -o, --output string   Output format: text, json, csv, markdown (default "text")
```

### SEE ALSO

* [andamio course](andamio_course.md)	 - Manage courses

