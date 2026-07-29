## andamio tx build

Build an unsigned transaction via the API

### Synopsis

POST to an Andamio API transaction-building endpoint and return the unsigned transaction.

The endpoint should be a /v2/tx/ path. The request body is passed via --body (inline JSON)
or --body-file (path to JSON file).

Returns the full API response including unsigned_tx and any endpoint-specific fields.

List available transaction types with: andamio tx types

Note: initiator_data is a WalletData object, not a plain address string:
  {"change_address":"addr_test1...", "used_addresses":["addr_test1..."]}

Examples:
  andamio tx build /v2/tx/instance/owner/course/create --body-file create-course.json --output json
  andamio tx build /v2/tx/global/user/access-token/mint \
    --body '{"alias":"dev1","initiator_data":{"change_address":"addr_test1...","used_addresses":["addr_test1..."]}}'

```
andamio tx build <endpoint> [flags]
```

### Options

```
      --body string        Inline JSON request body
      --body-file string   Path to JSON file (mutually exclusive with --body)
  -h, --help               help for build
```

### Options inherited from parent commands

```
  -o, --output string   Output format: text, json, csv, markdown (default "text")
```

### SEE ALSO

* [andamio tx](andamio_tx.md)	 - Transaction operations

