## andamio dev keys delete

Revoke a developer API key by id

### Synopsis

Revoke a developer API key. The id is the local UUID returned in
'dev keys list' / 'dev keys create'. Both mainnet and preprod ids are
accepted — the gateway routes the revoke to the correct environment.

A 404 is returned for both unknown ids and ids owned by another developer
(intentionally indistinguishable, per the gateway's threat model).

```
andamio dev keys delete <id> [flags]
```

### Options

```
  -h, --help   help for delete
```

### Options inherited from parent commands

```
  -o, --output string   Output format: text, json, csv, markdown (default "text")
```

### SEE ALSO

* [andamio dev keys](andamio_dev_keys.md)	 - Manage developer API keys (mainnet + preprod)

