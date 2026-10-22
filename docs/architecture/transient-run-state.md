# Transient Run and Session State (Candidates C and D)

This document distinguishes implemented storage boundaries from the approved
Challenge exit and normal-adventure targets below. The legacy CGI owns turn,
timeout, reward, and penalty rules. The storage policy is in
[the database rules](../../.agents/rules/05-database-and-caching.md).

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

### PvP financial publication

PvP keeps live teams and rounds in Valkey. The SQL `pvp_room_transitions` record
retains each room's latest financial version, resulting room and final outcome;
`pvp_character_claims` reserves each character's participation. The PvP transition
port commits bets, refunds or prizes and victory counters with this recovery state
in one SQL transaction. It locks the room claim (Rank 0), affected characters in
ascending ID order (Rank 2), then participation claims (Rank 8). A unique character
claim rolls back a competing admission, including its debit. Terminal claims reject
further financial transitions. Financial versions also reject stale cached rosters.

Valkey publication happens after commit. Before subsequent room mutations, pending
publication is retried under the room lock without repeating financial effects or
combat. Creation can discover unpublished rooms through the character's SQL claim.
Removed claims remain reserved until room and reverse-mapping writes all succeed;
mapping deletion compares the room ID so it cannot erase newer membership. Final
round retries return the stored result. Password hashes and financial versions use
private persistence fields and remain absent from public room responses.

