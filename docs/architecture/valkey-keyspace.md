# Valkey Keyspace Specification & Operational Guidelines

This document serves as the **Single Source of Truth (SSOT)** for all Valkey key patterns, data types, expiration policies, and operational rules in Party2 Re.

Whenever new keys or caching patterns are introduced, this document MUST be updated in the same Pull Request to maintain architectural clarity, prevent key collisions, and eliminate memory leaks.

---

## 1. Architectural Role & Storage Authority Tiers

In accordance with **RFC #356** and [`.agents/rules/05-database-and-caching.md`](../../.agents/rules/05-database-and-caching.md), storage in Party2 Re is divided into three tiers:

```text
┌─────────────────────────────────────────────────────────────────────────────┐
│ 1. MariaDB Master (Canonical Relational Persistence)                        │
│    - Durable player assets, progression, currencies, inventories, audits    │
│    - ACID transactions, foreign keys, deterministic lock hierarchy (Rank 0..8)│
└──────────────────────────────────────┬──────────────────────────────────────┘
                                       │
                    ┌──────────────────┴──────────────────┐
                    ▼                                     ▼
┌──────────────────────────────────────┐┌──────────────────────────────────────┐
│ 2. Valkey Master (Authoritative)     ││ 3. Valkey Cache (Read Acceleration)  │
│    - Ephemeral state (NO SQL table)  ││    - Read-optimized projections      │
│    - Sessions, wait queues, locks    ││    - Leaderboard snapshots           │
│    - Governed by native TTL expiry   ││    - 100% reconstructible on miss    │
└──────────────────────────────────────┘└──────────────────────────────────────┘
```

1. **MariaDB Master**: Canonical persistence. Absolute source of truth for wealth, progression, inventories, and audit trails.
2. **Valkey Master**: Primary authoritative store for **ephemeral or volatile state** with **no underlying SQL table**. Governed strictly by native TTL or explicit application lifecycle hooks. Restart loss (up to 1s with AOF `everysec`) causes zero economic or progression corruption.
3. **Valkey Cache**: Read acceleration layer. MariaDB is the source of truth. Projections (e.g. Leaderboard Sorted Sets) are fully rebuildable on cache miss or eviction.

---

## 2. Key Naming Conventions & Taxonomy Hierarchy

### 2.1 Standard Format

All keys across Party2 Re MUST conform to the hierarchical colon-delimited taxonomy:

```text
party2:<namespace>:<entity>[:<identifier>...]
```

- **Root Prefix**: Always `party2:`. Enforces namespace isolation on shared or multi-tenant Valkey clusters.
- **Namespace**: The domain or architectural subsystem (`session`, `player`, `maintenance`, `scheduled`, `ratelimit`, `ranking`, `test`).
- **Entity**: The specific resource or data collection (`action`, `lock`, `snapshot`, `status`, `sessions`).
- **Identifier**: Dynamic identifier (`token`, `player_id`, `action_id`, `category`, etc.).
- **Case**: Strictly lowercase ASCII alphanumeric with colons `:` as delimiters. Compound entity terms use snake_case (`status`, `pending`, `snapshot`).

### 2.2 Test Isolation Prefix

Automated tests connecting to live Valkey instances MUST use the `test` namespace:

```text
party2:test:<module>:<entity>[:<identifier>]
```

Examples:
- `party2:test:session:<token>`
- `party2:test:player:sessions:<player_id>`
- `party2:test:maintenance:status`

---

## 3. Master Key Inventory (Comprehensive SSOT Catalog)

The table below catalogs all production key patterns currently active in the codebase:

