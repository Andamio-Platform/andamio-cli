---
name: cli-comprehensive-tester
description: Systematically test every command in a CLI application and generate comprehensive reports. Use when testing CLI tools for bugs, documenting behavior, validating new CLI implementations, regression testing after changes, or conducting QA sessions. Triggers on "test every command", "comprehensive CLI testing", "systematically test CLI", "find bugs in CLI", "CLI testing session".
allowed-tools: Bash, Read, Write
---

# CLI Comprehensive Tester

Systematically tests every command in a CLI application, creating structured outputs, tracking bugs, and generating comprehensive reports with safety measures.

## Quick Start

```bash
# Test a CLI thoroughly
/cli-comprehensive-tester andamio

# Test specific command groups  
/cli-comprehensive-tester kubectl --focus="get,create,delete"

# Test with authentication context
/cli-comprehensive-tester gh --auth-required
```

## Instructions

### Step 1: Discovery and Planning

1. **Identify the CLI binary** and verify it exists
2. **Get help output** to understand command structure
3. **Map command hierarchy** (subcommands, flags, arguments)
4. **Identify auth requirements** and safety considerations
5. **Create test plan** with command priorities

### Step 2: Setup Test Environment

1. **Create output directory structure:**
   ```
   cli-outputs/
   ├── commands/           # Individual command outputs
   │   ├── help/          # Help text for each command
   │   ├── success/       # Successful command runs
   │   ├── errors/        # Error condition tests
   │   └── formats/       # Different output formats
   ├── auth-states/       # Tests with different auth levels
   ├── bugs.md           # Bug tracking with severity
   ├── cli-test-summary.md # Comprehensive report
   └── test-metadata.json # Test run metadata
   ```

2. **Initialize tracking files:**
   - `bugs.md` with severity template
   - `cli-test-summary.md` with report structure
   - `test-metadata.json` with CLI info and timestamps

### Step 3: Systematic Testing

#### A. Help Text Collection
Test help output for every discoverable command:
- Root help: `cli --help`
- Subcommand help: `cli subcommand --help` 
- Deep subcommands: `cli group subcommand --help`
- Version info: `cli --version`

#### B. Command Execution Testing
For each command, test:
- **Basic execution** with minimal valid args
- **Different output formats** (json, csv, text, markdown)
- **Common flags** (--verbose, --quiet, --dry-run)
- **List vs Get patterns** (if applicable)

#### C. Error Condition Testing
- **Missing required arguments**
- **Invalid argument values** 
- **Malformed flags**
- **Non-existent resources**
- **Permission errors** (if safe to test)

#### D. Authentication State Testing
- **No auth** (before login)
- **Valid auth** (after login)
- **Expired auth** (if detectable)
- **Invalid credentials**

### Step 4: Safety Measures

**NEVER run commands that:**
- Delete or destroy data (`delete`, `destroy`, `rm`)
- Modify production systems (`deploy`, `apply`, `create` in prod)
- Send emails/notifications (`send`, `notify`, `alert`)
- Spend money (`buy`, `purchase`, `billing`)
- Make permanent changes without `--dry-run`

**Safety patterns:**
- Use `--dry-run`, `--preview`, `--simulate` flags when available
- Test in read-only mode first
- Check for confirmation prompts and avoid them
- Use `--help` liberally to understand command effects
- Test with invalid/fake IDs to trigger safe errors

### Step 5: Bug Documentation

Track issues in `bugs.md` with this structure:

```markdown
# CLI Testing Bugs

## Critical (Breaks core functionality)
- [ ] **Command crashes**: `cli user me` segfaults
- [ ] **Data corruption**: `cli export` produces invalid JSON

## High (Major usability issues)  
- [ ] **Inconsistent output**: `--output json` not supported on `cli course list`
- [ ] **Misleading errors**: `cli login` says "success" but fails

## Medium (Minor issues)
- [ ] **Missing help**: `cli project task` has no --help
- [ ] **Inconsistent naming**: `course-id` vs `courseId` in different commands

## Low (Polish/UX)
- [ ] **Verbose output**: Too much debug info in normal mode
- [ ] **Missing examples**: Help text lacks usage examples
```

