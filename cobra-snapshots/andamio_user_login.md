## andamio user login

Authenticate via browser wallet signing or .skey file

### Synopsis

Authenticate to use owner and manager commands.

Browser login (default):
  andamio user login
  Opens your browser for wallet signing.

Headless login (for CI/CD, scripting, agents):
  andamio user login --skey ./payment.skey --alias myalias --address $(cat wallet.addr)
  Signs a nonce with your .skey file — no browser needed.

```
andamio user login [flags]
```

### Options

```
      --address string   Bech32 address (required with --skey, e.g. from .addr file)
      --alias string     Andamio alias (required with --skey)
  -h, --help             help for login
      --skey string      Path to .skey file for headless authentication (no browser)
```

### Options inherited from parent commands

```
  -o, --output string   Output format: text, json, csv, markdown (default "text")
```

### SEE ALSO

* [andamio user](andamio_user.md)	 - User information and authentication

