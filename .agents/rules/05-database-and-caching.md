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
    | **Rank 0** | **Shared Peer Entities** | `auctions`, `delivery_parcels`, `fleamarket_listings`, `parties`, `contest_rounds`<br>(`GetListingByIDForUpdate`, `GetParcelByIDForUpdate`, `GetPartyForUpdate`, `GetActiveRoundForUpdate`, `GetPreparingRoundForUpdate`) | Serializes concurrent contenders on shared state upfront before touching player/character assets. |
    | **Rank 1** | **Player Account** | `players` (`playerRepo.*ForUpdate`, `players.*ForUpdate`) | Account-level mutations and security credentials. |
    | **Rank 2** | **Character Primary Entity** | `characters` (`charRepo.FindByIDForUpdate`, `characterRepo.FindByIDForUpdate`, `characters.FindByIDForUpdate`) | Primary game actor; multiple characters MUST be locked in ascending ID order (`id1 < id2`). |
    | **Rank 3** | **Inventory & Equipment** | `inventory_items`, `equipment_slots`<br>(`invRepo.FindByCharacterIDForUpdate`, `inventories.FindByCharacterIDForUpdate`) | Dependent character items; must NEVER be locked before Character. |
    | **Rank 4** | **Job Progression** | `character_jobs`, `character_job_masteries`<br>(`jobRepo.*ForUpdate`) | Job changes and skill loadouts. |
    | **Rank 5** | **Depot Storage** | `character_depots`, `depot_items`<br>(`depotRepo.FindByCharacterIDForUpdate`, `depots.FindByCharacterIDForUpdate`) | Long-term bank/item storage. |
    | **Rank 6** | **Bank Account** | `bank_accounts`, `bank_transfers`<br>(`bankRepo.*ForUpdate`) | Player/character banking and currency transfers. |
    | **Rank 7** | **Guilds** | `guilds`, `guild_members`<br>(`guildRepo.*ForUpdate`) | Guild management; multiple guilds MUST be locked in ascending ID order (`id1 < id2`). |
    | **Rank 8** | **Secondary Feature Records** | `character_achievements`, `farm_plots`, `character_points`, `character_monsters`<br>(`achievementRepo.GetAchievementForUpdate`, `blackMarketRepo.GetCharacterPointsForUpdate`, `monsters.FindByIDForUpdate`, `farmRepo.*ForUpdate`) | Secondary domain features and progression counters. |
- **BANNED ANTI-PATTERNS (Lost Updates & Deadlocks):**
  - **Unprotected Read-Modify-Write:** Do NOT read structs (e.g., Character) outside a transaction, mutate them in Go memory, and then blindly save them back. This will erase concurrent changes (like Adventure rewards).
  - **Direct `BeginTx` in Repositories:** Do NOT call `r.db.BeginTx` directly in repositories. Always use `RunInTx(ctx, r.db, ...)` and `ExecutorFromContext(ctx, r.db)`. (Note: This is automatically validated by the Go AST linter in `internal/database/tx_lint_test.go` on every `make check`).
  - **Non-deterministic Row Locking:** Never lock rows in random, hash-map, or caller-dependent order; always sort IDs ascending when locking multiple rows of the same table.
  - **Sub-Resource Pre-Locking (Inverted Lock Hierarchy):** NEVER lock sub-resources (e.g. `inventory_items`) before locking parent `characters`. Enforced mechanically by `internal/database/lock_hierarchy_lint_test.go`; see [`docs/architecture/cross-domain-primitives.md`](../../docs/architecture/cross-domain-primitives.md) §4.3.
  - **Collection Wipe-and-Insert:** Do NOT implement inventory/collection updates by executing `DELETE FROM ...` followed by re-inserting all items from an unprotected in-memory slice. Use targeted `UPSERT` / `ON DUPLICATE KEY UPDATE` or atomic `DELETE` of specific rows.
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
   - Used **exclusively** for volatile, ephemeral, or deterministically rebuildable state where data loss on application restart (up to 1s with AOF `everysec`) causes zero corruption to player wealth, progression, or economic trust.
   - Features utilizing Valkey Master rely on native TTL expiration or explicit application lifecycle hooks for garbage collection.
