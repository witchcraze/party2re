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
  - When acquiring pessimistic locks (`SELECT ... FOR UPDATE`) across multiple domain tables or rows in a single transaction, locks MUST be acquired in a strictly deterministic numeric rank order (Rank 0 -> Rank 8). Mechanically verified via Go AST linter (`internal/database/lock_hierarchy_lint_test.go`, `make lock-lint`):
    | Rank | Category | Target Tables & Methods | Concurrency Role |
    | :--- | :--- | :--- | :--- |
    | **Rank 0** | **Shared Peer Entities** | `fleamarket_listings`, `store_sales`, `contest_rounds`, `takarakuji_rounds`<br>(listing, sale, round, or room `*ForUpdate` methods) | Serializes concurrent contenders on shared state upfront before touching player/character assets. Valkey lobby locks are separate coordination mechanisms. |
    | **Rank 1** | **Player Account** | `players` (`playerRepo.*ForUpdate`, `players.*ForUpdate`) | Account-level mutations and security credentials. |
    | **Rank 2** | **Character Primary Entity** | `characters` (`charRepo.FindByIDForUpdate`, `characterRepo.FindByIDForUpdate`, `characters.FindByIDForUpdate`) | Primary game actor; multiple characters MUST be locked in ascending ID order (`id1 < id2`). |
    | **Rank 3** | **Inventory & Equipment** | `inventory_items`, `equipment_slots`<br>(`invRepo.FindByCharacterIDForUpdate`, `inventories.FindByCharacterIDForUpdate`) | Dependent character items; must NEVER be locked before Character. |
    | **Rank 4** | **Job Progression** | `character_jobs`, `character_job_masteries`<br>(`jobRepo.*ForUpdate`) | Job changes and skill loadouts. |
    | **Rank 5** | **Depot Storage** | `character_depots`, `depot_items`<br>(`depotRepo.FindByCharacterIDForUpdate`, `depots.FindByCharacterIDForUpdate`) | Long-term bank/item storage. |
    | **Rank 6** | **Reserved Bank Category** | `bankRepo.*ForUpdate` classification in the lock linter | MUST NOT infer separate account/transfer tables; current bank savings are `characters.deposit` and use Rank 2. |
    | **Rank 7** | **Guilds** | `guilds`, `guild_members`<br>(`guildRepo.*ForUpdate`) | Guild management; multiple guilds MUST be locked in ascending ID order (`id1 < id2`). |
    | **Rank 8** | **Secondary Feature Records** | `character_achievements`, `plantation_plots`, `blackmarket_character_points`, `character_monsters`<br>(achievement, points, monster, or plantation `*ForUpdate` methods) | Secondary domain features and progression counters. |
- **BANNED ANTI-PATTERNS (Lost Updates & Deadlocks):**
  - **Unprotected Read-Modify-Write:** Do NOT read structs (e.g., Character) outside a transaction, mutate them in Go memory, and then blindly save them back. This will erase concurrent changes (like Adventure rewards).
  - **Direct `BeginTx` in Repositories:** Do NOT call `r.db.BeginTx` directly in repositories. Always use `RunInTx(ctx, r.db, ...)` and `ExecutorFromContext(ctx, r.db)`. (Note: This is automatically validated by the Go AST linter in `internal/database/tx_lint_test.go` on every `make check`).
  - **Non-deterministic Row Locking:** Never lock rows in random, hash-map, or caller-dependent order; always sort IDs ascending when locking multiple rows of the same table.
  - **Sub-Resource Pre-Locking (Inverted Lock Hierarchy):** NEVER lock sub-resources (e.g. `inventory_items`) before locking parent `characters`. Enforced mechanically by `internal/database/lock_hierarchy_lint_test.go`; see [`docs/architecture/cross-domain-primitives.md`](../../docs/architecture/cross-domain-primitives.md) §4.3.
  - **Collection Wipe-and-Insert:** Do NOT implement inventory/collection updates by executing `DELETE FROM ...` followed by re-inserting all items from an unprotected in-memory slice. Use targeted `UPSERT` / `ON DUPLICATE KEY UPDATE` or atomic `DELETE` of specific rows.
  - **Silent Error Suppression on Repositories/Stores:** NEVER discard errors returned from database repositories, persistent stores, or state mutations using blank identifiers (`_ =`, `_, _ =`, `val, _ :=`) or unassigned expressions. All errors must be propagated to the caller or handled within transactional rollback boundaries. If a call is strictly best-effort or compensatory (e.g. defer rollback, post-commit cache eviction), annotate with `//lint:ignore error-swallow <reason>`. Enforced mechanically by `internal/architecture/error_swallow_lint_test.go`; see [`docs/development/ast-linters.md`](../../docs/development/ast-linters.md).
