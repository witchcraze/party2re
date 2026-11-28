# Adventure System Design

## Overview

The Adventure system reconstructs the operation-driven stage adventure of legacy Party2 (`quest.cgi`, `vs_monster.cgi`, `_battle.cgi`). Evidence paths are under `party2/` at revision `ff97fc549932a146cc945d7edf71b44c868a840e`.

Solo and Party starts enroll a floor-0 run on the run host (`NormalRunService`); each later operation is a Gateway [run command](../architecture/client-agent-api.md#normal-run-commands-and-recovery).

## Original interaction contract and reconstruction target

These are source-inspected requirements, not a claim of full CGI execution or complete skill/formula parity.

- **Admission and progress are separate.** A normal quest is created at floor 0 (`lib/quest.cgi:579–586`). The first `すすむ` requires the leader; later advances are not leader-only. Living enemies prevent advancing (`lib/vs_monster.cgi:35–82`). Advancing creates the next encounter; it does not resolve the battle or continue to another floor.
- **Players choose actions.** `lib/_skill.cgi:set_action` derives available actions from the actor's job, learned skills, resources and situation; `lib/_battle.cgi:561–659` parses an action and target and checks actor state. A server-selected skill or lowest-HP target is not a replacement. Some skills have additional-action behavior.
- **Actions have an interval.** `read_member` assigns the room's speed to `act_time` (`lib/_battle.cgi:450–468`); an action is rejected when elapsed time since that actor's previous action is below that interval (`641–644`). The accepted path updates actor timing and persists shared state (`1169–1178`). This is not a round barrier, not an animation delay, and a due deadline does not execute an action.
- **NPC responses are bounded by the submitted operation.** The eligible action path invokes `npc_turn` unless suppressed (`lib/_battle.cgi:1169–1171`); `lib/_npc_action.cgi:89–155` selects at most one living, interval-ready enemy and rotates its order. Advancing and treasure-room operations disable the response (`lib/vs_monster.cgi:37–53`). GETs never execute combat; there is no background auto-combat or agility-sorted party round.
- **Treasure and exit are individual operations.** `しらべる` selects a chest and checks the actor's allowance/item rules (`lib/_npc_action.cgi:5–85`); entering the treasure room does not open chests. `にげる` distinguishes a participant from a spectator, removes that actor and applies participant exit effects (`lib/_battle.cgi:1184–1278`). Participants leave at different times; one exit is not a party-wide finish.
- **Scene capture follows actual submissions.** `party.cgi:20–27` records an eligible operation, including commentary, using the pre-operation location. Rejected operations and refreshes do not record. Content, visibility and copy rules belong to [Replay](replay.md).

The state between operations must be enough to resume the same encounter: roster and roles, living/fallen participants, resources, statuses/effects and lifetimes, chosen encounter, field state, consumed items, actor/NPC timing, progression and earned-reward/exit facts. Persisting only HP/MP and restarting the batch resolver loses these facts. SQL is the run authority ([transient run state](../architecture/transient-run-state.md#run-host-durable-command-boundary-1325)); Party lobbies and adventure audit rows cannot resume combat.

Open: comments, whispers (`ささやき`), manual screenshots (`すくしょ`) and spectating (`けんがく`) have no native normal-adventure command contract yet (#948). Private whispers and manual screenshots are not automatic Replay payloads.

## Resumable normal-run transitions

**Decision (#1325, 2026-10-11):** SQL authority for the run, and the explicit idle-cleanup timing below.

| Transition | Preconditions and resulting facts |
| --- | --- |
| Waiting admission | Party owns create/join; solo uses the same enrollment boundary without a lobby. Enrollment keeps ordered participants, leader, stage/mode and speed and creates floor 0 without combat |
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

**Decision (#1327, 2026-10-12):** normal-run discovery follows original behavior. The [Battle action matrix](battle.md#normal-adventure-action-and-target-matrix) owns shared targeting and effect rules. The legacy free-text path supplies the optional second command; its typed Gateway form is `run_operation`'s additional submission.

**Decision ([#2528](https://github.com/witchcraze/party2re/pull/2528), 2026-11-21):** although original `lib/_battle.cgi:28–275` passes defeated chests into `defeat`, intentionally suppress chest defeat rewards, monster capture, and monster-book registration; defeating a box makes it fallen and removes it from the next request's opposing pool without opening it or granting defeat drops (approved departure from the `get_exp`/`get_money` field-overload side effect).

Whisper recipients can be outside the encounter, and an omitted recipient makes a public comment (`system.cgi:251–262,622–630`). Manual screenshots capture the owned scene and public lines while omitting private lines (`system.cgi:165–207`). [#948](https://github.com/witchcraze/party2re/issues/948) tracks the remaining room dispatch and [Replay](replay.md#capture-and-display-projection) owns capture.

### Earnings and inactivity evidence

`_battle.cgi:199–274` gives ordinary defeat earnings to living allies at the defeat operation: directly for the acting character and through per-user stock for others. `get_stock` (`1459–1473`) releases stock on applicable actions (`912`) and personal exit (`1208`). Ordinary earnings survive later defeat; defeat grants no new victory drops or clear credit. A release only adds EXP: exit never levels, and an action tail, including `すすむ`/`しらべる`, grows at most one level (`935–937`).

**Decision (#2347).** The acting character's direct earnings use the recipient ledger and settle with the same command. The player step sees their direct EXP as soon as it is earned, alongside saved EXP and stock released in that operation. This preserves the timing in `_battle.cgi:209–228,912,927` while letting the ledger, progression and receipt commit together. Try stock stays conditional and is excluded from that branch. A crystal drop is decided once per encounter identity, including a failed roll. The original guard at `_battle.cgi:145–150` remembers successful drops only.

**Try mode.** `vs_monster.cgi:91–102` calls `success_try` only on explicit ordinary treasure-floor creation. `_battle.cgi:1915–1949` multiplies each real recipient's pending EXP/gold/crystals by 1.5 (truncated), rewrites pending stock and returns the room to normal mode. The generic release at `912` excludes Try mode. A retry must not apply 1.5 twice. **Pitfall:** on exit while Try is still active, `get_stock` reads and clears pending stock **without crediting it**; after treasure entry the converted stock is credited once on a later action or exit. Falling alone does not trigger the exit disposition.

**Idle cleanup.** `quest.cgi:80–91,172–183` checks **strictly more than 1800 seconds** since the room log changed, clears each non-NPC member's location, sets `sleep=3600` and deletes the room. It calls neither `nigeru` nor `get_stock`, and user-owned stock lives outside the room, so pending earnings survive and are credited once at the owner's next stock-releasing normal command. There is no timeout bonus and no release at cleanup. GET never performs cleanup or renews its deadline.

**Decision (#1325, 2026-10-11):** keep the strict 30-minute threshold; trigger cleanup by an explicit owner command (withdrawn by the owner on 2026-11-28 in favor of the scheduled sweep, [#2985](https://github.com/witchcraze/party2re/issues/2985)); keep earned assets; apply a recoverable one-hour sleep penalty to remaining real participants, anchored to the committed cleanup instant. Persist its deadline before any timer projection; a failed timer write cannot release ownership or extend the hour on retry. Record a distinct timeout history disposition. No exit fatigue, history or recovery is inferred for timeout. (The original writes a duration via `system.cgi:861–883` that `sleep.cgi:19` later reduces by time since the member's last save; matching that anchor is [#2987](https://github.com/witchcraze/party2re/issues/2987).)

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

Travel can be used between encounters and on floor 0, bypassing the leader restriction for initial advance (`_skill.cgi:150–151,758–764`; `vs_monster.cgi:37–82`). Native travel takes an explicit floor or stage argument in place of Perl's implicit actor default and unanchored digit match.

**Coordinate assignment preserves the encounter.** Old enemies can respond under new coordinates, and old unopened boxes still block advance (`_battle.cgi:761–1178`; `_npc_action.cgi:89–155`). Explicit `すすむ` increments the assigned floor, so a stage-changing tool leads to destination floor **2** and tool107 can lead to treasure without a boss fight (`vs_monster.cgi:37–82`).

**Tool150 continues after reload.** `reset_status_all` includes accessory revival/reset hooks (`_battle.cgi:1390–1430`), not HP/MP restoration; unlike `susumu`, temporary/anti fields stay and NPC response is not disabled. `set_monster` replaces `@members` with allies and adds new NPCs but leaves `@enemys` and old `%ms` intact (`vs_monster.cgi:85–155`; `_npc_action.cgi:600–681`); `auto_reload` does not exit the action (`system.cgi:456–470`), so the tool is consumed and the tail runs. **Pitfall:** if an old living interval-ready enemy responds, `npc_turn` rebuilds members from the old enemy list, potentially restoring old enemies and dropping new ones; a generated NPC with the same legacy name overwrites that old payload. An NPC's own job78.2 hits the same rebuild, so it keeps the encounter and leaves only the all-member reset and the floor advance (`_skill.cgi:6116–6133`). If none responds, the generated members stay. Native encounter IDs keep this distinction without making names authoritative. Treasure-floor discovery suppresses NPC response before the callback (`vs_monster.cgi:38–41`), so leaving that room keeps suppression for the operation.

Job79's time callback removes the enemy head from the current enemy pool while member-based helpers can still reach it. It returns last when the next request rebuilds sides (`_battle.cgi:453–493`).

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

An EX room continues 11→12→13, where 13 is terminal; a room without EX ends at 11 (`vs_monster.cgi:54–153`). Entering 12 restores every active ally, including helpers and fallen members, before stages 24/25 apply the boss cap. The cap lowers maxima but leaves current HP/MP above them (`_npc_action.cgi:717–721`; `_battle.cgi:1485–1530`).

**Pitfall (fixed chests).** `shiraberu` marks a fixed chest (`over_lv`) in `$ms{$m}{is_get_fixed}`, but that key is not in `@battle_datas` (`config.cgi_origin:102–104`), so it is lost when the request ends. Fixed chests therefore have no lasting per-actor limit and never consume item215. They also leave the ordinary allowance untouched (`_npc_action.cgi:42–54`). The reconstruction reproduces this rather than repairing it.

## Stage admission and clean-room enemy data

- **Stage admission (#1194, #2833).** Where legacy `quest.cgi` (`tsukuru` / `get_limited_stages`) and `check_create_quest` / `exclude_job_lv_stage` diverged on stages 22, 23, 25 and 27, party2re adopts the selection UI's job-level and JST seasonal rules across both observation and admission, with no character-level gate (`internal/adventure/stage_catalog.go`). Gates apply only to the room's creator (solo start or Party creation); legacy `add_member` and the first `すすむ` never re-check members (`vs_monster.cgi:56–82`).
- **Stage 26 season fallback.** Explicit ordinary rooms preserve their selected season marker; missing markers default to spring (`stage/26.cgi:16–28`).
- **Shared enemy data and clean-room naming (#2402).** Ordinary stage encounters and Dungeon maps share the legacy transcription in `internal/stagedata` (retaining native combat payloads and source-position identities rather than the batch catalog's last-monster boss convention; see [Battle](battle.md#normal-adventure-action-and-target-matrix)). Stage enemies keep their legacy stage-file name, except franchise-distinctive names ([rule 00 §3](../../.agents/rules/00-migration-constraints.md#3-asset-and-intellectual-property-rules)) map one-to-one to generic replacements across every shipped catalog with `A`/`B` suffixes kept (`internal/stagedata/names_test.go`).

## Batch crawl

The batch crawl (`internal/adventure`, `internal/party`) no longer serves production starts; #2474 deletes it. These decisions and source findings remain current:

- **Treasure count** (`vs_monster.cgi:90–101`; `_npc_action.cgi:498–519`). The loop is inclusive (`for 0 .. $count`), so an index of P−1−D yields one chest per surviving member. P is `@partys`, which includes living helpers; fallen helpers were dropped at the previous save (`_battle.cgi:486–493,521`). Job, accessory (equipped slot only) and chapel blessing bonuses use the acting player (`%m`) only, never other members. **Pitfall:** `my $count = shift || $#partys;` treats an index of 0 as falsy and falls back to P−1 before the blessing and dead-member penalty.
- **Treasure pocket** (item215). **Decision (2026-11-07, #1899):** `shiraberu` clears `$m{ite}` on use and the hand holds one unit, so a character carries at most one pocket at a time. Batch crawls claim only the base one-chest allowance and never consume a pocket automatically.
- **Once-daily lock** (stage26, dungeon16; `quest.cgi:594–599` `$m{once}=1`). The quota is consumed at party creation or solo start, only for the creator/leader, and is not restored by leaving or start failure. It clears only on waking at Home.
- **Chronicle**: the legacy owner-view unlock belongs to the entry command so record GETs remain read-only under the [Gateway contract](../architecture/client-agent-api.md). Historical party-log backfill cannot recover members who fell before the final floor, seasons or per-member rewards; backfilled rows record zero rewards and unclassified season rather than guesses.

Grid dungeons and endurance challenges are separate modes; see [Dungeon](dungeon.md) and [Challenge](challenge.md).
