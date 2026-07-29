## andamio course lesson

Get lesson content by SLT index (integer)

### Synopsis

Get lesson content for a specific SLT in a course module.

The slt-index must be a positive integer (e.g., 1, 2, 3), not a code or hash.

Examples:
  andamio course lesson my-course 101 1
  andamio course lesson my-course 101 3

```
andamio course lesson <course-id> <module-code> <slt-index> [flags]
```

### Options

```
  -h, --help   help for lesson
```

### Options inherited from parent commands

```
  -o, --output string   Output format: text, json, csv, markdown (default "text")
```

### SEE ALSO

* [andamio course](andamio_course.md)	 - Manage courses

