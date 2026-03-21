# CLI Testing Templates

## Test Metadata Template

```json
{
  "cli_name": "andamio",
  "version": "0.1.0",
  "test_run_id": "test-2024-03-20-17-30",
  "start_time": "2024-03-20T17:30:00Z",
  "end_time": "2024-03-20T17:45:00Z",
  "total_commands": 47,
  "commands_tested": 45,
  "commands_failed": 2,
  "auth_required": true,
  "auth_tested": true,
  "output_formats": ["text", "json", "csv", "markdown"],
  "environment": {
    "os": "darwin",
    "shell": "zsh",
    "terminal": "iterm2"
  }
}
```

## Bug Report Template

```markdown
## Bug: [Brief Description]

**Severity**: Critical/High/Medium/Low
**Command**: `cli command args`
**Expected**: What should happen
**Actual**: What actually happened
**Error Output**: 
```
[Error message or output]
```

**Reproduction Steps**:
1. Run `cli command`
2. Observe error
3. Verify with `cli --help`

**Environment**: 
- CLI version: 0.1.0
- OS: macOS 14.2
- Auth state: logged in

**Notes**: Additional context or workarounds
```

## Test Case Template

```markdown
### Test: [Command Name]

**Command**: `cli subcommand action`
**Category**: Help/Success/Error/Format
**Auth Required**: Yes/No

**Test Cases**:
- [ ] Help text: `cli subcommand --help`
- [ ] Basic execution: `cli subcommand valid-args`
- [ ] JSON output: `cli subcommand --output json`
- [ ] CSV output: `cli subcommand --output csv`
- [ ] Missing args: `cli subcommand` (should error)
- [ ] Invalid args: `cli subcommand invalid-id`

**Results**:
- ✅ Help text complete and accurate
- ✅ Basic execution works
- ❌ JSON output malformed (missing closing brace)
- ✅ CSV output correct
- ✅ Missing args error clear
- ❌ Invalid args cause panic instead of error

**Files Generated**:
- `commands/help/subcommand.txt`
- `commands/success/subcommand.json`
- `commands/errors/subcommand-missing-args.txt`
```

## Report Summary Template

```markdown
# CLI Test Summary: [CLI Name] v[Version]

**Test Run**: [Date] | **Duration**: [Duration] | **Commands Tested**: [X/Y]

## Executive Summary

- **Success Rate**: 89% (40/45 commands)
- **Critical Issues**: 1 (command crashes)
- **High Issues**: 3 (broken functionality)  
- **Medium Issues**: 8 (inconsistencies)
- **Low Issues**: 12 (UX improvements)

## Coverage Analysis

| Category | Commands | Tested | Success | Notes |
|----------|----------|---------|---------|-------|
| auth | 3 | 3 | 100% | All working |
| user | 5 | 5 | 80% | JWT issue |
| course | 12 | 12 | 92% | Format bugs |
| project | 8 | 8 | 88% | Help missing |
| tx | 15 | 12 | 75% | 3 untestable |

## Output Format Support

| Format | Commands Supporting | Success Rate | Issues |
|--------|-------------------|--------------|--------|
| text | 45/45 | 100% | Default format |
| json | 38/45 | 84% | 7 malformed outputs |
| csv | 25/45 | 68% | 17 missing support |
| markdown | 15/45 | 33% | Limited support |

## Authentication Testing

- **No Auth**: 15 commands tested, 13 working
- **Valid Auth**: 30 commands tested, 27 working  
- **Invalid Auth**: Error handling tested, 5 unclear errors

## Priority Issues

### Critical (Fix Immediately)
1. **Command crash**: `user me` segfaults with invalid token
2. **Data corruption**: `course export` produces invalid JSON

### High (Fix Next Release)  
1. **Missing --output json**: 7 commands don't support JSON
2. **Inconsistent errors**: Auth failures return HTTP 200
3. **Missing help**: 5 commands have no --help

## Recommendations

### Immediate Actions
- [ ] Fix segfault in `user me` command
- [ ] Validate JSON output in `course export`
- [ ] Add error handling for invalid auth tokens

### Next Release
- [ ] Standardize output format support across all commands
- [ ] Implement consistent error response format
- [ ] Add missing help text for all commands
- [ ] Add --dry-run support for destructive operations

### Documentation
- [ ] Update CLI docs with format support matrix
- [ ] Add troubleshooting guide for auth issues
- [ ] Create example usage guide

## Integration Opportunities

**With `/ce-plan`**:
- Create detailed plans for critical bug fixes
- Plan output format standardization
- Design consistent error handling

**With `/ce-work`**: 
- Implement bug fixes systematically
- Add missing help text
- Standardize output formats

**Regression Testing**:
- Baseline established in `cli-outputs/`
- Ready for version comparison testing
- Performance benchmarks captured

## Files Generated

- `cli-outputs/` - Complete test results
- `bugs.md` - Tracked issues by severity  
- `test-metadata.json` - Test run details
```

## Command Discovery Script Template

```bash
#!/bin/bash
# discover-commands.sh - Extract all CLI commands

CLI="$1"
OUTPUT_FILE="discovered-commands.txt"

echo "Discovering commands for: $CLI"

# Get root help
$CLI --help > help-root.txt 2>&1

# Extract command list (adapt regex for each CLI)
grep -E "^\s+[a-z]" help-root.txt | awk '{print $1}' > commands.txt

# Get subcommands for each command
while read cmd; do
    echo "Discovering subcommands for: $cmd"
    $CLI $cmd --help > "help-$cmd.txt" 2>&1
    
    # Extract subcommands (adapt pattern)
    grep -E "^\s+[a-z]" "help-$cmd.txt" | awk '{print $1}' | while read subcmd; do
        echo "$cmd $subcmd" >> "$OUTPUT_FILE"
    done
done < commands.txt

echo "Command discovery complete: $OUTPUT_FILE"
```