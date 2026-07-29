## andamio course slts

List SLTs for a course module

### Synopsis

List SLTs for a course module.

The course can be specified by ID (first positional arg) or by name (--course flag):
  andamio course slts <course-id> <module-code>
  andamio course slts <module-code> --course "Intro to Cardano"

Find your course IDs with: andamio teacher courses

```
andamio course slts [course-id] <module-code> [flags]
```

### Options

```
      --course string   Course name or substring (alternative to course-id arg)
  -h, --help            help for slts
```

### Options inherited from parent commands

```
  -o, --output string   Output format: text, json, csv, markdown (default "text")
```

### SEE ALSO

* [andamio course](andamio_course.md)	 - Manage courses