| Key Pattern / Template | Storage Tier | Data Type | Expiration Policy (TTL) | Value / Serialization Format | Owner Module | Mutating Operations & Invalidation Hooks |
| :--- | :--- | :--- | :--- | :--- | :--- | :--- |
| `party2:session:<token>` | Valkey Master | `String` | 7 days (`604800s`), sliding or fixed | JSON (`PlayerSession`: `token`, `player_id`, `created_at`, `expires_at`) | `internal/player` | `CreateSession` (SET EX), `GetSession` (GET), `DeleteSession` (DEL), `DeleteSessionsByPlayerID` (bulk DEL). |
| `party2:player:sessions:<player_id>` | Valkey Master | `Sorted Set (ZSet)` | 7 days (`604800s`), refreshed on login | Member: session token, Score: `ExpiresAt.Unix()` (`float64`) | `internal/player` | `Save` (ZADD + EXPIRE + lazy ZREMRANGEBYSCORE), `FindByID` (lazy ZREMRANGEBYSCORE), `Revoke` (ZREM + lazy ZREMRANGEBYSCORE), `DeleteByPlayerID` (ZRANGE -> DEL tokens + DEL key). Automatic expiration score tracking eliminates stale token accumulation. |
| `party2:maintenance:status` | Valkey Master / In-Memory | `String` | None (Persistent / Admin managed) | JSON (`SystemMaintenance`: `enabled`, `message`, `starts_at`, `ends_at`, `updated_at`) | `internal/maintenance` | `SetStatus` (SET without TTL), `GetStatus` (GET with in-memory sync), admin endpoints (`POST/PUT /admin/maintenance`). Backed by `system_maintenance` MariaDB table. |
| `party2:scheduled:pending` | Valkey Master | `Sorted Set (ZSet)` | None (Dynamic queue) | Member: Action ID (`string`), Score: `ExecuteAt.Unix()` (`float64`) | `internal/scheduling` | `ScheduleAction` (ZADD), `ClaimPendingActions` (ZRANGEBYSCORE + ZREM), `CancelAction` (ZREM). |
| `party2:scheduled:action:<id>` | Valkey Master | `String` | 1 hour after execution or cancel | JSON (`ScheduledAction`: `id`, `action_type`, `payload`, `execute_at`, `state`) | `internal/scheduling` | `ScheduleAction` (SET), `GetAction` (GET), `CompleteAction` / `FailAction` / `CancelAction` (EXPIRE 1h). |
| `party2:scheduled:lock:<id>` | Distributed Coordination | `String` | 30 seconds (`30s` lock timeout) | Worker Node ID (`string`) | `internal/scheduling` | `ClaimPendingActions` (SET NX EX 30), released via DEL on completion or auto-released on worker crash. |
| `party2:scheduled:actor:<actor_id>` | Distributed Coordination | `String` | Execution duration + safety margin | Scheduled Action ID (`string`) | `internal/scheduling` | `ScheduleAction` (SET NX EX), released via DEL on action completion/failure to prevent concurrent actions on same actor. |
| `party2:ratelimit:<key>` | Distributed Coordination | `String` (Atomic Int) | Window duration (e.g. 60s or 900s) | Integer counter | `internal/ratelimit` | `Allow` (`INCR` + conditional `EXPIRE` on count == 1). |
| `party2:ranking:snapshot:<category>` | Valkey Cache | `String` | 5 minutes (`300s`) | JSON (`RankingSnapshot`: entries, refreshed_at) | `internal/ranking` | `SetSnapshot` (SET EX 300), `GetSnapshot` (GET). Rebuilt on miss from MariaDB `ranking_snapshots` table. |
| `party2:ranking:refresh` | Scheduled Task ID | Task Payload | Executed via ScheduledAction | Constant task identifier string | `internal/ranking` | Registered in background worker scheduler to periodically refresh ranking snapshots. |
| `party2:party:lobby:<party_id>` | Valkey Master | `String` | 15 minutes (`900s`), refreshed on activity | JSON (`LobbyState`: `Party`, `Members`) | `internal/party` | `SaveParty`, `GetParty`, `UpdateParty`, `DeleteParty`. Automatic expiration of abandoned lobbies. |
| `party2:party:lobbies` | Valkey Master | `Sorted Set (ZSet)` | None (Dynamic index) | Member: `party_id`, Score: `CreatedAt.Unix()` | `internal/party` | `SaveParty` (ZADD), `DeleteParty` (ZREM), `ListParties` (ZREVRANGE / ZRANGE). |
| `party2:party:character:<character_id>` | Valkey Master | `String` | 15 minutes (`900s`), refreshed on activity | Party ID (`string`) | `internal/party` | `AddMember` (SET EX), `RemoveMember` / `DeleteParty` (DEL), `GetActivePartyByCharacter` (GET). O(1) single-party membership check. |
| `party2:party:ready:<party_id>:<character_id>` | Valkey Master | `String` | 60 seconds (`60s` countdown) | Flag (`"1"`) | `internal/party` | `UpdateMemberReady` (SET EX 60 or DEL), `GetMembers` (EXISTS). Automatic ready countdown timeout. |

