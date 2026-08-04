## andamio dev

Developer-portal operations (login, manage API keys)

### Synopsis

Developer-portal commands operate on the developer JWT slot — distinct
from the wallet/user JWT used by course/project commands. The dev JWT is
required for /v2/keys and other developer-scoped endpoints.

Run 'andamio dev login --skey <path> --alias <name> --address <bech32>' to
mint one. The flow mirrors 'user login --skey' but binds the resulting JWT
to your developer account rather than your end-user account.

Environment:
  ANDAMIO_DEV_JWT             Override the stored developer JWT for this
                              process. Parallel to ANDAMIO_JWT for the user
                              slot. Useful for one-off scripted requests.
  ANDAMIO_DEV_REFRESH_TOKEN   Override the stored 30-day rotation refresh
                              token. Lets ephemeral CI/CD agents inject a
                              rotation credential without committing it to
                              the image, run 'dev refresh' once, and read
                              the rotated token from the resulting config.
                              NOTE: env-sourced values are written to
                              ~/.andamio/config.json on the next config
                              save (every successful login, refresh, or
                              logout triggers a save). For truly ephemeral
                              runs, point HOME at a tmpfs or remove the
                              .andamio directory on exit.

```
andamio dev [flags]
```

### Options

```
  -h, --help   help for dev
```

### Options inherited from parent commands

```
  -o, --output string   Output format: text, json, csv, markdown (default "text")
```

### SEE ALSO

* [andamio](andamio.md)	 - Developer CLI for authoring and assessing on the Andamio Protocol
* [andamio dev keys](andamio_dev_keys.md)	 - Manage developer API keys (mainnet + preprod)
* [andamio dev login](andamio_dev_login.md)	 - Authenticate as a developer (browser wallet, or headless CIP-8 with --skey)
* [andamio dev logout](andamio_dev_logout.md)	 - Clear stored developer JWT and refresh token
* [andamio dev refresh](andamio_dev_refresh.md)	 - Rotate the developer JWT using the stored refresh token
* [andamio dev status](andamio_dev_status.md)	 - Show developer authentication status

