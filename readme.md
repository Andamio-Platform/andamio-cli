## Welcome to Andamio CLI

### About

Andamio CLI provides quick access to Andamio with a set of command-line tools, so that developers can build upon Andamio.

Developers can:

1. Query the Andamio Network
2. Build valid transactions
3. Write valid Andamio data

This project is developed by [Andamio](https://andamio.io) and supported by:

- [Blink Labs](https://blinklabs.io/), who build amazing tools and who advised this project.
- [Project Catalyst](https://projectcatalyst.io/): Where this project was made possible with our [Fund 11 Proposal: Open-Source Cardano Go Libraries + Docs + Andamio CLI](https://milestones.projectcatalyst.io/projects/1100216/)

### Roadmap

- [x] 2024 Q2: Build and test initial features
- [x] 2024 Q3: Integrate Andamio Course transactions and queries
- [ ] 2024 Q4: Integrate Andamio Contribution transactions and queries
- [ ] 2024 Q4: Expand upon provided examples in [Cardano Go PBL Course](https://www.andamio.io/course/gpbl2024)

### Install Andamio CLI

1. Download executables from [release page](https://github.com/Andamio-Platform/andamio-cli/releases)

2. Or by building from source (requires Go)

```
git clone https://github.com/Andamio-Platform/andamio-cli
cd andamio-cli
go mod tidy
go build
./andamio-cli
```

### Links

- [Andamio CLI Source Code on Github](https://github.com/Andamio-Platform/andamio-cli)

## Learning with Andamio CLI

### Config

Some functions require access to a Cardano Node. Create a `.env` and specify these values:

```bash
CARDANO_NODE_MAGIC="1"
CARDANO_NODE_SOCKET_PATH="<PATH TO>/node.socket"
```

Then run `andamio-cli` from the same directory.

## Credits

Thank you to Blink Labs for sharing examples.

- https://github.com/blinklabs-io/adder-library-starter-kit/tree/main is used in `/sync`