These records have no automatic expiry. Recovery covers committed financial
transitions awaiting publication; it does not reconstruct later team/round changes
after an already published lobby expires or is evicted, or identify losses from
older deployments. Active claims survive that loss, preventing another debit but
requiring explicit recovery rather than silently abandoning the escrow. This adds
no timeout refund rule. Retry after a new join/leave cycle is a new operation; the
API has no client request identity to distinguish a delayed request across cycles.
SQL versions protect the financial claim; live combat and publication still use
the existing room lease, whose wider multi-instance guarantees are unresolved in
[#1574](https://github.com/witchcraze/party2re/issues/1574).

### GvG reward publication

`StandingRepository.CommitRoundTransition` delegates to the GvG SQL repository.
One transaction locks the room's `gvg_match_transitions` record, then all affected
`guilds` rows in ascending ID order (Rank 7), including zero-reward draws so that
standing creation cannot take a guild foreign-key lock after a standing lock.
It awards GP through `GuildRepository.AddPoints`, then locks `gvg_standings` rows
in ascending guild ID order (Rank 8). It commits
spendable GP, cumulative GvG reward counters, any final settlement and the resulting
room/outcome together. The room ID and resolved round identify
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
round and start it. The [retry identity target below](#gvg-round-retry-identity-target)
addresses that ambiguity; it is not implemented yet. Lobby deletion or expiration
does not delete the durable guard. These records have no automatic expiry, and a
missing Valkey room is not reconstructed by this operation.

This guarantee applies to transitions committed through the guard. It cannot
retroactively identify rewards committed by an older deployment. Reward amounts
and recipients remain a separate [GvG domain contract](../design/gvg.md).

#### GvG round retry identity target

**Decision: expected round, selected in [#1573](https://github.com/witchcraze/party2re/issues/1573)
on 2026-10-22 to preserve gameplay results.** This is an implementation target;
the current advance API still has no operation identity. Implementation is tracked
in [#1589](https://github.com/witchcraze/party2re/issues/1589). No reward, combat,
round-limit or participation rule changes as part of this decision.

The legacy dispatch adds `かいし` and pre-start `しらべる`
(`party2/lib/vs_guild.cgi:31–38`). `かいし` initializes the match or evaluates the
finished round and starts the next one (`44–154`); `しらべる` is observation.
`party2/lib/system.cgi:456–469` reloads without an action, so observing the result
does not issue another `かいし`. Neither path provides a request ID or an expected
round. Expected-round validation is a transport safeguard, not a claimed legacy
parameter. The existing Go split into `StartMatch` and `AdvanceRound` and retained
inspection reads stays in place; restoring per-turn combat or reconciling other
legacy actions remains outside this decision.

**Identity and compatibility.** The retained
`POST /characters/{id}/gvg/rooms/{room_id}/advance` requires JSON
`{"expected_round": 1}`. Identity is the immutable room ID and the round to resolve;
the actor must own the authenticated character and be the current room leader on
every call, including replay. Knowing a room ID or a previously valid request
does not confer authorization. Missing, null, non-integer or out-of-range values
(valid range: 1 through `MaxRounds`) return 400 before combat or writes. Do not
default a missing value to the current round. Existing callers must update at the
same cutover; there is no identity-free compatibility mode. Preserve the existing
`RoundResolution` response: its `round` is the resulting room round, not necessarily
the round just resolved. Future Gateway migration must carry the same explicit
input through the shared service, including its strict continuation schema.

**Execution and replay.** Under the existing room mutation lock, authorize before
returning any saved result. Compare the requested number with SQL's resolved
`round_number`, not merely with the resulting room or an `>=` lookup match:

| Authorized request with a live room | Outcome |
| --- | --- |
| Exactly the latest committed round | 200 with that stored `RoundResolution`, including final settlement; never rerun combat or rewards |
| Older than the latest committed round | 409; never substitute a newer result or publish an older snapshot |
| Uncommitted request in recruiting/completed phase | Existing phase rejection (422); exact final-result replay takes precedence |
| Uncommitted current round of an in-progress room | Resolve that round once and return 200 after publication |
| Other uncommitted number | 409 without combat, rewards or room changes |

After SQL commit but before successful Valkey publication, retrying that exact
round republishes the saved room before returning its result. Resolve pending
publication before accepting a new round; recovery itself does not satisfy a
different requested round. If publication still fails, propagate the failure
and retry with the same number. Once already published, replay returns the result
without rewriting the room or renewing its TTL. A lost SQL commit acknowledgement
is likewise an unknown outcome: retry the original identity and consult durable
evidence, never infer from the error that rewards rolled back. Preserve the
existing reward transaction and lock order described above. These guarantees use
the current single-process room coordination boundary; they do not certify the
multi-instance lease behavior tracked in #1574.

**Retention and client recovery.** Keep the latest SQL transition per room, with
its existing indefinite retention; no request-ID table, historical round results
or new expiry policy is required. A later commit replaces replay availability
for the previous round, which then returns 409. A missing/expired/deleted Valkey
room returns 404 even if SQL retains a result; do not resurrect it. Live room TTL
remains 30 minutes after its last write, and reads do not renew it. Storage errors
remain errors, not absence. Authentication/ownership failures retain their common
HTTP handling and a nonleader receives 403 before result disclosure.

Until implementation, clients must not automatically retry advance after an
unknown outcome. After cutover, capture `room.round` before an intentional advance
and retain that number with the request. On timeout or lost response, resend the
same room/number; do not replace it with an observed newer round. After 409, read
the room and require a separate intentional next-round action; after 404, stop.
Observation alone cannot prove which older request completed and cannot recover
an overwritten battle log. Request IDs and historical replay would add storage
and lifecycle policy without being needed to prevent unintended next-round
execution. Observation alone was rejected as the sole safeguard because it leaves
delayed or concurrent resends indistinguishable from a new action.

**Implementation verification.** Cover these cases through the service, HTTP
contract and real SQL/Valkey stores, observing combat calls and durable GP,
standings and medals as well as responses:

- Consecutive rounds: resolving 1 and replaying 1 leaves round 2 untouched; an
  explicit request for 2 alone advances it. A delayed 1 after committing 2 returns
  409 with no changes. Future and invalid inputs do not execute.
- Intermediate and final results: discard a successful HTTP response after the
  room write, then resend the identical body. Return the saved result, including
  after `completed`; award intermediate GP, final prizes and medals only once.
  Include a final draw, which grants no final GP or medal.
- Partial publication/commit uncertainty: inject failure before the room write
  and after a successful write/SQL commit whose acknowledgement is discarded.
  Same-identity recovery must preserve the original result and assets. A next-round
  request must not receive the preceding result or bypass pending publication.
- Concurrent identical requests: use the shared concurrency harness; only one
  combat result commits, successful replies agree and assets are conserved. Race
  an old retry against an explicit next-round request: replay or 409 is allowed
  according to lock order, but the retry never executes the next round.
- Lifetime and access: replay does not renew an already published room; GET does
  not renew or repair it. Expiry/leave deletion returns 404 without reconstruction.
  Ownership/nonleader rejection applies equally to fresh, replay and final calls.
  Verify legacy rows need no migration and a retained newer SQL round is never
  treated as an exact older receipt.

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
(`internal/challenge/lua/challenge_round.lua`) verifies session ID, active
state and the round used to calculate combat. It advances the round and saves
leader HP, the complete member snapshot and provisional rewards in one atomic
step, preserving party identity and refreshing TTLs. A stale round returns
`ErrSessionRoundMismatch` (HTTP 409) without changing the buffer; callers must
read the current run before computing another round. There is no later full
session save that could overwrite a newer step. Exact fields,
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
Terminal payout/history idempotency is separate from start reconciliation; see
the Challenge guarantee below and the remaining Dungeon work in #1205.

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

### Challenge finalization

`ChallengeRepository.FinalizeSession` requires the SQL row created at start and
a `defeated` or `claimed` terminal snapshot. In one transaction it locks the
admitted Characters in ascending ID order, conditionally updates the owned session
from `active`, checks `RowsAffected`, and uses the shared battle adapter to save
all participants' HP/MP and consumed inventory/equipment. Each member receives
their earned EXP/Gold and challenge record in that transaction; shared treasure
retains its designated leader recipient rather than being cloned. The terminal
roster must match the durable admission snapshot. Fallen characters use the
shared combat survival floor of 1 HP on durable exit. It reuses an
ambient transaction. An error rolls back the claim with its coupled effects.
Missing sessions return `ErrSessionNotFound`; ownership mismatches return
`ErrForbidden`; completed sessions return `ErrSessionNotActive` without changing
their snapshot or granting rewards again. Finalization never recreates a row.

The retained SQL row protects sequential and concurrent retries independently
of Valkey deletion. Post-commit cleanup failure leaves the buffer observable;
another defeat settlement returns `ErrSessionNotActive` and grants nothing.
This guard does not repair that buffer. Live victories use the existing round
CAS to commit the complete member snapshot, including MP, cumulative consumption
and individual rewards. A fresh participant is rebuilt through the shared battle
adapter with consumed items removed before equipment and abilities are evaluated.
Inventory/equipment read failures from configured repositories propagate before
combat, leaving the round buffer, rewards and Hall of Fame unchanged. Equipment
read failures during settlement roll back the SQL claim and its coupled effects;
successful empty reads remain valid.
Pre-upgrade snapshots without MP initialize it from durable state once and retain
their shared reward balance; already-lost MP/consumption history cannot be recovered.
No new HTTP exit operation or automatic retry is introduced; voluntary exit is
tracked separately in #1208 and requires the target below.

### Challenge participant exit target

**Decision: option A in [#1584](https://github.com/witchcraze/party2re/issues/1584),
user-approved 2026-10-22.** Retain Valkey live-round authority, make run identity
independent of leadership, and add SQL participant settlement receipts and
recoverable publication. This is an implementation target, not a claim that the
current service supports individual exit. It does not change Dungeon or introduce
a generic cancellation framework. Gameplay evidence and exit effects belong to
[Challenge](../design/challenge.md#individual-exit).

#### Identity and admission

One immutable session ID identifies the canonical buffer. Current leader, ordered
active roster and designated treasure recipients are separate facts. Every actor,
including the leader, discovers the run through a run-qualified reference; no
leader handoff moves canonical keys. The [keyspace target](valkey-keyspace.md#challenge-run-identity-target)
owns names and lifetime. Public feature readers verify actor enrollment, not just
possession of a reference. Historical session ownership never authorizes an exited
leader to control the remaining party.

SQL retains immutable admission evidence, active participant claims and each
participant's settlement outcome. A unique unfinished claim per character blocks
competing enrollment, including an exit awaiting required publication. Admission
and other activity gates use the owner's public reader to recognize these claims
when Valkey discovery is absent. Creating the SQL admission/publication intent
precedes publishing live state. Lobby readiness still comes from Party's public
handoff; a run never infers consent from a stale lobby or changes another actor's
lobby membership as a side effect of exit.

#### Mutation ordering and settlement

All Challenge writers adopt one owner-specific transition boundary, including
start, round advancement, individual exit and defeat. The durable run coordination
record is locked first (Rank 0), then affected Characters in ascending ID order
(Rank 2), inventory/equipment (Rank 3), conditional overflow depot (Rank 5) and
secondary participant/record/fullness state (Rank 8). Use the injected Unit of Work;
feature code does not access SQL directly. No expiring Valkey lease is the sole
guard against a second writer or payout.

Under this boundary a writer first resolves any committed publication awaiting
completion. Only then may it read and calculate from the current live revision.
It validates role, phase, immutable run ID and expected revision and commits the
exact accepted result, next revision and pending publication intent in SQL before
changing Valkey. Every accepted mutation advances the revision, including an exit
that leaves the round unchanged. Admission claims and financial effects commit
with this intent; failure rolls them all back. No new operation can overtake an
unacknowledged transition. The buffered combat state and provisional rewards
remain live Valkey state; SQL transition evidence is a recovery boundary, not a
blanket migration of live authority or a promise to reconstruct expired runs.

Participant exit claims are unique by run and admitted character. The common
settlement owner uses the shared battle adapter to save only the departing
participant's HP/MP, cumulative consumptions, earned rewards, record and required
exit effects. It retains the other participants' exact buffered state. Terminal
`FinalizeSession` reuses that same implementation and pays only unsettled
participants; it neither requires a reduced roster to impersonate the original
admission nor rewards a departed member again. Stored admission and exit receipts
remain authorization evidence. Final departure marks the run claimed; defeat
marks it defeated. Record round counts follow the gameplay owner, not a revision
counter. Treasure delivery retains its recorded recipient and durable delivered
state; it is never cloned or reassigned by handoff.

#### Publication, replay and observation

After SQL commit, publication conditionally installs the recorded revision in the
run's canonical Valkey hash slot. Repeating the same publication succeeds without
advancing again; an older publication cannot overwrite a newer revision. Update
remaining actor references and conditionally remove departed references by run
ID. Only after all required writes succeed may SQL acknowledge publication and
release departed claims. Cleanup cannot delete a replacement run or enrollment.
The last exit deletes only that run's buffer. No cross-slot or SQL/Valkey atomicity
is implied.

Retain actor-scoped command results independently of current enrollment. Mutation
identity includes run, executing actor and expected revision, with canonical
action/inputs checked against the receipt. An identical retry returns the stored
result; reuse with different inputs conflicts. Expose only the authenticated
actor's receipt. Receipt lookup precedes current-membership/phase rejection so a
successful exit remains observable after departure. Mutation adapters must carry
the expected revision; an ID-only advance must not silently execute the next
round after a lost response. Existing REST callers require a verified contract
update at cutover; there is no silent compatibility replay with a guessed revision.

| Failure boundary | Required behavior |
|---|---|
| Validation, computation or SQL failure before commit | No payout, receipt or live publication; the original roster and buffer remain |
| SQL commit acknowledgement lost | Resolve the owned receipt outside the failed transaction with a bounded context; preserve state and report unknown outcome if evidence is unreadable |
| Committed payout, Valkey body/reference/cleanup write fails | Retain the exact pending publication and participant claims; another explicit command can finish publication without rerunning combat, bonuses or payout |
| Publication succeeded, acknowledgement failed | Repeating publication is revision-idempotent; acknowledge the same durable transition before accepting new work |
| HTTP response lost | Owned receipt returns the original outcome; clients recover by GET and never automatically replay POST |
| Missing live buffer with no pending publication | Report unresolved owned activity; do not invent zero rewards, refunds, a new run or complete crash recovery |

GET is strictly read-only: it does not settle, publish, remove references or renew
TTL. Pending publication is observable through the public activity reader even
without a usable buffer; no ordinary-entry controls are offered for unresolved
claims. A successful financial receipt and pending publication are distinct
facts. The Gateway's known-result/context-refresh-failure contract still applies.
Retain required REST detail reads until actual replacement coverage exists.

#### Upgrade and verification

Quiesce old Challenge writers before activating the new transition boundary;
mixed old/new writers cannot honor the new revision/receipt guard. Until an
explicit migration command runs, new readers can observe old character-keyed
buffers and original member references without renewing or rewriting them. The
migration validates the SQL admission and complete old snapshot, records a pending
transition under the same immutable run ID, publishes run-keyed state and all
actor references, then conditionally removes only the matching old keys. It
preserves roster order, resources, consumptions, individual earnings, treasure
and the remaining deadline; migration is not activity and does not reset TTL.
Failure is recoverable through the same publication intent. Missing or unreadable
old state blocks that run for explicit recovery; never silently discard it or
guess already-lost resources. Restore/rollback must not re-enable old writers
against already-migrated runs.

The runtime prerequisite couples run-keyed buffers, conditional publication,
migration compatibility, participant claims/shared settlement and adoption by
every existing Challenge writer. Keep that correctness boundary together; the
Gateway exit adapter is a separate dependent unit. Each runtime PR owns its
public-port, OpenAPI and Guidance changes; this decision adds no executable
schema or port.

Use real SQL/Valkey tests for solo/party, zero/one/multiple wins, follower/leader/
last exit, ordered handoff and former-leader re-entry. Check untouched survivors
independently of the departing actor. Exercise duplicate and different-member
exits, exit versus advance/defeat, pending publication, lost acknowledgements,
old-buffer migration and read-only GET. Standard concurrency stress tests must
prove one payout per participant and no asset duplication, loss, deadlocks or
resurrection of departed combatants. Mocks alone do not certify this protocol.

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