3. **Valkey Cache (Projection / Read Acceleration Layer)**:
   - MariaDB remains the canonical Single Source of Truth. Valkey holds read-optimized projections (e.g. Leaderboard Sorted Sets, profile caches).
   - Loss on crash or eviction is completely harmless because projections can be deterministically reconstructed from MariaDB on demand.

### 3.2 Hierarchical Persistence Decision Tree (Durability & Rebuildability First)
High mutation frequency or throughput alone is **NOT** a justification for making Valkey the primary store (all currency movements must remain in MariaDB). Storage authority MUST follow this hierarchical evaluation:

```text
                    ┌─ Yes ─→ MariaDB Master (Wallets, Inventories, Progression)
Durability critical?
                    │
                    No
                    ↓
              Naturally expiring (TTL)?
                    │
             ┌──────┴──────┐
            Yes            No
             ↓              ↓
        Valkey Master   Rebuildable from SQL / audit logs?
        (Sessions)          │
                       ┌────┴────┐
                      Yes       No
                       ↓         ↓
                  Valkey Master MariaDB Master
                  (Queues)      (Audit Records)
```

### 3.3 State Migration Decision Table (Candidates A–F)
State authority decisions for candidates evaluated under RFC #356. Details in [`docs/architecture/valkey-keyspace.md`](../../docs/architecture/valkey-keyspace.md) §4.

| Candidate | Domain | Authority | Key Constraint |
| :--- | :--- | :--- | :--- |
| **A: Sessions** | `player` | Valkey Master | Delete on account cleanup; 7d TTL |
| **B: Maintenance** | `maintenance` | Valkey Master / In-Memory | Admin updates invalidate; fail-open |
| **C: Lobbies** | `party` | Valkey Master | Decoupled from durable adventure logs |
| **D: Run Buffers** | `dungeon`, `challenge` | Valkey Master (settle MariaDB) | 2h sliding TTL; Lua atomic mutations ([SSOT](../../docs/architecture/transient-run-state.md)) |
| **E: Boss Shared HP** | `boss` | Valkey Master (settle MariaDB) | Requires dedicated PoC before production adoption |
| **F: Leaderboards** | `ranking` | MariaDB Master + Valkey Cache | Read cache only; ZSET reconstructed on miss |

### 3.4 General Caching Constraints & Keyspace Taxonomy
- **Centralized Keyspace Specification (SSOT):** All Valkey key patterns, data types, and expiration policies MUST conform to the taxonomy defined in [`docs/architecture/valkey-keyspace.md`](../../docs/architecture/valkey-keyspace.md) (`party2:<namespace>:<entity>[:<id>]`). Mechanically enforced by Go AST lint test (`internal/architecture/valkey_lint_test.go`).
- **Strict Prohibition of `KEYS *`:** Never execute `KEYS *` or unindexed wildcard `SCAN` in production code. Use Set indexing (e.g. `party2:player:sessions:<player_id>`) for O(1) multi-key lookups or cascade invalidation.
- **Mandatory TTL Policy:** Every key written to Valkey Master MUST supply an explicit TTL at write time, with the sole exception of documented administrative singletons (`party2:maintenance:status`).
- **SQL First for Assets:** The relational database (SQL) is the primary source of truth for critical persistent player state. Do not use Valkey as the primary persistence for critical player data.
- **Concrete Requirements Only:** Do not introduce Valkey without a concrete feature requirement or measured performance benefit.
- **Performance Caching:** Do not pre-emptively cache static/master data (Items, Jobs) in Valkey. Introduce read-caching only if empirical measurement proves SQL is a bottleneck.

