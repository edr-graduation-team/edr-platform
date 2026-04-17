# Installation Token Fix and Optional Bypass

## Root cause

- **RegisterAgent** is implemented in `pkg/server/server.go` and delegates to `internal/service/agent_service.go` → `Register()`.
- Token validation is done in `Register()`:
  - `tokenRepo.GetByValue(ctx, req.InstallationToken)` — **exact SQL**:  
    `SELECT id, token_value, agent_id, used, used_at, created_at, expires_at FROM installation_tokens WHERE token_value = $1`
  - If the row is missing or schema does not match (e.g. column `value` instead of `token_value`), the query fails or returns no row → `ErrNotFound` → mapped to **"invalid or expired installation token"**.
- Validity is then checked with `token.IsValid()` (token must be `used = false` and `expires_at > now()`).

The Go code expects the schema from migrations **001** (agents) and **005** (installation_tokens). Any other schema (e.g. `value`, `is_active`, `used_count`) will cause validation to fail.

---

## Option A: Fix the database (recommended)

Run the script that recreates the tables to match the Go code and seeds the test token:

```bash
# From repo root
docker exec -i edr_server-postgres-1 psql -U sigma -d sigma < connection-manager/scripts/reset_installation_tokens_schema.sql
```

Or from inside the container:

```bash
docker exec -it edr_server-postgres-1 psql -U sigma -d sigma -f /path/to/reset_installation_tokens_schema.sql
```

The script:

- Drops `cleanup_expired_installation_tokens` and `installation_tokens`.
- Creates `agents` (if not exists) per migration 001.
- Creates `installation_tokens` with columns: `id` (UUID), `token_value`, `agent_id`, `used`, `used_at`, `created_at`, `expires_at` (matching migration 005 and `GetByValue` Scan order).
- Inserts `EDR-SUPER-SECRET-TOKEN-2026` with `expires_at = 2027-01-01` and `used = false`.

After that, restart the Connection Manager and run the agent; registration should succeed.

---

## Option B: Temporary hardcode bypass (testing only)

Use this only to unblock testing the Event stream when you cannot fix the DB yet. **Do not use in production.**

**File:** `internal/service/agent_service.go`  
**Function:** `Register` (starts around line 105).

**Change:** Right after the “1. Validate installation token” block, accept the known test token and skip DB token lookup and `MarkUsed`.

Replace this block (lines 107–114):

```go
	// 1. Validate installation token
	token, err := s.tokenRepo.GetByValue(ctx, req.InstallationToken)
	if err != nil {
		return nil, ErrInvalidToken
	}
	if !token.IsValid() {
		return nil, ErrExpiredToken
	}
```

with:

```go
	// 1. Validate installation token (temporary bypass for test token when DB schema is wrong)
	const testToken = "EDR-SUPER-SECRET-TOKEN-2026"
	var token *models.InstallationToken
	if req.InstallationToken == testToken {
		token = &models.InstallationToken{
			ID:         uuid.Nil,
			TokenValue: testToken,
			Used:       false,
			ExpiresAt:  time.Now().Add(365 * 24 * time.Hour),
		}
	} else {
		var err error
		token, err = s.tokenRepo.GetByValue(ctx, req.InstallationToken)
		if err != nil {
			return nil, ErrInvalidToken
		}
		if !token.IsValid() {
			return nil, ErrExpiredToken
		}
	}
```

Then guard **Mark token as used** so we do not call the DB with a nil token ID. Replace (lines 143–146):

```go
	// 4. Mark token as used
	if err := s.tokenRepo.MarkUsed(ctx, token.ID, agentID); err != nil {
		s.logger.WithError(err).Error("Failed to mark token as used")
	}
```

with:

```go
	// 4. Mark token as used (skip when using test token bypass)
	if token.ID != uuid.Nil {
		if err := s.tokenRepo.MarkUsed(ctx, token.ID, agentID); err != nil {
			s.logger.WithError(err).Error("Failed to mark token as used")
		}
	}
```

**Summary of edits:**

| Location | Action |
|----------|--------|
| `internal/service/agent_service.go` ~107–114 | Replace token lookup block with conditional: if token == test token, use a synthetic `*models.InstallationToken`; else keep existing GetByValue + IsValid. |
| `internal/service/agent_service.go` ~143–146 | Wrap `MarkUsed` in `if token.ID != uuid.Nil { ... }`. |

After this, any request with `InstallationToken == "EDR-SUPER-SECRET-TOKEN-2026"` will pass validation and you can test the Event stream. Revert these changes once the DB is fixed and use Option A.
