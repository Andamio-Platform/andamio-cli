## Draft Documentation

### Use Cases:
- Write NFT metadata files
- Query Andamio data
- Mint Andamio tokens
- Deploy Andamio reference scripts
- Write datums and redeemers


### Usage Example
```bash
andamio-cli write nft-metadata \
--policyid "036c625d15833ab9212f48c9daa56d82fe09bdf19393cb0630235e34" \
--asset-name "go-contrib-001" \
--name "Go Contributor" \
--image "ipfs://bafkreibm5ncfkuxm3u2xupnpujb4pr3tne6jzxxhhrupuknjt6fzwattmm" \
--media-type "img/jpg" \
--description "Contributor token for Cardano Go instance of Andamio Prototype. Learn more at andamio.io" \
--out-file "contrib-nft-metadata.json"
```

Why this matters: save time when working with minting scripts