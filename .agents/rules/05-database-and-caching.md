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
  - **Sub-Resource Pre-Locking (Inverted Lock Hierarchy):** When inspecting or mutating dependent sub-resources (such as `inventory_items` during item sales, trade, or material synthesis), always acquire the exclusive row lock on the parent entity (`characters`) first before locking the sub-resource. Never acquire a lock on `inventory_items` first and then delegate to a service or helper (such as `economy.Service`) that subsequently locks `characters`. This causes A-B / B-A lock ordering inversions and MariaDB deadlocks (Error 1213). (Note: This is mechanically prevented by `internal/database/lock_hierarchy_lint_test.go` on every `make check`).
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
