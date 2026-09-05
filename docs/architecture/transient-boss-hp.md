# Transient World Boss HP Architecture & Settlement Boundary (Candidate E)

This document establishes the architectural evaluation, schema design, Lua script contracts, crash-recovery semantics, and persistence boundaries for **Candidate E: World Boss Real-Time Shared HP**, per **RFC #356** and [`.agents/rules/05-database-and-caching.md`](../../.agents/rules/05-database-and-caching.md).

---

## 1. Executive Summary & Problem Context

In Party2 Re, World Bosses represent global raid encounters where dozens or hundreds of players collaborate simultaneously to defeat an imposing monster with massive HP (e.g. 50,000 to 1,000,000+ HP).

### The Relational Contention Bottleneck
Under a naive relational database design:
- Every player attack round performs a synchronous `SELECT ... FOR UPDATE` followed by `UPDATE bosses SET hp = hp - ? WHERE id = ?`.
- When 100+ players attack simultaneously at 1–5 attacks per second, all requests serialize on the exact same MariaDB row lock.
- **Lock Wait Timeouts (Error 1205)** and severe connection pool exhaustion occur, degrading responsiveness for the entire application.
- Primitive atomic increments (such as raw SQL `UPDATE bosses SET hp = GREATEST(0, hp - ?)`) cannot atomically determine which attacker landed the true killing blow or calculate overkill prevention without distributed locks.

### The Architectural Solution: Candidate E (Valkey Master via Atomic Lua)
Per **RFC #356** (Persistence Boundary Guidelines), volatile in-combat health pools that experience high-frequency concurrent writes belong in **Valkey Master**:
1. **Zero Row-Lock Contention**: Damage ticks execute entirely in Valkey Master memory in **sub-millisecond time (< 50 µs)** without touching MariaDB.
2. **Atomic Overkill Prevention**: Lua script evaluates `math.min(current_hp, incoming_damage)` atomically, preventing negative HP.
3. **Deterministic Killer Election**: The exact hit that reduces HP from `> 0` to `== 0` transitions status to `"defeated"`, records the killer ID, and designates that single attacker as the settlement coordinator.
4. **Contributor Tallying**: Atomic hash increments (`HINCRBY`) record cumulative damage per contributor for MVP determination and proportional reward distribution.
5. **Two-Phase Settlement Boundary**: MariaDB Master is touched **only once** upon boss defeat via an atomic transaction (`RunInTx`).

---

## 2. Key Architecture & Cluster Hash Tagging

### 2.1 Keyspace Specification
All Valkey keys associated with a World Boss raid MUST use the `{boss:<boss_id>}` **Cluster Hash Tag** to guarantee execution on the exact same Valkey cluster slot:

| Key Pattern | Data Type | Expiration Policy | Purpose / Contents |
| :--- | :--- | :--- | :--- |
| `party2:boss:{boss:<boss_id>}:hp` | `String` (Integer) | 2 hours (`7200s`), sliding | Remaining boss HP counter. |
| `party2:boss:{boss:<boss_id>}:status` | `String` | 2 hours (`7200s`), sliding | Encounter status: `"active"`, `"defeated"`, `"settled"`. |
| `party2:boss:{boss:<boss_id>}:contributors` | `Hash` | 2 hours (`7200s`), sliding | Field: `character_id`, Value: cumulative damage dealt. |
| `party2:boss:{boss:<boss_id>}:killer` | `String` | 2 hours (`7200s`), sliding | Character ID of elected killer who landed the final blow. |
| `party2:boss:{boss:<boss_id>}:run_id` | `String` | 2 hours (`7200s`), sliding | Unique execution UUID for idempotent settlement. |

---

## 3. Atomic Lua Script Contract (`boss_damage.lua`)

Preloaded via `valkey.NewLuaScript`:

```text
KEYS[1]: party2:boss:{boss:<boss_id>}:hp
KEYS[2]: party2:boss:{boss:<boss_id>}:status
KEYS[3]: party2:boss:{boss:<boss_id>}:contributors
KEYS[4]: party2:boss:{boss:<boss_id>}:killer
KEYS[5]: party2:boss:{boss:<boss_id>}:run_id

ARGV[1]: attacker_id (string)
ARGV[2]: incoming_damage (integer)
ARGV[3]: ttl_seconds (integer, e.g. 7200)
```

