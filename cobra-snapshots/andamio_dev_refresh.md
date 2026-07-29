## andamio dev refresh

Rotate the developer JWT using the stored refresh token

### Synopsis

Use the stored 30-day refresh token to mint a new 60-minute developer
JWT. Both tokens rotate atomically — the old refresh token is invalidated
server-side after a successful refresh, and the new pair is persisted to
config.

The refresh-token rotation is single-use server-side. If the rotation fails
on the gateway side AND the compensating revoke also fails, the gateway logs
a critical alert; the CLI sees a 5xx and a re-run will mint cleanly.

Examples:
  andamio dev refresh
  andamio dev refresh --output json

```
andamio dev refresh [flags]
```

### Options

```
  -h, --help   help for refresh
```

### Options inherited from parent commands

```
  -o, --output string   Output format: text, json, csv, markdown (default "text")
```

### SEE ALSO

* [andamio dev](andamio_dev.md)	 - Developer-portal operations (login, manage API keys)

