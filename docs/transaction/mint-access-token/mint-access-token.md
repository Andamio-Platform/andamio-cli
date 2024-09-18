# transaction mint-access-token
Mint an Andamio access token


About:
An access token is required to interact with Andamio. 

This transaction mints a unique Andamio access token. This transaction will fail if access token name is already minted.

  

### Usage:
```
andamio-cli transaction mint-access-token [flags]

```

### Options:
```
--alias string         Unique access token name
      --userAddress string   Preprod wallet address to receive access token. Minting transaction requires a signature from this address.
      --userInfo string      Optional string. (default "new Andamio access token")
```