### Step 6: Report Generation

Generate `cli-test-summary.md` with:

#### Executive Summary
- Total commands tested
- Success rate percentage
- Bug count by severity
- Authentication coverage
- Output format coverage

#### Command Coverage Matrix
Table showing which commands were tested with what scenarios:
- Help text ✓/✗
- Basic execution ✓/✗  
- Output formats ✓/✗
- Error conditions ✓/✗
- Auth states ✓/✗

#### Findings and Recommendations
- **Critical issues** requiring immediate attention
- **Patterns identified** (missing --help, inconsistent flags)
- **Missing features** (output formats, dry-run modes)
- **Documentation gaps** (unclear help text, missing examples)

#### Next Steps
- Priority order for bug fixes
- Suggestions for `/ce-plan` integration
- Areas needing deeper testing

## Integration Patterns

### With Bug Fixing Workflows
```bash
# After comprehensive testing
/ce-plan "Fix critical CLI bugs found in cli-outputs/bugs.md"
/ce-work  # Implement the fixes
/cli-comprehensive-tester andamio --regression  # Re-test
```

### With Documentation Workflows
```bash
# Document CLI behavior
/cli-comprehensive-tester myapp
# Use outputs in cli-outputs/ to write comprehensive docs
```

### With Release Validation
```bash
# Pre-release testing
/cli-comprehensive-tester --focus="core commands" --auth-required
# Post-release regression testing  
/cli-comprehensive-tester --compare-with=previous-outputs/
```

## Advanced Features

### Focused Testing
Use `--focus` to test specific command groups:
```bash
/cli-comprehensive-tester kubectl --focus="get,describe,logs"
```

### Regression Mode
Compare current run with previous outputs:
```bash
/cli-comprehensive-tester --regression --baseline=cli-outputs-v1.0/
```

### Performance Tracking
Time command execution and track in metadata:
```bash
# Automatically tracks timing for performance regression detection
```

## Output Structure Reference

```
cli-outputs/
├── commands/
│   ├── help/
│   │   ├── root.txt
│   │   ├── user.txt
│   │   ├── user-login.txt
│   │   └── course-list.txt
│   ├── success/
│   │   ├── user-me.json
│   │   ├── course-list.txt
│   │   └── course-list.json
│   ├── errors/
│   │   ├── user-me-no-auth.txt
│   │   ├── course-get-invalid-id.txt
│   │   └── missing-args/
│   └── formats/
│       ├── json/
│       ├── csv/
│       └── markdown/
├── auth-states/
│   ├── no-auth/
│   ├── valid-auth/
│   └── invalid-auth/
├── bugs.md
├── cli-test-summary.md
└── test-metadata.json
```

## Success Criteria

- [ ] **Complete command discovery** - All help text collected
- [ ] **Systematic execution** - Each command tested with valid args
- [ ] **Error coverage** - Common error conditions documented
- [ ] **Format testing** - All output formats validated where supported
- [ ] **Auth state coverage** - Tested with different authentication levels
- [ ] **Safety maintained** - No destructive operations executed
- [ ] **Structured outputs** - All results organized in logical hierarchy
- [ ] **Bug documentation** - Issues tracked with severity and reproduction steps
- [ ] **Comprehensive report** - Executive summary with actionable recommendations
- [ ] **Integration ready** - Outputs ready for `/ce-plan` and `/ce-work` workflows

## Examples

### Testing a Go CLI
```bash
/cli-comprehensive-tester andamio
# Discovers: auth, config, user, course, project, tx, apikey, spec commands
# Tests: help text, basic execution, output formats, auth states
# Reports: 47 commands tested, 3 critical bugs, 12 medium issues
```

### Testing Kubernetes CLI
```bash  
/cli-comprehensive-tester kubectl --focus="get,describe,logs" --auth-required
# Focuses on read-only operations to avoid cluster changes
# Tests different resource types and output formats
# Documents inconsistencies in flag support
```

### Regression Testing
```bash
/cli-comprehensive-tester myapp --regression --baseline=v1.0-outputs/
# Compares current behavior with baseline
# Highlights new failures or changed behavior
# Generates diff report for review
```