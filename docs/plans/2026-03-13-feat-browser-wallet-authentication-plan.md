---
title: Browser-Based Wallet Authentication for CLI
type: feat
status: completed
date: 2026-03-13
---

# Browser-Based Wallet Authentication for CLI

## Overview

Add the ability for Andamio Access Token holders to authenticate via a web browser where they sign with their Cardano wallet. This enables CLI users to get a user JWT from dbapi, allowing them to edit courses and tasks.

**Authentication model:**
- API key (already implemented) → enables CLI access to public API endpoints
- User JWT (this feature) → enables authenticated owner endpoints (course/project editing)

## Problem Statement / Motivation

Currently, andamio-cli only supports API key authentication which provides access to read-only endpoints. Developers with Access Tokens need to authenticate as themselves to:
- Edit courses they own
- Manage course modules and SLTs
- Edit projects they manage
- Access owner-specific endpoints (`/v2/course/owner/*`, `/v2/project/owner/*`)

The Andamio app already has robust wallet-based authentication via CIP-30 signData. We need to bridge this to the CLI.

## Proposed Solution

Implement a **local callback server pattern** where:
1. CLI starts a local HTTP server on an ephemeral port
2. Opens browser to a dedicated CLI auth page on the Andamio app
3. User signs a challenge with their Cardano wallet
4. Browser redirects back to CLI's local server with the authorization code
5. CLI exchanges code for JWT and stores it locally

This approach was chosen over device flow (polling) because:
- Better UX (no manual code entry)
- Faster completion (no polling delay)
- Browser wallet interaction is required anyway for CIP-30 signing

## Technical Approach

### Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                        CLI AUTH FLOW                             │
├─────────────────────────────────────────────────────────────────┤
│                                                                  │
│  1. CLI: andamio user login                                      │
│     ├── Start HTTP server on 127.0.0.1:{random-port}            │
│     ├── Generate state parameter (CSRF protection)              │
│     └── Open browser to auth URL with redirect_uri              │
│                                                                  │
│  2. Browser: https://preprod.app.andamio.io/auth/cli            │
│     ├── User connects CIP-30 wallet                              │
│     ├── API creates login session → returns nonce               │
│     ├── User signs nonce with wallet.signData()                 │
│     ├── API validates signature → returns JWT                   │
│     └── Redirect to http://127.0.0.1:{port}/callback?jwt=...    │
│                                                                  │
│  3. CLI: Receive callback                                        │
│     ├── Validate state parameter                                 │
│     ├── Extract JWT from query params                           │
│     ├── Store JWT in ~/.andamio/config.json                     │
│     └── Shutdown local server                                   │
│                                                                  │
│  4. Subsequent CLI requests                                      │
│     └── Include JWT in Authorization: Bearer header             │
│                                                                  │
└─────────────────────────────────────────────────────────────────┘
```

### Implementation Phases

#### Phase 1: CLI Authentication Commands (andamio-cli)

**Tasks:**

- [x] **Extend config structure** (`internal/config/config.go`)
  ```go
  type Config struct {
      APIKey       string `json:"api_key"`
      BaseURL      string `json:"base_url"`
      // New fields:
      UserJWT      string `json:"user_jwt,omitempty"`
      JWTExpiresAt string `json:"jwt_expires_at,omitempty"`
      UserAlias    string `json:"user_alias,omitempty"`
      UserID       string `json:"user_id,omitempty"`
  }
  ```

- [x] **Extend HTTP client** (`internal/client/client.go`)
  - Add `userJWT` field to Client struct
  - Add `SetUserJWT(jwt string)` method
  - Include `Authorization: Bearer` header when userJWT is set
  - Add POST method support (for owner endpoints)
  ```go
  func (c *Client) Post(path string, body interface{}, result interface{}) error
  func (c *Client) Put(path string, body interface{}, result interface{}) error
  ```

- [x] **Add `user login` command** (`cmd/andamio/user.go`)
  - Start local HTTP server on ephemeral port (127.0.0.1:0)
  - Generate cryptographically secure state parameter
  - Open browser to auth URL: `{baseURL}/auth/cli?redirect_uri=...&state=...`
  - Wait for callback with timeout (5 minutes)
  - Validate state, extract JWT, store in config
  - Display success with user alias and expiration

- [x] **Add `user logout` command** (`cmd/andamio/user.go`)
  - Clear user_jwt, jwt_expires_at, user_alias, user_id from config
  - Display confirmation

- [x] **Add `user status` command** (`cmd/andamio/user.go`)
  - Show current auth state (API key status, user JWT status)
  - Show user alias if authenticated
  - Show JWT expiration time
  - Warn if JWT is expired or expiring soon

- [ ] **JWT validation on startup** (`cmd/andamio/root.go`)
  - Check if stored JWT is expired
  - Warn user if expired (don't auto-clear, they might want to re-auth)
  - Consider auto-refresh before expiry

**Dependencies:**
- `github.com/pkg/browser` - cross-platform browser opening
- Standard library `net/http` for local server
- `crypto/rand` for state parameter generation

#### Phase 2: App Auth Page (andamio-app-v2)

**Tasks:**

- [ ] **Create `/auth/cli` page** (`src/app/auth/cli/page.tsx`)
  - Parse `redirect_uri` and `state` from query params
  - Validate redirect_uri is localhost (security)
  - Render wallet connect UI
  - After successful auth, redirect to CLI callback with JWT

- [ ] **Create CLI auth component** (`src/components/auth/cli-auth-flow.tsx`)
  - Reuse existing `authenticateWithWallet()` from `andamio-auth.ts`
  - Display clear instructions ("Andamio CLI is requesting authentication")
  - Show wallet connection button
  - Handle signing flow
  - After JWT received, redirect with `?jwt={jwt}&state={state}`

- [ ] **Security validations**
  - Only allow redirect to `http://127.0.0.1:*` or `http://localhost:*`
  - Validate state parameter is passed through
  - Consider: Show user the requesting CLI version for trust