---

## 4. Codified Guidelines for Future State Candidates (RFC #356 Roadmap)

The following specifications define the key patterns, data types, and lifecycle semantics for upcoming state migration milestones:

### 4.1 Candidate C: Party & Matchmaking Wait Lobbies (Issue #368, Issue #380 - Completed)

- **Status**: Completed in PR #376 (Issue #368) and finalized in Issue #380 with MariaDB schema cleanup (Migration 052 dropped `parties` and `party_members`).
- **Goal**: Move transient multiplayer recruitment, wait lobbies, and ready check states from MariaDB to Valkey Master to eliminate lock contention.
- **Implemented Key Patterns**:
  - `party2:party:lobby:<party_id>`: `String (JSON)` containing lobby metadata and member roster. TTL: 15 minutes (`900s`), refreshed by member activity.
  - `party2:party:lobbies`: `Sorted Set (ZSet)` indexing active recruiting parties scored by `CreatedAt.Unix()`.
  - `party2:party:character:<character_id>`: `String` mapping character ID to current party ID. TTL: 15 minutes (`900s`). Ensures atomic single-party membership check.
  - `party2:party:ready:<party_id>:<character_id>`: `String` flag (`"1"`). TTL: 60 seconds (`60s` countdown expiration).
- **Lifecycle & Boundaries**:
  - Waiting lobbies have a natural TTL of 15 minutes and expire automatically if abandoned with zero orphaned SQL rows.
  - When the countdown expires without full ready status, the unready member's readiness flag expires automatically without blocking the lobby.
  - When the leader starts the adventure, the party transitions into durable quest resolution: final outcomes are saved exclusively to MariaDB (`party_adventure_logs`), while characters are locked and updated via `RunInTx` in ascending ID order. Legacy `parties` and `party_members` MariaDB tables were officially dropped via Migration 052.

### 4.2 Candidate D: In-Progress Run Buffers (Issue #369)

- **Goal**: Buffer active multi-turn dungeon expeditions and challenge gauntlets in Valkey Master, eliminating relational write amplification per turn.
- **Key Patterns**:
  - `party2:dungeon:run:<expedition_id>`: `String (JSON)` or `Hash` holding current floor, player HP/MP, turn counter, and floor seed. TTL: 2 hours.
  - `party2:dungeon:rewards:<expedition_id>`: `String (JSON)` holding uncommitted tentative item drops, gold, and experience gathered during the run. TTL: 2 hours.
  - `party2:challenge:run:<session_id>`: `String (JSON)` holding current endurance round and tentative rewards. TTL: 2 hours.
- **Lifecycle & Boundaries (Two-Phase Settlement)**:
  - Phase 1 (Active Run): All turn updates occur in Valkey Master. MariaDB is not touched.
  - Phase 2 (Settlement): Upon victory, retreat, or defeat, an atomic MariaDB transaction executes: durable inventory items are awarded, progression updated, and the Valkey run buffer is immediately deleted (`DEL`).

### 4.3 Candidate E: World Boss Real-time Shared HP (Issue #370)

