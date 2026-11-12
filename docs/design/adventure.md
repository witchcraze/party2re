# Adventure System Design

## Overview

The Adventure system reconstructs the operation-driven stage adventure of legacy Party2 (`quest.cgi`, `vs_monster.cgi`, `_battle.cgi`). Evidence paths are under `party2/` at revision `ff97fc549932a146cc945d7edf71b44c868a840e`.

Production starts still resolve all battles, floor advances and treasure inspections in one request (the batch crawl). This is an implementation gap, not the original game: the user directed restoration of player-command progression (#1323). Removing the invented one-hour expedition timer does not remove the original per-actor action interval. The native interactive run (`NormalRunService`) still lacks explicit combat (#1329) and its Gateway cutover (#1330).

## Original interaction contract and reconstruction target

These are source-inspected requirements, not a claim of full CGI execution or complete skill/formula parity.

- **Admission and progress are separate.** A normal quest is created at floor 0 (`lib/quest.cgi:579–586`). The first `すすむ` requires the leader; later advances are not leader-only. Living enemies prevent advancing (`lib/vs_monster.cgi:35–82`). Advancing creates the next encounter; it does not resolve the battle or continue to another floor.
- **Players choose actions.** `lib/_skill.cgi:set_action` derives available actions from the actor's job, learned skills, resources and situation; `lib/_battle.cgi:561–659` parses an action and target and checks actor state. A server-selected skill or lowest-HP target is not a replacement. Some skills have additional-action behavior.
- **Actions have an interval.** `read_member` assigns the room's speed to `act_time` (`lib/_battle.cgi:450–468`); an action is rejected when elapsed time since that actor's previous action is below that interval (`641–644`). The accepted path updates actor timing and persists shared state (`1169–1178`). This is not a round barrier, not an animation delay, and a due deadline does not execute an action.
- **NPC responses are bounded by the submitted operation.** The eligible action path invokes `npc_turn` unless suppressed (`lib/_battle.cgi:1169–1171`); `lib/_npc_action.cgi:89–155` selects at most one living, interval-ready enemy and rotates its order. Advancing and treasure-room operations disable the response (`lib/vs_monster.cgi:37–53`). GETs never execute combat; there is no background auto-combat or agility-sorted party round.
- **Treasure and exit are individual operations.** `しらべる` selects a chest and checks the actor's allowance/item rules (`lib/_npc_action.cgi:5–85`); entering the treasure room does not open chests. `にげる` distinguishes a participant from a spectator, removes that actor and applies participant exit effects (`lib/_battle.cgi:1184–1278`). Participants leave at different times; one exit is not a party-wide finish.
- **Scene capture follows actual submissions.** `party.cgi:20–27` records an eligible operation, including commentary, using the pre-operation location. Rejected operations and refreshes do not record. Content, visibility and copy rules belong to [Replay](replay.md).

The state between operations must be enough to resume the same encounter: roster and roles, living/fallen participants, resources, statuses/effects and lifetimes, chosen encounter, field state, consumed items, actor/NPC timing, progression and earned-reward/exit facts. Persisting only HP/MP and restarting the batch resolver loses these facts. SQL is the run authority ([transient run state](../architecture/transient-run-state.md#normal-adventure-durable-command-boundary-1325)); Party lobbies and adventure audit rows cannot resume combat.

Open: comments, whispers (`ささやき`), manual screenshots (`すくしょ`) and spectating (`けんがく`) have no native normal-adventure command contract yet (#948). Private whispers and manual screenshots are not automatic Replay payloads.

## Resumable normal-run transitions

**Decision (#1325, 2026-10-11):** SQL authority for the run, and the explicit idle-cleanup timing below.

| Transition | Preconditions and resulting facts |
| --- | --- |
| Waiting admission | Party owns create/join/readiness; solo uses the same enrollment boundary without a lobby. Enrollment keeps ordered participants, leader, stage/mode and speed and creates floor 0 without combat |
| Encounter creation (`すすむ`) | Living participant, interval satisfied, no living enemies; floor 0 also requires the leader. Clear applicable statuses and temporary/anti fields, advance exactly one floor and create its encounter; no NPC response or automatic allied action |
| Battle action | Authorized current living actor (or a verified fallen-actor exception), registered action and valid target/resources. Process that operation, its effects and the bounded NPC response |
| Defeat | Resolve revival and death effects within the operation. Keep fallen membership and earned facts; do not exit everyone, halve prior EXP or erase gold. With no living allies, offer permitted exit/recovery, not an automatic payout |
| Ordinary boss defeat | Keep clear facts and allow explicit advance into treasure floor 11; treasure is not generated or opened by victory alone |
| Treasure inspection (`しらべる`) | Living actor selects a current unopened chest; validate allowance and any consumable key. Claim/delivery is individual and atomic; no NPC response. Stay in the room until explicit exit |
| Individual exit (`にげる`) | Participant receives only outstanding personal effects/history, leaves the roster and releases ownership; others' encounter/resources/timing are kept. Spectators receive no combat rewards |
| Leadership and room disposal | On leader exit, choose the first remaining real participant in roster order, not lock order. Dispose the room only after the last real participant leaves and settlement is durable; NPCs do not keep an empty room alive |
| Idle cleanup | Separate due detection, participant recovery/penalty and physical deletion; expired storage never proves rewards or mandatory work were processed |

Evidence: `quest.cgi:579–586`; `vs_monster.cgi:37–82,89–109`; `_battle.cgi:640–665,1169–1178,1184–1227,1330–1345`. Fallen actors are restricted but not universally barred: `_battle.cgi:646–665` allows whisper/screenshot before the fallen check and has a specific self-revival exception. Spectators get only whisper/exit/screenshot (`_skill.cgi:22–30`). An unsupported exception must stay an explicit cutover blocker, not disappear from discovery.

### Normal-run operation discovery

Discovery uses current encounter facts, not the batch resolver's priority list. Shared combat dispatch, targeting, resource and effect gates are owned by the [Battle action matrix](battle.md#normal-adventure-action-and-target-matrix). **Decision (#1327, 2026-10-12):** original behavior is the default.

| Legacy operation | Discovery, target and result |
| --- | --- |
| `こうげき`, `ぼうぎょ`, `てんしょん`; current/previous-job, equipment/book/seal, custom and tool actions | `_skill.cgi:set_action:33–151,219–309`: resources, job/SP, curse, equipment and encounter predicates select callbacks. Tool category 1 is also discovered outside the enemy-present block. No automatic action when the interval elapses |
| `すすむ` → `susumu` | `vs_monster.cgi:37–82`: no enemies, initial leader only, one-floor advance; resets effects, creates the encounter, disables NPC response. Treasure entry is a further explicit advance |
| `しらべる` → `shiraberu` | Treasure floor only (`vs_monster.cgi:38–41`); an existing unopened chest, individual allowance/key (`_npc_action.cgi:6–80`). No NPC response; delivery and chest state commit together |
| `にげる` → `nigeru` | `_skill.cgi:157–160`, `_battle.cgi:1184–1345`: individual exit; spectator exits without reward. The departing participant is removed from active battle membership and the room roster atomically; others continue in roster order |
| `まえ`, `うしろ` | `_skill.cgi:152–154,894–929`: party size >1, move actor to front/back; no listed MP. Ordinary accepted-action/status/tail processing applies; no blanket NPC suppression |
| `サバイバル`, `フォームチェンジ`, calling | `_skill.cgi:166–217,226–258`: no-enemy/job/SP/curse/item gates. These cannot be omitted because there is no enemy |
| `すとっく` → `stock_lv_up` | `_skill.cgi:312–317`: no enemies, level/EXP gates; `_battle.cgi:358–379` performs accumulated level processing, no listed MP. Distinct from pending reward stock |
| `さそう`, `きっく` | `_skill.cgi:161–164,769–783`: floor 0; kick is leader only. Invitation redirects its log to the public quest area, so it is not room activity |
| Plain comment / unknown command | `party.cgi:21–27`: a nonempty submission writes a comment unless `$mes`; `_battle.cgi:640` returns on unknown dispatch without an error, so unknown text remains a comment. Unknown typed Gateway actions stay schema errors |
| `ささやき` | `_battle.cgi:640–649` checks dispatch/interval, then returns before fallen/status/resource/tail processing. `system.cgi:251–262` validates the named recipient and disables NPC response; no recipient means a public comment; the recipient may be outside the encounter. Visible to sender/recipient only (`system.cgi:622–630`) |
| `すくしょ` | `_battle.cgi:650–652` returns before the combat tail; `system.cgi:165–207` copies owned scene/public lines, excludes private lines and sets `$mes`, so no room comment is written |

### Earnings and inactivity evidence

`_battle.cgi:199–274` gives ordinary defeat earnings to living allies at the defeat operation: directly for the acting character and through per-user stock for others. `get_stock` (`1459–1473`) releases stock on applicable actions (`912`) and personal exit (`1208`). Ordinary earnings survive later defeat; defeat grants no new victory drops or clear credit.

**Try mode.** `vs_monster.cgi:91–102` calls `success_try` only on explicit ordinary treasure-floor creation. `_battle.cgi:1915–1949` multiplies each real recipient's pending EXP/gold/crystals by 1.5 (truncated), rewrites pending stock and returns the room to normal mode. The generic release at `912` excludes Try mode. A retry must not apply 1.5 twice. **Pitfall:** on exit while Try is still active, `get_stock` reads and clears pending stock **without crediting it**; after treasure entry the converted stock is credited once on a later action or exit. Falling alone does not trigger the exit disposition.

**Idle cleanup.** `quest.cgi:80–91,172–183` checks **strictly more than 1800 seconds** since the room log changed, clears each non-NPC member's location, sets `sleep=3600` and deletes the room. It calls neither `nigeru` nor `get_stock`, and user-owned stock lives outside the room, so pending earnings survive and are credited once at the owner's next stock-releasing normal command. There is no timeout bonus and no release at cleanup. GET never performs cleanup or renews its deadline.

**Decision (#1325, 2026-10-11):** keep the strict 30-minute threshold; trigger cleanup by an explicit owner command; keep earned assets; apply a recoverable one-hour sleep penalty to remaining real participants, anchored to the committed cleanup instant. Persist its deadline before any timer projection; a failed timer write cannot release ownership or extend the hour on retry. Record a distinct timeout history disposition. No exit fatigue, history or recovery is inferred for timeout. (The original writes a duration via `system.cgi:861–883` that `sleep.cgi:19` later reduces by time since login; the native anchor is a deliberate representation choice.)

### Room activity contract

Room activity and actor battle eligibility are separate clocks. `party.cgi:21–27` writes a submitted comment only when `$mes` is unset; `system.cgi:660–676` rewrites the room log (`_battle.cgi:19–20`), whose modification time `quest.cgi:80–91` uses. So the rule is accepted **room-log-equivalent submission**, not successful damage or any POST.

| Operation / result | Renews room activity? | Evidence |
| --- | --- | --- |
| Accepted combat, including incapacitated action or accepted no-effect callback | Yes | `_battle.cgi:686–728,761–762,1174–1178` |
| Accepted advance, treasure claim, formation change, between-encounter recovery/transformation/call, stock-level processing | Yes | Each returns through the `party.cgi` comment write; a callback `$mes` means rejected |
| Accepted exit with the room remaining | Yes | `_battle.cgi:1330–1345`: last-participant disposal redirects the log to the quest area |
| Plain comment, unknown legacy text, public or private whisper | Yes, including permitted spectator/fallen submissions | Unknown action returns before the interval check (`640`); whisper checks interval then returns before the combat tail (`641–649`). Plain comments do not inherit combat cooldown |
| Banned skill silently ignored | Yes, as a comment; no actor combat timestamp | `_battle.cgi:667–675` returns without `$mes` |
| Successful floor-0 kick | Yes | `_skill.cgi:796–837` also writes a quest-area notice without redirecting the room log |
| Invitation or other dispatch redirected to another log | No | `sasou`, `_skill.cgi:772–776` |
| Screenshot, even successful | No | `system.cgi:175–191` sets `$mes` |
| Rejected operation (`$mes`) | No | `party.cgi:25` |
| GET, discovery, elapsed time, due detection | Never | #1325 observation boundary |
| Retried committed command / receipt read | No new instant | #1325 receipt boundary; a new intentional comment is a new submission |

Room activity is persisted in the same commit as the accepted operation; rolled-back commands cannot renew it.

### Combat travel and encounter replacement

**Decision (#1345, 2026-10-12):** tool150's old-enemy response/reappearance is retained, not repaired.

All travel entries use `どうぐ` → `dougu` → the held item's callback with listed MP 0 (`_skill.cgi:150–151,758–764`). A living participant may use them with enemies, between encounters, at floor 0 or in the treasure room; there is no leader gate. Spectators cannot discover the tool and fallen actors cannot execute it (`_skill.cgi:25–30`; `_battle.cgi:640–665`). Interval, ban and incapacitation gates apply; incapacitation can skip the callback and consumption while completing the action tail.

| Tool / callback (`lib/_data.cgi`) | Argument, condition and result |
| --- | --- |
| 66 / 1055–1067; 67 / 1069–1080; 68 / 1082–1093; 69 / 1095–1107; 70 / 1109–1120 | No target; normal adventure only. Assign stage 15/16/17/18/19 and floor 1 |
| 71 / 1122–1137 | No target; normal adventure and the actor's weapon35 plus armor37 (`_data.cgi:452,582`). Assign stage 20/floor 1; missing set rejects without consumption |
| 104 / 1335–1346 | No target; normal adventure only. Assign stage 21/floor 1 |
| 184 / 1765–1789 | Destination stage 23 or 25; assign it/floor 1. Missing/other destination rejects without consumption |
| 83 / 1243–1253 | No target; increment floor only in normal adventure with floor <9 (including 0). Otherwise accepted no-effect: consume, keep coordinates, run the normal tail |
| 107 / 1350–1361 | No target; normal adventure only. Assign floor 10; already 10 is still accepted and consumed. Creates no chests and sets no clear |
| 150 / 1614–1639 | Explicit floor: reset all participants' status, assign floor, call `set_monster`, request reload. Missing/invalid floor or wrong mode rejects without consumption |

Except tool83's no-effect branch, wrong-mode callbacks set an error and keep the tool. `dougu` defaults a missing argument to the actor, but floor/destination arguments are not participant IDs. Native input is an integer floor 1–10 or a destination 23/25; the original's unanchored digit match and Perl numeric coercion are not native validation rules.

**Coordinate assignment preserves the encounter.** Tools 66–71, 83, 104, 107 and 184 do not reset statuses/fields, call `set_monster`, clear enemies or reload. Their callbacks clear the held tool and continue player effects, stock/level processing, field counters and the bounded NPC response, then update actor time and write room state (`_battle.cgi:761–1178`; `_npc_action.cgi:89–155`). Old living enemies can respond and survive under new coordinates; an old treasure encounter, including unopened boxes, survives and keeps blocking advance. Only explicit `すすむ` resets, increments the assigned floor once and creates an encounter (`vs_monster.cgi:37–82`). **Pitfalls:** a stage-changing tool next creates destination floor **2**; tool83 at floor 8 next creates floor 10; tool107 next creates floor 11 without fighting a boss; a nonleader using travel at floor 0 bypasses the initial-advance leader restriction. Do not add a leader gate or auto-complete bypassed encounters.

**Tool150 continues after reload.** `reset_status_all` includes accessory revival/reset hooks (`_battle.cgi:1390–1430`), not HP/MP restoration; unlike `susumu`, temporary/anti fields stay and NPC response is not disabled. `set_monster` replaces `@members` with allies and adds new NPCs but leaves `@enemys` and old `%ms` intact (`vs_monster.cgi:85–155`; `_npc_action.cgi:600–681`); `auto_reload` does not exit the action (`system.cgi:456–470`), so the tool is consumed and the tail runs. **Pitfall:** if an old living interval-ready enemy responds, `npc_turn` rebuilds members from the old enemy list, potentially restoring old enemies and dropping new ones; a generated NPC with the same legacy name overwrites that old payload. If none responds, the generated members stay. Native encounter IDs keep this distinction without making names authoritative. Treasure-floor discovery suppresses NPC response before the callback (`vs_monster.cgi:38–41`), so leaving that room keeps suppression for the operation.

Job callbacks share this replacement and tail contract (`_skill.cgi`):

| Job position | Room or ordering rule |
| --- | --- |
| 78.2 / 6119–6133 | Normal stage adventure, floor <9: a 1/3 draw resets all members, increments the floor and creates its encounter. Floor ≥9 accepts without a draw or reset |
| 78.4 / 6147–6174 | A 1/6 draw immobilizes the actor and resets tension. Otherwise direct breath base50 reads the package cursor, ignoring the submitted target; absence uses enemy-helper fallback. Move the returned living enemy last, except an ally or breath reflector |
| 78.6 / 6201–6234 | Resolve the enemy helper first. A treasure target gets a 3/5 draw: recreate the current floor, then set current/previous job78 SP to 100 for every ally, including fallen. A failed draw changes only the actor's matching positions; a non-treasure target changes no SP |
| 79.2 / 6258–6275 | Player: 1/2 self-sleep, remove the enemy head for this operation, save members with that head last, suppress only NPC response and add a three-operation ban. NPC: general random once, then again if alive, even with no enemies |

Room recreation keeps room identity, earnings, fields and clear/EX markers; only the shortcut performs the all-member reset. Treasure recreation shares advance's box generation, Try conversion, retained-clear handling and stage19 unseal event. The removed time-callback head stays available to member-based helpers, then returns last when the next request rebuilds sides (`_battle.cgi:453–493`). Mixed-sequence projections remain open (#1733).

**Room facts survive travel.** Room ID/log, ordered allies, leader, mode, record header and earned facts are not reinitialized; member persistence writes changed stage/floor but keeps `$r_stage` and records (`_battle.cgi:464–468,496–554`; `quest.cgi:1287–1296`). Travel neither pays nor erases earnings. Assignment keeps `$is_clear` and EX markers. Tool150's encounter creation at floor 10 sets clear; at stage24 it also creates the EX marker and applies the [ordinary NPC/field contract](battle.md#normal-adventure-action-and-target-matrix). Ordinary treasure creation at floor 11 runs Try success if applicable and, independently of clear/victory, unseals for **stage19 only**; clear credit consumes the retained flag for the current destination (`vs_monster.cgi:91–110`). Assignment, inspection and exit emit no unseal event; each genuine treasure recreation is a new event. The unseal clock follows the [Boss unseal lifecycle](boss.md#6-unseal-to-sealing-room-lifecycle); calling Boss's counter-writing helper as well would double-count. The next advance from assigned floor 10 still runs the destination's EX flag path before floor 11 (`52–80,227–249`). Exit uses the changed floor for fatigue, not the encounters actually fought (`_battle.cgi:1208–1222`).

### EX unlock and continuation

**Decision (#1191, 2026-11-03).** EX eligibility is a durable **room-wide marker**, separate from stage clear, the special boss stat cap and named fields. Moving stages never clears it; the current stage must also support EX (22–26). All five stage predicates read the same `ex.cgi`, not a per-stage achievement.

| Marker producer | Condition and evidence |
| --- | --- |
| Stage22 defeat hook | Each player-credited defeat of the source Royal Bee identity draws `rand(3) < 1`, in defeat order and even when the marker exists; NPC killers, self-defeat and revival never reach the hook. Use a server-owned catalog identity, never display names (`_battle.cgi:28–51,142,277–280`) |
| Stage23 explicit advance | Leaving assigned floor 10 with at least two ordered allies in active battle membership, including helpers/fallen allies; no boss-victory requirement (`vs_monster.cgi:52,240–245`) |
| Stage24 encounter creation | Creating ordinary floor-10 enemies, including tool150; coordinate assignment alone does not qualify (`vs_monster.cgi:143–152`) |
| Stage25 explicit advance | Leaving assigned floor 10, with no party-size or victory requirement (`vs_monster.cgi:245–249`) |
| Stage26 room creation | After selecting the season, draw `rand(3) < 1` exactly once (`quest.cgi:596–600`); travel never repeats it. Lobbies select the season at creation; solo admission in the run's creation transaction (`quest.cgi:1246–1255`) |

With no living enemies, floor 10 enters ordinary treasure at 11. An EX room may then advance 11→12; otherwise it ends at 11. Entering 12 creates the EX roster, restores every active ally's HP/MP/CMP to current maxima (helpers and fallen, not banished) and installs the declared default field (stage23 snow, stage24 sanctuary); stages 24/25 then apply the boss cap/reset, which lowers maxima but leaves current HP/MP above them. After the EX encounter, 12→13 creates EX treasure; 13 is terminal. Defeat at 12 grants no treasure or automatic exit and records floor 12 without clear credit (`vs_monster.cgi:54–153`; `_npc_action.cgi:682–779`; `_battle.cgi:1485–1530`). EX treasure uses the EX item pool with duplicate weights; stages 24/25/26 also create fixed-item chests with a separate allowance (`_npc_action.cgi:6–84,555–599,725–779`).

Open: EX treasure and inspection (#1808) — 12→13 is rejected before reset, draws, costs or effects. NPC-only EX jobs stay whole-action gates ([Battle](battle.md#normal-adventure-action-and-target-matrix)). The batch crawl is not an EX fallback.

## Batch crawl

The batch crawl (`internal/adventure`, `internal/party`) is the current production path. It is not authorization to keep automatic battles, advance, inspection or exit in the interactive adventure. Its formulas live in code and tests; these points are decisions or non-obvious source behavior:

- **Treasure count** (`vs_monster.cgi:90–101`; `_npc_action.cgi:498–519`). The loop is inclusive (`for 0 .. $count`), so an index of P−1−D yields one chest per surviving member. Job, accessory (equipped slot only) and chapel blessing bonuses use the acting player (`%m`) only, never other members. **Pitfall:** `my $count = shift || $#partys;` treats an index of 0 as falsy and falls back to P−1 before the blessing and dead-member penalty.
- **Treasure pocket** (item215). **Decision (2026-11-07, #1899):** `shiraberu` clears `$m{ite}` on use and the hand holds one unit, so a character carries at most one pocket at a time. Batch crawls claim only the base one-chest allowance and never consume a pocket automatically.
- **Once-daily lock** (stage26, dungeon16; `quest.cgi:594–599` `$m{once}=1`). The quota is consumed at party creation or solo start, only for the creator/leader, and is not restored by disbanding or start failure. It clears only on waking at Home.
- **Early defeat** keeps previously earned unconditional rewards for cleared floors; Try stock follows the [conditional release rule](#earnings-and-inactivity-evidence).
- **Exit fatigue** is `round × 3 + 1` (halved in calm mode) where round is the exit floor, not turns or floors cleared (`_battle.cgi:1187–1188,1216`).
- **Room speed** values (3/18/25 seconds) are the per-actor action interval in the original; the batch crawl stores but does not enforce them.
- **Unsupported modes** (`calm`, `try`, `hard`) are rejected at party creation until their reward and unlock rules are reconciled.
- **Chronicle** (`adventure_record.cgi`; `vs_monster.cgi:111–120`): stage26 clears count separately per season. Solo and party runs write the same history source with a deterministic per-run identity, so a retry updates rather than duplicates. Historical party-log backfill cannot recover members who fell before the final floor, seasons or per-member rewards; backfilled rows record zero rewards and unclassified season rather than guesses.

Grid dungeons and endurance challenges are separate modes; see [Dungeon](dungeon.md) and [Challenge](challenge.md).
