## andamio course modules

List modules for a course

### Synopsis

List modules for a course.

The course can be specified by ID (positional arg) or by name (--course flag):
  andamio course modules <course-id>
  andamio course modules --course "Intro to Cardano"

Find your course IDs with: andamio teacher courses

```
andamio course modules [course-id] [flags]
```

### Options

```
      --course string   Course name or substring (alternative to course-id arg)
  -h, --help            help for modules
```

### Options inherited from parent commands

```
  -o, --output string   Output format: text, json, csv, markdown (default "text")
```

### SEE ALSO

* [andamio course](andamio_course.md)	 - Manage courses

