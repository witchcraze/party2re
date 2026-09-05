# Transient Run State Architecture & Persistence Boundary (Candidate D)

This document establishes the architectural evaluation, schema design, Lua script contracts, crash-recovery semantics, and persistence boundaries for **Candidate D: In-Progress Run Buffers** (Dungeon exploration and Endurance Challenge sessions), per **RFC #356** and [`.agents/rules/05-database-and-caching.md`](../../.agents/rules/05-database-and-caching.md).

---

## 1. Executive Summary & Problem Context

In Party2 Re, multi-turn exploration and survival mini-games involve high-frequency player interactions:
- **Dungeon Exploration (`internal/dungeon`)**: Players navigate multi-floor grid labyrinths (3–5 floors, 20–40 tiles per floor). Every tile step triggers coordinate updates, stamina/turn consumption, HP/MP alterations, and tentative reward accumulation.
- **Endurance Challenge (`internal/challenge`)**: Players fight successive monster waves (20–100+ rounds) with scaling difficulty, accumulating provisional EXP, Gold, and milestone item drops per wave.

### The Current Relational Bottleneck
Currently, intermediate state is saved directly into relational MariaDB tables:
- Every dungeon move updates `dungeon_active_expeditions` (coordinates, remaining turns, HP, and a JSON column of accumulated drops).
- Every challenge round updates `challenge_sessions` (round counter, HP, and accumulated rewards JSON).

Writing intermediate turn state directly to MariaDB causes:
1. **Severe Write Amplification**: A single dungeon expedition produces 60–120 synchronous SQL `UPDATE` queries. 50 concurrent explorers generate thousands of database writes per minute purely for tentative state.
2. **Connection Pool Contention**: Each step occupies a MariaDB connection during application execution, reducing connection availability for player authentication, shops, and financial transactions.
3. **JSON Column Rewrites**: MariaDB must re-serialize and write entire JSON arrays of provisional loot on every single tile or wave.
4. **Table Bloat from Abandoned Sessions**: Players who close their browser mid-run leave orphaned rows in MariaDB requiring periodic cleanup sweeper crons.

### The Architectural Solution: Candidate D
Per **RFC #356** (Persistence Boundary Guidelines), provisional working buffers that are only committed upon exit or cashout are prime candidates for **Valkey Master**.

By migrating active run buffers to Valkey Master with **Valkey Lua Scripting**:
- All high-frequency moves and turn resolutions execute in Valkey in **sub-millisecond time (< 1ms)** with zero MariaDB queries.
- Atomic Lua scripts prevent race conditions and eliminate application-level distributed locks.
- Sliding TTL (2 hours) automatically evicts abandoned sessions without database sweepers.
- Final settlement executes as an atomic **Two-Phase Settlement** into MariaDB Master (`RunInTx`), guaranteeing 100% economic and progression durability.

---

## 2. Performance & Concurrency Trade-Offs

| Metric / Dimension | Current MariaDB Architecture | Proposed Valkey Master Architecture |
| :--- | :--- | :--- |
| **Active Turn Mutation Latency** | 5ms – 25ms (SQL parse, disk sync/buffer pool, InnoDB locking) | 0.2ms – 0.8ms (Single-threaded in-memory Lua execution) |
| **Write Amplification on DB** | High (1 write per tile/round, rewriting full JSON loot columns) | Zero during active runs (0 SQL queries while exploring) |
| **Concurrency & Row Locking** | Row-level locking (`SELECT ... FOR UPDATE`), risk of contention under rapid inputs | Single-threaded atomic evaluation in Valkey; zero application locking required |
| **Connection Pool Impact** | Consumes pooled SQL connections on every player move | Single multiplexed TCP client; zero MariaDB connection pool pressure |
| **Abandoned Session Cleanup** | Orphaned records remain indefinitely until swept by batch cron | Native sliding TTL (`7200s` / 2 hours) automatically evicts expired state |
| **Durability Guarantee** | Relational ACID (InnoDB redo/undo logs) | Ephemeral in-flight buffer with AOF fsync (`everysec`); MariaDB ACID upon settlement |

---

## 3. Storage Authority & Two-Phase Settlement Boundary

