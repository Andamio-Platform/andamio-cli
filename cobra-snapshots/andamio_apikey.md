## andamio apikey

API key management

### Synopsis

Developer API key management.

Subcommands query the developer-portal apikey surface
(/api/v2/apikey/developer/*), which is a dual-credential gateway surface:

  - X-API-Key (gateway V2AuthMiddleware)        → andamio auth login --api-key <key>
  - Authorization: Bearer <devJWT>              → andamio dev login --skey <path> --alias <name> --address <bech32>

The wallet/user JWT slot (`user login`) is NOT accepted on this surface
— the gateway's developerJWTAuth middleware rejects it. Run BOTH login commands
before invoking apikey subcommands; an empty slot short-circuits with an
actionable hint pointing at the missing command.

```
andamio apikey [flags]
```

### Options

```
  -h, --help   help for apikey
```

### Options inherited from parent commands

```
  -o, --output string   Output format: text, json, csv, markdown (default "text")
```

### SEE ALSO

* [andamio](andamio.md)	 - Developer CLI for authoring and assessing on the Andamio Protocol
* [andamio apikey profile](andamio_apikey_profile.md)	 - Get API key profile
* [andamio apikey usage](andamio_apikey_usage.md)	 - Get API key usage stats

