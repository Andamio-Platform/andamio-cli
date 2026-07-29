## andamio course teacher register-module

Register a course module from on-chain data

### Synopsis

Register a course module from on-chain data.

Idempotent on slt_hash match. When the module already exists in the DB
(typically because 'course import --create' created it, or a prior run
partially completed), behavior depends on its current status:

  DRAFT          + matching hash -> advances to APPROVED
  APPROVED       + matching hash -> no-op (exit 0)
  PENDING_TX     + matching hash -> no-op (exit 0)
  ON_CHAIN       + matching hash -> no-op (exit 0)
  hash mismatch  (any status)    -> error; suggests delete-module

With --output json, success branches emit RegisterModuleEnvelope (see
the Go struct for the authoritative field-level contract). Scripts
should branch on 'action' — text mode is for humans, --output json is
the stable surface for automation. Error branches return the global
{"error": "..."} shape, not the envelope.

Examples:
  andamio course teacher register-module --course-id <id> --module-code 101 --slt-hash <hash>
  andamio course teacher register-module --course-id <id> --module-code 101 --slt-hash <hash> --output json

```
andamio course teacher register-module [flags]
```

### Options

```
      --course-id string     Course ID (required)
  -h, --help                 help for register-module
      --module-code string   Module code (required)
      --slt-hash string      SLT hash — on-chain module identifier (required)
```

### Options inherited from parent commands

```
  -o, --output string   Output format: text, json, csv, markdown (default "text")
```

### SEE ALSO

* [andamio course teacher](andamio_course_teacher.md)	 - Course teacher operations (requires user login)

