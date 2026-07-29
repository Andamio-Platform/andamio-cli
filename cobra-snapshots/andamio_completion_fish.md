## andamio completion fish

Generate the autocompletion script for fish

### Synopsis

Generate the autocompletion script for the fish shell.

To load completions in your current shell session:

	andamio completion fish | source

To load completions for every new session, execute once:

	andamio completion fish > ~/.config/fish/completions/andamio.fish

You will need to start a new shell for this setup to take effect.


```
andamio completion fish [flags]
```

### Options

```
  -h, --help              help for fish
      --no-descriptions   disable completion descriptions
```

### Options inherited from parent commands

```
  -o, --output string   Output format: text, json, csv, markdown (default "text")
```

### SEE ALSO

* [andamio completion](andamio_completion.md)	 - Generate the autocompletion script for the specified shell

