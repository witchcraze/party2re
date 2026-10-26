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

### Boss completion recovery

Party sealing locks the SQL `boss_room_settlements` row (Rank 0) before ascending
character locks (Rank 2). Its room identity, roster and battle result commit with
all participant rewards, history and mandatory Collection records. Valkey's
`in_progress` status excludes ordinary lobby activity; the SQL receipt prevents
repeat settlement even after the short party lock expires. An interrupted room
without a committed receipt can resume under that SQL lock.

No room, roster, reverse mapping or Ready key is deleted before SQL commit.
On a transaction error, a bounded, cancellation-independent locking read waits
for its outcome. Confirmed rollback releases the room to recruiting. A committed
receipt leads only to cleanup. If outcome confirmation or release fails, the
error is returned and the room is preserved for another attempt; an unknown
commit is never treated as permission to repeat rewards.

A retry of the same sealing request by the saved leader returns the recorded
result and retries cleanup using the saved roster, even if the lobby is already
missing. Cleanup failures propagate with the committed result retained in SQL.
Reverse mappings are removed only if they still name that room, preserving new
membership. Ready keys and the lobby are room-scoped, and index removal is
idempotent. Recovery uses the next sealing request, without a background worker.
Receipts have no automatic expiry: their room IDs must not be reused, and deletion
would remove the replay guard. This is recovery evidence, not a second live lobby.
Unsettled lobbies retain their ordinary idle TTL; this protocol does not rebuild
expired uncommitted rooms or make Valkey loss harmless. Sealing must own its SQL
transaction boundary; cleanup must not run inside a caller-owned transaction.

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

