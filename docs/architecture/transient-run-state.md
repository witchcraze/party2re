# Transient Run and Session State (Candidates C and D)

This document distinguishes implemented storage boundaries from the planned
normal-adventure boundary below. The legacy CGI owns turn, timeout, reward, and penalty
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

PlayerContext observes these owners through public service ports. Party's actor
reader propagates lobby/member storage failures and performs no stale-index
deletion; missing live state remains absent. Casino discovers actual admission
through its character-room mapping, then uses its authorized masked projection.
Navigation has no authority over these lobbies and never joins or leaves them.

Lobby TTL is an implementation lifetime, not proof of legacy timeout parity.
Do not invent generic auto-fold, forfeiture/refund, turn-history keys, or a
60-second post-game retention policy. Each owning game must define and verify
its actual transitions against the original CGI.

### GvG reward publication

`StandingRepository.CommitRoundTransition` delegates to the GvG SQL repository.
One transaction locks the room's `gvg_match_transitions` record, then standing
rows in ascending guild ID order, and commits round GP, any final settlement,
and the resulting room/outcome together. The room ID and resolved round identify
the claim; a completed match rejects further reward claims at any round.
The record retains only the latest committed transition for each room.

`AdvanceRound` checks this record before combat. When a cached room is behind,
it republishes the committed room and returns the saved outcome without rerunning
combat or awarding rewards again. Valkey publication is outside the SQL transaction.
`ValkeyRoomRepository.SaveRoom` updates the candidate index before writing the room
with its 30-minute TTL. An index-command rejection cannot advance the room; a
rejected room write may leave the index ahead, while the room remains available
for replay from SQL. Listing loads the authoritative room and prunes index entries
by age; it does not erase a missing body that may still be publishing.

