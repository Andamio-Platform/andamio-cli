## andamio dev keys

Manage developer API keys (mainnet + preprod)

### Synopsis

Developer API key management.

Wraps the gateway's /api/v2/keys surface — list, create, and delete API
keys across both mainnet and preprod environments. Requires both an
X-API-Key (for app-level auth and billing) AND the developer JWT minted
by 'andamio dev login' (for developer identity). The wallet/user JWT
slot is not used by this endpoint family.

Run 'andamio auth login --api-key <key>' AND 'andamio dev login --skey
<path> --alias <name> --address <bech32>' first if you have not yet
configured both credentials.

```
andamio dev keys [flags]
```

### Options

```
  -h, --help   help for keys
```

### Options inherited from parent commands

```
  -o, --output string   Output format: text, json, csv, markdown (default "text")
```

### SEE ALSO

* [andamio dev](andamio_dev.md)	 - Developer-portal operations (login, manage API keys)
* [andamio dev keys create](andamio_dev_keys_create.md)	 - Create a developer API key
* [andamio dev keys delete](andamio_dev_keys_delete.md)	 - Revoke a developer API key by id
* [andamio dev keys list](andamio_dev_keys_list.md)	 - List developer API keys (mainnet + preprod, unified)

