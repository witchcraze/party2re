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
retains each room's latest financial version, resulting room, final outcome, and
durable room-log activity timestamp (`last_activity_at`); `pvp_character_claims`
reserves each character's participation, including unleft `completed` rooms until
each member leaves or times out. The PvP transition port commits bets, refunds or
prizes, victory counters, and timeout `PendingWake` flags with this recovery state
in one SQL transaction. It locks the room claim (Rank 0), affected characters in
ascending ID order (Rank 2), then participation claims (Rank 8). A unique character
claim rolls back a competing admission, including its debit. Terminal `disbanded`
claims reject further transitions, while `completed` rooms permit only departure,
abandonment, or inactivity cleanup. Pre-start departure refunds only the departing
participant; post-start departure preserves remaining escrow for active teams or
records forfeited escrow (`ForfeitedPool`) on last-member forfeit, undecided
max-round draw, settlement remainder, lost-room abandonment, and inactivity
timeout. Financial versions also reject stale cached rosters, and team
selection, match start, and non-final rounds commit `last_activity_at`, the
resulting room, non-final `PendingPvPKills` and round outcome through the same
versioned transition and pending-publication boundary.

Valkey publication happens after commit. Before subsequent room mutations, pending
publication is retried under the room lock without repeating financial effects,
combat, or activity extension (retrying with the remaining activity TTL rather than
extending it, and never resurrecting an already-expired room). Creation discovers
unpublished or overdue rooms through the character's SQL claim. Removed claims
remain reserved until room, reverse-mapping, and any timeout sleep-timer writes
all succeed; mapping deletion compares the room ID so it cannot erase newer
membership. Retrying a pending team selection, match start, or non-final round
completes publication and returns its stored result without advancing another
round; once acknowledged, non-final rounds advance normally while final round
retries return the stored result until the room times out.
Password hashes, activity timestamps, and financial versions use private
persistence fields and remain absent from public room responses.

