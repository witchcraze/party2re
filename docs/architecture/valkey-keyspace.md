# Valkey Keyspace and Lifetimes

This inventory describes current storage names and lifetimes. Mandatory rules
are in [05-database-and-caching.md](../../.agents/rules/05-database-and-caching.md).
Storage choices do not define game behavior; legacy Party2 remains authoritative.

## Storage authority

- MariaDB owns durable wealth, inventory, progression, and audit records.
- Valkey owns ephemeral sessions, lobbies, run buffers, locks, and scheduled work.
- Valkey caches SQL-backed maintenance state and JSON ranking snapshots.

A TTL or explicit cleanup lifecycle must be specified. Unfinished scheduled
work deliberately has no TTL; it remains Pending/Processing until terminal
processing or cancellation. Deadlines do not imply completion. Valkey loss is
not generally harmless: active sessions/work may be lost, and financial
settlement needs its own verified idempotency and recovery boundary.

## Naming and Cluster Hash Tag

Keys use `party2:<namespace>:<entity>[:<identifier>]`; identifiers can contain
UUID punctuation and hash tags. The registered taxonomy includes session,
player, maintenance, scheduled, ratelimit, ranking, party, dungeon, challenge,
timer, daily, pvp, gvg, eventplaza, casino, and playercontext. Tests can use `party2:test:`.
The historical `party2:boss:` namespace remains registered in the linter but
does not imply a live shared-HP boss store.

Multi-key scripts require a common Hash Tag for a clustered deployment.
Dungeon/Challenge use `{char:<id>}`. **Current Party lobby scripts use untagged
keys**; they run against the standalone deployment and do not establish cluster
readiness. Introducing Cluster requires resolving those multi-key placements.

## Current key inventory

