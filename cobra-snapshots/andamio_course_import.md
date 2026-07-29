## andamio course import

Import a compiled module to update course content

### Synopsis

Import a compiled module directory to update an existing course module.

The directory should contain:
  - outline.md (with YAML frontmatter: title, code)
  - lesson-N.md files (one per SLT)
  - introduction.md (optional)
  - assignment.md (optional)

Examples:
  andamio course import ./compiled/my-course/101 --course-id abc123
  andamio course import ./compiled/my-course/101 --course "Intro to Cardano"

New images in assets/ are automatically uploaded to the CDN.
Previously uploaded images are preserved via .image-manifest.json.

Requires user authentication via 'andamio user login'.

```
andamio course import <path> [flags]
```

### Options

```
      --course string      Course name or substring (alternative to --course-id)
      --course-id string   Course ID to import into
      --create             Create the module if it doesn't exist
      --dry-run            Preview the import without sending. Shows summary only; use --show-payload to also emit the full API payload
  -h, --help               help for import
      --show-payload       Also print the full API payload (Tiptap JSON etc.) on --dry-run. Noisy for multi-lesson modules
      --sort-order int     Sort order when creating a new module (used with --create)
```

### Options inherited from parent commands

```
  -o, --output string   Output format: text, json, csv, markdown (default "text")
```

### SEE ALSO

* [andamio course](andamio_course.md)	 - Manage courses