Storage authority strictly respects the hierarchical decision tree in [`.agents/rules/05-database-and-caching.md`](../../.agents/rules/05-database-and-caching.md):
- **MariaDB Master**: Canonical Single Source of Truth for durable assets (wallets, permanent inventory, character progression, and permanent audit logs).
- **Valkey Master**: Primary authoritative store for **in-flight working buffers** during active sessions. No SQL rows exist for active runs.

```text
┌─────────────────────────────────────────────────────────────────────────────────────────┐
│ Phase 1: Session Initiation (MariaDB Gate -> Valkey Seed)                               │
│ 1. Validate entry prerequisites (min level, character status)                           │
│ 2. Debit entry fee / stamina in MariaDB transaction if applicable                        │
│ 3. Initialize run buffer keys in Valkey Master with 2-hour sliding TTL                  │
└────────────────────────────────────────────┬────────────────────────────────────────────┘
                                             │
                                             ▼
┌─────────────────────────────────────────────────────────────────────────────────────────┐
│ Phase 2: In-Flight Active Run (100% Valkey Master via Atomic Lua)                       │
│ - Advance coordinates or wave rounds atomically via Lua scripts                         │
│ - Validate HP > 0 and turn allowances within the script                                 │
│ - Buffer provisional EXP, Gold, Medals, and Item Drops                                  │
│ - Refresh 2-hour sliding TTL on every atomic mutation                                   │
│ - ZERO MariaDB queries or locks                                                         │
└────────────────────────────────────────────┬────────────────────────────────────────────┘
                                             │
                                             ▼
┌─────────────────────────────────────────────────────────────────────────────────────────┐
│ Phase 3: Two-Phase Settlement Boundary (Valkey Buffer -> MariaDB Master)                │
│ 1. Read accumulated tentative rewards from Valkey buffer                                │
│ 2. Execute MariaDB Unit of Work (RunInTx):                                              │
│    - Acquire exclusive lock on Character (Rank 2)                                       │
│    - Acquire exclusive lock on Inventory / Equipment (Rank 3)                           │
│    - Apply final rewards (EXP, Gold, Medals, Items) or defeat penalty (50% / forfeiture)│
│    - Insert permanent record (dungeon_history / challenge_records)                      │
│    - Commit transaction                                                                 │
│ 3. On MariaDB commit success: Delete Valkey run buffer keys (DEL)                       │
└─────────────────────────────────────────────────────────────────────────────────────────┘
```

### 3.1 Strict Persistence Invariants
1. **Zero Premature Credit**: No reward (item, currency, or experience) buffered in Valkey may be credited to permanent character inventory or stats until MariaDB transaction successfully commits.
2. **Defeat / Forfeiture Guarantee**: If a player wipes out or runs out of turns, penalty rules (e.g. 50% EXP/Gold consolation, item loss) are computed deterministically from the buffered state during the settlement transaction.
3. **Idempotent Finalization**: The finalization endpoint uses CAS or one-time token consumption to prevent duplicate cashout claims. Once settled in MariaDB, repeated requests return the final result without re-awarding rewards.

---

## 4. Crash Recovery & Failure Semantics

### 4.1 Application Server Crash or Restart
- **Valkey Durability**: Valkey operates with Append-Only File (`AOF`) set to `appendfsync everysec`. At most 1 second of in-flight moves could be lost during an abrupt host power outage.
- **Application Disconnection / Pod Restart**: On application server restart, Valkey is unaffected. When the player reconnects, `GetActiveExpedition` or `GetActiveChallenge` reads the active session from Valkey. The player resumes their run exactly where they left off.
- **Player Abandonment**: If the player closes their browser and never returns, the session key's sliding TTL (2 hours) expires cleanly. Valkey frees the memory automatically with zero orphaned SQL rows.

### 4.2 Settlement Failure Recovery (Two-Phase Atomicity)
- **Scenario A: Crash during Phase 3 before MariaDB commits**:
  MariaDB automatically rolls back the uncommitted transaction. The Valkey run buffer remains intact. The player can retry settlement or continue the run. No items or currencies are duplicated or lost.
- **Scenario B: Crash after MariaDB commits but before Valkey `DEL` executes**:
  MariaDB has permanently recorded the completed run. Upon subsequent query, the service checks MariaDB history or status, determines that the expedition/session was already finalized, and issues the cleanup `DEL` to Valkey. Double-claiming is strictly impossible because MariaDB is the authority.

---

## 5. Keyspace Schema & Cluster Hash Tagging