| Key Pattern / Template | Storage Tier | Data Type | Expiration Policy (TTL) | Value / Serialization Format | Owner Module | Mutating Operations & Invalidation Hooks |
| :--- | :--- | :--- | :--- | :--- | :--- | :--- |
| `party2:playercontext:navigation:<character_id>` | Valkey Master | `String` | 7 days (`604800s`), renewed only by successful navigation writes | JSON (`Selection`: registered destination, typed subject, bounded offset/limit); no actor override or feature activity | `internal/playercontext` | `Load` (GET only), `Save` (one atomic SET EX 604800). Missing/expired selection defaults to town without writing. |
| `party2:session:<token>` | Valkey Master | `String` | 7 days (`604800s`), sliding or fixed | JSON (`PlayerSession`: `token`, `player_id`, `created_at`, `expires_at`) | `internal/player` | `CreateSession` (SET EX), `GetSession` (GET), `DeleteSession` (DEL), `DeleteSessionsByPlayerID` (bulk DEL). |
| `party2:player:sessions:<player_id>` | Valkey Master | `Sorted Set (ZSet)` | 7 days (`604800s`), refreshed on login | Member: session token, Score: `ExpiresAt.Unix()` (`float64`) | `internal/player` | `Save` (ZADD + EXPIRE + lazy ZREMRANGEBYSCORE), `FindByID` (lazy ZREMRANGEBYSCORE), `Revoke` (ZREM + lazy ZREMRANGEBYSCORE), `DeleteByPlayerID` (ZRANGE -> DEL tokens + DEL key). Automatic expiration score tracking eliminates stale token accumulation. |
| `party2:maintenance:status` | SQL-backed projection | `String` | None (Persistent / Admin managed) | JSON (`SystemMaintenance`: `enabled`, `message`, `starts_at`, `ends_at`, `updated_at`) | `internal/maintenance` | `SetStatus` (SET without TTL), `GetStatus` (GET with in-memory sync), admin endpoints (`POST/PUT /admin/maintenance`). Backed by `system_maintenance` MariaDB table. |
| `party2:scheduled:pending` | Valkey Master | `Sorted Set (ZSet)` | None (Dynamic queue) | Member: Action ID (`string`), Score: `ExecuteAt.Unix()` (`float64`) | `internal/scheduling` | `ScheduleAction` (ZADD), `FetchDue` (ZRANGEBYSCORE; transient read errors preserve queue membership, stale entries cleaned via ZREM; terminal Save removes pending membership), `CancelAction` (ZREM). |
| `party2:scheduled:action:<id>` | Valkey Master | `String` | No TTL while unfinished; 24h terminal retention; cancellation deletes | JSON (`ScheduledAction`: `id`, `action_type`, `payload`, `execute_at`, `state`) | `internal/scheduling` | `ScheduleAction` (SET), `GetAction` (GET), `Save` (SET with RetainUntil for terminal actions), `CancelByActorID` (DEL). |
| `party2:scheduled:lock:<id>` | Distributed Coordination | `String` | 5 minutes (`300s` worker lock) | Flag (`"1"`) | `internal/scheduling` | `AcquireLock` (SET NX EX 300), released via DEL on completion or auto-released on worker crash. |
| `party2:scheduled:actor:<actor_id>` | Valkey Master | `Set` | None; membership removed on completion/failure, index deleted on actor cancellation | Scheduled Action IDs (`string`) | `internal/scheduling` | `Schedule` (SADD prior to ZADD enqueueing; failure halts and cleans up), `FindPendingByActorID` (SMEMBERS + MGET, Pending/Processing only), `Save` (SREM for terminal states), `CancelByActorID` (DEL on success; write errors halt and propagate without destructive index deletion). An index, not an exclusive actor lock. |
| `party2:ratelimit:<key>` | Distributed Coordination | `String` (Atomic Int) | Window duration (e.g. 60s or 900s) | Integer counter | `internal/ratelimit` | `Allow` (`INCR` + conditional `EXPIRE` on count == 1). |
| `party2:ranking:snapshot:<category>` | Valkey Cache | `String` | 5 minutes (`300s`) | JSON (`RankingSnapshot`: entries, refreshed_at) | `internal/ranking` | `SetSnapshot` (SET EX 300), `GetSnapshot` (GET). Rebuilt on miss from MariaDB `ranking_snapshots` table. |
| `party2:party:lobby:<party_id>` | Valkey Master | `String` | 15 minutes (`900s`), refreshed on activity | JSON (`LobbyState`: `Party`, `Members`) | `internal/party` | `SaveParty`, `GetParty`, `UpdateParty`, `DeleteParty`. Automatic expiration of abandoned lobbies. |
| `party2:party:lobbies` | Valkey Master | `Sorted Set (ZSet)` | None (Dynamic index) | Member: `party_id`, Score: `CreatedAt.Unix()` | `internal/party` | `SaveParty` (ZADD), `DeleteParty` (ZREM), `ListParties` (ZREVRANGE / ZRANGE). |
| `party2:party:character:<character_id>` | Valkey Master | `String` | 15 minutes (`900s`), refreshed on activity | Party ID (`string`) | `internal/party` | `AddMember` (SET EX), `RemoveMember` / `DeleteParty` (DEL), `GetActivePartyByCharacter` (GET). O(1) single-party membership check. |
| `party2:party:ready:<party_id>:<character_id>` | Valkey Master | `String` | 60 seconds (`60s` countdown) | Flag (`"1"`) | `internal/party` | `UpdateMemberReady` (SET EX 60 or DEL), `GetMembers` (EXISTS). Automatic ready countdown timeout. |
| `party2:party:lock:adventure:<party_id>` | Valkey Master | `String` | 10 seconds (`10s`) safety TTL | Lock token (`id.New()`) | `internal/party` | Distributed lock for atomic adventure crawl execution serialization and concurrent start gating (Issue #653). Acquired via `SET NX EX 10`; released via atomic token-safe Lua script. |
| `party2:timer:<category>:<id>` | Valkey Master | `String` | Dynamic (e.g. 60s–180s for sleep, 5d–20d for house estate lease) | Flag (`"1"`) | `internal/core/timer` | `SetLock` (SET EX), `IsLocked` (EXISTS), `GetRemainingLock` (TTL), `ReleaseLock` (DEL). Ephemeral action cooldown, sleep locks, and house estate lease cache. |
| `party2:daily:<action>:<id>` | Valkey Master | `String` | Until next midnight JST (`EXAT` / seconds) | Date string or flag (`"1"`) | `internal/core/timer` | `ConsumeDailyQuota` (SET NX EX), `HasUsedDailyQuota` (EXISTS), `ResetDailyQuota` (DEL). Daily action and prayer quotas. |
| `party2:daily:costume:<character_id>` | Valkey Master | `String` | Until next midnight JST (seconds) | JSON (`ActiveCostume`: `item_no`, `name`, `icon`, `expires_at`) | `internal/costume` | `SaveCostume` (SET EX), `GetCostume` (GET), `ClearCostume` (DEL). Active Oracle Shop costume rental state, auto-expiring at midnight JST or cleared upon rest / job change. |
| `party2:pvp:room:<room_id>` | Valkey Master | `String` | 30 minutes (`1800s`), refreshed on activity | JSON (`RoomDetail`: `Room`, `Members`) | `internal/pvp` | `SaveRoom` (SET EX), `GetRoom` (GET), `DeleteRoom` (DEL). Ephemeral Colosseum room state. |
| `party2:pvp:character:<character_id>` | Valkey Master | `String` | 30 minutes (`1800s`), refreshed on activity | Room ID (`string`) | `internal/pvp` | `SetCharacterRoom` (SET EX), `GetCharacterRoom` (GET), `DeleteCharacterRoom` (DEL). Single active room check. |
| `party2:pvp:rooms:active` | Valkey Master | `Sorted Set (ZSet)` | None (Dynamic index) | Member: `room_id`, Score: `UpdatedAt.Unix()` | `internal/pvp` | `SaveRoom` (ZADD), `DeleteRoom` (ZREM), `ListRooms` (ZREVRANGE + lazy ZREMRANGEBYSCORE). Active Colosseum rooms list. |
| `party2:pvp:lock:room:<room_id>` | Valkey Master | `String` | 10 seconds (`10s`) safety TTL | Lock token (`id.New()`) | `internal/pvp` | Distributed lock for atomic Colosseum room member mutations and match round execution (Issue #652). Acquired via `SET NX EX 10` with retry; released via atomic Lua script. |
| `party2:gvg:room:<room_id>` | Valkey Master | `String` | 30 minutes (`1800s`), refreshed on activity | JSON (`RoomDetail`: `Room`, `Members`) | `internal/gvg` | `SaveRoom` (SET EX), `GetRoom` (GET), `DeleteRoom` (DEL). Ephemeral GvG room state. |
| `party2:gvg:character:<character_id>` | Valkey Master | `String` | 30 minutes (`1800s`), refreshed on activity | Room ID (`string`) | `internal/gvg` | `SetCharacterRoom` (SET EX), `GetCharacterRoom` (GET), `DeleteCharacterRoom` (DEL). Single active GvG room check. |
| `party2:gvg:rooms:active` | Valkey Master | `Sorted Set (ZSet)` | None (Dynamic index) | Member: `room_id`, Score: `UpdatedAt.Unix()` | `internal/gvg` | `SaveRoom` (ZADD), `DeleteRoom` (ZREM), `ListRooms` (ZREVRANGE + lazy ZREMRANGEBYSCORE). Active GvG rooms list. |
| `party2:gvg:lock:room:<room_id>` | Valkey Master | `String` | 10 seconds (`10s`) safety TTL | Lock token (`id.New()`) | `internal/gvg` | Distributed lock for atomic GvG room member mutations and match round execution (Issue #652). Acquired via `SET NX EX 10` with retry; released via atomic Lua script. |
| `party2:eventplaza:presence` | Valkey Master | `Sorted Set (ZSet)` | 1 hour (`3600s`), sliding | Member: `character_id`, Score: `LastSeenAt.Unix()` (`float64`) | `internal/eventplaza` | `RecordPresence` (ZADD + EXPIRE 3600), `CountActiveParticipants` (lazy ZREMRANGEBYSCORE + ZCARD). Real-time Event Plaza member presence. |
| `party2:casino:room:<room_id>` | Valkey Master | `String` | 30 minutes (`1800s`), refreshed on activity | JSON (`CasinoRoomState`: room config, members, deck, turn, pot, status) | `internal/casino` | Candidate C standard pattern (Issue #635). Active store for in-flight card games (Indian Poker, High-Low, Doppelganger). |
| `party2:casino:character:<character_id>` | Valkey Master | `String` | 30 minutes (`1800s`), refreshed on activity | Room ID (`string`) | `internal/casino` | Single active casino room per character invariant check. |
| `party2:casino:rooms:active` | Valkey Master | `Sorted Set (ZSet)` | None (Dynamic index) | Member: `room_id`, Score: `UpdatedAt.Unix()` | `internal/casino` | Active casino rooms list for indexed discovery and lazy TTL pruning. |
| `party2:casino:lock:room:<room_id>` | Valkey Master | `String` | 10 seconds (`10s`) safety TTL | Lock token (`id.New()`) | `internal/casino` | Distributed lock for atomic multiplayer turn serialization and room member mutations (Issue #642). Acquired via `SET NX EX 10` with retry; released via atomic Lua script. |

Dungeon and Challenge additionally use the hash-tagged state/reward keys in
[transient-run-state.md](transient-run-state.md), with 7200s sliding lifetimes.
`party2:ranking:refresh` is a scheduled **action type**, not a separately stored
Valkey key. The withdrawn Candidate E `party2:boss:` keys and `boss_damage`
script are documented only in [the historical note](transient-boss-hp.md).

## Scheduled terminal persistence and recovery

`ValkeyRepository.Save` persists Completed/Failed before deleting the worker
lock, removing actor membership, and finally removing pending queue membership.
Each required write error propagates and stops subsequent cleanup. If the
terminal SET fails before being applied, the stored Processing payload, queue,
actor index and lock remain unchanged. A lost response can leave the write's
outcome uncertain; this ordering does not make the writes atomic.

Terminal retention uses the remaining whole seconds until the original
`RetainUntil` (normally 24 hours after completion/failure). Retrying metadata
does not start a new retention window. A zero deadline keeps a persistent
terminal payload. With less than one whole second remaining, Save first writes
a persistent terminal payload, then deletes it after lock/actor cleanup and
before queue removal. A failed payload DEL therefore leaves a terminal record
queued for recovery, rather than an unfinished record hidden from discovery.

After a successful terminal SET, later cleanup failures leave the known outcome
and its retention policy intact. `FetchDue` may return such terminal records;
Worker retries Save as metadata-only finalization, even if a lock remains,
without transitioning to Processing or invoking the feature handler. If an
already expired/deleted payload is missing, FetchDue removes only its stale
queue entry. If prolonged cleanup failure outlasts retention, the actor Set may
retain a stale ID until actor cancellation; actor observation skips missing or
terminal payloads without modifying the index.

Processing has no TTL and remains visible to actor observation. Worker never
redispatches Processing, including after lock expiry: the feature handler may
already have applied effects before final persistence failed. This state requires
feature-specific reconciliation or explicit cancellation, not automatic replay.
Worker logs persistence/cleanup errors; Save returns them to direct callers.
These guarantees concern scheduling metadata, not exactly-once feature execution
or lossless SQL/Valkey settlement across a crash.

## Approved navigation storage boundary

The [progressive observation/navigation contract](client-agent-api.md#approved-navigation-and-progressive-observation-contract)
approved in #1047 uses the existing Valkey deployment for ordinary facility,
subject and page selection. The repository is composed with the production
Valkey client and registered in the key inventory above. Saved identifiers are
at most 128 ASCII letters/digits/underscore/hyphen; the fixed record has one
subject and one page, with offset at most 1,000,000 and limit at most 100.
The reader rejects malformed/unknown fields and records exceeding 2 KiB.

Use `party2:playercontext:navigation:<character_id>` as one bounded String record,
owned through the `playercontext` navigation repository. Its TTL is seven days
(`604800s`), renewed on successful navigation/selection writes, without renewal
or creation on GET. Selection is ephemeral primary interaction state, not a
SQL-backed cache; loss/expiry supplies the default town selection. Concurrent
navigation uses the last successful write. No new SQL table, arbitrary form
storage, history stack or distributed game-state revision is needed.

Room membership, active run buffers and sleep remain in their existing owning
keys. Observe these authoritative facts to compose active scenes; never duplicate
them in navigation or end/settle an activity because its selection key is absent.
Using a separate key separates ownership and lifetime within the same deployment;
it does not introduce another infrastructure service or cross-store transaction.

## Index lifetimes: TTL-Scored Sorted Set with Lazy Purging

Session tokens have independent expiry; their index scores are `ExpiresAt` and
reads/writes purge expired members with `ZREMRANGEBYSCORE`. Room indices can
instead score last activity and purge entries older than the owner's TTL.
Do not confuse these expiration indices with `party2:scheduled:pending`, whose
score is **execution time**, or with the unfinished-action actor Set. Purging
overdue scheduled work as if it expired would lose pending actions.

`WRONGTYPE` migration handling is owner-specific: the session adapter can
replace a legacy Set index and revoke its indexed tokens. This is not permission
to blindly delete arbitrary financial/session state on any type error.

Candidate D: In-Progress Run Buffers currently store provisional rewards in
hashes (including JSON item arrays), not in hypothetical `active_nodes` or
`turn_history` ZSets. Matchmaking/invitation patterns need an actual feature
requirement before adding keys.

## Lua Script Registry

The following registry describes current scripts. Exact arguments and error
codes are owned by the embedded `.lua` source; this table is navigation, not
a claim that every service operation consists of one atomic script.

| Script Identifier | Source Location | Target Keys (`KEYS[...]`) | Parameters (`ARGV[...]`) | Operation Description & Invariant Guarantees |
| :--- | :--- | :--- | :--- | :--- |
| `party_add_member` | `internal/party/valkey_repository.go` (`addMemberLua`) | `KEYS[1]`: `lobbyKey`<br>`KEYS[2]`: `characterKey`<br>`KEYS[3]`: `readyKey` | `ARGV[1]`: Member JSON<br>`ARGV[2]`: Character ID<br>`ARGV[3]`: Party ID<br>`ARGV[4]`: Lobby TTL<br>`ARGV[5]`: Ready TTL<br>`ARGV[6]`: Ready Flag | Atomically adds a member to the party lobby roster if `status != 'disbanded'` and `current_members < max_members`, sets character-to-party mapping and ready countdown key with TTL. Prevents lobby overfilling races under concurrent join attempts. |
| `party_remove_member` | `internal/party/valkey_repository.go` (`removeMemberLua`) | `KEYS[1]`: `lobbyKey`<br>`KEYS[2]`: `characterKey`<br>`KEYS[3]`: `readyKey` | `ARGV[1]`: Character ID<br>`ARGV[2]`: Lobby TTL | Atomically removes a member from the party lobby roster, deletes their character-to-party reverse index and ready check countdown key, and refreshes lobby TTL. |
| `party_update_member_ready` | `internal/party/valkey_repository.go` (`updateMemberReadyLua`) | `KEYS[1]`: `lobbyKey`<br>`KEYS[2]`: `readyKey` | `ARGV[1]`: Character ID<br>`ARGV[2]`: Ready Flag (`"1"` or `"0"`)<br>`ARGV[3]`: Lobby TTL<br>`ARGV[4]`: Ready TTL | Atomically toggles a member's ready flag in the lobby state and synchronizes the 60-second ready countdown key (`SET EX 60` or `DEL`). Rejects if character is not a member (`ERR_CHAR_NOT_IN_PARTY`). |
| `party_update_party` | `internal/party/valkey_repository.go` (`updatePartyLua`) | `KEYS[1]`: `lobbyKey`<br>`KEYS[2]`: `lobbiesIndexKey` | `ARGV[1]`: Party JSON<br>`ARGV[2]`: Lobby TTL<br>`ARGV[3]`: Status (`"recruiting"`, `"disbanded"`, `"completed"`)<br>`ARGV[4]`: Party ID | Atomically updates party configuration (e.g. stage, speed, max members) and removes the party from the recruiting index set (`ZREM`) if transitioned to `disbanded` or `completed`. |
| `dungeon_step` | `internal/dungeon/lua/dungeon_step.lua` (`dungeonStepLua`) | `KEYS[1]`: `party2:dungeon:{char:<id>}:state`<br>`KEYS[2]`: `party2:dungeon:{char:<id>}:rewards` | `ARGV[1]`: Expected Expedition ID<br>`ARGV[2]`: Floor<br>`ARGV[3]`: Pos X<br>`ARGV[4]`: Pos Y<br>`ARGV[5]`: HP Delta<br>`ARGV[6]`: Turns Delta<br>`ARGV[7]`: Exp Delta<br>`ARGV[8]`: Gold Delta<br>`ARGV[9]`: Medals Delta<br>`ARGV[10]`: Item ID<br>`ARGV[11]`: Timestamp<br>`ARGV[12]`: TTL (7200s) | Atomically advances exploration coordinates, updates HP/turns budget, and buffers provisional room loot without touching MariaDB during active run. |
| `challenge_advance_round` | `internal/challenge/lua/challenge_round.lua` (`challengeRoundLua`) | `KEYS[1]`: `party2:challenge:{char:<id>}:session`<br>`KEYS[2]`: `party2:challenge:{char:<id>}:rewards` | `ARGV[1]`: Expected Session ID<br>`ARGV[2]`: Surviving HP<br>`ARGV[3]`: Exp Delta<br>`ARGV[4]`: Gold Delta<br>`ARGV[5]`: Item ID<br>`ARGV[6]`: Timestamp<br>`ARGV[7]`: TTL (7200s) | Atomically advances endurance challenge wave round, persists surviving HP, and accumulates wave rewards without touching MariaDB during active run. |
| `casino_release_room_lock` | `internal/casino/lua/release_room_lock.lua` (`releaseRoomLockLua`) | `KEYS[1]`: `lockKey` (`party2:casino:lock:room:<room_id>`) | `ARGV[1]`: Lock Token (`token`) | Atomically deletes the room lock key if and only if its value matches the caller's lock token, preventing accidental release of expired or re-acquired locks. |
| `pvp_release_room_lock` | `internal/pvp/lua/release_room_lock.lua` (`releaseRoomLockLua`) | `KEYS[1]`: `lockKey` (`party2:pvp:lock:room:<room_id>`) | `ARGV[1]`: Lock Token (`token`) | Atomically deletes the room lock key if and only if its value matches the caller's lock token, preventing accidental release of expired or re-acquired locks (Issue #652). |
| `gvg_release_room_lock` | `internal/gvg/lua/release_room_lock.lua` (`releaseRoomLockLua`) | `KEYS[1]`: `lockKey` (`party2:gvg:lock:room:<room_id>`) | `ARGV[1]`: Lock Token (`token`) | Atomically deletes the room lock key if and only if its value matches the caller's lock token, preventing accidental release of expired or re-acquired locks (Issue #652). |

Lua scripts live in feature `lua/` directories and use `go:embed`. Party scripts
operate on bounded member JSON; Dungeon/Challenge reward arrays also use JSON.
Payload limits, complexity, and execution budgets require review/measurement.
Do not claim a sub-millisecond guarantee from embedding or command linting.

## Mechanical verification

`internal/architecture/valkey_lint_test.go` checks registered string prefixes,
selected required documentation terms, banned commands, and Lua embedding.
It does not validate TTL values, exhaustively discover dynamic keys, verify
cluster colocation, prove crash recovery, or benchmark runtime complexity.
Run the architectural tests and `make check` after keyspace changes.
