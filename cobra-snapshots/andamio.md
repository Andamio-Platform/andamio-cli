## andamio

CLI for interacting with the Andamio Protocol

### Synopsis

Andamio CLI provides commands for interacting with the Andamio Protocol.

Query courses, credentials, and more from the command line.

Machine-readable output: pass --output json to any list/get/action command for
structured JSON. "andamio --version --output json" emits
{"version":"<x>","commit":"<sha7>","built":"<timestamp>"} so scripts and agents
can identify the CLI version before invoking commands. See CHANGELOG.md for the
envelope contract and breaking-change history.

### Options

```
  -h, --help            help for andamio
  -o, --output string   Output format: text, json, csv, markdown (default "text")
```

### SEE ALSO

* [andamio apikey](andamio_apikey.md)	 - API key management
* [andamio auth](andamio_auth.md)	 - Authenticate with the Andamio API
* [andamio completion](andamio_completion.md)	 - Generate the autocompletion script for the specified shell
* [andamio config](andamio_config.md)	 - Manage CLI configuration
* [andamio course](andamio_course.md)	 - Manage courses
* [andamio dev](andamio_dev.md)	 - Developer-portal operations (login, manage API keys)
* [andamio manager](andamio_manager.md)	 - Project manager operations (requires user login)
* [andamio project](andamio_project.md)	 - Manage projects
* [andamio spec](andamio_spec.md)	 - Manage OpenAPI spec
* [andamio teacher](andamio_teacher.md)	 - Teacher operations (requires user login)
* [andamio token](andamio_token.md)	 - Native asset token registry
* [andamio tx](andamio_tx.md)	 - Transaction operations
* [andamio user](andamio_user.md)	 - User information and authentication

