## andamio tx register

Register a submitted transaction for tracking

### Synopsis

Register a submitted transaction hash with the Andamio API for async confirmation tracking.

After submitting a transaction to the Cardano network, register it so the platform
can track its confirmation status.

List valid transaction types with: andamio tx types

Examples:
  andamio tx register --tx-hash abc123... --tx-type access_token_mint
  andamio tx register --tx-hash abc123... --tx-type course_create --instance-id <course-id>

```
andamio tx register [flags]
```

### Options

```
  -h, --help                 help for register
      --instance-id string   Course or project ID (optional, for types that return one during build)
      --tx-hash string       Transaction hash (64-character hex)
      --tx-type string       Transaction type (see 'andamio tx types')
```

### Options inherited from parent commands

```
  -o, --output string   Output format: text, json, csv, markdown (default "text")
```

### SEE ALSO

* [andamio tx](andamio_tx.md)	 - Transaction operations

