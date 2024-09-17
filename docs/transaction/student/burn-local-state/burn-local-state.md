# burn-local-state
Un-enroll in a course


About:
When a student is ready to leave a course, they can un-enroll. Un-enrollment can happen any time, whether the student has completed all course modules or not.

This transaction un-enrolls userAccessToken in the course specified by policy.

In this transaction, any earned course credentials are moved to the access token credentials of userAccessToken.

The transaction must be signed by the holder of userAccessToken.

Example:
  andamio-cli transaction student burn-local-state \ 
    --userAccessToken ASSET_ID (POLICY_ID+ASSET_NAME) \
    --policy POLICY_ID




**Usage:**
```
andamio-cli transaction student burn-local-state [flags]

```


```

**Options:**
```
--policy string            Course NFT policy id
      --userAccessToken string   Cardano Asset ID of student access token. The wallet holding this asset must sign the generated transaction.
```


