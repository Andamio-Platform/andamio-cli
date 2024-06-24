# Andamio CLI

[View Andamio CLI Roadmap](./roadmap.md)

## Using Andamio CLI
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