- **Goal**: High-frequency concurrent boss raid damage resolution without MariaDB single-row lock serialization.
- **Key Patterns**:
  - `party2:boss:hp:<boss_id>`: `String` (Atomic integer counter). Mutated via `DECRBY`.
  - `party2:boss:lock:<boss_id>`: `String` (Distributed lock) for synchronization during final defeat settlement.
- **Lifecycle & Boundaries**:
  - Atomic Valkey `DECRBY` resolves concurrent player attack rounds.
  - Upon reaching `<= 0`, the winning worker acquires `party2:boss:lock:<boss_id>` and commits the defeat record, reward distribution, and banquet announcement atomically in MariaDB.

---

## 5. Operational Hygiene & Anti-Patterns

### 5.1 Strict Prohibition of `KEYS *` / Unindexed `SCAN` in Production

> [!CAUTION]
> **Never use `KEYS *` or unindexed wildcard `SCAN` in production code.**
> Valkey is single-threaded for command execution. Scanning the entire keyspace blocks all incoming client requests, leading to server timeouts and cascading failures.

- **Anti-Pattern (Banned)**:
  ```go
  // BANNED: Never search for keys using wildcard patterns
  keys, _ := client.Do(ctx, client.B().Keys().Pattern("party2:session:*").Build()).AsStrSlice()
  ```
- **Approved Pattern (Index Tracking with Purging)**:
  Maintain a dedicated tracking `Sorted Set` for reverse lookup, ordered retrieval, and lazy expiration purging:
  ```go
  // APPROVED: Query the index sorted set to retrieve specific keys in O(1)
  tokens, _ := client.Do(ctx, client.B().Zrange().Key("party2:player:sessions:" + playerID).Min("0").Max("-1").Build()).AsStrSlice()
  for _, token := range tokens {
      client.Do(ctx, client.B().Del().Key("party2:session:" + token).Build())
  }
  client.Do(ctx, client.B().Del().Key("party2:player:sessions:" + playerID).Build())
  ```

### 5.2 Mandatory TTL for Valkey Master Keys

> [!IMPORTANT]
> **Every key stored in Valkey Master MUST have an explicit TTL or a documented application lifecycle hook.**
> Writing keys without expiration to Valkey Master causes unbounded memory growth and eventual OOM eviction.

- Any key without a natural TTL is considered a memory leak bug, with the sole exception of static administrative singletons (`party2:maintenance:status`) whose lifecycle is strictly governed by admin mutations and MariaDB backup.
- In-flight execution keys (such as `party2:scheduled:action:<id>`) MUST apply a terminal TTL (e.g. 1 hour) upon completion or cancellation.

### 5.3 Test Isolation & Safe Cleanup

- All unit and integration tests connecting to Valkey MUST use key prefixes starting with `party2:test:`.
- Tests MUST clean up only their specific created test keys using `DEL`.
- **`FLUSHDB` and `FLUSHALL` are strictly forbidden**, even in automated test suites, to prevent wiping state from other tests running concurrently or local developer sessions.

### 5.4 Centralized Client Management

- Valkey client configuration and connection setup MUST use `internal/valkey.NewClient()` and `internal/valkey.GetConfig()`.
- The application process (`cmd/party2/main.go`) initializes a single multiplexed Valkey client instance and injects it into domain repositories.
- The client connection is gracefully closed during application shutdown after background workers and HTTP listeners have stopped.

---

## 6. Mechanical Verification & CI Enforcement

Conformance to this keyspace specification is mechanically verified during CI:

1. **AST Keyspace Linting (`internal/architecture/valkey_lint_test.go`)**:
   - Inspects all Go source files under `internal/`.
   - Validates that every string literal starting with `party2:` adheres to registered namespaces.
   - Forbids any call to the banned `Keys()` command in production code.
   - Verifies that all registered production key patterns are documented in this specification file.
2. **Execution in CI Pipeline (`make check`)**:
   - Runs automatically as part of step `[4/7]` via `go test ./internal/architecture`.
