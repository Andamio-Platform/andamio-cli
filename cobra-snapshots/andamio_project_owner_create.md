## andamio project owner create

Create a new project

### Synopsis

Create a new off-chain project record.

For on-chain project creation, use: andamio tx run /v2/tx/instance/owner/project/create
Then register the project with: andamio project owner register

Examples:
  andamio project owner create --project-id abc123 --pending-tx-hash tx123 --title "Community Development"
  andamio project owner create --project-id abc123 --pending-tx-hash tx123 --description "Build things" --public

```
andamio project owner create [flags]
```

### Options

```
      --category string          Project category
      --description string       Project description
  -h, --help                     help for create
      --image-url string         Project image URL
      --pending-tx-hash string   Transaction hash of the pending on-chain creation (required)
      --project-id string        Project ID (required — derived from on-chain NFT policy)
      --public                   Make project publicly visible
      --title string             Project title
      --video-url string         Project video URL
```

### Options inherited from parent commands

```
  -o, --output string   Output format: text, json, csv, markdown (default "text")
```

### SEE ALSO

* [andamio project owner](andamio_project_owner.md)	 - Project owner operations (requires user login)

