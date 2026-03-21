# CLI Testing Safety Patterns

## Destructive Command Detection

### Dangerous Keywords to Avoid
Never execute commands containing these patterns without `--dry-run`:
- `delete`, `destroy`, `remove`, `rm`
- `drop`, `truncate`, `purge`
- `deploy`, `apply`, `push` (to production)
- `send`, `notify`, `email`, `alert`
- `buy`, `purchase`, `charge`, `bill`
- `format`, `wipe`, `reset` (without confirmation)

### Safe Testing Approaches
1. **Use help flags first**: `command --help` before any execution
2. **Read-only operations**: Prefer `get`, `list`, `show`, `describe`
3. **Dry-run flags**: Use `--dry-run`, `--preview`, `--simulate` when available
4. **Invalid IDs**: Test with fake/invalid identifiers to trigger safe errors
5. **Timeout commands**: Use timeouts to prevent hanging operations

## CLI-Specific Safety Patterns

### Kubernetes
- Never use `kubectl apply` without `--dry-run=client`
- Avoid `kubectl delete` without specific resources
- Use `kubectl get` and `kubectl describe` for safe testing

### Docker
- Never use `docker system prune` or `docker volume prune`
- Avoid `docker rm` or `docker rmi` 
- Use `docker ps`, `docker images`, `docker inspect` for safe operations

### Cloud CLIs (AWS, GCP, Azure)
- Never create/delete resources without cost estimation
- Use `--dry-run` flags when available
- Test with non-existent resource IDs first

### Git Operations
- Never use `git reset --hard` or `git clean -fd`
- Avoid `git push --force`
- Use `git status`, `git log`, `git show` for safe operations

## Error Condition Testing Templates

### Missing Arguments
```bash
# Test with no arguments
cli command

# Test with partial arguments  
cli command arg1

# Expected: Clear error message about required arguments
```

### Invalid Arguments
```bash
# Test with invalid IDs
cli get user invalid-id

# Test with malformed data
cli create --data "invalid-json"

# Expected: Validation errors, not crashes
```

### Authentication Errors
```bash
# Test without authentication
cli user me  # Should fail gracefully

# Test with invalid credentials
CLI_TOKEN=invalid cli user me

# Expected: Clear auth error messages
```

### Permission Errors
```bash
# Test operations requiring higher permissions
cli admin delete-all  # Should reject with permission error

# Expected: Permission denied, not crash
```

## Testing Methodology

### Progressive Testing
1. **Help text first**: Understand what command does
2. **Safe read operations**: Get/list with valid data
3. **Error conditions**: Invalid inputs, missing auth
4. **Edge cases**: Empty results, malformed responses

### Output Format Validation
```bash
# Test all supported formats
cli list --output json
cli list --output csv  
cli list --output table
cli list --output yaml

# Verify structure and data integrity
```

### Performance Boundaries
```bash
# Test with timeouts
timeout 10s cli long-running-command

# Test with limits
cli list --limit 1000  # Large but reasonable

# Expected: Commands should complete or timeout gracefully
```