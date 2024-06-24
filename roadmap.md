
# Andamio Roadmap:

1. Utilities Phase
2. Queries Phase
3. Deployment Phase

## Utilities Phase
Next Steps (see also roadmap):
- Deploy reference scripts
- Compile contract instances

## Queries Phase
Next Steps (see also roadmap):
- Query course status
- Query learner status
- Aggregate course learner status
- Aggregate contributor status

## Deployment Phase
- Contract interaction + all transaction building
    - For Course and Contributor Admins
- Custom indexer instantiation

# Feature List
### Q1 2024:
- [x] Initialize project. Create project structure and share at Live Coding
- [x] Outline possible project features

### Q2 2024:
- [x] Local node configuration in .env: network and
- [x] Simple metadata writer
- [x] Querying Andamio Network (see Blog Post)
- [x] Proof of Concept: Custom data serialization for contract datum transaction
- [ ] Add .env config as command line argument

### Q3 2024:
- [ ] Signing transactions with Bursa
- [ ] Importing keys with Bursa - ask for this to be on Bursa roadmap
- [ ] Wallet configuration in .env: network and keys
- [ ] Reference script deployment
- [ ] Setting up own Andamio index (see Blog Post)
- [ ] Custom data serialization for Andamio transactions

**Transaction Building**
- [x] Wallet to Wallet
- [ ] Lock tokens at contract with Datum
- [ ] Validator interaction transactions: start with Manage Contract Token tx - to prove that people can interact with Andamio without the front-end
- [ ] Additional transactions: any transactions that are currently built with bash scripts can be implemented in Andamio CLI

**R&D: Querying a side-chain**
Pending results of [Andamio Purpose Sidechain / Layer 2 Concept](https://cardano.ideascale.com/c/idea/122585)
- [ ] Define the possibilities of the "Andamio Node"?
- [ ] Define role of Andamio CLI in Andamio sidechain

### Q4 2024:
- [ ] Andamio Public API MVP
- [ ] CLI includes server functions to run APIs
- [ ] Course Creators can get insights from their courses using Andamio CLI