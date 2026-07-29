## andamio course credential compute-hash

Compute SLT hash from SLT texts or an outline file

### Synopsis

Compute the Blake2b-256 hash of SLT texts, matching the on-chain Plutus validator.

This is the same hash used for credential verification on-chain. Use it to
pre-compute the slt_hash before minting a module.

Provide SLTs either as repeated --slt flags or via --file pointing to an outline.md.

No authentication required — this is a purely local computation.

Examples:
  andamio course credential compute-hash --slt "Describe how X works" --slt "Build Y"
  andamio course credential compute-hash --file ./compiled/my-course/101/outline.md
  andamio course credential compute-hash --file outline.md --output json

```
andamio course credential compute-hash [flags]
```

### Options

```
      --file string       Path to outline.md file containing SLTs
  -h, --help              help for compute-hash
      --slt stringArray   SLT text (repeatable)
```

### Options inherited from parent commands

```
  -o, --output string   Output format: text, json, csv, markdown (default "text")
```

### SEE ALSO

* [andamio course credential](andamio_course_credential.md)	 - Credential verification commands

