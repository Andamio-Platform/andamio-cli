## andamio course owner register

Register an on-chain course with off-chain metadata

### Synopsis

Register a course that has been created on-chain. Links the on-chain record
to off-chain metadata (title, description, etc.).

Typical flow:
  1. andamio tx run /v2/tx/instance/owner/course/create --body '...' --skey ... --tx-type course_create
  2. andamio course owner register --course-id <id>

Examples:
  andamio course owner register --course-id <id> --title "My Course"
  andamio course owner register --course-id <id> --title "My Course" --public

```
andamio course owner register [flags]
```

### Options

```
      --category string      Course category
      --course-id string     Course ID (required)
      --description string   Course description
  -h, --help                 help for register
      --image-url string     Course image URL
      --public               Make course publicly visible
      --title string         Course title (required)
      --tx-hash string       Transaction hash from on-chain creation
      --video-url string     Course video URL
```

### Options inherited from parent commands

```
  -o, --output string   Output format: text, json, csv, markdown (default "text")
```

### SEE ALSO

* [andamio course owner](andamio_course_owner.md)	 - Course owner operations (requires user login)