- **Concurrency Protection:** You MUST use pessimistic locking (`SELECT ... FOR UPDATE`) during the read phase of the transaction when modifying complex state that cannot be done with simple SQL statements.
- **Alternative Safe Mechanisms:** Depending on the context, other concurrency control methods may be preferable to `FOR UPDATE`, such as:
  - Atomic `UPDATE` queries (e.g., `UPDATE ... SET money = money - X WHERE money >= X`).
  - Database-side arithmetic and `UPSERT` / `ON DUPLICATE KEY UPDATE` strategies (e.g. Casino coins).

## 2. Database Migrations (Current Script Workflow)
Until a standard migration tool is formally adopted, all SQL migrations MUST adhere strictly to the current `scripts/migrate.sh` logic:
- **No Annotations:** Do NOT use `sql-migrate` annotations like `-- +migrate Up` or `-- +migrate Down`. The script pipes the entire file directly to MariaDB. Doing so will execute both blocks sequentially, potentially destroying tables immediately after creation.
- **Manual Tracking:** You MUST manually append `INSERT IGNORE INTO schema_migrations (version) VALUES ('XXX_name');` at the very end of your `.sql` file to prevent infinite re-execution on startup.

## 3. Storage Authority & Persistence Boundaries (MariaDB vs. Valkey Master)

### 3.1 Storage Authority Tiers
To prevent conflating caching with primary persistence, the system strictly separates storage into two authoritative tiers and one acceleration tier:
1. **MariaDB Master (Canonical Relational Persistence)**:
   - The absolute Single Source of Truth for all durable player assets, progression, currencies, inventories, and audit records.
   - ACID transactions, foreign keys, and deterministic row-lock hierarchy (Rank 0 -> 8) guarantee consistency.
2. **Valkey Master (Primary Authoritative Ephemeral Store)**:
   - Valkey is the Single Source of Truth with **no underlying SQL table**.
   - MUST NOT use volatile state as the sole authority for durable player wealth or progression. MUST verify settlement/recovery behavior separately; AOF `everysec` and a passing test do not guarantee lossless unfinished work or cross-store atomicity.
   - Features utilizing Valkey Master rely on native TTL expiration or explicit application lifecycle hooks for garbage collection.
3. **Valkey Cache (Projection / Read Acceleration Layer)**:
   - MariaDB remains the canonical Single Source of Truth. Valkey holds read-optimized projections (e.g. JSON ranking snapshots and SQL-backed maintenance projections).
   - Loss on crash or eviction is completely harmless because projections can be deterministically reconstructed from MariaDB on demand.

### 3.2 Persistence Decision Criteria
High mutation frequency alone does NOT justify Valkey as primary store (all currency movements must remain in MariaDB).
- **Durable / Wealth / Inventory / Progression**: MUST use MariaDB Master.
- **Naturally Expiring (TTL) or Rebuildable from SQL**: Valkey Master or Valkey Cache.
- Full rationale and decision matrix reside in [`docs/architecture/valkey-keyspace.md`](../../docs/architecture/valkey-keyspace.md).

### 3.3 Transient Feature State
- Lobbies and run buffers MUST follow their owner-specific lifetime and settlement contract in [`docs/architecture/transient-run-state.md`](../../docs/architecture/transient-run-state.md) and [`docs/architecture/valkey-keyspace.md`](../../docs/architecture/valkey-keyspace.md).
- MUST NOT reintroduce the withdrawn Candidate E World Boss design as a legacy requirement. Current boss behavior is specified by [`docs/design/boss.md`](../../docs/design/boss.md).

