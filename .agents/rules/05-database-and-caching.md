---
name: Database and Caching Strategy
description: Guidelines for database transaction boundaries, concurrency control, and appropriate usage of Redis/Valkey.
---

# Database and Caching Strategy

## 1. Concurrency Control (Unit of Work & Ambient Transaction Propagation)
- **Atomicity:** All read-modify-write operations that alter critical state (e.g., gold, inventory, game progression) MUST occur within a single database transaction.
- **Ambient Transaction Propagation:**
  - Repositories MUST use `database.ExecutorFromContext(ctx, r.db)` for all queries and executions so they automatically participate in any ambient transaction started by an outer caller or application service.
  - Repository methods that perform multi-statement transactional operations MUST wrap their logic inside `database.RunInTx(ctx, r.db, func(txCtx context.Context) error { ... })`. If an outer transaction is already present on `ctx`, `RunInTx` reuses it without initiating a nested sub-transaction or committing prematurely.
- **Deterministic Lock Acquisition Ordering (Deadlock Prevention):**
  - When acquiring pessimistic locks (`SELECT ... FOR UPDATE`) across multiple domain tables or rows in a single transaction, locks MUST be acquired in a strictly deterministic hierarchy:
    1. `players`
    2. `characters` (when locking multiple characters, sort in ascending order by ID: `id1 < id2`)
    3. `inventory_items` / `equipment_slots`
    4. `character_jobs` / `character_job_masteries`
    5. `character_depots` / `depot_items`
    6. `bank_accounts` / `bank_transfers`
    7. `guilds` (sorted ascending by ID: `id1 < id2`) / `guild_members`
    8. Feature tables (e.g., `auction_listings`, `farm_plots`, `casino_accounts`, `gvg_standings`, `character_boss_records`, `challenge_records`, etc.)
- **BANNED ANTI-PATTERNS (Lost Updates & Deadlocks):**
  - **Unprotected Read-Modify-Write:** Do NOT read structs (e.g., Character) outside a transaction, mutate them in Go memory, and then blindly save them back. This will erase concurrent changes (like Adventure rewards).
  - **Direct `BeginTx` in Repositories:** Do NOT call `r.db.BeginTx` directly in repositories. Always use `RunInTx(ctx, r.db, ...)` and `ExecutorFromContext(ctx, r.db)`.
  - **Non-deterministic Row Locking:** Never lock rows in random, hash-map, or caller-dependent order; always sort IDs ascending when locking multiple rows of the same table.
  - **Collection Wipe-and-Insert:** Do NOT implement inventory/collection updates by executing `DELETE FROM ...` followed by re-inserting all items from an unprotected in-memory slice. Use targeted `UPSERT` / `ON DUPLICATE KEY UPDATE` or atomic `DELETE` of specific rows.
- **Concurrency Protection:** You MUST use pessimistic locking (`SELECT ... FOR UPDATE`) during the read phase of the transaction when modifying complex state that cannot be done with simple SQL statements.
- **Alternative Safe Mechanisms:** Depending on the context, other concurrency control methods may be preferable to `FOR UPDATE`, such as:
  - Atomic `UPDATE` queries (e.g., `UPDATE ... SET money = money - X WHERE money >= X`).
  - Database-side arithmetic and `UPSERT` / `ON DUPLICATE KEY UPDATE` strategies (e.g. Casino coins).

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