Per Section 2.3 of `docs/architecture/valkey-keyspace.md`, all multi-key operations touched by a Lua script MUST enclose their dynamic co-locating entity identifier in curly braces `{...}` to guarantee hash slot colocation in clustered topologies.

Because each character may have at most **one** active dungeon expedition and **one** active challenge session at any time, `{char:<character_id>}` serves as the co-locating hash tag:

### 5.1 Dungeon Exploration Keyspace

| Key Pattern | Storage Tier | Data Type | TTL Policy | Description & Fields |
| :--- | :--- | :--- | :--- | :--- |
| `party2:dungeon:{char:<character_id>}:state` | Valkey Master | `Hash` | 2 hours (`7200s`), sliding | Hash fields: `expedition_id`, `character_id`, `dungeon_id`, `current_floor`, `pos_x`, `pos_y`, `current_hp`, `turns_remaining`, `status`, `started_at`, `updated_at`. |
| `party2:dungeon:{char:<character_id>}:rewards` | Valkey Master | `Hash` | 2 hours (`7200s`), sliding | Hash fields: `exp` (int), `gold` (int), `medals` (int), `items` (JSON array string). |
| `party2:dungeon:{char:<character_id>}:revealed` | Valkey Master | `Set` | 2 hours (`7200s`), sliding | Set of visited `floor:x:y` coordinates for fog-of-war exploration tracking. |

### 5.2 Endurance Challenge Keyspace

| Key Pattern | Storage Tier | Data Type | TTL Policy | Description & Fields |
| :--- | :--- | :--- | :--- | :--- |
| `party2:challenge:{char:<character_id>}:session` | Valkey Master | `Hash` | 2 hours (`7200s`), sliding | Hash fields: `session_id`, `character_id`, `tier_id`, `current_round`, `current_hp`, `status`, `created_at`, `updated_at`. |
| `party2:challenge:{char:<character_id>}:rewards` | Valkey Master | `Hash` | 2 hours (`7200s`), sliding | Hash fields: `exp` (int), `gold` (int), `items` (JSON array string). |

---

## 6. Lua Script Contracts & Specification

In accordance with [`.agents/rules/05-database-and-caching.md`](../../.agents/rules/05-database-and-caching.md) Section 3.5, Lua scripts must:
- Complete in `< 1ms` with complexity `<= O(log N)`.
- Use hash tagging for all `KEYS[...]`.
- Enforce invariant checks atomically.

### 6.1 `dungeon_step` Contract
Advances player position in a dungeon floor, decrements turns, adjusts HP, accumulates tentative rewards, and refreshes the 2-hour sliding TTL in a single atomic round-trip.

- **Script Identifier**: `dungeon_step`
- **Target Keys**:
  - `KEYS[1]`: State Key (`party2:dungeon:{char:<character_id>}:state`)
  - `KEYS[2]`: Rewards Key (`party2:dungeon:{char:<character_id>}:rewards`)
- **Arguments**:
  - `ARGV[1]`: `expected_expedition_id` (string)
  - `ARGV[2]`: `new_floor` (string integer)
  - `ARGV[3]`: `new_x` (string integer)
  - `ARGV[4]`: `new_y` (string integer)
  - `ARGV[5]`: `hp_delta` (string integer: negative for damage, positive for recovery)
  - `ARGV[6]`: `turns_delta` (string integer, e.g. `-1`)
  - `ARGV[7]`: `exp_delta` (string integer)
  - `ARGV[8]`: `gold_delta` (string integer)
  - `ARGV[9]`: `medals_delta` (string integer)
  - `ARGV[10]`: `reward_item_id` (string, empty if none)
  - `ARGV[11]`: `now_unix` (string integer)
  - `ARGV[12]`: `ttl_seconds` (string integer, `7200`)
- **Preconditions & Atomic Invariants**:
  1. `KEYS[1]` must exist. If not, return error string `"ERR_EXPEDITION_NOT_FOUND"`.
  2. `status` must be `"exploring"`. If not, return `"ERR_EXPEDITION_NOT_ACTIVE"`.
  3. Stored `expedition_id` must match `expected_expedition_id`. If not, return `"ERR_EXPEDITION_ID_MISMATCH"`.
  4. If `current_hp + hp_delta <= 0`: set `status = "wiped_out"`, `current_hp = 0`.
  5. If `turns_remaining + turns_delta <= 0`: set `status = "wiped_out"`.
