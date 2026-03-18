---
review_agents:
  - kieran-typescript-reviewer
  - security-sentinel
  - performance-oracle
  - architecture-strategist
  - code-simplicity-reviewer
---

# Andamio CLI Review Context

This is a Go CLI tool (Cobra-based) for the Andamio Protocol. Key conventions:

- All progress/status messages MUST go to stderr (`fmt.Fprintf(os.Stderr, ...)`)
- Structured data MUST go to stdout via the `output` package
- `--output json` is the scripting surface — must be stable
- No interactive prompts, no stdin reads
- Commands use `RunE` and return errors; `main.go` handles exit codes
- Exit code contract: 0=success, 1=generic error, 2=not found, 3=auth required
