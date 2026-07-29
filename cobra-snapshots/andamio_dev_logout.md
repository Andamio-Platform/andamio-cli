## andamio dev logout

Clear stored developer JWT and refresh token

### Synopsis

Clear the stored developer JWT and refresh token. Does not affect the
wallet/user JWT — 'andamio user logout' clears that slot independently.

After logout, 'dev refresh' will fail; re-run 'dev login' to mint a new
session.

Caveat: if ANDAMIO_DEV_REFRESH_TOKEN (or ANDAMIO_DEV_JWT) is exported in
your environment, logout clears the on-disk slot but the next CLI
invocation re-injects the env value via Load(). For ephemeral CI/CD
runs that need true logout, unset the env var(s) before relying on
logout, or use a tmpfs HOME and discard the directory on exit.

```
andamio dev logout [flags]
```

### Options

```
  -h, --help   help for logout
```

### Options inherited from parent commands

```
  -o, --output string   Output format: text, json, csv, markdown (default "text")
```

### SEE ALSO

* [andamio dev](andamio_dev.md)	 - Developer-portal operations (login, manage API keys)

