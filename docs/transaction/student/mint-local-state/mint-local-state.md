# student mint-local-state
Enroll in a course on Andamio network


About:
The holder of an access token can enroll in courses on the Andamio Network.

This transaction enrolls userAccessToken in the course specified by policy.

The transaction must be signed by the holder of userAccessToken.



### Usage:
```
andamio-cli transaction student mint-local-state [flags]

```

### Options:
```
--policy string            Course NFT policy id
      --userAccessToken string   Cardano Asset ID of student access token. The wallet holding this asset must sign the generated transaction.
```