**UI Design:**
```
┌─────────────────────────────────────────────┐
│              Andamio CLI Login              │
├─────────────────────────────────────────────┤
│                                             │
│  The Andamio CLI is requesting access       │
│  to your account.                           │
│                                             │
│  [🦊 Connect Wallet]                        │
│                                             │
│  ─────────────────────────────────────────  │
│  This will allow the CLI to:                │
│  • View your courses and projects           │
│  • Edit content you own                     │
│  • Submit transactions on your behalf       │
│                                             │
│  [Cancel]                                   │
│                                             │
└─────────────────────────────────────────────┘
```

### API Endpoints Used

The app will use existing authentication endpoints (no new endpoints needed):

1. **Create session**: `POST /api/v2/auth/login/session` → `{id, nonce, expires_at}`
2. **Validate signature**: `POST /api/v2/auth/login/validate` → `{jwt, user}`

These already exist in dbapi and are used by the app's `authenticateWithWallet()`.

### Alternative Approaches Considered

1. **Device Flow (Polling)**
   - Rejected: Requires manual code entry, slower, but would work for headless environments
   - Could add as fallback for SSH sessions: `andamio user login --device-flow`

2. **Direct wallet signing in CLI**
   - Rejected: CLI cannot access browser wallet extensions (CIP-30 is browser-only)
   - Would require external signing tool or hardware wallet direct integration

3. **QR Code scanning**
   - Rejected: Requires mobile app or additional complexity
   - Not better UX than direct browser flow

4. **API key with elevated permissions**
   - Rejected: Doesn't provide user identity, can't track who made edits

## System-Wide Impact

### Interaction Graph

- CLI `user login` → Browser `/auth/cli` → `authenticateWithWallet()` → `/api/v2/auth/login/*` → JWT issued
- CLI commands → HTTP client → API with both `X-API-Key` and `Authorization: Bearer` headers
- JWT expiry → CLI warns user → User re-runs `user login`

