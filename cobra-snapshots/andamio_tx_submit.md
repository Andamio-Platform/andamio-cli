## andamio tx submit

Submit a signed transaction to the Cardano network

### Synopsis

Submit a signed Cardano transaction to the network via a submit API.

The submit URL can be provided via --submit-url flag or configured with:
  andamio config set-submit-url <url>

Custom headers (e.g., for Blockfrost API key) can be passed with --submit-header.

Examples:
  andamio tx submit --tx 84a4... --submit-url https://submit.example.com --output json
  andamio tx submit --tx-file signed.cbor --submit-header "project_id: preprodABC123"

```
andamio tx submit [flags]
```

### Options

```
  -h, --help                        help for submit
      --submit-header stringArray   Additional HTTP header (repeatable, format: "Key: Value")
      --submit-url string           Cardano submit API URL (falls back to config)
      --tx string                   Signed transaction CBOR hex string
      --tx-file string              Path to file containing signed CBOR hex (mutually exclusive with --tx)
```

### Options inherited from parent commands

```
  -o, --output string   Output format: text, json, csv, markdown (default "text")
```

### SEE ALSO

* [andamio tx](andamio_tx.md)	 - Transaction operations

