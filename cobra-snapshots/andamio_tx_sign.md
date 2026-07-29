## andamio tx sign

Sign an unsigned transaction with a local .skey file

### Synopsis

Sign an unsigned Cardano transaction locally using a .skey file.

The unsigned transaction CBOR hex can be provided via --tx flag or --tx-file.
The signing key is loaded via Bursa (cardano-cli JSON envelope format).

Outputs the signed transaction CBOR hex and transaction hash.

Examples:
  andamio tx sign --tx 84a4... --skey ./payment.skey --output json
  andamio tx sign --tx-file unsigned.cbor --skey ./payment.skey --output json

```
andamio tx sign [flags]
```

### Options

```
  -h, --help             help for sign
      --skey string      Path to .skey file (cardano-cli JSON envelope format)
      --tx string        Unsigned transaction CBOR hex string
      --tx-file string   Path to file containing CBOR hex (mutually exclusive with --tx)
```

### Options inherited from parent commands

```
  -o, --output string   Output format: text, json, csv, markdown (default "text")
```

### SEE ALSO

* [andamio tx](andamio_tx.md)	 - Transaction operations

