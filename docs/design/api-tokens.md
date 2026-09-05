# Personal Access Tokens (API Keys) Specification

## 1. Overview

In Party2, player accounts can generate, inspect, and revoke Personal Access Tokens (PAT / API Keys) for external programmatic access (e.g., AI agents, CLI scripts, MCP servers).

Personal Access Tokens provide an alternative to interactive browser session tokens, allowing automated clients to authenticate securely without requiring interactive login credentials to be stored or exposed.

---

## 2. Token Format & Cryptographic Security Model

### 2.1 Token Format
- Plaintext API tokens always use the `p2_sk_` prefix followed by 64 hexadecimal characters (32 bytes of cryptographically secure random entropy from `crypto/rand`):
  ```text
  p2_sk_0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef
  ```
- The `p2_sk_` prefix allows instant disambiguation between interactive Valkey session tokens (UUIDs) and API tokens during authentication routing.

### 2.2 Plaintext vs Hash Storage Invariant
- **Plaintext Secret**: Returned **exactly once** in the HTTP response of token creation (`POST /player/tokens`). The system never stores the plaintext token.
- **Persistent Hash**: The SHA-256 digest (64 hex characters) of the plaintext token is computed and stored in the `player_api_tokens` table in MariaDB.
- **Zero Hash Leakage**: Token listing (`GET /player/tokens`) and revocation responses never expose either the plaintext secret or the underlying hash.

---

## 3. Endpoints & Operations

### 3.1 Create Token (`POST /player/tokens`)
- **Authentication**: Requires authenticated player (`Bearer <token>`).
- **Input**:
  - `name`: string (1-64 characters, required, whitespace-trimmed).
  - `expires_at`: optional RFC 3339 timestamp (must be in the future if supplied).
- **Behavior**:
  - Generates a new 32-byte secret prefixed with `p2_sk_`.
  - Computes SHA-256 hash.
  - Inserts into `player_api_tokens`.
- **Response**: `201 Created` with token metadata and the plaintext secret token.

### 3.2 List Tokens (`GET /player/tokens`)
- **Authentication**: Requires authenticated player (`Bearer <token>`).
- **Behavior**: Retrieves all active tokens owned by the player, ordered by creation time descending.
- **Response**: `200 OK` containing a list of token metadata (`id`, `name`, `created_at`, `last_used_at`, `expires_at`).

### 3.3 Revoke Token (`DELETE /player/tokens/{id}`)
- **Authentication**: Requires authenticated player (`Bearer <token>`).
- **Authorization**:
  - If the token does not exist: returns `404 Not Found`.
  - If the token belongs to another player: returns `403 Forbidden`.
- **Behavior**: Permanently deletes the token record from `player_api_tokens`.
- **Response**: `200 OK` with `{"revoked": true, "token_id": "<id>"}`.

---

## 4. Dual Authentication Routing

Authentication via `Authorization: Bearer <token>` in `internal/player/application.go`:
```text
                  Incoming Authorization: Bearer <token>
                                    |
                                    v
                    Token has "p2_sk_" prefix?
                       /                  \
                     YES                  NO
                     /                      \
                    v                        v
            Compute SHA-256 Digest        Look up Session
                    |                     in Valkey Master
                    v                        |
            Query MariaDB                    v
           player_api_tokens              Active Session?
                    |                        |
                    v                       YES -> Return Player
             Token Found &                  NO  -> ErrInvalidSession
             Not Expired?
              /          \
            YES          NO -> ErrInvalidSession
            /
           v
    Update last_used_at (async/best-effort)
           |
           v
      Return Player
```

- Any existing protected endpoint across all game features works identically with either a session token or an API token without changes to downstream handlers.

---

## 5. Lifecycle & Cascade Deletion

- **Account Deletion**: When a player account is deleted (`DELETE /players/me` or `DELETE /players/{id}`), all associated API tokens are deleted:
  - Enforced via relational database foreign key `ON DELETE CASCADE` on `player_api_tokens(player_id)`.
  - Enforced defensively in application transaction logic (`playerRepo.Delete` and `application.DeleteAccount`).
