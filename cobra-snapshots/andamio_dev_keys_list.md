## andamio dev keys list

List developer API keys (mainnet + preprod, unified)

### Synopsis

List all active developer API keys. Returns a unified view across
mainnet and preprod environments, sorted newest-first. Mainnet entries do
not carry the last4 hint (legacy storage shape); preprod entries do.

```
andamio dev keys list [flags]
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

* [andamio dev keys](andamio_dev_keys.md)	 - Manage developer API keys (mainnet + preprod)

