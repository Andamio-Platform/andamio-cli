## andamio config set-submit-header

Set a submit API header (e.g., Blockfrost project_id)

### Synopsis

Set a persistent HTTP header for the Cardano submit API.

Headers are sent with every tx submit/tx run invocation. Useful for
API keys required by providers like Blockfrost or Maestro.

Flag-level --submit-header values override config headers with the same key.

Examples:
  andamio config set-submit-header project_id preprodABC123
  andamio config set-submit-header Authorization "Bearer tok_xyz"

```
andamio config set-submit-header [key] [value] [flags]
```

### Options

```
  -h, --help   help for set-submit-header
```

### Options inherited from parent commands

```
  -o, --output string   Output format: text, json, csv, markdown (default "text")
```

### SEE ALSO

* [andamio config](andamio_config.md)	 - Manage CLI configuration