### State Transitions & Guarantees
```text
                             Incoming Attack(ARGV[1], ARGV[2])
                                            │
                                            ▼
                           Is boss status 'defeated' or 'settled'?
                                     /              \
                                   YES              NO
                                   /                  \
                                  ▼                    ▼
                      Return 'already_dead'      Current HP <= 0?
                      (damage = 0, no credit)    /              \
                                               YES              NO
                                               /                  \
                                              ▼                    ▼
                                  Return 'already_dead'    Calculate actual damage:
                                                           math.min(hp, incoming_dmg)
                                                                   │
                                                                   ▼
                                                           new_hp = hp - actual_dmg
                                                           HINCRBY contributors actual_dmg
                                                           Refresh 2h TTL on all keys
                                                                   │
                                                                   ▼
                                                            new_hp == 0?
                                                            /          \
                                                          YES          NO
                                                          /              \
                                                         ▼                ▼
                                            Set status = 'defeated'   Return 'hit'
                                            Set killer = attacker_id  (actual_dmg, new_hp)
                                            Return 'killed'
                                            (Designated Coordinator)
```

---

## 4. Two-Phase Settlement Boundary & Exactly-Once Semantics

### 4.1 Coordinator Execution
1. The single attacker whose Lua response is `status: "killed"` is elected as the settlement coordinator.
2. The coordinator invokes `settleDefeatedBoss`:
   - Checks `settlementRepo.IsRunSettled(ctx, run_id)` (idempotency guard).
   - Fetches all participant damage tallies from `party2:boss:{boss:<boss_id>}:contributors`.
   - Identifies the MVP (highest contributor) and Last-Hit (`killer_id`).
   - Executes MariaDB Unit of Work (`RunInTx`):
     - Awards MVP bonus, Last-Hit bonus, and participation rewards.
     - Logs permanent completion in audit tables (`boss_challenge_history`).
   - On successful transaction commit, transitions Valkey status to `"settled"`.
   - Triggers event plaza victory banquet announcements (`VictoryBanquetHook`).

### 4.2 Crash Recovery & Idempotent Reconciliation
If the coordinator application process crashes or loses network connectivity immediately after the boss is marked `"defeated"` but before the MariaDB transaction commits:
1. The boss remains in `party2:boss:{boss:<boss_id>}:status = "defeated"` with `killer` and `run_id` securely preserved in Valkey Master.
2. A background worker or query invokes `coordinator.ReconcileUnsettledBoss(ctx, bossID)`.
3. The reconciliation worker safely completes settlement using the recorded `killer` and `contributors`.
4. If MariaDB already committed before the crash, `IsRunSettled(ctx, run_id)` detects the existing durable record and simply marks Valkey as `"settled"` without double-granting loot.

---

## 5. In-Memory Fallback Parity

When Valkey is not configured (`valkey.Client == nil`), `ValkeyRaidRepository` falls back to an internal thread-safe in-memory store (`sync.RWMutex`):
- Provides 100% behavioral parity: identical overkill prevention, killer election, contributor tracking, and state transitions.
- Allows unit tests and development environments to run fast and reliably without external infrastructure dependencies.

---

## 6. Benchmark & Concurrency Verification

Benchmarked on AMD Ryzen 7 5700U (16 threads):

```text
BenchmarkApplyDamage_Memory-16    6,702,422 ops    208.1 ns/op     0 B/op     0 allocs/op
BenchmarkApplyDamage_Valkey-16       21,613 ops     48.6 µs/op  1173 B/op    24 allocs/op
```

- **Valkey Lua Throughput**: ~20,000+ operations/second per client connection over loopback TCP with 48.6 µs execution latency.
- **In-Memory Throughput**: ~4.8 million operations/second.
- **Concurrency Verification**: High-concurrency stress tests with 120 simultaneous raiders hammering 50,000 HP confirmed:
  - Exactly 1 killer elected.
  - Zero overkill: total damage dealt strictly equals initial HP.
  - Exactly 1 MariaDB settlement execution.
  - Zero lock contention or database timeouts.