### 3.4 General Caching Constraints & Keyspace Taxonomy
- **Centralized Keyspace Specification (SSOT):** All Valkey key patterns, data types, and expiration policies MUST conform to the taxonomy defined in [`docs/architecture/valkey-keyspace.md`](../../docs/architecture/valkey-keyspace.md) (`party2:<namespace>:<entity>[:<id>]`). Mechanically enforced by Go AST lint test (`internal/architecture/valkey_lint_test.go`).
- **Strict Prohibition of `KEYS *`:** Never execute `KEYS *` or unindexed wildcard `SCAN` in production code. Use Set indexing (e.g. `party2:player:sessions:<player_id>`) for O(1) multi-key lookups or cascade invalidation.
- **Mandatory Lifetime Policy:** Every Valkey key MUST have an explicit TTL or a documented removal lifecycle in the keyspace inventory. MUST preserve unfinished scheduled work until terminal processing/cancellation; MUST NOT expire a live queue or actor index merely to satisfy a generic TTL rule. New non-expiring keys MUST document ownership and cleanup.
- **SQL First for Assets:** The relational database (SQL) is the primary source of truth for critical persistent player state. Do not use Valkey as the primary persistence for critical player data.
- **Concrete Requirements Only:** Do not introduce Valkey without a concrete feature requirement or measured performance benefit.
- **Performance Caching:** Do not pre-emptively cache static/master data (Items, Jobs) in Valkey. Introduce read-caching only if empirical measurement proves SQL is a bottleneck.

### 3.5 Valkey Lua Scripting Standards
See [`docs/architecture/valkey-keyspace.md`](../../docs/architecture/valkey-keyspace.md).
- **When to Use**: ONLY for atomic conditional state transitions or CAS operations unachievable via native commands (`INCR`, `SET NX`). NEVER use for single-key updates or when native commands suffice.
- **Budgets & Complexity**: Execution time MUST be < 1ms; complexity MUST NOT exceed O(1) or O(log N).
- **BANNED in Lua**: Unbounded loops, large iterations or JSON payloads, wildcard key scans (`KEYS *`), blocking calls. Existing bounded party-roster JSON operations MUST stay bounded by the lobby capacity. The linter verifies banned commands and embedding; timing, complexity, and payload bounds MUST be reviewed separately.
- **Mandatory Hash Tagging**: Multi-key operations MUST use `{...}` hash tags (e.g. `{char:<id>}`). Cross-slot multi-key scripts are BANNED.
- **In-Memory Parity**: Test adapters in each owning feature package MUST implement equivalent atomic logic and error codes. MUST NOT assume a shared `internal/valkey/memory.go` adapter exists or introduce a production fallback on connectivity failure.

### 3.6 Collection Data Type Selection & Ephemeral Element Expiration (Set vs ZSet vs Hash)
- **String or Hash (`HSET`, `HGETALL`)**: Single-record entities with known fields, CAS semantics, or direct key-value state.
- **Standard Set (`SADD`, `SMEMBERS`, `SREM`)**: Use ONLY where all elements share the exact same lifetime as parent key, or elements are bounded and never independently expire. Standard Sets MUST NOT be used when child elements have distinct or rolling expiration times.
- **TTL-Scored Sorted Set (`ZADD`, `ZREMRANGEBYSCORE`, `ZRANGE`) [Approved SSOT Pattern]**:
  - MUST be used for indices whose members expire independently (such as sessions/tokens). MUST NOT apply this rule to lifecycle-managed ScheduledAction actor Sets; Pending/Processing work remains indexed until terminal processing or cancellation.
  - **Score**: Unix timestamp seconds (`float64(ExpiresAt.Unix())`).
  - **Lazy Purging**: Read/write paths MUST purge expired elements via `ZREMRANGEBYSCORE key -inf <now.Unix()>`. Background polling daemons are BANNED.
  - **Zero-Downtime Upgrade (`WRONGTYPE`)**: Catch `WRONGTYPE` on migration from legacy Set keys and gracefully upgrade.

### 3.7 Physical Organization (`//go:embed`)
- Scripts MUST reside in `<pkg>/lua/*.lua` files and be embedded via `//go:embed`.
- Raw multiline string script constants in Go source files are BANNED. Preload with `valkey.NewLuaScript` (`EVALSHA`).

## 4. Sub-Resource Repository SQL Scoping and Ownership Authorization
- **Strict SQL Scoping:** When modifying, finalizing, or deleting sub-resources belonging to a player or character (e.g., `character_challenge_records`, `character_boss_records`, `character_letters`, `home_companion_phrases`), SQL queries MUST include ownership predicates in the `WHERE` clause:
  - `WHERE id = ? AND character_id = ?` (or `WHERE id = ? AND player_id = ?`)
- **Defense in Depth:** In addition to API handler layer authorization (`withAuthenticatedCharacter` / `authorizeCharacter`), repositories and domain services MUST verify sub-resource ownership so that direct calls or bypassed routing cannot perform IDOR (Insecure Direct Object Reference) mutations.
- **Differentiating Status vs Ownership Errors:** Repositories and domain services should distinguish between non-existent resources (`ErrNotFound`), unauthorized ownership mismatches (`ErrForbidden`), and invalid lifecycle states (`ErrNotActive`, `ErrAlreadyClaimed`).

