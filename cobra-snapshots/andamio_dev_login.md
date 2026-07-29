## andamio dev login

Authenticate as a developer (browser wallet, or headless CIP-8 with --skey)

### Synopsis

Mint a developer JWT by signing a gateway nonce with your wallet. The
resulting JWT + 30-day refresh token are bound to your developer account
and required for /v2/keys, /v2/apikey/developer/*, and other developer-portal
endpoints.

Two modes:

  Browser mode (default — no flags):
    andamio dev login
    Opens your browser to the andamio.io dev-portal sign-in page. Connect
    your wallet (Eternl/Lace/Nami/etc.), sign the nonce in-browser, and the
    CLI receives the JWT pair via an ephemeral localhost callback. Same flow
    you used to claim your API key at app.andamio.io.

  Headless mode (--skey/--alias/--address — all three required):
    andamio dev login --skey ./payment.skey --alias myalias --address $(cat wallet.addr)
    Signs the nonce locally with a .skey file on disk. Suitable for CI/CD,
    devkit, and ops automation that has access to raw signing keys.

Both modes require an API key (run 'andamio auth login --api-key <key>' first
— dev-portal endpoints are dual-credential surfaces requiring both
X-API-Key and the developer JWT).

Browser mode waits up to 5 minutes for the callback. Ctrl-C aborts; the OS
releases the ephemeral listener port on process exit.

```
andamio dev login [flags]
```

### Options

```
      --address string   Bech32 wallet address bound to the access-token alias (required for headless mode)
      --alias string     Developer access-token alias (required for headless mode)
  -h, --help             help for login
      --skey string      Path to .skey file (required for headless mode)
```

### Options inherited from parent commands

```
  -o, --output string   Output format: text, json, csv, markdown (default "text")
```

### SEE ALSO

* [andamio dev](andamio_dev.md)	 - Developer-portal operations (login, manage API keys)

