## andamio project manager qualified-contributors

List aliases qualified to commit to the project's tasks

### Synopsis

List aliases qualified to commit to the project's tasks.

An alias is qualified iff they hold every (course_id, slt_hash) prerequisite
pair declared in the project's current on-chain state. This is a reverse
lookup over the credential graph — the chain-side gate remains the source of
truth for who can actually commit.

Results are capped at 500 aliases; when exceeded, the response carries
truncated=true. In text mode this surfaces as a stderr warning line; in
JSON mode the flag is passed through on the envelope.

Find your project IDs with: andamio project list --output json

Examples:
  andamio project manager qualified-contributors --project-id <id>
  andamio project manager qualified-contributors --project-id <id> --output json

```
andamio project manager qualified-contributors [flags]
```

### Options

```
  -h, --help                help for qualified-contributors
      --project-id string   Project ID (required)
```

### Options inherited from parent commands

```
  -o, --output string   Output format: text, json, csv, markdown (default "text")
```

### SEE ALSO

* [andamio project manager](andamio_project_manager.md)	 - Project manager operations (requires user login)

