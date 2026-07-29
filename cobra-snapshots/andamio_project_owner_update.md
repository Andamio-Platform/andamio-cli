## andamio project owner update

Update project metadata

### Synopsis

Update an existing project's metadata. Only specified flags are updated; omitted fields are unchanged.

Examples:
  andamio project owner update --project-id <id> --title "New Title"
  andamio project owner update --project-id <id> --description "Updated description" --public=false

```
andamio project owner update [flags]
```

### Options

```
      --description string   Project description
  -h, --help                 help for update
      --image-url string     Project image URL
      --project-id string    Project ID (required)
      --public               Set project public visibility
      --title string         Project title
      --video-url string     Project video URL
```

### Options inherited from parent commands

```
  -o, --output string   Output format: text, json, csv, markdown (default "text")
```

### SEE ALSO

* [andamio project owner](andamio_project_owner.md)	 - Project owner operations (requires user login)

