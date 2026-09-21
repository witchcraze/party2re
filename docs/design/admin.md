# Administration Specification

## 1. Overview

Administrative operations provide privileged server and account management tools for system operators. In legacy Party2 (`party2/admin.cgi`), administrative capabilities were concentrated into user inspection, user deletion/blacklisting, and player rescue.

In Party2 Reconstruction, administrative operations are secured via authoritative HTTP endpoints guarded by master administrative API keys, providing:
- **System Maintenance Mode**: Emergency maintenance toggle and schedule broadcast (`/admin/maintenance`).
- **Player Account Management**: Multi-criteria player listing and account soft-banning (`/admin/players`, `/admin/players/{id}/ban`).
- **Rescue Operations**: Emergency unstuck action clearing (`/rescue`).

---

## 2. Authentication & Access Control

### 2.1 Credentials
All `/admin/*` routes enforce strict credential verification:
1. **Header Inspection**:
   - `X-Admin-Key: <key>`
   - `Authorization: Bearer <key>`
2. **Constant-Time Comparison**:
   - Compares provided credentials against `h.adminAPIKey` using `crypto/subtle.ConstantTimeCompare` to defend against timing side-channel attacks.
3. **Rejection Semantics**:
   - `401 Unauthorized`: Credentials missing from the request.
   - `403 Forbidden`: Credentials present but invalid, or server started without an administrative key configured (`ADMIN_API_KEY` unset).

---

## 3. Player Administration

### 3.1 Player Listing (`GET /admin/players`)
- **Query Parameter**: `sort` (optional)
  - `addr` (default): Sorted by `last_ip ASC, id ASC`. Reproduces legacy Party2 default sorting by user IP address.
  - `name`: Sorted by `username ASC`.
  - `updated_at` (or `ldate`): Sorted by `updated_at DESC, id ASC`. Displays most recently active players first.
- **Response**: List of players containing `id`, `username`, `created_at`, `updated_at`, `last_ip`, `banned_at`, and `is_banned`.

### 3.2 Account Soft-Ban (`POST /admin/players/{id}/ban`)
- **Semantics**: Sets `banned_at = CURRENT_TIMESTAMP(6)` in MariaDB `players` table.
- **Session Revocation**: Synchronously purges all active player sessions from Valkey Master via `sessions.DeleteByPlayerID(ctx, playerID)`.
- **Token Invalidation**: Deletes active Personal Access Tokens (PATs) for the player.
- **Enforcement**:
  - `POST /sessions` (Login): Returns HTTP `403 Forbidden` (`coreplayer.ErrPlayerBanned`).
  - Authenticated character and gameplay requests: Returns HTTP `403 Forbidden` (`coreplayer.ErrPlayerBanned`).

---

## 4. API Endpoints

| Method | Endpoint | Auth | Description |
|---|---|---|---|
| `POST` | `/admin/maintenance` | Admin (`X-Admin-Key`) | Enables or configures system maintenance mode. |
| `PUT` | `/admin/maintenance` | Admin (`X-Admin-Key`) | Updates system maintenance mode configuration. |
| `GET` | `/admin/players` | Admin (`X-Admin-Key`) | Returns registered players list with optional sorting (`sort=addr\|name\|updated_at`). |
| `POST` | `/admin/players/{id}/ban` | Admin (`X-Admin-Key`) | Soft-bans player account and revokes active sessions. |

---

## 5. Scope & Boundary Constraints

- **Physical Deletion**: Hard physical database deletions are prohibited for administrative banning. Soft-banning preserves database integrity, foreign keys, and audit logs while immediately blocking access.
- **Inventory/Gold Manipulation**: Arbitrary gold or item cheating/modifications by admin endpoints are explicitly excluded from scope (YAGNI). Economy invariants remain strictly enforced by core domain aggregates.
