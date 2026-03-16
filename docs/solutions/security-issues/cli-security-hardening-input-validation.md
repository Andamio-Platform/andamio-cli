# CLI Security Hardening: Input Validation and Credential Protection

---
title: "Andamio CLI Security Hardening - Input Injection and Credential Exposure Fixes"
category: security-issues
tags: [go, cli, input-validation, credential-management, url-injection, http-security, path-traversal]
severity: medium
components: [config-management, http-client, authentication, user-endpoints, spec-endpoints]
date_solved: 2026-03-16
related_commits: [75ae43e, c6a31a1]
---

## Problem Symptoms

The Andamio CLI contained multiple security vulnerabilities that could be exploited:

### Input Injection Risks (H1/H2 severity)

- **Unescaped URL path parameters**: User-supplied course IDs, project IDs, user aliases, and transaction hashes were directly concatenated into REST API paths without URL encoding. An attacker could inject special characters (`/`, `?`, `#`, `..`) to manipulate API routing or access unintended endpoints.
  - Affected: `/api/v2/course/user/course/get/{courseID}`, `/api/v2/project/user/project/{projectID}`, `/api/v2/user/exists/{alias}`, `/api/v2/tx/status/{txHash}`
  - Example attack: `courseID="../../admin"` → `/api/v2/course/get/../../admin` → `/api/v2/admin`

- **No base URL validation**: Users could set any URL via `config set-url`, allowing redirection to malicious servers

- **Config file tampering**: Validation only occurred on command entry, not on config load - direct file modification bypassed all checks

### Credential Exposure (M1/M2 severity)

- **Visible API key in output**: Status commands displayed first 8 characters (`Authenticated (key: sk_abc123de...)`)
- **Untruncated API errors**: Full error response bodies exposed, potentially leaking internal information

### Reliability Issues (M3-M5 severity)

- **No HTTP timeout**: Client could hang indefinitely on slow/unresponsive servers
- **Permissive config permissions**: Directory created with `0755` (world-readable)
- **Missing HTTP method validation**: Auth callback accepted any HTTP method (GET, POST, etc.)

## Root Cause

The initial implementation prioritized functionality over security:
1. String concatenation used for URL building without encoding
2. No domain allowlist for API endpoints
3. Credentials displayed for "debugging convenience"
4. Default Go HTTP client settings used without hardening

## Solution

### 1. URL Path Escaping

Added `url.PathEscape()` to all user-provided URL path parameters:

```go
// Before (vulnerable)
return getJSON("/api/v2/course/user/course/get/" + args[0])

// After (safe)
return getJSON("/api/v2/course/user/course/get/" + url.PathEscape(args[0]))
```

Applied to 9 commands across `course.go`, `project.go`, `user.go`, `tx.go`.

### 2. Centralized URL Validation

Created `ValidateBaseURL()` in config package:

```go
func ValidateBaseURL(rawURL string) error {
    // Allow bypass for CI/automation
    if os.Getenv("ANDAMIO_ALLOW_ANY_URL") == "1" {
        return nil
    }

    parsed, err := url.Parse(rawURL)
    if err != nil {
        return fmt.Errorf("invalid URL: %w", err)
    }

    hostname := parsed.Hostname()

    // Allow localhost (including IPv6)
    isLocalhost := hostname == "localhost" || hostname == "127.0.0.1" || hostname == "::1"
    if isLocalhost {
        return nil
    }

    // Require HTTPS for non-localhost
    if parsed.Scheme != "https" {
        return fmt.Errorf("URL must use HTTPS (got %s)", parsed.Scheme)
    }

    // Domain allowlist
    if hostname != "andamio.io" && !strings.HasSuffix(hostname, ".andamio.io") {
        return fmt.Errorf("URL must be an andamio.io domain or localhost (got %s)", hostname)
    }

    return nil
}
```

Validation now runs on **config load** (not just set-url command):

```go
func Load() (*Config, error) {
    // ... load config ...

    if cfg.BaseURL != "" {
        if err := ValidateBaseURL(cfg.BaseURL); err != nil {
            return nil, fmt.Errorf("invalid base URL in config: %w", err)
        }
    }
    return &cfg, nil
}
```

### 3. Credential Masking

```go
// Before
fmt.Printf("Authenticated (key: %s...)\n", cfg.APIKey[:8])

// After
fmt.Println("Authenticated (API key configured)")
fmt.Println("API Key: ****... (configured)")
```

### 4. HTTP Client Hardening

```go
const (
    httpTimeout      = 30 * time.Second
    maxErrorBodySize = 500
)

func New(cfg *config.Config) *Client {
    return &Client{
        httpClient: &http.Client{Timeout: httpTimeout},
    }
}

func truncateErrorBody(body []byte) string {
    s := string(body)
    if len(s) > maxErrorBodySize {
        return s[:maxErrorBodySize] + "... (truncated)"
    }
    return s
}
```

### 5. Auth Callback Security

```go
mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodGet {
        http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
        return
    }
    // ... process callback
})
```

### 6. File Permission Hardening

```go
// Before
os.MkdirAll(configDir, 0755)

// After
os.MkdirAll(configDir, 0700)
```

## Prevention Strategies

### Code Review Checklist

- [ ] All user inputs in URL paths use `url.PathEscape()`
- [ ] Base URL validation occurs on config load, not just command entry
- [ ] No credentials displayed in output (use `****` masking)
- [ ] HTTP clients have explicit timeouts
- [ ] Error messages truncated to prevent info leakage
- [ ] Config files use `0600`/`0700` permissions
- [ ] HTTP handlers validate request methods
- [ ] HTML responses escape user-controlled data

### Patterns to Follow

```go
// URL building - always escape user input
path := "/api/endpoint/" + url.PathEscape(userInput)

// HTTP client - always set timeout
client := &http.Client{Timeout: 30 * time.Second}

// Config files - restrictive permissions
os.WriteFile(path, data, 0600)
os.MkdirAll(dir, 0700)

// Error messages - truncate
if len(errMsg) > 500 {
    errMsg = errMsg[:500] + "..."
}
```

## Testing Recommendations

### Path Injection Testing

```bash
./andamio course get "../../../admin"
./andamio course get "test#anchor"
./andamio course get "test?query=1"
./andamio tx status "hash/../../../"
```

### URL Validation Testing

```bash
# Should reject
./andamio config set-url "https://evil.com"
./andamio config set-url "http://andamio.io"  # HTTP not HTTPS

# Should accept
./andamio config set-url "https://preprod.api.andamio.io"
./andamio config set-url "http://localhost:8080"

# Automation bypass
ANDAMIO_ALLOW_ANY_URL=1 ./andamio config set-url "https://staging.test.io"
```

### Permission Testing

```bash
ls -ld ~/.andamio          # Should be drwx------ (0700)
ls -l ~/.andamio/config.json  # Should be -rw------- (0600)
```

## Related Documentation

### Internal References

- **CLAUDE.md** - Architecture guide with auth flow details
- **docs/plans/2026-03-14-test-wallet-auth-preprod-plan.md** - Wallet auth test cases including CSRF validation

### External References

- [OWASP Top 10](https://owasp.org/Top10/) - Web security risks
- [OWASP Credential Storage Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Credential_Storage_Cheat_Sheet.html)
- [Go's net/url package](https://pkg.go.dev/net/url) - URL parsing and encoding

## Commits

- `75ae43e` - Initial hardening (PathEscape, timeouts, masking, permissions)
- `c6a31a1` - Config load validation, automation override, IPv6 localhost support
