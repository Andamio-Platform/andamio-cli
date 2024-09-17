# accept-assignment
Approve a student commitment to course assignment and issue credential for completion.


About:
A teacher can accept or deny student commitments to assignments.

This transaction accepts the current assignment for the student with studentAlias in the course specified by policy. 

The transaction must be signed by the holder of userAccessToken.

Example:
  andamio-cli transaction course-creator accept-assignment \ 
    --userAccessToken ASSET_ID (POLICY_ID+ASSET_NAME) \
    --studentAlias STRING \
    --policy POLICY_ID


  

**Usage:**
```
andamio-cli transaction course-creator accept-assignment [flags]

```


```

**Options:**
```
--policy string            Course NFT policy id
      --studentAlias string      Access token name of student with committed assignment
      --userAccessToken string   Cardano Asset ID of teacher access token. The wallet holding this asset must sign the generated transaction.
```


