---
name: Database and Caching Strategy
description: Guidelines for database transaction boundaries, concurrency control, and appropriate usage of Redis/Valkey.
---

# Database and Caching Strategy

## 1. Concurrency Control (Unit of Work)
- **Atomicity:** All read-modify-write operations that alter critical state (e.g., gold, inventory, game progression) MUST occur within a single database transaction.
- **Concurrency Protection:** Use pessimistic locking (`SELECT ... FOR UPDATE`) when concurrent modifications can cause lost updates or invariant violations, **unless** an equally safe concurrency-control mechanism is deliberately used.
- **Alternative Safe Mechanisms:** Depending on the context, other concurrency control methods may be preferable to `FOR UPDATE`, such as:
  - Atomic `UPDATE` queries (e.g., `UPDATE ... SET money = money - X WHERE money >= X`).
  - Optimistic locking (version columns).
  - Database-side arithmetic and `UPSERT` / `ON DUPLICATE KEY UPDATE` strategies.
  - Unique constraints to prevent duplicate insertions.
- **Closure Pattern:** Prefer executing domain logic inside a transactional closure (`RunInTx(ctx, func(tx) error)`) to ensure that reads and writes are safely enclosed in the same database transaction boundary.

## 2. Database Migrations (Current Script Workflow)
Until a standard migration tool is formally adopted, all SQL migrations MUST adhere strictly to the current `scripts/migrate.sh` logic:
- **No Annotations:** Do NOT use `sql-migrate` annotations like `-- +migrate Up` or `-- +migrate Down`. The script pipes the entire file directly to MariaDB. Doing so will execute both blocks sequentially, potentially destroying tables immediately after creation.
- **Manual Tracking:** You MUST manually append `INSERT IGNORE INTO schema_migrations (version) VALUES ('XXX_name');` at the very end of your `.sql` file to prevent infinite re-execution on startup.

## 3. Caching and Volatile Data (Valkey)
- **SQL First:** The relational database (SQL) is the primary source of truth for critical persistent player state. Do not use Valkey as the primary persistence for critical player data.
- **Concrete Requirements Only:** Do not introduce Valkey without a concrete feature requirement or measured performance benefit.
- **Feature Use Cases:** Introduce Valkey when building features that inherently demand it, such as:
  - Scheduled Actions / Matchmaking queues
  - Session tokens / Rate limiting
  - Real-time transient state (Presence, World Boss HP)
- **Performance Caching:** Do not pre-emptively cache static/master data (Items, Jobs) in Valkey. Introduce read-caching only if empirical measurement proves SQL is a bottleneck.