## 5. CAS (Compare-And-Swap) / Conditional Status Update Pattern for Shared State
- **Conditional Lifecycle State Transitions:** For peer-to-peer and shared state entities undergoing lifecycle state transitions (Flea Market listings, player store sales, and contest/lottery rounds):
  - SQL `UPDATE` queries MUST include conditional status guards:
    ```sql
    UPDATE fleamarket_listings
    SET status = 'sold'
    WHERE id = ? AND status = 'active'
    ```
- **RowsAffected Validation:** Repositories executing conditional state transition `UPDATE` queries MUST inspect `result.RowsAffected()`. If `affected == 0`, return the appropriate domain conflict error rather than treating 0 affected rows as a silent success.
- **Pessimistic Locking Order for Shared Peer-to-Peer Entities:** When processing a shared listing or sale, acquire its Rank 0 row lock before dependent characters/wallets/storage. Direct auction/depot transfers have no listing entity and MUST start with ascending character locks. MUST NOT invent bidding, buyout, parcel, or guild-donation mechanics from transaction examples.

## 6. Mandatory Concurrency Stress Testing for P2P and Shared-Resource State Mutations
- **Mandatory Paired Stress Test:** Any feature introducing or altering state mutations on shared resources, peer-to-peer asset transfers, purchases, or parallel contender claims (e.g. Auction/Depot direct transfers, Flea Market listings/purchases, Store sales, Guild activity points, or contest/lottery settlement) MUST include a paired concurrency stress test.
- **Standardized Concurrency Test Harness:** Tests MUST use the shared concurrency harness (`testutil.RunConcurrentStressTest` or `testutil.RunRace` / `testutil.RunRace2` in `internal/testutil` or `internal/database/testutil`):
  - **Deadlock & Timeout Assertion:** The harness automatically verifies that zero deadlocks or lock wait timeouts occur (`IsDeadlockError` count == 0). Any detected deadlock fails the test immediately via `t.Fatalf`.
  - **Asset Conservation Invariant:** Tests MUST assert strict asset conservation across all accounts/inventories (no duplicated gold/items, no phantom claims, no double-spending).
  - **Adaptive Load Scaling:** Concurrency workers and iteration counts MUST be scaled via `GetStressConfig()` (`PARTY2_STRESS_ENABLED`): fast verification in standard CI (15 workers, 10 ops/worker) and deep stress verification when `PARTY2_STRESS_ENABLED=1` (50 workers, 20 ops/worker).
- **Centralized Test Entity Factories:** Integration and stress tests MUST utilize centralized entity factories (`CreateTestPlayer`, `CreateTestCharacterWithFunds`, `CreateTestGuildWithLeader`, `CreateTestInventoryWithItems`, `CreateTestDepot`) rather than duplicating ad-hoc player/character creation boilerplate. Factories automatically enforce domain name length constraints (<= 32 chars) and unique suffixes (`id.New()[:8]`) to prevent primary key / unique constraint collisions across repeated test runs.

## 7. Lifecycle: Fail-Fast Startup Connectivity & Clean Teardown
- **Mandatory Fail-Fast Validation**: Both MariaDB and Valkey are strictly required runtime dependencies for the production application. During application bootstrap (`cmd/party2`), the startup sequence MUST perform blocking, timeout-bounded connectivity checks (`database.PingContext` and `valkey.Ping`) against both stores before binding network listeners or serving traffic.
- **Prohibition of Silent In-Memory Fallbacks in Production**: Application bootstrap MUST NOT catch connectivity/authentication errors and silently degrade or fall back to in-memory mocks/stores. If either data store is unreachable, unconfigured, or unhealthy, the process MUST abort startup immediately with a non-zero exit code and structured error log.
- **Ordered Clean Teardown (Zero Orphan State)**: If startup aborts at any point during initialization (e.g. MariaDB connected, but Valkey ping timed out; or HTTP listener bind failed):
  - Any partially allocated resources (database connection pools, Valkey client sockets, background workers, listeners) MUST be cleanly and deterministically released using structured `defer` or context cancellation.
  - The process MUST NOT leave hanging connection pools, orphaned sockets, or leaking goroutines.
- **Test Isolation Boundary**: In-memory adapters MAY be selected by factories for explicitly injected test dependencies. Production bootstrap MUST supply validated real stores; factory nil-client test branches MUST NOT become connectivity-error fallbacks.