### 3.5 Valkey Lua Scripting Standards & Physical Organization
See [`docs/architecture/valkey-keyspace.md`](../../docs/architecture/valkey-keyspace.md) §5.5–5.6 for details and performance benchmarks.
- **When to Use**: ONLY for atomic conditional state transitions or CAS operations unachievable via native commands (`INCR`, `SET NX`). NEVER use for single-key updates or when native commands suffice.
- **Budgets & Complexity**: Execution time MUST be < 1ms; complexity MUST NOT exceed O(1) or O(log N).
- **BANNED in Lua**: Unbounded loops, large iterations, wildcard key scans (`KEYS *`), JSON parsing inside Lua, blocking calls. Enforced by `internal/architecture/valkey_lint_test.go`.
- **Mandatory Hash Tagging**: Multi-key operations MUST use `{...}` hash tags (e.g. `{char:<id>}`). Cross-slot multi-key scripts are BANNED.
- **In-Memory Parity**: Services MUST implement 100% equivalent atomic logic and error codes in their Go in-memory fallback (`internal/valkey/memory.go`).
- **Physical Organization (`//go:embed`)**: Scripts MUST reside in `<pkg>/lua/*.lua` files and be embedded via `//go:embed`. Raw multiline string script constants in Go source files are BANNED. Preload with `valkey.NewLuaScript` (`EVALSHA`).

### 3.6 Collection Data Type Selection & Ephemeral Element Expiration (Set vs ZSet vs Hash)
When designing multi-element collection keys in Valkey Master, agents and developers MUST evaluate the lifecycle of child elements according to the following rules:
- **String or Hash (`HSET`, `HGETALL`)**:
  - Use for single-record entities with known fields, CAS semantics, or direct key-value state (e.g. `party2:party:{lobby:<id>}:state`).
- **Standard Set (`SADD`, `SMEMBERS`, `SREM`)**:
  - Use ONLY for collections where all elements share the exact same lifetime as the parent key (all-or-nothing parent TTL), or where elements are bounded, static, and never independently expire (e.g. static tag indexes or fixed party member IDs bounded by lobby lifecycle).
  - Standard Sets MUST NOT be used when child elements have distinct or rolling expiration times.
- **TTL-Scored Sorted Set (`ZADD`, `ZREMRANGEBYSCORE`, `ZRANGE`) [Approved SSOT Pattern]**:
  - MUST be used whenever child elements represent independently expiring ephemeral resources (e.g. player session tokens `party2:player:sessions:<player_id>`, matchmaking wait queues, candidate challenge tokens, or in-progress run reward buffers).
  - **Score**: The element's expiration timestamp in Unix seconds (`float64(ExpiresAt.Unix())`).
  - **Lazy Purging**: Read/write paths (`Save`, `Find`, `Revoke`) MUST purge expired elements via `ZREMRANGEBYSCORE key -inf <now.Unix()>`.
  - **No Background Daemons**: Do NOT implement background ticker goroutines to poll and purge expired elements from Valkey; lazy purging at query/write time is O(log(N) + M) and eliminates thread lifecycle overhead.
  - **Zero-Downtime Upgrade (`WRONGTYPE`)**: When migrating from legacy Set keys, repository logic MUST catch `WRONGTYPE` errors on `ZADD` or `ZRANGE` and gracefully upgrade or fallback to avoid downtime or manual key purges.

## 4. Sub-Resource Repository SQL Scoping and Ownership Authorization
- **Strict SQL Scoping:** When modifying, finalizing, or deleting sub-resources belonging to a player or character (e.g., `challenge_sessions`, `lottery_tickets`, `auction_listings`, `character_challenge_records`, `character_boss_records`, `dungeon_expeditions`, `letters`, `companion_phrases`), SQL queries MUST include ownership predicates in the `WHERE` clause:
  - `WHERE id = ? AND character_id = ?` (or `WHERE id = ? AND player_id = ?`)
