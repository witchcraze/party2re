# Adventure System & 10-Floor Dungeon Crawl Design Specification

## Overview

The Adventure system targets a clean-room reconstruction of the operation-driven
adventure in legacy Party2 (`quest.cgi`, `vs_monster.cgi`, `_battle.cgi`).

The fictional one-hour expedition timer was removed by #478 / PR #583. The
replacement currently resolves all battles, floor advances and treasure
inspections in one start request. This is an implementation gap, not equivalent
to the original adventure or approval to omit player actions. #478 requested
strategy selection, explicit advance and exit; #1014 / PR #1020 connected the
existing service to the Gateway without deciding new domain rules. The user
directed restoration of player-command progression in
[#1323](https://github.com/witchcraze/party2re/issues/1323). Removing an invented
one-hour wait does not remove the original per-actor action interval.

## Original interaction contract and reconstruction target

References in this section use `party2/` at legacy revision
`ff97fc549932a146cc945d7edf71b44c868a840e`. These are source-inspected requirements,
not a claim of full CGI execution or complete skill/formula parity.

- **Admission and progress are separate.** A normal quest is created at floor 0
  (`lib/quest.cgi:579–586`). The first `すすむ` requires the leader; subsequent
  advances are not universally leader-only. Living enemies prevent advancing
  (`lib/vs_monster.cgi:35–82`). Advancing creates the next encounter; it does not
  resolve the entire battle or automatically continue to another floor.
- **Players choose actions.** `lib/_skill.cgi:set_action` derives available
  actions from the actor's job, learned skills, resources and current situation.
  `lib/_battle.cgi:561–659` parses an action and target and checks actor state.
  A server-selected skill/lowest-HP target for every ally is not a replacement
  for this choice. Some skills have additional-action behavior; this contract
  does not reduce every command to exactly one hit.
- **Actions have an interval.** `read_member` assigns the room's speed to
  `act_time` (`lib/_battle.cgi:450–468`); a registered action is rejected when
  elapsed time since that actor's previous action is below that interval
  (`641–644`). The accepted action path updates actor timing and persists the
  shared state (`1169–1178`). This is not a round barrier waiting for all players
  and not an animation-only delay. A clock deadline becoming due does not by
  itself execute an action.
- **NPC responses are bounded by the submitted operation.** The eligible action
  path invokes `npc_turn` unless suppressed (`lib/_battle.cgi:1169–1171`).
  `lib/_npc_action.cgi:89–155` selects at most one living, interval-ready enemy
  for its action and rotates its order. Advancing and treasure-room operations
  disable this response (`lib/vs_monster.cgi:37–53`). Repeated GETs do not execute
  combat; no background auto-combat or full agility-sorted party round is implied.
- **Treasure and exit remain individual operations.** `しらべる` selects a chest
  and checks the actor's allowance/item rules (`lib/_npc_action.cgi:5–85`);
  entry to the treasure room does not auto-open everyone's chest. `にげる`
  distinguishes an actual participant from a spectator, removes that actor and
  applies participant exit effects (`lib/_battle.cgi:1184–1278`). Different
  participants can leave at different times; one exit is not a party-wide finish.
- **Scene capture follows actual submissions.** `party.cgi:20–27` records an
  eligible operation, including submitted commentary, using the pre-operation
  normal-adventure location. Rejected operations and refreshes do not record.
  Personal copying precedes possible capture from the exit operation. Content,
  visibility and copy rules belong to [Replay](replay.md).

The dispatch reconciliation for this interaction boundary is:

| Original operation | Current native path | Reconstruction responsibility |
| --- | --- | --- |
| `つくる` / `さんか` (`quest.cgi:189–199`) | Party `CreateParty` / `JoinParty`; solo `StartStage` immediately executes | Keep admission separate from combat and preserve actor/roster validation |
| `けんがく` | Party detail reads exist; no equivalent active normal-adventure view is established | Reconcile admission and actor-dependent observation under #948; spectators are not combatants or archive recipients |
| `すすむ` | `CrawlSession.AdvanceFloor`, called in an automatic loop | Separate encounter creation from action resolution; persist progress between requests |
| Registered battle action and target (`_skill.cgi:set_action`) | `ResolvePartyBattle` automatically selects all combatants' actions until termination | Add resumable explicit-action processing, reusing battle effects and the participant adapter |
| `しらべる` | `ExamineTreasure`, automatically invoked for each participant | Expose actor-selected inspection; reuse #1196 for allowance/key correctness |
| `にげる` | Whole-crawl settlement; Party lobby `LeaveParty` is not an equivalent active-combat exit | Restore individual exit/settlement while remaining participants can continue |
| Submitted comments / `ささやき` / `すくしょ` | No equivalent normal-adventure command contract established | Reconcile separately under #948; private whispers and manual screenshots are not automatic Replay payloads |
| Refresh | Current reads and Gateway context | Observe without advancing, granting rewards, recording a scene or renewing game deadlines |

The reconstruction must retain enough authoritative state between operations
to resume the same encounter: roster and roles, living/fallen participants,
resources, statuses/effects and their lifetimes, chosen encounter, field state,
consumed items, actor/NPC timing, progression and earned-reward/exit facts.
Persisting only HP/MP and restarting the batch resolver loses these facts.
Current `CrawlSession` is request-local; it is not a production run store.

Native storage and recovery must be established before replacing production
starts; [#1325](https://github.com/witchcraze/party2re/issues/1325) records the
selected boundary below and its bounded runtime implementation scopes. Existing
Party Valkey lobbies and SQL audit rows are not a resumable
normal-adventure session. Durable assets, consumption and settlement/history
require SQL transaction and retry protection; the current lobby lock is not
cross-store atomicity. In solo, settlement currently commits before adventure
history writes (#1224). In Party, SQL settlement/logging is inside `runInTx`,
but Valkey status/readiness mutations are outside SQL rollback. Neither path
proves durable command-by-command recovery.

Restore shared action processing, resumable adventure/individual settlement,
then verified Gateway commands and owned observations as bounded implementation
units under #1323. Keep unrelated combat modes unchanged and retain current REST
routes until #948 verifies replacements. EX progression (#1191), inspection
allowances (#1196), consumed-item carry (#1197), per-operation earnings (#1328/#1329),
exit fatigue (#1221), chronicle records (#1222) and transaction repair (#1224)
remain separately owned; no full parity for these is certified here.

## Resumable normal-run transitions

The following lifecycle is the #1325 reconstruction target. The user approved
SQL authority and the explicit idle-cleanup timing below on 2026-10-11. Legacy
requirements remain separate from these native decisions; the SQL boundary is in
[Transient run state](../architecture/transient-run-state.md#normal-adventure-durable-command-boundary-1325).
No production interactive run store exists yet.

| Transition | Preconditions and resulting facts |
| --- | --- |
| Waiting admission | Party owns create/join/readiness; solo uses the same Adventure enrollment boundary without a lobby. Enrollment preserves ordered participants, leader, stage/mode and speed; it creates floor 0 without combat |
| Encounter creation (`すすむ`) | Living participant, actor interval satisfied, no living enemies; floor 0 additionally requires leader. Clear applicable statuses and temporary/anti fields, advance exactly one floor and create its encounter; no NPC response or automatic allied action |
| Battle action | Authorized current living actor (or verified fallen-actor exception), registered action and valid target/resources. Process that explicit operation, applicable effects and bounded eligible NPC response; persist all resulting state |
| Defeat | Resolve revival and death effects within the operation. Keep fallen membership and earned facts; do not silently exit everyone, halve prior EXP or erase gold. With no living allies expose permitted exit/recovery rather than fabricating an automatic run-wide payout |
| Ordinary boss defeat | Preserve clear/chronicle facts and allow explicit advance into treasure floor 11; do not generate/open treasure merely because the batch resolver returned victory. EX beyond the ordinary boundary remains #1191 |
| Treasure inspection (`しらべる`) | Living actor selects a current unopened chest; validate actor allowance and any consumable key. Claim/delivery is individual and atomic; no NPC response. Remain in the room until explicit exit |
| Individual exit (`にげる`) | Distinguish enrolled participant from spectator. Participant receives only outstanding personal effects/history, leaves roster and releases ownership after mandatory processing; preserve others' encounter/resources/timing. Spectator receives no combat rewards |
| Leadership and room disposal | On leader exit choose the first remaining real participant in preserved roster order, not sorted lock order. Dispose room only after last real participant leaves and required settlement is durable; NPCs do not keep an empty normal room alive |
| Idle cleanup | Separate due detection, participant recovery/penalty and physical deletion; expired storage is never proof that rewards or mandatory work were processed |

Evidence: `quest.cgi:579–586`; `vs_monster.cgi:37–82,89–109`;
`_battle.cgi:640–665,1169–1178,1184–1227,1330–1345`.
Fallen actors are restricted, but not universally barred from operations:
`_battle.cgi:646–665` allows whisper/screenshot before the fallen check and
contains a specific self-revival exception. Spectators receive only
whisper/exit/screenshot in `_skill.cgi:22–30`. These operations' transport and
visibility reconciliation remains #948; screenshot copying is owned by Replay.
An unsupported exception must remain an explicit cutover blocker, not silently
disappear from discovery.

### Normal-run operation discovery

Normal runs discover actions from current encounter facts, not the automatic
batch resolver's priority list. Shared combat dispatch, targeting, resource and
effect gates are owned by the [Battle action matrix](battle.md#normal-adventure-action-and-target-matrix).
Evidence below uses legacy revision `ff97fc549932a146cc945d7edf71b44c868a840e`.
The user requested original behavior as the default on 2026-10-12 (#1327).

| Legacy operation / dispatch | Discovery, target and result | Native owner / verification |
| --- | --- | --- |
| `こうげき`, `ぼうぎょ`, `てんしょん`; current/previous-job, equipment/book/seal, custom and tool actions | `_skill.cgi:set_action:33–151,219–309`; current resources, job/SP, curse, equipment and encounter predicates select callbacks. Tool category 1 is also discovered outside the enemy-present skill block | #1188 supplies jobs; #1332/#1333 effect gates; #1197 consumption; #1329 command consumer; #1330 discovery. No automatic action when interval elapses |
| `すすむ` → `susumu` | `vs_monster.cgi:37–82`: no enemies, initial leader only, explicit one-floor advance; resets effects, creates encounter, disables NPC response. Ordinary treasure entry is a further explicit advance | #1328 owns phase/encounter state, #1329 reset effects, #1330 transport. Test floor 0/nonleader, living enemies and final ordinary floor |
| `しらべる` → `shiraberu` | Treasure floor only (`vs_monster.cgi:38–41`); chosen existing unopened NPC chest, individual allowance/key (`_npc_action.cgi:6–80`). No NPC response; inventory/depot outcome and chest state commit together | #1196 allowance/keys, #1197 consumption, #1328 individual claim, #1330 choice/observations |
| `にげる` → `nigeru` | `_skill.cgi:157–160`, `_battle.cgi:1184–1345`: participant exits individually; spectator exits without reward. Living and fallen roles differ from combat admission. Remaining roster continues; leader follows preserved order | #1328 owns settlement/removal and outstanding earnings; #1225 cooperative multiplier, #1221 fatigue, #1222 chronicle; #1330 tests survivor continuation |
| `まえ`, `うしろ` → `mae`, `ushiro` | `_skill.cgi:152–154,894–929`: party size >1, move actor to front/back; no listed MP. Ordinary accepted-action/status/tail processing applies; no blanket noncombat NPC suppression | #1329 owns ordered formation mutation and applicable response, #1330 advertises it; test persisted order and status-blocked action |
| `サバイバル`, `フォームチェンジ`, calling | `_skill.cgi:166–217,226–258`: no-enemy/job/SP/curse/item gates; self recovery, forward/reverse transformation and dynamic helpers; original costs in Battle matrix | #1333 owns missing native effects, #1329 executes certified entries. These cannot be omitted just because there is no enemy |
| `すとっく` → `stock_lv_up` | `_skill.cgi:312–317`: no enemies, level/EXP gates; `_battle.cgi:358–379` performs accumulated level processing, no listed MP. Distinct from pending recipient reward stock | #1328 uses existing progression/settlement primitives, #1329 handles operation facts, #1330 discovery; test threshold, repeated command and restart |
| `さそう`, `きっく` → `sasou`, `kick` | `_skill.cgi:161–164,769–783`: floor 0; kick leader only. Invitation dispatch redirects its log to the public quest area, not the room | Party owns waiting admission/permissions; #948/#1330 reconcile equivalent commands. Invitation is not room activity just because it was submitted from that room |
| Plain comment / unknown command | `party.cgi:21–27`: nonempty submission invokes action and writes comment unless `$mes`; `_battle.cgi:640` returns on unknown dispatch without setting an error, so unknown command can remain a comment | #948/#1330 own explicit comment acceptance and visibility. Unknown typed Gateway actions remain schema errors; any faithful text-submission path must treat unknown text as a comment rather than silently equate the two |
| `ささやき` → `sasayaki` | `_battle.cgi:640–649` checks dispatch/interval first, then returns before fallen/status/resource/tail processing. `system.cgi:251–262` validates named recipient, disables NPC response. No target is a public comment; private recipient may be outside encounter | #948/#1330 own allowed audience and sender/recipient-only observation (`system.cgi:622–630`); accepted writes renew room activity, not actor battle time |
| `すくしょ` → `sukusho` | `_battle.cgi:650–652` returns before combat tail; `system.cgi:165–207` copies owned scene/public comment lines, excludes private lines, sets `$mes` on success or failure, so no room comment write | #1320 owns archives/capture and #948/#1330 permitted transport; does not renew room activity or run battle effects |
| Spectator/fallen exceptions | `_skill.cgi:25–30`: spectator discovers whisper/exit/screenshot only. Fallen actor is blocked after whisper/screenshot dispatch except exit and the gated self-revival callback (`_battle.cgi:654–665`) | #1329/#1330 role tests; #1333 self-revival prerequisite. Discovery cannot imply spectators may fight; blanket rejection of fallen submissions loses original exceptions |

These are domain mappings, not newly registered HTTP routes. #1330 must supply
strict commands/owned observations through the existing Gateway; #948 owns
remaining combat transport reconciliation and #1320 Replay privacy/archive
work. Shared core does not send messages, archive screenshots or own room time.

### Earnings and inactivity evidence

`_battle.cgi:199–274` gives ordinary defeat earnings to living allies at the
defeat operation: directly for the acting character, and through a per-user
`stock.cgi` for other recipients. `get_stock` (`1459–1473`) consumes that stock
on applicable actions (`912`) and personal exit (`1208`). Try-mode stock has
different release behavior (`1915–1939`); ordinary unconditional earnings must
not be confused with conditional mode rewards. Ordinary earnings survive later
defeat; the batch crawl settles cleared-floor earnings even when its final
outcome is defeat. Defeat does not grant new victory drops or clear credit.
Per-operation durable recipient grants remain with #1328/#1329;
#1225 owns the cooperative multiplier, #1221 personal exit fatigue/flags, and #1222
chronicle consistency. Native durable entitlements replace stock files without
copying their architecture or claiming all reward formulas were audited here.

Try-mode release is a normal-adventure continuation gate, not another combat
resolver: `vs_monster.cgi:91–102` calls `success_try` only on explicit ordinary
treasure-floor creation. `_battle.cgi:1915–1949` aggregates each real party
recipient's pending EXP/gold/crystals, multiplies each aggregate by 1.5 with
integer truncation, rewrites pending stock and returns room mode to normal.
The generic action stock release at `912` excludes Try mode. #1328 owns this
conditional entitlement disposition and atomic transition; #1225
owns the separate cooperative multiplier. A retry must not apply 1.5 twice,
and ordinary unconditional earned assets must not be replaced by conditional
Try stock. On participant exit, `nigeru:1208` calls `get_stock`; while Try is
still active, `get_stock:1459–1473` reads and clears pending stock **without
crediting it**. After successful treasure entry, the mode is normal and the
converted stock can be credited once on a subsequent action or exit. Falling
alone does not invoke that personal exit disposition. #1328 must test
pre-success exit forfeiture separately from normal earned assets and timeout
preservation; timeout's already-approved retention is not an ordinary exit.

`quest.cgi:80–91,172–183` checks **strictly more than 1800 seconds** since the
room log modification, clears each non-NPC member's location and sets
`sleep=3600`, then deletes the room directory. It does not call `nigeru` or
`get_stock`, and user-owned stock is outside the deleted directory. Thus idle
cleanup is neither ordinary personal exit nor permission to delete pending
earnings. Participants already removed from the roster are not penalized again.
GET must never perform this mutation or renew its deadline in the reconstruction.

**User-approved native timeout contract (2026-10-11, #1325):** retain the strict 30-minute
inactivity threshold, trigger cleanup by an explicit owner command, retain
earned recipient assets/entitlements and apply a recoverable one-hour sleep
penalty to remaining real participants. Anchor that hour to the committed
cleanup instant; persist its deadline/processing identity before optional timer
projection, and preserve explicit Wake/recovery. A failed timer write cannot
release active ownership or extend the hour on retry. This fixes a native time
representation rather than certifying literal CGI timing: the original writes
a duration through `system.cgi:861–883`, while `sleep.cgi:19` later subtracts
time since previous login. No ordinary exit fatigue/history or full recovery
is inferred for timeout. Record a distinct timeout history disposition; any
additional loss, reward release or penalty rule needs evidence/approval.

### Room activity contract

Room activity and actor battle eligibility are separate clocks. The original
`party.cgi:21–27` writes a submitted comment only when `$mes` is unset;
`system.cgi:660–676` rewrites `$this_file.cgi`, normally the room log
(`_battle.cgi:19–20`). `quest.cgi:80–91` uses that file's modification time.
Thus the rule is accepted **room-log-equivalent submission**, not merely
successful damage or any POST. Preserve original admission/visibility and
distinguish an accepted no-effect result from a rejected operation.

| Operation / result | Update room last-activity time? | Evidence / required verification |
| --- | --- | --- |
| Accepted combat, including incapacitated action or accepted no-effect callback | Yes | `_battle.cgi:686–728,761–762,1174–1178`; no `$mes` → comment write. Test a status-blocked actor separately from a cooldown rejection |
| Accepted advance, treasure claim, formation change, between-encounter recovery/transformation/call, stock-level processing | Yes | Each normal dispatch returns through `party.cgi` comment write; callback `$mes` instead means rejected. Test no-effect versus error without inferring from HP changes |
| Accepted exit with room remaining | Yes, if its resulting log belongs to that room | `_battle.cgi:1330–1345`: last-participant disposal redirects `$this_file` to the quest area; there is then no surviving room to renew. Preserve participant/spectator distinction |
| Accepted plain comment, unknown legacy text command, public or private whisper | Yes, including permitted spectator/fallen submissions | Unknown action returns before interval check (`640`); recognized whisper checks interval then returns before combat tail (`641–649`). `system.cgi:251–262,660–676` writes it, even if no recipient was supplied. Do not make plain comments inherit combat cooldown; do not expose whispers in public observations |
| Banned skill silently ignored by legacy dispatch | Yes, as a comment; no actor combat timestamp update | `_battle.cgi:667–675` returns without `$mes`, so `party.cgi:25` still writes the submission. A combat effect not executing does not prove that its submission was rejected |
| Successful floor-0 kick | Yes | `_skill.cgi:796–837` removes a member and additionally writes a quest-area notice without redirecting `$this_file`; the caller still writes its room comment |
| Invitation or any dispatch redirected to another area's log | No | `sasou`, `_skill.cgi:772–776`, redirects `$this_file`. Posting from a room is insufficient evidence of room activity |
| Screenshot, including successful capture | No | `system.cgi:175–191` sets `$mes`; `party.cgi:25` skips room comment write. Archive mutation is not room activity |
| Rejected operation: cooldown, forbidden role, resource/target validation error with `$mes` | No | `$mes` prevents comment write (`party.cgi:25`). Unknown **text** or silently ignored skill accepted as a comment is different from a typed API action rejected by schema/validation |
| GET/refresh/discovery, elapsed time or due detection | Never | Existing approved #1325 observation boundary; no synthetic log write or effect tick |
| Same committed command retried / receipt read | No new activity instant | Existing approved #1325 receipt boundary: return committed facts, no second submission/log write. A new intentional comment with a new command identity is a new submission |

#1328 persists room activity in the same SQL commit as the accepted operation;
failed/rolled-back commands cannot renew it. #1330/#948 reconcile the actual
comment/whisper submission path and privacy, while #1320 owns screenshot capture.
Tests must cover accepted/rejected/no-effect text, private whispers, screenshot,
redirected logs, GET and receipt retries, strict `>1800` seconds and the existing
cleanup-time one-hour penalty anchor. This gate affects #1328 idle cleanup only;
its other lifecycle work does not wait for transport implementation.

### Combat travel and encounter replacement

This contract traces category-1 travel tools through discovery, callback,
action tail, persistence and the next operation at the revision above. These
are source-inspected requirements, not executed CGI parity or native coverage.
The user explicitly retained tool 150's old-enemy response/reappearance on
2026-10-12 (#1345); it must not be silently repaired during reconstruction.

All entries use `どうぐ` → `dougu` → the held item's callback, with listed MP
cost zero (`_skill.cgi:150–151,758–764`). A living enrolled participant may use
them with enemies, between encounters, at floor 0 or in the treasure room;
there is no travel-specific leader gate. Spectators cannot discover the tool,
and fallen actors cannot execute it (`_skill.cgi:25–30`;
`_battle.cgi:640–665`). The ordinary interval, ban and incapacitation gates
still apply. Incapacitation can skip the callback and consumption while
completing the applicable action tail; it is not a selection error.

| Tool / callback evidence (`lib/_data.cgi`) | Argument, condition and callback result |
| --- | --- |
| 66 / 1055–1067; 67 / 1069–1080; 68 / 1082–1093; 69 / 1095–1107; 70 / 1109–1120 | No participant target; normal adventure only. Assign stage 15/16/17/18/19 respectively and floor 1 |
| 71 / 1122–1137 | No participant target; normal adventure and actor's weapon 35 plus armor 37 (original names checked, catalog identities at `_data.cgi:452,582`). Assign stage 20/floor 1; missing set rejects without consumption |
| 104 / 1335–1346 | No participant target; normal adventure only. Assign stage 21/floor 1 |
| 184 / 1765–1789 | Explicit destination matching stage 23 or 25; assign that stage/floor 1. Missing/other destination returns selection error without consumption |
| 83 / 1243–1253 | No participant target; increment floor only in normal adventure with floor <9 (including floor 0). Otherwise accepted no-effect: consume, retain coordinates and process the normal tail |
| 107 / 1350–1361 | No participant target; normal adventure only. Assign floor 10; unchanged floor 10 is still accepted and consumed. Does not create chests or set clear. Distinct from chest allowance item 215 (#1196) |
| 150 / 1614–1639 | Explicit floor: reset all participants' status, assign floor, call `set_monster`, request reload. Missing/invalid floor or wrong mode rejects without consumption |

Other than tool 83's accepted no-effect branch, wrong-mode callbacks set an
error and retain the tool. `dougu` defaults a missing/falsy argument to the
actor, but floor/destination arguments are not participant IDs. Native input
uses an integer floor 1–10 or a destination stage ID restricted to 23/25;
parameter-free entries need no target. Reject null, strings, fractions,
out-of-range floors and foreign destinations before mutation. The original
150 predicate is an unanchored digit match plus Perl numeric comparisons
(`!=0`, `<=10`), not strict integer parsing; its malformed-string, fractional
and negative-number coercions are not native validation rules. This typed
boundary is required by #1345, not permission to change valid travel behavior.

**Coordinate assignment preserves the encounter.** Tools 66–71, 83, 104, 107
and 184 do not reset statuses/fields, call `set_monster`, clear enemies or
reload. Their accepted callbacks clear the held tool and continue the ordinary
player effects, stock/level processing, field counters and eligible bounded
NPC response, then update actor time/turn count and write room state
(`_battle.cgi:761–1178`; `_npc_action.cgi:89–155`). Thus old living enemies can
respond and survive under new coordinates. With no enemies, no encounter is
created by the tool. On the next request `read_member` rebuilds sides from the
persisted members (`453–493`). Only explicit `すすむ`, after the no-enemy and
end/EX gates, resets statuses, deletes temporary/anti fields, increments the
assigned floor once and creates an encounter (`vs_monster.cgi:37–82`). A
stage-changing tool therefore next creates destination floor 2, not floor 1;
83 at floor 8 next creates floor 10; 107 next creates floor 11 without fighting
a newly generated boss. A nonleader using travel at floor 0 can bypass the
initial-advance leader restriction by assigning a positive floor. Do not add a
leader-only travel restriction or auto-complete the bypassed encounters.

**Tool 150 continues after reload.** `reset_status_all` includes its accessory
revival/reset hooks (`_battle.cgi:1390–1430`); it is not HP/MP restoration.
Unlike `susumu`, the callback does not delete temporary/anti fields or disable
NPC response. `set_monster` replaces `@members` with allies and adds new NPCs
but leaves `@enemys` and old `%ms` entries intact (`vs_monster.cgi:85–155`;
`_npc_action.cgi:600–681`). `auto_reload` only emits navigation and sets waiting
time; it does not exit the callback/action (`system.cgi:456–470`). Consequently
the tool is consumed and the normal action tail still runs. If an old living
interval-ready enemy responds, `npc_turn` uses the old enemy list and rebuilds
members from it, potentially restoring old enemies and dropping newly created
ones; a generated NPC with the same legacy name overwrites that old payload.
If none responds, this NPC path does not replace the generated members. With
no old enemies, the new encounter gets no immediate response. Native encounter IDs
must preserve this observable distinction without making user-supplied names
authoritative. Test both branches, name collision and reload after each result.
Treasure-floor discovery suppresses NPC response before the tool callback
(`vs_monster.cgi:38–41`), so travelling out of that room retains suppression
for the submitted operation. New discovery is computed on the next request.

**Room facts survive travel.** The room ID/log destination, ordered allies,
leader, mode, record header and earned recipient facts are not reinitialized.
Member persistence writes changed stage/floor but preserves `$r_stage` and
records (`_battle.cgi:464–468,496–554`); the record header is not a travel stage
mirror (`quest.cgi:1287–1296`). Accepted travel writes the same room log;
Replay's title was selected before action, while captured members reflect its
result (`party.cgi:20–27`; `_skill.cgi:23`; `system.cgi:660–677,1499–1519`).
Refresh observes the committed result without running another action. Native
SQL receipts must freeze draws/results and consumption once (#1328/#1197).

Travel itself neither pays nor erases earnings; normal stock processing still
runs through the lifecycle owner (#1328). Assignment-only callbacks retain
`$is_clear` and existing EX markers. Tool 150's encounter creation at floor 10 sets clear; stage 24 also
creates its EX marker and applies its ordinary special boss-field reset/caps
owned by #1342. Ordinary
treasure creation at floor 11 consumes the retained clear flag for the
**current** stage, runs Try success if applicable and can unseal stage 19
(`vs_monster.cgi:91–155`). It is not proof that the destination boss was beaten.
Next advance from assigned floor 10 still executes the destination's EX flag
path before creating floor 11 (`52–80,227–249`); existing EX files are not
deleted by travel. Try mode is retained until treasure creation invokes
`success_try`, multiplies pending stock by 1.5/floors it and changes mode to
normal (`_battle.cgi:1915–1949`). Preserve these transition hooks under the
existing EX (#1191) and Try lifecycle (#1328) owners, including their ordering
before any old-enemy response; do not replay rewards or infer clear solely from coordinates. Exit
uses the changed floor for fatigue/other floor-dependent effects (#1221), not
the number of encounters actually fought (`_battle.cgi:1208–1222`).

### Remaining reconciliation gates

Native travel transitions remain owned by
[#1353](https://github.com/witchcraze/party2re/issues/1353), with the full action
tail consumed by #1329 and typed discovery/submission by #1330. Other tool
arguments also include recorded jobs and participants; this travel contract
does not certify those callbacks (`lib/_data.cgi:1879–1924`).

MP restoration, MP drain and resource-consuming gems use the
[shared battle resource contract](battle.md#resources-revival-and-roster-changes).
Run consumers persist the resulting facts and apply enclosing costs in order.

The operation matrix establishes the family boundary. Production cutover still
requires #1188 job supply, #1332 effect lifetimes and the effects/transactions
decomposed by [#1333](https://github.com/witchcraze/party2re/issues/1333) into
#1337–#1346. #1342 preserves full ordinary NPC/boss payloads, multiple/conditional
rosters and stage 24's ordinary-floor special field. The
[entry ownership reconciliation](battle.md#dynamic-effects-and-catalog-admission)
distinguishes those payloads from their concrete callback bodies, descendant
closure, registration and production wiring. Its implementation owners remain
cutover prerequisites after the audit itself closes; #1346 enforces support for
the whole reachable effect in both discovery and execution. Indirect invocation
must not bypass that gate or narrow the original random pool to implemented
effects. Missing original choices remain cutover blockers. #1328 implements
Try success/exit disposition and #1196/#1197 item rules.
Random/delegated discovery and submission also depend on the
[transitive job closure](battle.md#random-and-delegated-job-closure): discovery
uses `skill.BattleDefinitions.InvocationAction` and execution shares core
invocation preflight. #1358 owns arithmetic selectors, #1359 job wallet/held-tool
mutations and #1360 job-driven room transitions. Tool-only travel coverage does
not certify a job callback that replaces an encounter, changes job SP or
reorders enemies. Discovery must retain current/previous-job and caller context
when applying the shared whole-effect gate; a supported direct child is
insufficient when any hidden descendant is unsupported. The whole submitted
operation must reject before consumption or durable mutation in that case.
Resolve the outer tool argument (default self) or custom target before passing
the callback frame. Keep complete ordered tables and source-context variants,
rebuild selection against current run actors, and preserve each generated
child's captured identity. Nested callback changes must retain their ordered
rosters and assets; each later custom slot receives the original resolved target
and runs only while the actor remains alive. The run service must freeze command
draws/results with its receipt and return committed facts on retry, without
invoking the executor again. Executor support alone certifies no production
job/gem/tool table and does not provide SQL run/receipt storage.
For [arithmetic effects](battle.md#arithmetic-selectors-and-digit-reversal),
discovery and submission share `skill.BattleDefinitions.ArithmeticAction` with
the same ordered discovered names and submitted text. Keep the selected callback
identity through nested invocation and the divisor within that operation. Apply
ordinary costs after the callback, so MP-divisibility reads the source's current
resources. Persist resulting HP, temporaries, revival/drop facts and completed
end processing with the command receipt; reload must not rebuild participants
or rerun selection. An unavailable defeat/reset closure or unsupported discovered
child rejects before any outer consumption or committed draw. This finite
effect support does not enable the full random pool or the SQL command consumer.
The [combat asset contract](battle.md#combat-books-stored-tools-and-purchases)
also applies to discovery and nested submission: distinguish requester/payer,
temporary actor, ordered depot selection and held-item instances. Job50 binds
the outer submitted command/item/target, even through indirect invocation;
its used-definition history is scoped by room and actor color, not by inventory
instance or individual player. Book exchange selects the first matching stored
instance; typed book targets allow the same encounter participant twice.
Enhanced tools use their own 22-entry table and evidenced numeric-argument
binding; ordinary tool84 and enhanced84 are different effects. Return after
second-child death and requester-wallet debit during NPC purchase must survive
reload. Missing native context or capacity policy rejects the whole entry;
native storage and callback implementation prerequisites remain cutover gates.
Persist nested effects, histories, debit, item transfer and command receipt in
one transaction. A committed retry observes its prior draw/result; GET cannot
execute a purchase, mark a stored tool used or return a book. #1139 owns the
unresolved capacity-unchecked return policy, and #1197 owns consumed instances.
The [shared modifier contract](battle.md#shared-modifiers-and-callback-outcomes)
also gates direct entries: helper invocation/returned target does not prove an
effect applied, and callback category differs from elemental names. Preserve
accepted no-effect versus rejection, actor/target context and suppressed tails.
The #1343 decomposition separates native target, damage, recovery/stat, status/reset
and field/composite work. Explicit recovery/stat operations preserve base/current
stats, helper outcomes and ordered incidental effects as described in the shared
contract. Explicit single/area damage, finite damage-gem formulas and immediate
recoil preserve ordered modifiers, hit counts and success-dependent tails in
that same contract; gear terms must be saved rather than redrawn on reload.
Their finite producer formulas do not admit outer custom/random/calling
closures. The consumer must preflight all descendants and required reset/defeat, damage,
requester and cursor context before costs or committed RNG.
The finite damage-gem registry and its already-applied target MP facts follow
[Battle's shared damage contract](battle.md#shared-modifiers-and-callback-outcomes);
the consumer must keep slot CMP contributions separate from MP and persist
absorption/damage results without replaying them on reload.
The [shared status/reset contract](battle.md#shared-modifiers-and-callback-outcomes)
also supplies the seventeen status/cure/death gem bodies and job35.1's ordered
dispel. Preflight the complete reset/defeat closure, including later members and
post-revival continuations, before any costs or draws. Retain raw equipment and
request table-load seal facts; supply requester level, current actor and reset
target independently. Reward and fortress callbacks remain their owners' work;
helper registration alone does not connect an Adventure command or admit mixed
custom slots. Item110 and unrepresented recursive descendants remain rejected.

Player/NPC equipment tails, player recovery, CMP and extended-effect bodies use
[Battle's operation-tail contract](battle.md#explicit-action-resumption-requirements).
Capture fallen-party and equipment-tail snapshots before callbacks, retain
source-specific player/NPC and normal-action/combat-tail guards, and finish the
operation counters once.

Revival and sacrifice use [Battle's ordered revival contract](battle.md#resources-revival-and-roster-changes).
Ordinary fallen-self discovery requires the66.6 entry and its living designated
helper; direct callback admission is not ordinary role/learning admission.
Persist resulting revival, consumed-accessory and ban facts without replaying
the callback on reload. Mixed slot transition projection remains #1436's owner.

Summoned identity, command-actor restoration and save-boundary
roster rules are owned by [Battle's roster contract](battle.md#summoned-roster-and-helper-commands).
`Stage.SummonPool` requires complete certified payloads in the current seasonal
stage's original table order; it never filters through ordinary-floor boss
exclusion or silently fills missing job/custom effects from scalar monster stats.
Weighted appearance positions remain separate server-owned input. Service/run
storage and Gateway execution retain #1329/#1330 ownership.
The #1367
[callback context contract](battle.md#callback-context-and-failure-admission)
distinguishes the requesting player, temporary actor, package target cursor,
category and operation-entry field snapshot. Preserve those explicit contexts;
missing native representation remains a whole-command gate. Gems101/102 have no
successful execution through the original queue consumers and remain unavailable;
do not repair or skip them, narrow a random pool, or execute an earlier custom
slot before rejecting the command. Item110's recursive roster/reset failure also
remains rejected. Specification resolution does not certify native coverage or
remove #1329/#1330's remaining implementation blockers.
Shared [dynamic-effect rules](battle.md#dynamic-effects-and-catalog-admission)
and [resumption requirements](battle.md#explicit-action-resumption-requirements)
are owned by Battle; commands and owned observations by
[Client/Agent API](../architecture/client-agent-api.md#planned-normal-adventure-commands-and-recovery).

The following sections describe the **current batch crawl** and existing models.
They are not authorization to preserve automatic battles, advance, inspection
or exit in the reconstructed interactive adventure.

---

## 1. Dungeon Crawl Loop (10 Floors + Floor 11 Treasure Room)

Adventures proceed floor-by-floor in accordance with legacy `vs_monster.cgi` and `_battle.cgi`:

```text
[Floor 1–9: Normal Monsters] ──> [Floor 10: Stage Boss] ──> [Floor 11: Treasure Room]
            │                                 │
            └── (Defeat) ───┐                 └── (Defeat) ───┐
                            ▼                                 ▼
                     [Early Defeat]                    [Early Defeat]
                     - Half EXP, 0 Gold                - Half EXP, 0 Gold
                     - 0 Crystals                      - 0 Crystals
                     - Characters keep 1 HP            - Characters keep 1 HP
```

### Floors 1 to 9: Stage Monsters
- Spawns 1 to 3 enemies chosen from the stage's normal monster roster (`stage.GetNormalMonsterIDs()`).
- Normal monsters exclude designated stage bosses.
- Multi-participant turn combat is resolved using `corebattle.PartyBattleResolver`.
- On victory: allies advance to the next floor with their remaining HP and MP preserved across floors. Total EXP, Gold, and monster Crystal drops (`TotalCrystals`, 刻印晶) accumulate.

### Floor 10: Stage Boss Battle
- Spawns the stage boss (`stage.GetBossIDs()`). If boss IDs are not explicitly configured, the final monster in the stage's monster list is designated as the stage boss.
- On defeat of the boss: the stage is marked as cleared (`is_cleared = true`), and the party gains access to Floor 11.

### Early Defeat Handling
- If all living party members are knocked unconscious at any floor (1–10), the crawl terminates immediately.
- `FloorsCleared` is set to the last fully cleared floor number.
- Previously earned unconditional EXP, gold and crystals are retained and settled, without granting rewards for the lost floor. Try stock follows the separate [conditional release contract](#earnings-and-inactivity-evidence).
- Participating characters survive with 1 HP (preventing permanent death).

---

## 2. Floor 11 Treasure Room & Bonus Formulas

Upon defeating the Floor 10 boss, the party enters Floor 11 (Treasure Room) per legacy `vs_monster.cgi` (`$boss_round + 1`) and `_npc_action.cgi` (`add_treasure`).

### Treasure Box Count Calculation
The number of treasure chests generated in Floor 11 follows authentic legacy Party2 rules (`vs_monster.cgi:90-101`, `_npc_action.cgi:498-519`):

$$\text{Index} = \text{BaseIndex} + \text{MerchantBonus} + \text{TreasureHunterBonus} + \text{LuckyPendantBonus} + \text{GamblerBonus} + \text{TreasureBlessingBonus} - \text{DeadMembers}$$

1. **Base Index & Inclusive Generation**:
   In legacy Party2, treasure generation initializes a loop upper bound `$count` (`vs_monster.cgi:92`) and spawns chests via an inclusive loop `for my $i (0 .. $count)` (`_npc_action.cgi:519`):
   - Standard Stages: $\text{BaseIndex} = P - 1$ (where $P$ is total party members). With $D$ dead members, $\text{Index} = P - 1 - D = A - 1$. The inclusive loop $0 .. \text{Index}$ produces $A$ (surviving members) chests when no job/item bonuses apply.
   - Multiplier Stages (Stage 17 幻の秘境, Stage 20 天空城, Stage 21 カオスフィールド): $\text{BaseIndex} = 3 \times P$. With $D$ dead members, $\text{Index} = 3P - D$. The inclusive loop $0 .. \text{Index}$ produces $3P - D + 1$ chests ($3A + 2D + 1$; e.g., $N=2$ with 2 alive yields $2 \times 3 + 1 = 7$ chests).
2. **Acting Player (操作主体) Bonus Scope**:
   Job, accessory, and chapel blessing bonuses are evaluated strictly for the session leader / acting player (matching legacy `%m` in `vs_monster.cgi` and `_npc_action.cgi:501`), never aggregated across arbitrary party members:
   - **Merchant Job Bonus** (`job-07` / 商人): +1 treasure chest.
   - **Treasure Hunter Job Bonus** (`job-78` / トレジャーハンター): +1 to +2 treasure chests ($1 + \text{rand}(2)$).
   - **Gambler / Pirate Job Bonus** (`job-81` / 海賊): $+\text{rand}(3) - \text{rand}(2)$ treasure chests. If the leader holds Lucky Pendant (`item-191`), the index cannot drop below $P - 1$.
   - **Lucky Pendant Bonus** (`item-191` / ラッキーペンダント / 海賊ハット): $1/3$ chance ($\text{rand}(3) == 0$) of +1 treasure chest. Checked strictly on the equipped accessory slot (`$m{ite}`), not unequipped inventory items.
   - **Chapel Treasure Blessing Bonus** (`DROP` / `4` / `treasure_wish` / `宝箱がほしい`): $1/5$ chance ($\text{rand}(5) == 0$) of +1 treasure chest (`_npc_action.cgi:501`: `$m{wish} eq '4'`), evaluated strictly for the session leader once when spawning chests in the treasure room. Non-leader member blessings do not apply.
3. **Legacy Falsy Index Fallback**:
   In legacy `_npc_action.cgi:499` (`my $count = shift || $#partys;`), if the index passed from `vs_monster.cgi` is `0` (Perl falsy, e.g., a 2-player party whose Pirate roll resulted in $1 - 1 = 0$), it falls back to $P - 1$ before evaluating the Chapel Blessing and dead member penalty.
4. **Lower Bound**: If surviving party members $\le 0$, 0 chests are spawned. If the final index $< 0$, 0 chests are spawned; otherwise, $\text{Index} + 1$ chests are spawned.

### Treasure Box Loot Pool Distribution & Weekday Orbs
Loot in Floor 11 chests is drawn from the stage's drop tables (`treasure_weapons`, `treasure_armors`, `treasure_items`) per `_npc_action.cgi:540-550`:
- **Distribution**: Category roll $v = \text{rand}(4) + 1$ (if $v > 3$, $v = 3$):
  - $v = 1$: Weapon (25% chance, drawn from stage `treasure_weapons`).
  - $v = 2$: Armor (25% chance, drawn from stage `treasure_armors`).
  - $v = 3$: Item / Tool (50% chance, drawn from stage `treasure_items`).
- **Weekday Orb Pool Injection**: The current day-of-week orb is dynamically injected into the Tool pool:
  - Sunday: Random orb (`item-060` through `item-065`).
  - Monday: 月のオーブ (`item-060`).
  - Tuesday: 火のオーブ (`item-061`).
  - Wednesday: 水のオーブ (`item-062`).
  - Thursday: 木のオーブ (`item-063`).
  - Friday: 金のオーブ (`item-064`).
  - Saturday: 土のオーブ (`item-065`).

### Once-Daily Challenge Lock (`CategoryDungeonOnce` / `$m{once}`)
- Certain limited dungeons can only be challenged once per day (until the character sleeps and wakes at Home):
  - Stage 26: 神秘の森 / 四季のダンジョン (`stage-26`)
  - Dungeon 16: 機軍要塞
- **Admission & Consumption Rules**:
  - Enforced via Valkey/ephemeral `TimerService` (`timer.CategoryDungeonOnce = "dungeon_once"`).
  - Shared across solo crawl admission (`adventure.Service.ExecuteCrawl`) and party creation (`party.Service.CreateParty`).
  - Quota is consumed immediately upon party creation (matching `party2/lib/quest.cgi:594-599` `$m{once}=1`) or upon initiating solo crawl. If creation/crawl fails before transaction commit, acquired locks are compensated via token-bound release (`ReleaseLockWithToken`).
  - Once a party is created, disbanding or adventure start failure does not restore the creator's once-per-wake quota (quota remains consumed).
  - Quota is consumed only for the creator/starter (the solo explorer or the party leader). Non-leader party participants do not have their once-lock consumed and are not blocked from creating/leading their own runs.
  - Locks persist without arbitrary TTL expiration (duration = 0) and are cleared exclusively upon awakening from sleep at Home (`internal/home.Service.Wake` via `ResetDungeonOnceWithClaim`).

### Treasure Chest Examination (`ExamineTreasure`) & Post-Battle Settlement
- Each surviving party member may examine and claim treasure chests on Floor 11 (`_npc_action.cgi:6-56 shiraberu`).
- **Inspection Allowance & Treasure Pocket (`item-215`)**:
  - Each surviving participant has a base inspection allowance of 1 treasure chest (`1人1箱`).
  - To open additional chests beyond the 1-chest allowance, the participant must possess Treasure Pocket (`item-215` / トレジャーポケット / `ItemTreasurePocket`, also referenced as Master Key / `ItemMasterKey` in earlier tickets).
  - Stack quantity: Multiple stacked Treasure Pockets allow opening additional chests up to the carried quantity; each additional chest opened consumes 1 `item-215`, accumulated in `ConsumedItems` alongside combat consumption and deducted during post-battle settlement.
  - Rejection conditions: non-participants (`ErrNotParticipant`), unconscious characters (`ErrUnconsciousCharacter`), exceeding allowance without sufficient Treasure Pockets (`ErrTreasureLimitReached`), or no remaining unopened chests (`ErrNoUnopenedTreasure`) are rejected.
  - Automatic solo/party crawl loops only claim the base 1-chest allowance per surviving participant. Opening additional chests with `item-215` is not executed automatically to avoid unintended item loss; explicit manual chest selection is reserved for player action commands (#948 / #1330).
- Post-battle settlement delegates to `battle.Service.ApplyPostBattleResult` (`PostBattleSettler`), enforcing atomic single-transaction persistence obeying the global lock hierarchy (Rank 2 `characters` -> Rank 3 `inventory_items` -> Rank 5 `character_depots`):
  - Obtained treasure items are added to the character's inventory (`coreinventory.Inventory`).
  - When the inventory is full, items automatically route to the character's depot (`_npc_action.cgi:60-80`).
  - If the depot is also full, drops are tracked in `LostDrops` without silent phantom delivery or data loss.
  - Surviving HP and MP are persisted to `characters` in MariaDB. If defeated, fallen characters survive with 1 HP.
  - **Exit Fatigue (`CalculateExitFatigue`, `_battle.cgi:1187-1188, 1216`)**: `CrawlSession.Round` tracks the current/exit floor (`$round`, starting at `0` on room creation, updated on floor entry `1..11` or travel floor transitions, distinct from `TotalTurns`, `FloorsCleared`, and `len(FloorResults)`). At crawl conclusion, each participating character gains `add_tired = round * 3 + 1` fatigue (halved with integer truncation `int(add_tired * 0.5)` when `mode == "calm"`), clamped to `MaxTired = 100`.
  - Standard job progression, EXP with job mastery triggers, gold, and crystal rewards are applied atomically.

---

## 3. Party & Quest Room Rules (`quest.cgi`)

Multiplayer co-op quest rooms allow up to 4 adventurers to form an expedition lobby before embarking on a dungeon crawl.

### Room Settings
- **Capacity**: 1 to 4 characters (`max_members`).
- **Speed Settings**: Original per-actor action interval in seconds (not only animation/log pacing); currently stored by Party but not enforced by the batch crawl:
  - `3`: さくさく (Fast)
  - `18`: まったり (Relaxed) — **Default** ($def\_spd = 18$)
  - `25`: じっくり (Deliberate)
- **Secret Passphrase (`合言葉`)**: Optional password requirement to enter private quest rooms.
- **Participation Conditions (`need_join`)**:
  - Encoded condition string: `{key}_{val}_{u|o}`
    - `key`: `hp` (MaxHP) or `joblv` (JobLevel).
    - `val`: Target threshold integer.
    - `u`: Under / less than (`< val`).
    - `o`: Over / greater than or equal to (`>= val`).
  - Validated on both room creation (leader must qualify) and member join (`ErrNeedJoinConditionFailed`).
- **Stage Access Gates (`$job_lv[$stage]`)**:
  - Each stage specifies a `RequiredJobLevel` based on the character's total job changes (`JobLevel`).
  - Novice stages (0–2) require JobLevel 0, normal stages 3–14 require `JobLevel >= stage - 2`, and special stages require up to JobLevel 50 (stage 22: 6, stage 24: 50, stage 25: 10), with seasonal availability enforced in JST for winter (stages 23 and 25 in Dec–Feb) and Halloween (stage 27 in Oct).
- **Mode Settings (`mode`)**:
  - `normal` (Default; empty string defaults to `normal`): Standard dungeon crawl mode.
  - Unimplemented modes (`calm`, `try`, `hard`): Explicitly rejected on party creation (`ErrUnsupportedMode`) to prevent running crawls with incomplete reward/unlock rules until milestone unlocks and full reward scaling are reconciled. Unrecognized mode strings return `ErrInvalidMode`.
- **Idle Lobby Auto-Disband**:
  - Waiting lobbies have a 30-minute idle TTL (`DefaultLobbyTTL = 30 * time.Minute`), matching legacy `quest.cgi` 30-minute auto-disband.

---

## 4. Cooperative Synergy & Progression

### Synergy Multipliers
Multi-player party crawls grant cooperative reward boosts:
- 1 Member: 0% bonus (base rewards)
- 2 Members: +10% bonus EXP and Gold
- 3 Members: +20% bonus EXP and Gold
- 4 Members: +30% bonus EXP and Gold

### Event Hooks & Integrations
- **Victory Hook (`adventure.VictoryHook`)**: Invoked when an adventure or party expedition concludes with a victory. Automatically records milestone achievements in `internal/medal` (`adventure_victories`, `monsters_slain`, `gold_earned`).
- **Post-Adventure Hook (`adventure.PostAdventureHook` / `party.PostAdventureHook`)**: Invoked after crawl rewards, exit fatigue, and character state are committed (on both victory and defeat). Resets the character's Tavern meal fullness flag (`$m{is_eat} = 0` via `tavern.Service.ResetFullness`) and claims any pre-ordered meal delivery from the Adventurer's Tavern (`tavern.Service.ClaimDelivery`).
- **Hook Error Semantics**: Both hooks execute as best-effort post-commit side-effects outside the settlement transaction. Errors returned by hook implementations are logged as warnings (`s.logger.Warn`) and do not fail the completed adventure or corrupt committed character/history state.
- **Settlement & Persistence Atomicity**: Post-battle reward/item/fatigue settlement (`settlePostBattle` or fallback character updates) and `adventures` history persistence (`Repository.Save`) execute within the same `TransactionProvider` (`RunInTx`) Unit of Work, ensuring that any history save error rolls back all rewards, fatigue changes, and partial participant records without firing post-adventure hooks.

---

## 5. Adventure Chronicle & Milestone Progression (`adventure_record.cgi`)

The Adventure Chronicle provides authenticated players with historical logs, statistical summaries, stage completion breakdowns, and milestone progression tracking for their characters' past adventures.

### Paginated History (`GET /characters/{id}/adventures`)
Retrieves chronological records of previous expeditions with catalog-enriched stage and monster display names (`started_at DESC`).
- `limit`: 1 to 100 (default: 20).
- `offset`: `>= 0` (default: 0).
- Response fields: `id`, `character_id`, `stage_id`, `stage_name`, `monster_id`, `monster_name`, `outcome`, `battle_turns`, `experience_reward`, `currency_reward`, `started_at`, `resolved`.

### Statistical Aggregation (`GET /characters/{id}/adventure-chronicle`)
Aggregates overall statistics and per-stage completion records:
- **Overall Stats**: `total_adventures`, `total_victories`, `total_defeats`, `total_draws`, `win_rate` (rounded to 4 decimals), `total_turns`, `total_exp_earned`, `total_gold_earned`.
- **Stage Breakdown**: `stage_id`, `stage_name`, `seasonal_sub_id`, `season`, `clear_count`, `total_attempts`.
- **Seasonal Sub-Identity**: Stage 26 (神秘の森 / 四季のダンジョン, `vs_monster.cgi:111-120`, `adventure_record.cgi:49-86`) tracks and aggregates clears separately by seasonal variant (`seasonal_sub_id`: 1=spring, 2=summer, 3=autumn, 4=winter; 0 for normal stages) rather than collapsing into a single stage count.
- **Unified Solo & Party Clear Source**: Completed multiplayer party crawls (`internal/party`) and solo crawls (`internal/adventure`) persist individual participant records into the shared `adventures` repository within their transactional units, ensuring unified chronicle counts (`solo1 + party1 = 2`). Deterministic run identifiers ensure retrying the same run updates rather than duplicates clear counts (`same run retry = 1`).
- **Backfill Feasibility from Historical Logs**: Historical `party_adventure_logs` can be backfilled into `adventures` for standard stages using participant IDs from `details_json` (`AlliesSurvived` + `AlliesFallen`). For Stage 26, historical party logs lack persisted seasonal variant metadata and are backfilled as unclassified (`seasonal_sub_id=0`). Moving forward, `StartPartyAdventure` writes exact seasonal sub-identity directly at crawl completion.
- **Combat Travel vs. Clear Flag Contract**: Per #1345/#1353, coordinate-changing tools (tools 66–71, 104, 184, 107) do not modify `is_clear`; boss defeat sets `is_clear`, which is retained across coordinate changes and consumed when Floor 11 treasure room runs for the destination stage coordinates, with travel itself recording no duplicate achievements.
- **Milestone Tiers** (Unlocks based on total cleared stage counts per `adventure_record.cgi` / `vs_monster.cgi`):

| Milestone Key | Display Name | Clear Count Threshold | Description |
| :--- | :--- | :--- | :--- |
| `try_mode` | トライモード (Try Mode) | 50 | Unlocks Try Mode adventure expeditions |
| `image_setting` | イメージ設定 (Image Setting) | 100 | Unlocks custom character image configuration |
| `calm_mode` | カームモード (Calm Mode) | 150 | Unlocks Calm Mode adventure expeditions |
| `hard_mode` | ハードモード (Hard Mode) | 300 | Unlocks Hard Mode adventure challenges |
| `avatar_setting` | アバター設定 (Avatar Setting) | 500 | Unlocks special avatar portrait customizations |
| `extreme_mode` | エクストリームモード (Extreme Mode) | 1000 | Unlocks Extreme Mode high-difficulty adventure expeditions |

---

## 6. Exploration Subsystem Boundaries

Party2 features three distinct exploration and dungeon crawl modes with specialized responsibilities:

| Exploration Mode | Go Package | Legacy Reference | Primary Mechanics | Canonical Document |
| :--- | :--- | :--- | :--- | :--- |
| **Stage Adventure** | `internal/adventure` | `quest.cgi`, `vs_monster.cgi` | 10-floor sequential stage crawl, Floor 11 treasure room, job gating | This document (`adventure.md`) |
| **Grid Dungeon** | `internal/dungeon` | `vs_dungeon.cgi`, `map/` | 2D tile matrix exploration, hazard traps, `@ちず` scouting, escape portals | [`dungeon.md`](dungeon.md) |
| **Endurance Challenge** | `internal/challenge` | `vs_challenge.cgi`, `challenge/` | Consecutive wave survival, dynamic enemy scaling, no inter-round healing, Hall of Fame | [`challenge.md`](challenge.md) |

---

## 7. Storage & Database Schema (Migration 073, 109)

The `adventures` table schema reflects immediate crawl resolution and seasonal sub-identity tracking:

| Column | Type | Constraints | Description |
| :--- | :--- | :--- | :--- |
| `id` | VARCHAR(64) | PRIMARY KEY | Unique adventure session identifier |
| `character_id` | VARCHAR(64) | NOT NULL, INDEX | Foreign key referencing `characters(id)` |
| `adventure_type` | VARCHAR(64) | NOT NULL | Stage identifier (e.g. `stage-01`) |
| `stage_id` | VARCHAR(64) | NOT NULL DEFAULT '' | Explicit stage reference |
| `monster_id` | VARCHAR(64) | NOT NULL DEFAULT '' | Encounter monster reference |
| `started_at` | DATETIME | NOT NULL | Crawl execution timestamp |
| `floors_cleared` | INT | NOT NULL DEFAULT 0 | Number of floors cleared (0–10) |
| `is_cleared` | BOOLEAN | NOT NULL DEFAULT FALSE | True if Floor 10 boss was defeated |
| `seasonal_sub_id`| INT | NOT NULL DEFAULT 0 | Seasonal sub identity (0: normal, 1: spring, 2: summer, 3: autumn, 4: winter) |
| `season` | VARCHAR(32) | NOT NULL DEFAULT '' | Seasonal variant name (`spring`, `summer`, `autumn`, `winter`) |
| `party_size` | INT | NOT NULL DEFAULT 1 | Total party size (1–4) |
| `experience_reward`| INT | NOT NULL DEFAULT 0 | Total EXP awarded |
| `outcome` | VARCHAR(32) | NOT NULL DEFAULT '' | Battle outcome (`WIN` or `DEFEAT`) |
| `turns` | INT | NOT NULL DEFAULT 0 | Total combat turns across all floors |
| `resolved` | BOOLEAN | NOT NULL DEFAULT TRUE | Immediate crawl resolution status |
| `created_at` | DATETIME | NOT NULL | Record creation timestamp |
| `updated_at` | DATETIME | NOT NULL | Record update timestamp |

> [!NOTE]
> Fictional columns `available_at` and `claimed`, along with index `idx_adventures_character_claimed`, were permanently dropped in migration `073_purge_adventure_timer.sql`. Migration `109_adventure_seasonal_sub_identity.sql` added `seasonal_sub_id` and `season` along with composite index `idx_adventures_character_stage_season (character_id, resolved, adventure_type, seasonal_sub_id)` to preserve four-seasons sub-identities and unify solo/party clear counting in chronicles.
