# System Maintenance Mode Specification

## 1. Overview

System Maintenance Mode allows administrators to temporarily disable general gameplay and user mutations during server upgrades, batch migrations, or emergency maintenance, while keeping health checks, OpenAPI metadata, and admin control channels active.

---

## 2. Architecture & Data Model

### 2.1 Database Schema (`system_maintenance`)
The maintenance state is stored in the `system_maintenance` table with a singleton row `id = 'global'`:

```sql
CREATE TABLE IF NOT EXISTS system_maintenance (
    id VARCHAR(32) PRIMARY KEY,
    enabled BOOLEAN NOT NULL DEFAULT FALSE,
    message VARCHAR(500) NOT NULL DEFAULT '',
    estimated_end_time DATETIME NULL,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
);
```

### 2.2 Invariants
- `enabled`: Boolean indicating whether maintenance mode is active.
- `message`: Optional announcement message explaining the maintenance reason (max 500 characters).
- `estimated_end_time`: Optional estimated completion timestamp.
- Default state on new installations: `enabled = false`, `message = 'System is operating normally.'`.

---

## 3. Middleware Behavior

The HTTP maintenance middleware sits at the edge of the router pipeline:

```text
Request
  -> securityHeadersMiddleware
    -> corsMiddleware
      -> rateLimitMiddleware
        -> maintenanceMiddleware
          -> Endpoint Handler
```

### 3.1 Whitelisted Routes & Admin Bypass
When maintenance mode is active (`IsEnabled() == true`):
1. **Public system routes** are whitelisted and bypass 503:
   - `GET /health`
   - `GET /openapi.json`
   - `GET /maintenance`
2. **Administrator requests** bypass 503 if authenticated via:
   - Header `X-Admin-Key: <key>`
   - Header `Authorization: Bearer <key>`
3. **All other player requests** are rejected with HTTP `503 Service Unavailable`:
   ```json
   {
     "error": "Emergency maintenance in progress.",
     "code": "MAINTENANCE_MODE",
     "estimated_end_time": "2026-09-02T00:00:00Z"
   }
   ```
   and header `Retry-After: 300`.

---

## 4. API Endpoints

| Method | Endpoint | Auth | Description |
|---|---|---|---|
| `GET` | `/maintenance` | Public | Returns current maintenance mode status, message, and estimated end time. |
| `POST` | `/admin/maintenance` | Admin (`X-Admin-Key`) | Enables or configures system maintenance mode. |
| `PUT` | `/admin/maintenance` | Admin (`X-Admin-Key`) | Updates system maintenance mode configuration. |