Every advance supplies the expected round under the
[retry identity contract below](#gvg-round-retry-identity-target). A lost publication
acknowledgement or HTTP response can be retried with the original number without
executing the next round. Lobby deletion or expiration does not delete the durable
guard. These records have no automatic expiry, and a missing Valkey room is not
reconstructed by this operation.

This guarantee applies to transitions committed through the guard. It cannot
retroactively identify rewards committed by an older deployment. Reward amounts
and recipients remain a separate [GvG domain contract](../design/gvg.md).

<a id="gvg-round-retry-identity-target"></a>

#### GvG round retry identity

**Decision: expected round, selected in [#1573](https://github.com/witchcraze/party2re/issues/1573)
on 2026-10-22 to preserve gameplay results.** No reward, combat, round-limit or
participation rule changes as part of this decision.

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
| Next round while preceding publication is pending | Recover publication and return 409 without executing this request |
| Other uncommitted number | 409 without combat, rewards or room changes |

After SQL commit but before successful Valkey publication, retrying that exact
round republishes the saved room before returning its result. Resolve pending
publication before accepting a new round. If a next-round request encounters
pending publication, publish the preceding transition and return 409; this does
not execute the requested round or return the preceding result. Older or future
requests do not repair the room. If publication still fails, propagate the failure
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

Capture `room.round` before an intentional advance and retain that number with
the request. On timeout or lost response, resend the same room/number; do not
replace it with an observed newer round. After 409, read
the room and require a separate intentional next-round action; after 404, stop.
Observation alone cannot prove which older request completed and cannot recover
an overwritten battle log. Request IDs and historical replay would add storage
and lifecycle policy without being needed to prevent unintended next-round
execution. Observation alone was rejected as the sole safeguard because it leaves
delayed or concurrent resends indistinguishable from a new action.

**Verification.** The service/HTTP tests and
[SQL/Valkey integration tests](../../internal/gvg/expected_round_integration_test.go)
cover the following contract, observing combat calls and durable GP, standings
and medals as well as responses:

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

Dungeon and Challenge use Valkey as the live authority. Dungeon uses character-keyed
state/reward/revealed buffers; Challenge uses immutable run-keyed session/reward
hashes. The [keyspace owner](valkey-keyspace.md) defines their 7200-second activity
lifetime and run-qualified member references.

Dungeon's `dungeon_step` atomically updates its existing buffer. Challenge's
[transition boundary](#challenge-participant-exit-target) serializes every start,
advance, participant exit and defeat through SQL coordination before publishing
the accepted live revision. Publication renews activity only for a newly accepted
command; retries and GET never extend its recorded deadline. Required state and
reward-read failures propagate rather than certifying idle activity.

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

Dungeon retains its canonical leader buffer and leader-only advance/escape.
Challenge leadership is ordered live roster data and may transfer without moving
keys. All admitted actors discover the same immutable run, independently of the
Party lobby's shorter lifetime. Challenge's unfinished SQL claims also remain
activity when discovery or the live body is missing. GET never repairs state.

### Start outcome and publication

Dungeon retains the approved start reconciliation: an admission error after
publication is resolved from the owned run buffer outside the failed transaction;
unreadable evidence returns `ErrStartOutcomeUnknown` without deleting it. Its
publication precedes commit and does not exclude a still-in-flight server commit.

Challenge commits admission, participant claims, its actor-owned revision-0 start
receipt and pending publication before any Valkey write. Lost SQL acknowledgement
is resolved from that receipt with a bounded cancellation-independent context.
Unreadable evidence returns `ErrOutcomeUnknown`; failed publication returns
`ErrRecoveryRequired` while retaining the committed outcome and claims. HTTP uses
503 for these recovery states. Clients observe the owned active-run and command
readers and never automatically replay POST.

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

### Dungeon finalization

`DungeonRepository.FinalizeExpedition` claims expedition settlement once per run
by inserting `dungeon_expedition_history` keyed by the active expedition's run ID
(`dungeon_expedition_history.id = exp.ID`). Inside `RunInTx`, it locks the owning
`characters` row first (`SELECT ... FOR UPDATE`, Rank 2), claims the history row,
applies experience, gold, and small medal rewards directly to the locked character
row, delivers reward items to inventory/depot (Rank 3/5), and atomically increments
`character_dungeon_records` (Rank 8). Duplicate run-ID claims map to
`ErrExpeditionAlreadyFinished`, which `internal/dungeon` treats as idempotent
completion so post-commit Valkey `DeleteActiveExpedition` can safely retry without
double-granting rewards or overwriting concurrent wallet updates. Clear, escape
and wipeout pass both the owner and finalized run ID to each cleanup port;
missing or replaced runs are a successful no-op. Valkey compares that identity
and deletes state/rewards in one Lua invocation, while memory compares and
deletes under its store lock. Member references are released only when their
saved owner/run pair still matches, independently in each member's hash slot.
SQL fallback deletion, including inside finalization, likewise requires both
owner and run ID; replacing a fallback row also saves the new run's identity.
Thus delayed finalization or cleanup retries cannot remove a later run or its
unsettled rewards. SQL commit and Valkey cleanup remain separate boundaries.

### Challenge finalization

`ChallengeRepository.FinalizeSession` shares one settlement implementation for
participant exit and terminal defeat. Under the run coordination lock, it saves
only unsettled admitted participants through the shared battle adapter and commits
resources, cumulative consumption, individual earnings, designated treasure,
records and required exit effects with the command receipt/publication intent.
Fallen characters retain the shared survival floor of 1 HP. Equipment read errors
and Tavern fullness failures roll back all coupled effects.

The SQL admission roster remains immutable authorization evidence; it is not the
current leader or active roster. The legacy full-roster finalizer remains only for
unmigrated storage compatibility and cannot mutate a coordinated run outside its
accepted revision. [Individual exit](../design/challenge.md#individual-exit) owns
fatigue, attempted-versus-cleared round counts and accessory bonuses.

### Challenge participant exit target

**Decision: option A in [#1584](https://github.com/witchcraze/party2re/issues/1584),
user-approved 2026-10-22.** Retain Valkey live-round authority, make run identity
independent of leadership, and add SQL participant settlement receipts and
recoverable publication. The runtime implements this boundary; Gateway
exit adoption remains separate in #1208. It does not change Dungeon or introduce
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

Character/account deletion treats the original leader recorded on admission as
historical identity. It preserves that row while another participant has a durable
run claim, including terminal settlement awaiting publication acknowledgement.
Deletion neither cancels nor settles the survivors and leaves their run coordination,
participant outcomes and command receipts intact. Once no other claim remains,
the existing character-deletion cleanup may remove the admission row.

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
The last exit removes that run's live body/reward ledger; a revision fingerprint
remains until its recorded deadline to reject stale publication. No cross-slot or SQL/Valkey atomicity
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

SQL coordination, participant claims and command receipts are owned by the
Challenge repository. Acknowledgement discards the pending publication snapshot;
it does not turn SQL into the authority for an expired live run. Public
`AdvanceRound` and `ExitSession` require the observed revision. Retained REST
advance requires `expected_revision`, and owned GET routes expose active activity
and actor-scoped command receipts. The Gateway exit adapter remains #1208.

Upgrade procedure: quiesce all old writers, apply migration
`107_challenge_run_transitions`, then run
`go run ./cmd/challenge-migrate -character-id <admitted-actor> -run-id <run-id>`
for each active legacy run with the normal SQL/Valkey environment. Repeating this
explicit command finishes pending publication. Incomplete resource history,
missing ledgers and expired buffers require recovery; they are never guessed.
The migration's SQL triggers reject old-binary starts and updates to migrated
admissions, including after application rollback. Committed migration publication
first freezes the matching legacy session so old advance scripts reject it even
if reference installation or cleanup later fails. Do not roll back these guards
or restart old writers against retained legacy keys during partial migration.

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
