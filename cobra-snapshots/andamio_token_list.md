## andamio token list

List registered tokens available as task rewards

### Synopsis

List all registered native asset tokens from the global token registry.

These are the tokens that can be attached to project tasks via --token flag.

Use the policy_id and asset_name values with project task create/update:
  andamio project task create <project-id> --title "..." --lovelace 5000000 \
    --expiration 2026-06-01 --token "policy_id,asset_name,quantity"

```
andamio token list [flags]
```

### Options

```
  -h, --help   help for list
```

### Options inherited from parent commands

```
  -o, --output string   Output format: text, json, csv, markdown (default "text")
```

### SEE ALSO

* [andamio token](andamio_token.md)	 - Native asset token registry

