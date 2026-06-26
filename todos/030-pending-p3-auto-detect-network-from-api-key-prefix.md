---
status: pending
priority: p3
issue_id: "030"
tags: [config, ux, api-key, network]
dependencies: []
---

# Auto-Detect Network from API Key Prefix

## Problem

API keys are prefixed to indicate their environment — `ant_pp_` = preprod,
(mainnet prefix TBC). The CLI currently ignores this prefix entirely. Users
must manually set the correct base URL via `andamio config set-url` or risk
running preprod keys against the mainnet API (or vice versa), which will
silently 401.

## Proposed Solution

When an API key is stored via `andamio auth login --api-key <key>`, inspect
the key prefix and automatically set `BaseURL` to the matching environment:

```
ant_pp_  →  https://preprod.api.andamio.io
ant_mn_  →  https://mainnet.api.andamio.io  (confirm prefix with API team)
```

Warn the user if the prefix is unrecognised rather than silently ignoring it.

### Suggested implementation

In `cmd/andamio/auth.go`, after storing the API key, add a prefix check:

```go
switch {
case strings.HasPrefix(key, "ant_pp_"):
    cfg.BaseURL = "https://preprod.api.andamio.io"
    fmt.Fprintln(os.Stderr, "Network: preprod (detected from API key prefix)")
case strings.HasPrefix(key, "ant_mn_"):
    cfg.BaseURL = "https://mainnet.api.andamio.io"
    fmt.Fprintln(os.Stderr, "Network: mainnet (detected from API key prefix)")
default:
    fmt.Fprintln(os.Stderr, "Warning: unrecognised API key prefix — base URL unchanged. Run 'andamio config set-url' to set manually.")
}
```

## Notes

- Confirm the mainnet key prefix with the Andamio API team before implementing
- Should also surface the detected network in `andamio auth status` output
- `andamio config set-url` should still override — explicit always wins