This does not make every failed request safe to retry. If the room write completed
but its reply or the HTTP response was lost, the next request can observe the next
round and start it. Operation identity for that ambiguity is undecided in
[#1573](https://github.com/witchcraze/party2re/issues/1573). Lobby deletion or expiration
does not delete the durable guard. These records have no automatic expiry, and a
missing Valkey room is not reconstructed by this operation.

This guarantee applies to transitions committed through the guard. It cannot
retroactively identify rewards committed by an older deployment. Reward amounts
and recipients remain a separate [GvG domain contract](../design/gvg.md).

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

Active Dungeon/Challenge observations propagate required state and reward-read
failures. A terminal buffer still present in Valkey remains observable until
owner cleanup; reading it never retries settlement or refreshes lifetime.

Cooperative entry resolves the saved, ready roster through Party's public
`WithReadyRoster` handoff, under its existing lobby lock. Lobby membership and
ready writers share that lock. Optional `member_ids` asserts the complete roster;
it cannot add, omit or duplicate participants. Consent belongs to each joining
character, including characters owned by different accounts. The composition
root then locks every participating Character in ascending ID order in a SQL
transaction shared by solo and party Dungeon/Challenge starts. It checks all
members for existing run buffers, sleep/pending wake, unfinished scheduled work,
PvP/GvG admission, unconsciousness and exhaustion before starting.
The ordinary Party adventure start reuses that activity check so it cannot
bypass an existing Dungeon/Challenge enrollment.

Each run retains one canonical leader buffer. A per-member reference discovers
that buffer through its immutable run ID and saved roster, independently of the
Party lobby's shorter TTL. Members can observe their current run; advance and
Dungeon escape remain leader operations. PlayerContext exposes member activity
without leader continuation controls. GET neither renews nor repairs state.
References have an explicit cleanup lifecycle, detailed in the
[keyspace owner](valkey-keyspace.md), and do not provide SQL/Valkey atomicity or
change the existing settlement guarantees below.

### Start outcome after publication (#1493)

**Decision status: approved 2026-10-18 in #1493.** Publication to Valkey precedes
the shared SQL transaction's commit, which is not atomic with it. An admission
error raised after the start callback succeeded therefore never implies that no
run exists. The owning service reconciles it by the immutable run ID, outside the
failed transaction and detached from request cancellation:

| Feature | Evidence | Result |
|---|---|---|
| Dungeon (no SQL writes) | Leader's canonical buffer still holds the run ID | Success; the run is the start result |
| Challenge | `challenge_sessions` row for the run ID exists | Success (commit acknowledgement was lost) |
| Challenge | Row confirmed absent (`ErrSessionNotFound`) | Aborted: remove the run and its member references atomically by run ID; return the original error on clean removal, or `ErrStartOutcomeUnknown` wrapping both causes if cleanup fails |
| Either | Evidence unreadable or run no longer verifiable | Preserve all state; return `ErrStartOutcomeUnknown` wrapping the cause |

The Challenge start inserts its SQL row before publishing, so an insert failure
never publishes, and a failed publication write discards its partial run using a
detached timeout context (reporting `ErrStartOutcomeUnknown` wrapping both the save
and cleanup errors if cleanup fails). A failed abort cleanup similarly returns
`ErrStartOutcomeUnknown` wrapping both the admission and cleanup errors, and leaves
the run discoverable; nothing deletes character keys blindly.
HTTP maps an unknown outcome to `500` with only the sentinel message. Clients observe the active run through the
owned readers and never replay the POST. Limitation: an absent row is treated as an
abort, so a commit still in flight server-side when it is read is not excluded;
excluding it needs durable start receipts like the Adventure contract below.
Terminal payout/history idempotency remains separate (#1205, #1211).

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

## Normal Adventure: durable command boundary (#1325)

**Decision status: selected in #1325, user-approved on 2026-10-11.** SQL is the
authority for enrolled normal runs, consumption, earnings and receipts; Party
Valkey remains waiting admission. The explicit idle cleanup and one-hour penalty
anchor in the gameplay owner were approved in the same session. These are
implementation targets, not an implemented store or blanket approval of
unreconciled skill/gameplay changes. Current Adventure
remains request-local batch execution. Candidate D's loss exposure does not
authorize losing normal-adventure earnings. Gameplay and timeout evidence live
in [Adventure](../design/adventure.md#resumable-normal-run-transitions);
shared effect state lives in [Battle](../design/battle.md#explicit-action-resumption-requirements).

### Authority and ownership

Adventure owns one SQL-authoritative normal run for solo and cooperative play:
immutable run identity, version, phase, ordered roster/roles, encounter state,
actor/NPC eligibility timestamps, per-recipient earnings and exit facts.
SQL also owns active character enrollment and durable command/settlement
receipts. Enforce at most one unfinished normal-run enrollment per character,
including an exit awaiting mandatory processing. Exact tables, encoding and Go
signatures belong to implementation; old SQL lobby tables are not reused.

Party retains waiting-lobby admission in Valkey. At enrollment, obtain the
existing lobby coordination lock, validate its roster/readiness and freeze the
admission snapshot. All roster writers must honor that handoff. In a bounded
SQL transaction create the run and enroll characters after validating their
current eligibility. Persist a unique lobby/admission generation mapping to the
run so repeated starts find the same run, including after a lost response.
The generation must survive a failed Valkey status update; a retry cannot mint
another generation merely because the lobby still says recruiting. Resolve
existing enrollment from SQL before accepting lobby mutations or another start.
On SQL failure no run exists; on SQL success SQL owns the enrolled actors even
if Valkey cleanup fails or the lobby expires. Close/remove only the matching
admission generation after commit. Do not hold the lobby lock between requests
or reacquire it from inside a run transaction. Any lobby replacement policy
must preserve this one-way handoff, not infer admission from an expired key.

PlayerContext queries Adventure through an injected public actor reader; it
does not scan Party lobbies or inspect Adventure's repositories. SQL enrollment
is the discovery source; a cache/index may accelerate reads only when it can be
rebuilt without losing ownership. Persisted run facts prevent another activity
from overwriting a participant's live vitality/loadout. Required reader errors
fail closed. Other actors' public combat projections are available through the
run, never unrestricted private Character records or inventory contents.
Admission and conflicting mutation services must recheck that ownership, not
rely only on a PlayerContext menu hiding their entry controls.

### One command, one bounded SQL transaction

1. Authenticate the owned actor at both transport and service boundaries. Scope
   receipt lookup by immutable run/command identity and recorded executing actor;
   verify that actor belongs to the authenticated player before exposing results.
2. Check canonical input against the actor-owned durable receipt before requiring
   a live run, current membership or version. A committed duplicate returns its
   original result even after exit or run deletion; different input with the same
   identity conflicts. Reject another actor's receipt access. Receipt identity,
   input and ownership evidence survive run deletion without a live-run join or
   cascading deletion.
3. Only for a new command, lock the shared run as Rank 0, then every affected
   Character at Rank 2 in ascending ID order. Acquire all required inventory/equipment (Rank 3), jobs
   (Rank 4), depots (Rank 5) and secondary records (Rank 8) in the existing
   hierarchy, sorting same-rank owners. Declare the complete affected set before
   invoking adapters; never settle A through Rank 8 then start locking B at
   Rank 2. Do not acquire a run lock from a character-locked callback. Register
   the new repository methods with the lock linter during implementation.
   Recheck the receipt after serialization to catch a concurrent commit before
   validating new-command preconditions. If the run disappeared during lookup,
   resolve the retained receipt first; never recreate a run to retry a command.
4. For a new command validate current membership/role under lock, expected
   version, phase, action/target/resources and server-clock eligibility.
   Apply the bounded player action and applicable
   NPC response. Persist the whole resulting encounter, command consumption,
   recipient earnings/deliveries and receipt with the new version atomically.
   Rejected input does not consume items, advance a clock or mutate the run.
5. Commit before optional Valkey cleanup, scene refresh or Replay publication.
   Return the recorded domain result; observation failure cannot undo success.

Use injected `TransactionProvider` for the shared aggregate and ambient
repositories; reuse `TransactionRunner` for applicable single-character asset
work within that scope. The participant builder and settlement adapter must use
the same transaction and already locked data. Core battle has no store, clock
service, feature import or transaction. A shared process mutex is insufficient
across servers. No transaction waits for a future request or action deadline.

Consumption references concrete owned item instances, including equipped
slots. Apply it in the command transaction and carry the consumed state into
future encounters; final settlement must not debit it again (#1197).
Chest claim, allowance/key debit and actual delivery disposition are atomic
(#1196). Preserve the delivery rules rather than granting all chests on exit.
Earned rewards are durable per recipient at their source operation (#1198);
already applied assets and still-pending entitlements must be distinguishable.
Ordinary earned assets cannot be erased by later defeat, exit or storage cleanup.
Mode-specific conditional earnings require their verified release condition.
Do not postpone an observable legacy grant until exit merely for convenience.

### Individual settlement and recovery

Use a unique settlement identity per run and enrolled recipient. Exit applies
only outstanding recipient effects and writes that recipient's history,
settlement receipt, roster removal/leadership change and active-enrollment
release in the same transaction (#1221/#1222/#1224). Keep original roster order
separate from lock order. Cooperative multipliers are calculated once at their
verified earning boundary (#1225); the common settler must not apply them again.
History links the immutable run/recipient identity; a failed history write rolls
back its coupled asset effects. Room disposal waits until all mandatory
recipient processing has a durable disposition. Keep receipts/history after
physical active-state cleanup so old retries cannot create another payout.

| Failure boundary | Required behavior |
| --- | --- |
| Before SQL commit, including state/asset/history write failure | Roll back the entire command; no success receipt or partial consumption |
| Commit result unknown or response lost | Observe owned command receipt from SQL; never infer failure from missing HTTP response or reroll a committed action |
| Restart after commit | Resume persisted encounter/version or return recorded command/exit result; no replay of effects |
| Commit succeeded, lobby cleanup/refresh/Replay failed | Preserve SQL success; retry only idempotent optional work for the matching identity |
| SQL unavailable or unsupported state encoding | Report recovery unavailable; do not rebuild from HP alone, fall back to memory or abandon earnings |
| Idle deadline passed | Owner command performs the verified idle transition under the same locks; GET only reports due/pending recovery |

An absent receipt is only definitive after the potentially committing
transaction has resolved. New-command execution serializes on the live run and
unique command identity; finalized receipt lookup/deduplication remains possible
without the run. Clients still use GET-only recovery, without
automatic POST replay; an explicit retry with the same identity can be safe
only once the new receipt contract is actually implemented.

Mandatory assets, history and penalties cannot rely on the in-process async
dispatcher or existing best-effort hooks. If a mandatory effect needs another
store, retain durable pending work and an idempotent completion identity;
ownership remains blocked until that work finishes. Optional Replay failure
must not rerun combat or settlement. No generic event/storage framework or
cross-store atomicity claim is introduced by this contract.