These records have no automatic expiry. Pending financial publication is recovered
before interpreting a missing live room as loss, and Valkey connectivity failures
always propagate without settling or penalizing. For a published room missing
before 1800 seconds of inactivity, PvP leave verifies the requesting participant
against SQL and closes the room without refunding bets or adding sleep. When
`now > last_activity_at + 1800s`, mutating PvP accesses run inactivity cleanup:
closing remaining escrow without refund, setting `PendingWake` in SQL and recording
the fixed `PenaltyUntil = now + 3600s` before deleting Valkey keys and publishing
`ExtendLockUntil` sleep locks (preserving longer sleep). Home's sleep penalty reader
observes unacknowledged PvP timeout penalties so failed timer publication cannot
permit premature wake or re-extend the deadline on retry. The character context
exposes `pvp_leave` for a claim without a live room as `recovery_pending` (blocking
wake while a timeout penalty is pending); rescue still requires the PvP owner to
finish recovery. Retry after a new join/leave cycle is a new operation; the API has
no client request identity to distinguish a delayed request across cycles. SQL
versions protect the financial claim; live combat and publication still use the
existing room lease, whose wider multi-instance guarantees are unresolved in
[#1574](https://github.com/witchcraze/party2re/issues/1574).

### GvG reward publication

`StandingRepository.CommitRoundTransition` commits spendable GP, cumulative
`gvg_standings`, `characters.pvp_wins`, final settlement, and the latest room
snapshot/outcome in one SQL transaction (locking per
[`.arch/modules/gvg.json`](../../.arch/modules/gvg.json)), rejecting further
claims after match completion. `gvg_match_transitions` retains only the latest
committed transition per room with no automatic expiry; it cannot retroactively
identify rewards from before this guard, and reward rules live in
[the GvG design](../design/gvg.md).

Valkey publication runs after SQL commit. `ValkeyRoomRepository.SaveRoom` updates
the active index before writing the 30-minute room key so index rejection cannot
advance the room and listing prunes by age rather than erasing a missing body
still being published.

Under the room lock, both `AdvanceRound` and `LeaveRoom` check
`gvg_match_transitions` before mutating state. When Valkey is behind (older
`Round`, or `in_progress` against a completed transition), they publish the
committed room snapshot first; if publication recovery fails, they abort before
combat or any SQL exit claim/settlement against stale state. Once `LeaveRoom`
commits its per-join `gvg_exit_claims` transaction (post-battle settlement,
Tavern fullness/delivery, and delivery completion) and updates or deletes the
Valkey room, a later `AdvanceRound` replay sees Valkey is current and returns
the saved `RoundResolution` without restoring departed members or prior
leadership. Failed Valkey cleanup keeps `party2:gvg:character:<id>` to block new
admission while SQL exit retries stay idempotent, and `DeleteCharacterRoom`
removes the mapping only if it still names that room. Neither operation
reconstructs a missing/expired Valkey room from `gvg_match_transitions`.

<a id="gvg-round-retry-identity-target"></a>

#### GvG round retry identity

**Decision: `expected_round`, selected in
[#1573](https://github.com/witchcraze/party2re/issues/1573) (2026-10-22) to
preserve gameplay results.** No combat, reward, or participation rule changes.
Legacy `vs_guild.cgi:31–38,44–154` and `system.cgi:456–469` have no request ID or
`expected_round`; `expected_round` is a transport safeguard, and the Go
`StartMatch`/`AdvanceRound` split stays.

- **Contract:** Both the advance route and `gvg_advance` require an explicit
  `expected_round` (never defaulting to the current round) and enforce ownership
  and current room leadership on every call including replays (403 on mismatch).
  `RoundResolution.round` is the resulting room round, not an operation ID.
- **Outcomes (authorized live room):**

| Request vs. latest committed SQL round | Outcome |
| --- | --- |
| Exact match | 200 with stored `RoundResolution`; never rerun combat or rewards |
| Older than committed | 409; never substitute a newer result or republish an older snapshot |
| Uncommitted, `recruiting`/`completed` | 422 phase rejection |
| Uncommitted, matches current `in_progress` round | Execute once and return 200 after publication |
| Next round while preceding publication is pending | Publish preceding transition and return 409 without advancing |
| Other uncommitted number | 409 without combat, rewards or room changes |

- **Recovery & rationale:** Clients capture `room.round` before advancing and
  resend the same `expected_round` on lost responses or SQL acknowledgements; 409
  requires re-reading the room before a separate advance, and 404 ends recovery.
  Observation-only detection and historical request-ID tables were rejected as
  ambiguous or unnecessary. Missing/expired rooms return 404, storage errors
  propagate, and multi-instance lease behavior remains tracked in
  [#1574](https://github.com/witchcraze/party2re/issues/1574).

## Candidate D: In-Progress Run Buffers

Dungeon and Challenge use Valkey as the live authority. Dungeon uses character-keyed
state/reward/revealed buffers; Challenge uses immutable run-keyed session/reward
hashes. The [keyspace owner](valkey-keyspace.md) defines their 7200-second activity
lifetime and run-qualified member references.

Dungeon's `dungeon_step` atomically updates its existing buffer. Unresolved normal
and boss encounters retain the enemy participant's HP/MP/status, allied statuses,
CMP and remaining elemental field state together with their original destination
in that buffer. These encounter-only facts are cleared when combat finishes; they
are not durable character ailments. All cardinal moves reject before tile dispatch while
an encounter remains, without consuming a turn, event or reward. The leader can
resume through `Service.Fight` (`POST /characters/{id}/dungeons/fight`) or escape.
Step revisions reject stale combat results and movement updates; the Memory
adapter follows the same checks. Victory clears the encounter and grants its
rewards once. Escape is the only terminal outcome; an all-fallen party stays in
the run until it escapes. Carried-over pre-update frozen `wiped_out` buffers
remain observable with leader Fight/Escape continuations and settle their
recorded outcome once via the same SQL run-ID claim and owner/run-qualified
cleanup, forfeiting unbanked run stock. Before SQL settlement, a revision-checked step freezes
the escape snapshot, rejecting further battle updates. Concurrent Escapes reuse
that frozen snapshot rather than their earlier reads. A stale Escape cannot claim
SQL settlement or delete a newer battle result. If SQL fails or post-commit
cleanup is lost, Fight/Escape retries the frozen outcome without re-running combat.
SQL's run-ID claim remains the durable idempotency boundary, and live cleanup
remains owner/run-qualified and occurs only after SQL succeeds.

Challenge's
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

Dungeon retains its canonical leader buffer; members move through the member index, and escape stays leader-only.
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
changing this boundary. Durable records include `dungeon_expedition_history` and
`character_challenge_records`; legacy SQL
active-state migrations are historical work, not a remaining rollout plan.

### Dungeon finalization

`DungeonRepository.FinalizeExpedition` claims expedition settlement once per run
by inserting `dungeon_expedition_history` keyed by the active expedition's run ID
(`dungeon_expedition_history.id = exp.ID`). Inside `RunInTx`, it locks all participating
expedition member characters in ascending ID order (`SELECT ... FOR UPDATE`, Rank 2),
claims the history row, delegates surviving vitality persistence (HP/MP, 1 HP floor for
fallen characters) and consumable inventory item deduction to shared post-battle settlement
(`battle.ApplyPostBattleResult`), applies experience and gold rewards to each participating
member, and delivers reward items to inventory/depot
(Rank 3/5). Pre-cutover
expedition `members` payloads that omit `current_mp` distinguish missing MP from explicit
`"current_mp": 0` (`ExpeditionMember.HasCurrentMP()`): combat participant construction
passes `nil` MP to `BuildParticipantWithState` so stored character MP is used and updated
into the run buffer on combat completion, and non-combat settlement omits missing member MP
from `RemainingMP` so `ApplyPostBattleResult` preserves the `SELECT ... FOR UPDATE` locked
character's stored MP while explicit `0` MP is committed as `0`.
If required member or run state cannot be read, settlement aborts before freezing or
deleting the active run. Duplicate run-ID
claims map to `ErrExpeditionAlreadyFinished`, which `internal/dungeon` treats as idempotent
completion so post-commit Valkey `DeleteActiveExpedition` can safely retry without
double-granting rewards or overwriting concurrent wallet updates. Escape passes
both the owner and finalized run ID to each cleanup port;
missing or replaced runs are a successful no-op. Valkey compares that identity
and deletes state/rewards in one Lua invocation, while memory compares and
deletes under its store lock. Member references are released only when their
saved owner/run pair still matches, independently in each member's hash slot.
SQL fallback deletion, including inside finalization, likewise requires both
owner and run ID; replacing a fallback row also saves the new run's identity.
Thus delayed finalization or cleanup retries cannot remove a later run or its
unsettled rewards. SQL commit and Valkey cleanup remain separate boundaries.

### Dungeon treasure claims

**Decision (#1762), target; implementation [#2119](https://github.com/witchcraze/party2re/issues/2119).**
Chest delivery is immediate ([gameplay](../design/dungeon.md#treasure-inspection-and-delivery)),
so it cannot wait for Two-Phase Settlement. One SQL transaction locks the actor
(Rank 2) and inserts a claim receipt that is unique per run and chest and records
the actor, event, the event's allowance key and the result. The same transaction
debits item 215 and delivers to the hand or Depot (Rank 3/5) and the Collection
(Rank 8). The allowance compares the event's key with the key of the actor's
latest receipt in the run, read under the actor lock and never from the possibly
stale live run. Hand eligibility uses in-run state: the transaction debits the actor's
pending consumed items, and publication removes them from the run so settlement
does not debit them twice.

After commit, a revision-checked step marks the chest opened in the Valkey run.
Every later Dungeon mutation, settlement included, first publishes committed
receipts the run does not show yet. An identical retry returns its receipt. A
failure before commit changes nothing. A lost commit acknowledgement is resolved
from the receipt with a bounded context; unreadable evidence reports an unknown
outcome and keeps state. Valkey loss never regrants a receipt, and the receipts
do not reconstruct the run.

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

The user approved SQL authority and the explicit idle-cleanup timing in #1325
on 2026-10-11. Adventure owns enrolled runs; Party Valkey owns waiting lobbies.
[NormalRunService](../../internal/adventure/normal_run.go) implements the lifecycle
boundary, and its production combat port rebuilds every callback port per
operation. Production starts retain batch execution until the Gateway cutover
(#1330). Neither this service nor Candidate D's loss limitations certify
combat effects whose ports remain whole-action gates.
Gameplay evidence remains in [Adventure](../design/adventure.md#resumable-normal-run-transitions)
and shared effect state in [Battle](../design/battle.md#explicit-action-resumption-requirements).

### Authority and ownership

SQL holds immutable admission identity, versioned run/effect state, ordered
roster, actor/NPC timing, unique active enrollment, recipient entitlements and
actor-owned receipts. See the [schema](../../migrations/111_normal_adventure_runs.sql)
and [adapter](../../internal/database/adventure_run_repository.go). These records
do not resurrect SQL waiting lobbies or establish a second Valkey authority.

The run snapshot stores assigned coordinates separately from encounter provenance,
pending clear credit and the room-wide EX marker. Save the final roster after
resolving the [old-enemy response](../design/adventure.md#combat-travel-and-encounter-replacement).

Party admission takes its existing lobby lock, then serializes roster validation
and the SQL handoff on the admission's durable run identity. Each member's
private battle-row snapshot is captured in the Valkey lobby on create or join
and handed off through the immutable admission record (pre-cutover lobbies
without a captured snapshot fall back to building at enrollment), while
remaining excluded from public lobby responses. Roster writers and
the retained Party batch start use that same SQL boundary, including when the
Valkey lease expires. Enrollment freezes floor 0 without combat. SQL ownership
survives lobby cleanup failure, expiry and restart; retry finds the same admission
before consulting the lobby. Cleanup removes only that lobby's reverse mappings
and occurs after commit. No lobby lock is retained between requests or acquired
from an active-run transaction.

PlayerContext consumes the public owned actor reader. Required read errors fail
closed. Active and unfinished penalty facts survive lobby loss; private Character
records and bag contents are never returned as another actor's combat projection.
Actor-owned asset transactions and live-state writers check SQL ownership after
Rank 2 character locking, allowing the already-serialized native command context.
Single-actor services reuse the guarded writer; mixed transfers and purchases
check only the initiator after all affected character locks, before asset changes.
Incoming money/items, proceeds from existing offers and committed reward delivery
keep ordinary recipient access. These checks only observe SQL ownership and never
acquire a run lock after character locks. Sleep and retained combat admission also
recheck ownership; deletion cannot orphan an enrolled actor.
Production cutover must preserve these checks rather than rely on menu eligibility.

### One command, one bounded SQL transaction

Commands verify the authenticated actor and immutable input against the retained
receipt before requiring current membership/version. Under the run lock (Rank 0),
recheck the receipt, then lock the complete affected character set in ascending
ID order (Rank 2). Inventory/equipment, jobs, depots and secondary records follow
ranks 3/4/5/8; adapters must not settle A through Rank 8 before acquiring B.
Player actions and NPC responses share the requester's economy transaction
and one character update for wallet changes and action-tail growth. In a party,
acquire all inventory locks in character ID order before the requester
depot. The ambient transaction commits run state, assets, finalized recipient
earnings, chest delivery, history and the receipt together. Rejected commands
change none of them or their clocks. A stored-tool selection message can still
record tool history and a receipt without spending assets or renewing activity.

The injected combat port supplies bounded explicit effects and activity/stock
release facts; no batch resolver is its fallback. It must finish action/effect
validation and any ordered external transitions within this contract. Finalized
recipient earnings bypass reward multipliers and level growth when released
through the common settler; only an action tail grows a level.
The same settler consumes the defeat owner's capture decisions, so box
delivery and the result in the command receipt share one commit. Applied assets
and outstanding normal/Try entitlements remain distinct;
Try conversion occurs once at explicit ordinary treasure creation. Exit cannot
consume already debited items or award unopened chests.

### Individual settlement and recovery

Personal exit atomically applies outstanding effects, resets fullness, writes
recipient history/disposition, removes the departing participant from active
combat membership (`Battle.Actors`) as well as room membership (`Roster`),
selects the first remaining real participant in roster order and releases enrollment.
Remaining actors, legitimate helpers, NPCs, resources and timestamps retain their
state and order; subsequent encounter scaling, Stage15 echoes, targeting and EX
marker checks observe only active members. Sorted lock order never changes leadership.
Last exit clears active state while preserving admission, recipient dispositions and
receipts; another actor cannot read those receipts.

Room-log-equivalent activity uses the [gameplay matrix](../design/adventure.md#room-activity-contract).
GET, rejected commands and receipt retries do not renew it. Explicit cleanup is
due strictly after 1800 seconds, preserves outstanding normal/Try entitlements,
and records timeout separately from personal exit. Remaining participants receive
a durable one-hour sleep deadline anchored to the committed cleanup; departed
participants receive no new penalty or ordinary exit effects. Pending timer work
keeps enrollment discoverable and entry-blocking. Recovery projects the same
absolute deadline, then releases ownership. Home independently reads SQL before
Wake; timer loss or an unavailable projection cannot bypass the penalty.

Recovery does not pay the retained stock. Following the legacy rule that user-owned
stock outlives the deleted room, the recipient ledger stays the only authority for
unpaid unconditional earnings of every disposed timeout, not just the latest
penalty record. The actor's next stock-releasing command in a later normal run reads
them without taking Rank 8 row locks before Rank 3/4/5 asset settlement, adds them
to its own release in the same settler call, then locks each released row at Rank 8,
verifies its unpaid balance matches the credited amount, and moves it to Applied
in that transaction; receipt replay, rollback and retry therefore credit once.
Conditional Try stock and the committed penalty are never touched by this claim.

A failed pre-commit write rolls back all effects, including history and receipt.
A lost commit acknowledgement is resolved by reading the owned receipt under the
same serialization boundary, never by rerolling. SQL-commit-before-cleanup failure
retries only matching cleanup; mandatory penalties remain durable pending work.
Unsupported encodings and store errors fail closed, without memory fallback.
Replay/scene publication is optional and must not repeat committed effects.
The [Replay source/copy contract](../design/replay.md#command-commit-and-publication-failure)
requires frozen scene and exit-prefix facts to survive command commit and live-state
disposal before optional publication; this seam is not yet implemented.
Clients retain GET-only recovery and do not automatically replay POST requests.
