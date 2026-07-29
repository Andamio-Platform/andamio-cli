## andamio project owner register

Register an on-chain project with off-chain metadata

### Synopsis

Register a project that has been created on-chain. Links the on-chain record
to off-chain metadata (title, description, etc.).

Typical flow:
  1. andamio tx run /v2/tx/instance/owner/project/create --body '...' --skey ... --tx-type project_create
  2. andamio project owner register --project-id <id>

Examples:
  andamio project owner register --project-id <id> --title "My Project"
  andamio project owner register --project-id <id> --title "My Project" --public

```
andamio project owner register [flags]
```

### Options

```
      --category string      Project category
      --description string   Project description
  -h, --help                 help for register
      --image-url string     Project image URL
      --project-id string    Project ID (required)
      --public               Make project publicly visible
      --title string         Project title (required)
      --tx-hash string       Transaction hash from on-chain creation
      --video-url string     Project video URL
```

### Options inherited from parent commands

```
  -o, --output string   Output format: text, json, csv, markdown (default "text")
```

### SEE ALSO

* [andamio project owner](andamio_project_owner.md)	 - Project owner operations (requires user login)

