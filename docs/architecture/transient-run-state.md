# Transient Run and Session State (Candidates C and D)

This document describes implemented storage boundaries and their limitations,
not new game mechanics. The legacy CGI owns turn, timeout, reward, and penalty
rules. The storage policy is in [the database rules](../../.agents/rules/05-database-and-caching.md).

## Candidate C: Ephemeral Turn & Session Lobby

| Owner | Current store | Idle lifetime |
|---|---|---|
| Party | `internal/party/valkey_repository.go` | 900s |
| PvP | `internal/pvp/repository.go` | 1800s |
| GvG | `internal/gvg/repository.go` | 1800s |
| Casino | `internal/casino/valkey_room_repository.go` | 1800s |

PvP/GvG/Casino use `party2:<domain>:room:<room_id>`,
`party2:<domain>:rooms:active`, and character reverse mappings. Party uses its
own lobby/ready keys. Room lists have activity-scored indices and lazy pruning;
these are indexed operations, not an O(1) full-list guarantee.
See [keyspace](valkey-keyspace.md) for precise names and coordination locks.

Casino migration #635 is complete; former `casino_rooms`/member tables and the
old SQL room repository are historical, not future migration targets. Durable
`casino_accounts` remains in SQL. Composition selects live Valkey stores after
startup checks. Factory nil-client branches exist for test injection; they are
not permitted runtime connectivity-error fallback.

Lobby TTL is an implementation lifetime, not proof of legacy timeout parity.
Do not invent generic auto-fold, forfeiture/refund, turn-history keys, or a
60-second post-game retention policy. Each owning game must define and verify
its actual transitions against the original CGI.

## Candidate D: In-Progress Run Buffers

Dungeon and Challenge are wired to Valkey working stores (#404/#405). Their
current key patterns use a Cluster Hash Tag `{char:<character_id>}`:

| Key | Type | Lifetime |
|---|---|---|
| `party2:dungeon:{char:<character_id>}:state` | Hash | 7200s sliding |
| `party2:dungeon:{char:<character_id>}:rewards` | Hash (items as JSON array) | 7200s sliding |
| `party2:dungeon:{char:<character_id>}:revealed` | Set of visited coordinates | 7200s |
| `party2:challenge:{char:<character_id>}:session` | Hash | 7200s sliding |
| `party2:challenge:{char:<character_id>}:rewards` | Hash (items as JSON array) | 7200s sliding |

`dungeon_step` (`internal/dungeon/lua/dungeon_step.lua`) verifies expedition ID
and active state, updates coordinates/HP/turns, accumulates provisional loot,
and refreshes state/reward TTLs. `challenge_advance_round`
(`internal/challenge/lua/challenge_round.lua`) verifies session ID and active
state, advances the round, updates HP/rewards, and refreshes TTLs. Exact fields,
argument order, errors, and return tuples are defined in the embedded sources.
These scripts protect their own transitions; application reads, combat
resolution, and SQL settlement are separate operations.

## Two-Phase Settlement

Valkey holds provisional rewards during a run. On clear, exit, cashout, or
defeat, the feature applies its legacy reward/penalty rules in a SQL transaction
and records durable history. Cleanup follows successful settlement.
The phrase Two-Phase Settlement describes this application boundary, **not a
distributed two-phase commit across MariaDB and Valkey**.

History and run/session identifiers must protect retries from duplicate grants.
Do not assert that an endpoint is exactly-once solely because it uses `RunInTx`.
Inspect the owning finalizer, durable idempotency guard, and failure tests when
changing this boundary. Durable records include `character_dungeon_records`,
`dungeon_expedition_history`, and `character_challenge_records`; legacy SQL
active-state migrations are historical work, not a remaining rollout plan.

## Crash Recovery

An application restart can resume a run whose Valkey state is still present.
TTL expiry or Valkey data loss can discard unfinished buffers. AOF `everysec`
reduces persistence exposure but is not a hard maximum-loss guarantee.
The failure window between SQL commit and Valkey cleanup requires verified
durable idempotency; do not promise a reconciler or automatic refund without
an implementation and tests. Storage migration alone proves neither legacy
parity nor complete cross-store recovery.

## In-Memory Fallback Parity and verification

Memory test adapters are owned by their feature packages. There is no shared
`internal/valkey/memory.go` implementation. Equivalent transitions and errors
must be checked against live Valkey; mocks do not certify Lua behavior.
Production requires real MariaDB and Valkey connections before serving traffic.

AST tests verify selected terms, script embedding, and banned commands. They do
not prove payload bounds, sub-millisecond latency, all service race conditions,
or full settlement recovery. Use feature integration and conservation tests
when changing run transitions; `make check` remains the required repository gate.
