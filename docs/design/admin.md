# Administration Specification

## 1. Overview

Administrative operations provide privileged server and account management tools for system operators. In legacy Party2 (`party2/admin.cgi`), administrative capabilities were concentrated into user inspection, user deletion/blacklisting, and player rescue.

In Party2 Reconstruction, administrative operations are secured via authoritative HTTP endpoints guarded by master administrative API keys, providing:
- **System Maintenance Mode**: Emergency maintenance toggle and schedule broadcast (`/admin/maintenance`).
- **Player Account Management**: Multi-criteria player listing and account soft-banning (`/admin/players`, `/admin/players/{id}/ban`).
- **Rescue Operations**: Emergency unstuck action clearing (`/admin/characters/{id}/unstuck`), rejected while feature activity remains like player rescue. Player rescue uses the separate Gateway `rescue_request` command under the [Rescue contract](rescue-and-helper.md#emergency-rescue).

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
   - `403 Forbidden`: Credentials present but invalid, or server started without an administrative key configured (`PARTY2_ADMIN_API_KEY`, with `ADMIN_API_KEY` as the compatibility fallback).

---

## 3. Player Administration

### 3.1 Player Listing (`GET /admin/players`)
- **Query Parameter**: `sort` (optional)
  - `addr` (default): Sorted by `last_ip ASC, id ASC`. Reproduces legacy Party2 default sorting by user IP address.
  - `name`: Sorted by `username ASC`.
  - `updated_at` (or `ldate`): Sorted by `updated_at DESC, id ASC`. Displays most recently active players first.
- **Response**: List of players containing `id`, `username`, `created_at`, `updated_at`, `last_ip`, `banned_at`, and `is_banned`.

### 3.2 Account Soft-Ban (`POST /admin/players/{id}/ban`)
- **Boundary**: Account banning replaces legacy physical deletion (#200); it does not reject new accounts from the same IP. IP registration denial is a separate, optional policy described below.
- **Semantics**: Sets `banned_at = CURRENT_TIMESTAMP(6)` in MariaDB `players` table.
- **Session Revocation**: Synchronously purges all active player sessions from Valkey Master via `sessions.DeleteByPlayerID(ctx, playerID)`.
- **Token Invalidation**: Deletes active Personal Access Tokens (PATs) for the player.
- **Guild Cleanup**: Automatically removes all characters owned by the banned player from their respective guilds, transferring leadership or disbanding as necessary.
- **Enforcement**:
  - `POST /sessions` (Login): Returns HTTP `403 Forbidden` (`coreplayer.ErrPlayerBanned`).
  - Authenticated character and gameplay requests: Returns HTTP `403 Forbidden` (`coreplayer.ErrPlayerBanned`).

### 3.3 Optional IP Registration Denial (Planned)

The approved decision in [#1107](https://github.com/witchcraze/party2re/issues/1107)
adopts IP registration denial independently of account banning. Management,
registration enforcement and the optional BAN integration are deferred together
to low-priority [#1817](https://github.com/witchcraze/party2re/issues/1817);
none is currently implemented.

- **Administration**: Administrators can add, list and release individual IPv4/IPv6 addresses. BAN can explicitly include denial of the target account's stored `last_ip`; omission means account BAN only. An unknown or invalid stored IP rejects the combined request before changes, rather than silently falling back to BAN alone. The two SQL changes commit or roll back together.
- **Scope and independence**: Denial applies only to new account registration (`POST /players`), returning 403 without creating an account. It does not block existing accounts' login or gameplay. Releasing an IP does not unban an account, and account BAN alone never adds an IP denial.
- **Identity**: Match normalized single IP addresses, treating IPv4-mapped IPv6 as IPv4, using the existing [client-IP trust boundary](../architecture/interfaces.md#application-api-boundary). Do not match hostnames, reverse DNS, CIDR or wildcard patterns. Missing/invalid client IP rejects registration with 400; a missing hostname does not matter when the IP is valid. Denylist lookup errors return 503 rather than admitting registration.
- **Retention**: Active entries persist in MariaDB until explicit administrative release, which removes the denial record. No automatic expiry, denial history or extra IP logging is introduced; existing `last_ip` retention is unchanged.
- **Operational limits**: Every new registrant sharing a denied NAT/CGNAT address is affected; the BAN option defaults off to avoid implicitly extending an account sanction to other people. Changing IP can bypass this control, so it is not proof of identity or guaranteed re-registration prevention.

Legacy evidence: `party2/admin.cgi:92,100–116` optionally appends the deleted
user's stored host; `new_entry.cgi:66–70,240–245` checks it before registration.
`lib/system.cgi:731–740,1016–1021` uses `REMOTE_HOST`, falling back to
`REMOTE_ADDR`, and appends without an expiry policy. The separate same-IP
registration rejection is disabled (`new_entry.cgi:140,251–265`). The adopted
policy preserves optional registration denial while replacing host matching with
trusted IP matching and adding independent management/release. Verification of
spoofed headers, proxy boundaries, shared addresses, unknown hosts, normal
registration and independent release belongs to the implementation issue.

---

Transport schemas: [OpenAPI](../api/openapi.json). Storage key lifetimes: [Valkey keyspace](../architecture/valkey-keyspace.md).

## 5. Scope & Boundary Constraints

- **Physical Deletion**: Hard physical database deletions are prohibited for administrative banning. Soft-banning preserves database integrity, foreign keys, and audit logs while immediately blocking access.
- **Inventory/Gold Manipulation**: Arbitrary gold or item cheating/modifications by admin endpoints are explicitly excluded from scope (YAGNI). Economy invariants remain strictly enforced by core domain aggregates.

## System maintenance mode

Maintenance state is the `system_maintenance` singleton `global`, projected into
Valkey. The schema is owned by [migrations](../../migrations/). It records enabled
state, a message (up to 500 characters) and optional estimated end time; a new
installation is disabled with message `System is operating normally.`

After security headers, CORS and rate limiting, maintenance middleware rejects
ordinary requests with 503, code `MAINTENANCE_MODE`, estimated end time and
`Retry-After: 300`. Health, OpenAPI and public maintenance status remain readable;
valid `X-Admin-Key` or Admin Bearer credentials bypass the gate. See
[admin operations](../api/paths/admin.json) and [system operations](../api/paths/system.json)
for the transport contract. This mode does not replace authentication/ownership checks.
