## andamio course owner update

Update course metadata

### Synopsis

Update an existing course's metadata. Only specified flags are updated; omitted fields are unchanged.

Examples:
  andamio course owner update --course-id <id> --title "New Title"
  andamio course owner update --course-id <id> --description "Updated description" --public=false

```
andamio course owner update [flags]
```

### Options

```
      --course-id string     Course ID (required)
      --description string   Course description
  -h, --help                 help for update
      --image-url string     Course image URL
      --live                 Set course live status
      --public               Set course public visibility
      --title string         Course title
      --video-url string     Course video URL
```

### Options inherited from parent commands

```
  -o, --output string   Output format: text, json, csv, markdown (default "text")
```

### SEE ALSO

* [andamio course owner](andamio_course_owner.md)	 - Course owner operations (requires user login)

