## andamio course owner create

Create off-chain course record (after on-chain creation)

### Synopsis

Create the off-chain metadata record for a course that has already been created on-chain.

Note: In most cases, 'andamio tx run' with course_create auto-registers the course in the DB.
Use 'andamio course owner update' to set metadata after that. This command is only needed when
the auto-registration did not occur (e.g., the TX confirmed but DB update failed).

Requires --course-id (from the on-chain NFT policy) and --pending-tx-hash.

Typical workflow:
  1. andamio tx run /v2/tx/instance/owner/course/create ...  (creates on-chain, auto-registers)
  2. andamio course owner update --course-id <id> --title ...  (set metadata)

Examples:
  andamio course owner create --course-id abc123 --pending-tx-hash tx123 --title "Introduction to Cardano"
  andamio course owner create --course-id abc123 --pending-tx-hash tx123 --description "Learn things" --public

```
andamio course owner create [flags]
```

### Options

```
      --category string          Course category
      --course-id string         Course ID (required — derived from on-chain NFT policy)
      --description string       Course description
  -h, --help                     help for create
      --image-url string         Course image URL
      --pending-tx-hash string   Transaction hash of the pending on-chain creation (required)
      --public                   Make course publicly visible
      --title string             Course title
      --video-url string         Course video URL
```

### Options inherited from parent commands

```
  -o, --output string   Output format: text, json, csv, markdown (default "text")
```

### SEE ALSO

* [andamio course owner](andamio_course_owner.md)	 - Course owner operations (requires user login)