### Error Propagation

- Browser auth failure → Show error in browser, user manually closes
- Callback timeout → CLI displays timeout error with retry instructions
- Invalid JWT → CLI clears stored JWT, prompts for re-auth
- Localhost binding failure → CLI shows error, suggests checking firewall

### State Lifecycle Risks

- JWT stored in plaintext config file (acceptable for CLI, similar to `gh`, `gcloud`)
- Browser window may remain open after auth (user can close manually)
- Concurrent login attempts could conflict (use unique state per attempt)

### API Surface Parity

After this feature, CLI users can access:
- All existing read endpoints (with API key)
- Owner endpoints (with user JWT):
  - `/v2/course/owner/course/*` - course CRUD
  - `/v2/course/owner/module/*` - module management
  - `/v2/project/owner/*` - project management
  - `/v2/user/*` - user profile management

## Acceptance Criteria

### Functional Requirements

- [ ] `andamio user login` opens browser and completes auth flow
- [ ] `andamio user status` shows current authentication state
- [ ] `andamio user logout` clears stored JWT
- [ ] Owner commands work with valid user JWT
- [ ] Graceful handling when JWT expires
- [ ] Works on macOS, Linux, Windows

### Non-Functional Requirements

- [ ] Auth flow completes in < 30 seconds (excluding user signing time)
- [ ] Local server binds only to localhost (security)
- [ ] State parameter prevents CSRF attacks
- [ ] Clear error messages for all failure modes

### Quality Gates

- [ ] Manual testing of full auth flow on macOS
- [ ] Test timeout handling (user closes browser)
- [ ] Test with different wallets (Nami, Eternl, Lace)
- [ ] Test JWT expiry handling

## Success Metrics

- CLI users can authenticate and edit their courses
- No security vulnerabilities in auth flow
- Works across major platforms and wallets

## Dependencies & Prerequisites

- Andamio app deployed with `/auth/cli` page
- dbapi authentication endpoints (already exist)
- Users have Andamio Access Tokens in their wallets

## Risk Analysis & Mitigation

| Risk | Impact | Mitigation |
|------|--------|------------|
| Browser popup blocked | Auth fails | Show clear instructions to allow popups |
| Localhost port conflict | Can't start server | Use port 0 (OS assigns free port) |
| User closes browser early | Timeout | Clear timeout message, easy retry |
| JWT stolen from config | Account compromise | File permissions, same risk as other CLIs |
| CSRF attack | Session hijacking | State parameter validation |

## Future Considerations

1. **Secure token storage** - Consider system keychain (`go-keyring`) for production
2. **Device flow fallback** - For SSH sessions where browser isn't available locally
3. **Token refresh** - Auto-refresh JWT before expiry
4. **Multi-account support** - Allow switching between different Access Token identities

## Sources & References

### Internal References

- Existing auth: `/Users/james/projects/01-projects/andamio-platform/andamio-app-v2/src/lib/andamio-auth.ts`
- Auth context: `/Users/james/projects/01-projects/andamio-platform/andamio-app-v2/src/contexts/andamio-auth-context.tsx`
- CLI config: `/Users/james/projects/01-projects/andamio-cli/internal/config/config.go`
- CLI client: `/Users/james/projects/01-projects/andamio-cli/internal/client/client.go`

### External References

- [CIP-30 Cardano dApp-Wallet Web Bridge](https://cips.cardano.org/cip/CIP-30)
- [cli/oauth - GitHub CLI auth library](https://github.com/cli/oauth)
- [int128/oauth2cli - Local callback server pattern](https://github.com/int128/oauth2cli)
- [pkg/browser - Cross-platform browser opening](https://github.com/pkg/browser)

### Related Work

- GitHub CLI auth flow (`gh auth login`)
- Google Cloud CLI auth (`gcloud auth login`)
- Heroku CLI auth (`heroku login`)
