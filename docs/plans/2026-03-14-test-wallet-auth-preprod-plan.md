---
title: Wallet Authentication Testing Plan (Preprod)
type: test
status: active
date: 2026-03-14
origin: docs/plans/2026-03-13-feat-browser-wallet-authentication-plan.md
---

# Wallet Authentication Testing Plan (Preprod)

Testing plan for the browser-based wallet authentication flow against `https://preprod.api.andamio.io`.

## Prerequisites

- [ ] CLI built locally: `go build -o andamio ./cmd/andamio`
- [ ] Config points to preprod: `./andamio config show` → `base_url: https://preprod.api.andamio.io`
- [ ] API key configured: `./andamio auth login --api-key <key>`
- [ ] A Cardano wallet with an Andamio Access Token (Nami, Eternl, or Lace installed in browser)
- [ ] App auth page deployed at `https://preprod.app.andamio.io/auth/cli`

## Test Cases

### 1. Happy Path — Full Login Flow

```bash
./andamio user login
```

**Expected:**
- [ ] Terminal prints "Opening browser for authentication..."
- [ ] Browser opens to `https://preprod.app.andamio.io/auth/cli?redirect_uri=http://127.0.0.1:{port}/callback&state={state}`
- [ ] Auth page renders with wallet connect prompt
- [ ] After signing, browser shows "Authentication Successful" with alias
- [ ] Terminal prints "Successfully authenticated as: {alias}"
- [ ] Terminal prints session expiration time

**Verify state persisted:**
```bash
./andamio user status
```
- [ ] Shows user alias
- [ ] Shows valid session with remaining time
- [ ] Shows API key status

**Verify config file:**
```bash
cat ~/.andamio/config.json | python3 -m json.tool
```
- [ ] `user_jwt` field populated
- [ ] `jwt_expires_at` field populated
- [ ] `user_alias` field populated
- [ ] `user_id` field populated

### 2. Authenticated API Calls

After login, verify JWT is sent with requests:

```bash
./andamio user me
./andamio user usage
```

- [ ] Both return user-specific data (not 401/403)

### 3. Logout Flow

```bash
./andamio user logout
```

- [ ] Prints "Logged out successfully (was: {alias})"
- [ ] `./andamio user status` shows "User: not authenticated"
- [ ] `~/.andamio/config.json` no longer has `user_jwt`, `user_alias`, `user_id`
- [ ] API key remains intact after logout

### 4. Already Authenticated Guard

```bash
./andamio user login   # first login
./andamio user login   # second login attempt
```

- [ ] Second attempt prints "Already authenticated as: {alias}"
- [ ] Does not open browser again
- [ ] Suggests running `andamio user logout` first

### 5. User Cancels in Browser

```bash
./andamio user login
# In browser: close the tab or click Cancel
```

- [ ] CLI waits (up to 5 minutes) then times out
- [ ] Timeout message: "authentication timed out after 5 minutes"
- [ ] No JWT stored in config

### 6. Browser Error Callback

Simulate by manually navigating to the callback URL with an error:

```bash
./andamio user login
# Note the port from the terminal output, then in another terminal:
curl "http://127.0.0.1:{port}/callback?error=auth_failed&state={state}"
```

- [ ] CLI prints "authentication failed: auth_failed"
- [ ] No JWT stored in config

### 7. CSRF Protection (Invalid State)

```bash
./andamio user login
# In another terminal, send callback with wrong state:
curl "http://127.0.0.1:{port}/callback?jwt=fake&state=wrong_state"
```

- [ ] CLI rejects callback with "invalid state parameter"
- [ ] No JWT stored in config

### 8. No Browser Available (Fallback Instructions)

Test in an environment where browser can't open (e.g., SSH session):

```bash
./andamio user login
```

- [ ] Prints the auth URL so user can copy/paste to a browser manually
- [ ] Flow still works if user manually opens the URL

### 9. Wallet Compatibility

Repeat Test Case 1 with each wallet:

- [ ] **Nami** — login completes successfully
- [ ] **Eternl** — login completes successfully
- [ ] **Lace** — login completes successfully

### 10. Expired JWT Behavior

After login, manually set `jwt_expires_at` to a past date in `~/.andamio/config.json`:

```bash
# Edit config to set expired time
./andamio user status
```

- [ ] Status shows "Session: EXPIRED"
- [ ] Suggests re-authentication command
- [ ] API calls with expired JWT return appropriate error

### 11. Output Format Flag

```bash
./andamio user status -o json
./andamio user me -o json
./andamio user me -o csv
./andamio user me -o markdown
```

- [ ] Each format renders correctly

### 12. Clean State — No Config File

```bash
mv ~/.andamio/config.json ~/.andamio/config.json.bak
./andamio user login
```

- [ ] Uses default preprod base URL
- [ ] Auth flow works, creates config file
- [ ] Restore: `mv ~/.andamio/config.json.bak ~/.andamio/config.json`

## Environment Matrix

| Environment | App URL | API URL | Status |
|-------------|---------|---------|--------|
| Preprod | `https://preprod.app.andamio.io` | `https://preprod.api.andamio.io` | Test first |
| Mainnet | `https://mainnet.app.andamio.io` | `https://mainnet.api.andamio.io` | Test after preprod passes |

## Known Limitations

- No automated tests yet — all manual verification
- JWT validation on startup not implemented (user must check `user status` manually)
- No device flow fallback for headless/SSH environments
