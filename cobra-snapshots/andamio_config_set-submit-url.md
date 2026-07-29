## andamio config set-submit-url

Set the Cardano submit API URL

### Synopsis

Set the URL for submitting signed transactions to the Cardano network.

This can be any Cardano submit API endpoint (Blockfrost, Maestro, self-hosted, etc.).
Requires HTTPS for non-localhost URLs. Set ANDAMIO_ALLOW_ANY_URL=1 to bypass.

Examples:
  andamio config set-submit-url https://cardano-mainnet.blockfrost.io/api/tx/submit
  andamio config set-submit-url https://submit-api.example.com

```
andamio config set-submit-url [url] [flags]
```

### Options

```
  -h, --help   help for set-submit-url
```

### Options inherited from parent commands

```
  -o, --output string   Output format: text, json, csv, markdown (default "text")
```

### SEE ALSO

* [andamio config](andamio_config.md)	 - Manage CLI configuration

