# student update-assignment
Update assginment evidence


About:
A student can update assignment info any time.  

This transaction allows the holder of userAccessToken to update assignmentInfo in the course specified by policy.

The transaction must be signed by the holder of userAccessToken.



### Usage:
```
andamio-cli transaction student update-assignment [flags]

```

### Options:
```
--assignmentInfo string    Evidence of assignment completion
      --policy string            Course NFT policy id
      --userAccessToken string   Cardano Asset ID of student access token. The wallet holding this asset must sign the generated transaction.
```