- **Defense in Depth:** In addition to API handler layer authorization (`withAuthenticatedCharacter` / `authorizeCharacter`), repositories and domain services MUST verify sub-resource ownership so that direct calls or bypassed routing cannot perform IDOR (Insecure Direct Object Reference) mutations.
- **Differentiating Status vs Ownership Errors:** Repositories and domain services should distinguish between non-existent resources (`ErrNotFound`), unauthorized ownership mismatches (`ErrForbidden`), and invalid lifecycle states (`ErrNotActive`, `ErrAlreadyClaimed`).

## 5. CAS (Compare-And-Swap) / Conditional Status Update Pattern for Shared State
- **Conditional Lifecycle State Transitions:** For peer-to-peer and shared state entities undergoing lifecycle state transitions (Auctions, Deliveries, Mailbox, Trades, Guild donations):
  - SQL `UPDATE` queries MUST include conditional status guards:
    ```sql
    UPDATE delivery_parcels
    SET status = ?, claimed_at = ?
    WHERE id = ? AND status = 'pending'
    ```
- **RowsAffected Validation:** Repositories executing state transition `UPDATE` queries MUST inspect `result.RowsAffected()`. If `affected == 0`, return a domain conflict error (e.g., `ErrParcelAlreadyClaimed`, `ErrListingNotActive`) rather than treating 0 affected rows as a silent success.
- **Pessimistic Locking Order for Shared Peer-to-Peer Entities:** When processing operations on shared peer-to-peer entities (such as delivery parcel claim/cancellation or auction buyout/bidding), acquire an exclusive row-level lock on the shared entity (`GetParcelByIDForUpdate` / `SELECT ... FOR UPDATE`) at the entry of the transaction boundary before executing mutations on dependent characters/wallets/inventories. This serializes concurrent contenders on the shared entity and avoids cross-table foreign key deadlocks.

## 6. Mandatory Concurrency Stress Testing for P2P and Shared-Resource State Mutations
- **Mandatory Paired Stress Test:** Any feature introducing or altering state mutations on shared resources, peer-to-peer asset transfers, competitive bids/purchases, or parallel contender claims (e.g. Bank transfers, Auction bidding/buyout, Flea Market listings/purchases, Delivery parcel claim/cancellation, Guild donations, World Boss raids) MUST include a paired concurrency stress test.
- **Standardized Concurrency Test Harness:** Tests MUST use the shared concurrency harness (`testutil.RunConcurrentStressTest` or `testutil.RunRace` / `testutil.RunRace2` in `internal/testutil` or `internal/database/testutil`):
  - **Deadlock & Timeout Assertion:** The harness automatically verifies that zero deadlocks or lock wait timeouts occur (`IsDeadlockError` count == 0). Any detected deadlock fails the test immediately via `t.Fatalf`.
  - **Asset Conservation Invariant:** Tests MUST assert strict asset conservation across all accounts/inventories (no duplicated gold/items, no phantom claims, no double-spending).
  - **Adaptive Load Scaling:** Concurrency workers and iteration counts MUST be scaled via `GetStressConfig()` (`PARTY2_STRESS_ENABLED`): fast verification in standard CI (15 workers, 10 ops/worker) and deep stress verification when `PARTY2_STRESS_ENABLED=1` (50 workers, 20 ops/worker).
- **Centralized Test Entity Factories:** Integration and stress tests MUST utilize centralized entity factories (`CreateTestPlayer`, `CreateTestCharacterWithFunds`, `CreateTestGuildWithLeader`, `CreateTestInventoryWithItems`, `CreateTestDepot`) rather than duplicating ad-hoc player/character creation boilerplate. Factories automatically enforce domain name length constraints (<= 32 chars) and unique suffixes (`id.New()[:8]`) to prevent primary key / unique constraint collisions across repeated test runs.
