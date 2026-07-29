## andamio course credential verify-hash

Verify credential hashes match computed SLT hashes

### Synopsis

Compute SLT hashes locally and compare against API-returned slt_hash values.

For each on-chain module, collects the SLT texts, encodes them as Plutus Data
CBOR (matching the on-chain validator), hashes with Blake2b-256, and compares
against the slt_hash stored in the API. Reports any mismatches.

Requires an API key or user authentication.

Examples:
  andamio course credential verify-hash <course-id>
  andamio course credential verify-hash <course-id> --output json

```
andamio course credential verify-hash <course-id> [flags]
```

### Options

```
  -h, --help   help for verify-hash
```

### Options inherited from parent commands

```
  -o, --output string   Output format: text, json, csv, markdown (default "text")
```

### SEE ALSO

* [andamio course credential](andamio_course_credential.md)	 - Credential verification commands

