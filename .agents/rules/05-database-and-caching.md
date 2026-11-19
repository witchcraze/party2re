---
name: Database and Caching Strategy
description: Rules for transaction boundaries, locking, migrations, MariaDB vs Valkey authority, Valkey usage and store lifecycle.
---

# Database and Caching Strategy

Rationale and examples: [`cross-domain-primitives.md`](../../docs/architecture/cross-domain-primitives.md), [`valkey-keyspace.md`](../../docs/architecture/valkey-keyspace.md), [`transient-run-state.md`](../../docs/architecture/transient-run-state.md).

## 1. Transactions and locking
- Read-modify-write of critical state (gold, inventory, progression) MUST run in one transaction. Prefer atomic SQL (`UPDATE ... SET money = money - X WHERE money >= X`, UPSERT); otherwise lock with `SELECT ... FOR UPDATE` in the read phase.
- Repositories MUST use `database.ExecutorFromContext(ctx, r.db)` and wrap multi-statement work in `database.RunInTx(ctx, r.db, ...)`, which joins an ambient transaction. MUST NOT call `r.db.BeginTx` (`tx_lint_test.go`). Root transactions use `READ COMMITTED`.
- Locks MUST be acquired in ascending rank order (`lock_hierarchy_lint_test.go`, `make lock-lint`):

  | Rank | Rows |
  |---|---|
  | 0 | Shared peer rows: flea-market listings/capacity, store name/town capacity/sales, house town capacity, contest and takarakuji rounds |
  | 1 | `players` |
  | 2 | `characters` (multiple: ascending ID); bank savings are `characters.deposit` |
  | 3 | `inventory_items`, `equipment_slots` |
  | 4 | `character_jobs`, `character_job_masteries` |
  | 5 | `character_depots`, `depot_items` |
  | 6 | reserved `bankRepo.*ForUpdate` classification; MUST NOT infer bank tables |
  | 7 | `guilds`, `guild_members` (multiple: ascending ID) |
  | 8 | secondary feature records: plantation plots, black-market points, character monsters |

- Banned: reading outside a transaction then saving the mutated struct; locking a child (e.g. inventory) before its character; locking same-table rows in non-ascending order; wipe-and-reinsert of collections (use targeted UPSERT/DELETE); discarding repository/store errors (see `01-development-workflow.md` §4).

## 2. Migrations
Until a migration tool is adopted, `scripts/migrate.sh` pipes each file whole to MariaDB:
- MUST NOT use `-- +migrate Up/Down` annotations.
- Each file MUST end with `INSERT IGNORE INTO schema_migrations (version) VALUES ('<file-name>');`.

## 3. Storage authority
- MariaDB is the sole authority for durable wealth, inventory, progression and audit records. Mutation frequency alone never justifies Valkey.
- Valkey Master (no SQL table) is only for naturally expiring or lifecycle-managed state. MUST NOT be the sole authority for durable wealth/progression; settlement and recovery MUST be verified separately.
- Valkey Cache holds projections rebuildable from MariaDB. Do not cache static master data or add caching without a measured SQL bottleneck; do not add Valkey without a concrete requirement.
- Lobbies and run buffers follow [`transient-run-state.md`](../../docs/architecture/transient-run-state.md). MUST NOT reintroduce the withdrawn Candidate E World Boss design; boss behavior is [`docs/design/boss.md`](../../docs/design/boss.md).

## 4. Valkey usage
- Keys MUST follow the `valkey-keyspace.md` taxonomy (`party2:<namespace>:<entity>[:<id>]`, `valkey_lint_test.go`) and have a TTL or a documented removal lifecycle. Unfinished scheduled work MUST stay until terminal processing or cancellation.
- MUST NOT use `KEYS *` or unindexed wildcard `SCAN`; index with Sets.
- Sets only when members share the parent's lifetime. Independently expiring members (sessions, tokens) MUST use a TTL-scored sorted set (score = expiry Unix seconds) purged lazily with `ZREMRANGEBYSCORE key -inf <now>` on access; no polling daemons. ScheduledAction actor Sets are lifecycle-managed, not TTL-scored.
- Lua only for atomic conditional transitions native commands cannot do: < 1ms, O(1)/O(log N), no unbounded loops, scans, blocking calls or large JSON (party rosters stay bounded by lobby capacity). Multi-key scripts MUST share a `{...}` hash tag. Scripts live in `<pkg>/lua/*.lua`, embedded with `//go:embed`, loaded with `valkey.NewLuaScript`.
- In-memory test adapters in the owning package MUST match the script's atomic logic and errors. No production fallback on connectivity failure.

## 5. Ownership scoping and conditional updates
- SQL changing a player/character sub-resource MUST include the owner predicate (`WHERE id = ? AND character_id = ?`), in addition to HTTP authorization. Distinguish `ErrNotFound`, `ErrForbidden` and lifecycle errors.
- Shared lifecycle transitions (listings, sales, rounds) MUST use a status guard (`WHERE id = ? AND status = 'active'`) and treat `RowsAffected() == 0` as a conflict.
- Lock a shared listing/sale (Rank 0) before characters and assets; direct auction/depot transfers start with ascending character locks. MUST NOT invent bidding, buyout, parcel or guild-donation mechanics.

## 6. Concurrency stress tests
Changes to shared-resource mutations, P2P transfers, purchases or contended claims (auction/depot, flea market, store, guild points, contest/lottery) MUST add a paired stress test using `testutil.RunConcurrentStressTest`, `RunRace` or `RunRace2` with `GetStressConfig()`. It MUST assert zero deadlocks and asset conservation. Use the central entity factories in `internal/database/testing.go` with unique explicit names when sharing a database.

## 7. Startup and teardown
- Production bootstrap MUST ping MariaDB and Valkey with timeouts before serving, and MUST abort with a non-zero exit on failure. MUST NOT fall back to in-memory stores.
- On aborted startup, release every allocated pool, socket, worker and listener.
- In-memory adapters are for explicitly injected test dependencies only.
