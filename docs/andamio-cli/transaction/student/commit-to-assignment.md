## andamio-cli transaction student commit-to-assignment

Commit to an assignment

### Synopsis


About:
When a student is enrolled in a course, they can commit to assignments and earn credentials.

This transaction commits userAccessToken to assignmentCode in the course specified by policy.

To make a commitment, the student must provide assignmentInfo as evidence.

The transaction must be signed by the holder of userAccessToken.

To view valid assigmentCodes, use andamio-cli query course module decoded-module-ref-datums 

Example:
  andamio-cli transaction student commit-to-assignment \ 
    --userAccessToken ASSET_ID (POLICY_ID+ASSET_NAME) \
    --policy POLICY_ID \
    --assignmentCode STRING \
    --assignmentInfo STRING




```
andamio-cli transaction student commit-to-assignment [flags]
```

### Options

```
      --assignmentCode string    Identifier for Assignment, corresponding to the asset name of a course module token.
      --assignmentInfo string    Evidence of assignment completion
  -h, --help                     help for commit-to-assignment
      --policy string            Course NFT policy id
      --userAccessToken string   Cardano Asset ID of student access token. The wallet holding this asset must sign the generated transaction.
```

