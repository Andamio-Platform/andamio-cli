## andamio

Developer CLI for authoring and assessing on the Andamio Protocol

### Synopsis

Andamio CLI is a developer tool for the people who author work and assess it:
course Owners and Teachers, and project Managers.

  Own       create courses and projects, register them on-chain, manage
            teachers and managers
  Teach     write and publish module content, review submissions, build
            assessment transactions
  Manage    create and mint project tasks, review contributions, assess work

Learners and contributors use the Andamio app, which signs and submits their
work in one flow.

BUILT TO BE DRIVEN BY PROGRAMS

Every list and get command takes --output json, and that is the stable surface
scripts and agents should use. Progress goes to stderr, data to stdout, and no
command reads stdin or prompts — everything works without a TTY.

Failures are distinguishable without parsing prose: each carries an exit code
and a "kind" field. An empty result is a success (exit 0, empty collection),
which is what keeps "nothing found" apart from "not permitted" (exit 3) and
"could not reach the service" (exit 5). Run 'andamio help exit-codes' for the
full table.

"andamio --version --output json" emits {"version":"<x>","commit":"<sha7>",
"built":"<timestamp>"} so a caller can identify the CLI before invoking it.
See CHANGELOG.md for the envelope contract and breaking-change history.

```
andamio [flags]
```

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

