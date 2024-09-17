# mint-module-tokens
Publish course credential criteria on-chain


About:
Before a student can commit to an assignment, the course creator must publish credential criteria on-chain.

This transaction mints course module tokens specifying Student Learning Targets (SLTs) and an assignment for each course module.

The transaction must be signed by the holder of userAccessToken.

Example:
  andamio-cli transaction course-creator mint-module-tokens \ 
    --userAccessToken ASSET_ID \
    --policy POLICY_ID \
    --moduleInfos STRING 

  

**Usage:**
```
andamio-cli transaction course-creator mint-module-tokens [flags]

```



**Options:**
```
--moduleInfos string       List of course module information. Use andamio-cli write module-info to generate valid module-info
      --policy string            Course NFT policy id
      --userAccessToken string   Cardano Asset ID of teacher access token. The wallet holding this asset must sign the generated transaction.
```


