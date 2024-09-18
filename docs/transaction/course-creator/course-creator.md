# transaction course-creator
Transactions for course creators

Course creators are responsible for publishing credential criteria and issuing credentials.

mint-module-tokens publishes credential criteria by minting a token 
on Cardano, accompanied by a list of SLTs and an Assignment reference

After credentials are published, students can commit to assignments. Creators can accept and deny
student commitments with the transactions included here.
  

### Usage:
```
andamio-cli transaction course-creator
andamio-cli transaction course-creator [command]
```

### Available Commands:
```
accept-assignment  Approve a student commitment to course assignment and issue credential for completion.
deny-assignment    Deny a student commitment to course assignment
mint-module-tokens Publish course credential criteria on-chain
```

### Options:
```

```

Use "andamio-cli transaction course-creator [command] --help" for more information about a command.

