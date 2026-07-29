## andamio course create-module

Create a new course module

### Synopsis

Create a new course module, optionally reading metadata from a compiled directory.

With a path argument, reads title and code from outline.md:
  andamio course create-module ./compiled/my-course/101 --course-id <id>

With explicit flags (no path needed):
  andamio course create-module --course-id <id> --code 101 --title "My Module"

Requires user authentication via 'andamio user login'.

```
andamio course create-module [path] [flags]
```

### Options

```
      --approve            Approve the module after adding SLTs (computes slt_hash automatically). Requires --slt.
      --code string        Module code (reads from outline.md if path provided)
      --course-id string   Course ID (required)
  -h, --help               help for create-module
      --slt stringArray    SLT text (repeatable). When provided, SLTs are added to the module after creation.
      --sort-order int     Sort order for the module (default: 0)
      --title string       Module title (reads from outline.md if path provided)
```

### Options inherited from parent commands

```
  -o, --output string   Output format: text, json, csv, markdown (default "text")
```

### SEE ALSO

* [andamio course](andamio_course.md)	 - Manage courses