- **Mutations**:
  - Update `current_floor`, `pos_x`, `pos_y`, `current_hp`, `turns_remaining`, `updated_at`.
  - Increment rewards (`exp`, `gold`, `medals`).
  - If `reward_item_id != ""`, append to `items` JSON array.
  - Refresh TTL on both `KEYS[1]` and `KEYS[2]` via `redis.call('EXPIRE', key, ttl_seconds)`.
- **Return Value**:
  - Array containing updated status, floor, x, y, hp, remaining turns, and reward totals.

### 6.2 `challenge_advance_round` Contract
Advances the survival challenge wave round, records surviving HP, accumulates monster defeat rewards, and refreshes sliding TTL.

- **Script Identifier**: `challenge_advance_round`
- **Target Keys**:
  - `KEYS[1]`: Session Key (`party2:challenge:{char:<character_id>}:session`)
  - `KEYS[2]`: Rewards Key (`party2:challenge:{char:<character_id>}:rewards`)
- **Arguments**:
  - `ARGV[1]`: `expected_session_id` (string)
  - `ARGV[2]`: `surviving_hp` (string integer)
  - `ARGV[3]`: `exp_delta` (string integer)
  - `ARGV[4]`: `gold_delta` (string integer)
  - `ARGV[5]`: `reward_item_id` (string, empty if none)
  - `ARGV[6]`: `now_unix` (string integer)
  - `ARGV[7]`: `ttl_seconds` (string integer, `7200`)
- **Preconditions & Atomic Invariants**:
  1. `KEYS[1]` must exist. If not, return `"ERR_SESSION_NOT_FOUND"`.
  2. `status` must be `"active"`. If not, return `"ERR_SESSION_NOT_ACTIVE"`.
  3. Stored `session_id` must match `expected_session_id`. If not, return `"ERR_SESSION_ID_MISMATCH"`.
- **Mutations**:
  - Increment `current_round` by 1.
  - Set `current_hp = surviving_hp`.
  - Increment rewards (`exp`, `gold`).
  - If `reward_item_id != ""`, append to `items` JSON array.
  - Update `updated_at`.
  - Refresh TTL on both keys via `redis.call('EXPIRE', key, ttl_seconds)`.
- **Return Value**:
  - Array containing new round number, surviving HP, total accumulated EXP, Gold, and items count.

---

## 7. In-Memory Fallback Parity & Test Mocking Guidelines

To comply with the project requirement for zero mock divergence:
1. **Thread Safety**: Any in-memory mock repository (`internal/dungeon/memory_repository.go` or `internal/valkey/memory.go`) MUST protect in-memory maps using `sync.RWMutex`.
2. **Error String & Behavior Parity**:
   The Go in-memory mock MUST return exact error constants matching Lua script return codes:
   - `ERR_EXPEDITION_NOT_FOUND`
   - `ERR_EXPEDITION_NOT_ACTIVE`
   - `ERR_EXPEDITION_ID_MISMATCH`
   - `ERR_SESSION_NOT_FOUND`
   - `ERR_SESSION_NOT_ACTIVE`
   - `ERR_SESSION_ID_MISMATCH`
3. **Contract Test Suites**:
   Unit tests verifying `dungeon.Service` and `challenge.Service` must be parameterized to execute against both:
   - Live Valkey instance with preloaded Lua scripts (`valkey.NewLuaScript`).
   - In-memory Go fallback mock.
   This guarantees that local development and CI container runs behave identically.

---

## 8. Migration Plan & Database Schema Evolution

When implementing Candidate D in subsequent production tickets:
1. **Phase 1 (Dual Interface)**: Introduce `ValkeyExpeditionRepository` and `ValkeyChallengeRepository` alongside existing SQL repositories.
2. **Phase 2 (Two-Phase Settlement Service)**: Wire services to use Valkey for `Start`, `Move`, `AdvanceRound` while retaining MariaDB `RunInTx` for `Escape`, `Clear`, `Cashout`, and `Defeat`.
3. **Phase 3 (Schema Deprecation)**:
   - Retain durable relational tables: `character_dungeon_records`, `dungeon_expedition_history`, `character_challenge_records`.
   - Remove ephemeral active tables via a dedicated migration: `DROP TABLE dungeon_active_expeditions` and `DROP TABLE challenge_sessions` (following the precedent of Migration 052 dropping `parties` and `party_members`).
