## andamio dev keys create

Create a developer API key

### Synopsis

Create a new developer API key in the requested environment.

The raw key value is returned EXACTLY ONCE in the response — store it
immediately. Subsequent 'dev keys list' calls return only the last4 hint
and metadata; the full key is not retrievable.

Examples:
  andamio dev keys create --name "preprod-bot" --environment preprod
  andamio dev keys create --name "mainnet-prod" --environment mainnet --output json

```
andamio dev keys create [flags]
```

### Options

```
      --environment string   Target environment: mainnet or preprod (required)
  -h, --help                 help for create
      --name string          Human-readable key label (3-64 chars, required)
```

### Options inherited from parent commands

```
  -o, --output string   Output format: text, json, csv, markdown (default "text")
```

### SEE ALSO

* [andamio dev keys](andamio_dev_keys.md)	 - Manage developer API keys (mainnet + preprod)

