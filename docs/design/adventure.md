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
starts; [#1325](https://github.com/witchcraze/party2re/issues/1325) owns that
decision and the resulting bounded runtime implementation scopes. Existing
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
allowances (#1196), consumed-item carry (#1197), earned-reward handling (#1198),
exit fatigue (#1221), chronicle records (#1222) and transaction repair (#1224)
remain separately owned; no full parity for these is certified here.

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
- Total accumulated EXP is halved (`TotalEXP / 2`), and Gold and Crystal rewards are reduced to 0 (`TotalGold = 0`, `TotalCrystals = 0`).
- Participating characters survive with 1 HP (preventing permanent death).

---

## 2. Floor 11 Treasure Room & Bonus Formulas

Upon defeating the Floor 10 boss, the party enters Floor 11 (Treasure Room) per legacy `vs_monster.cgi` (`$boss_round + 1`) and `_npc_action.cgi` (`add_treasure`).

### Treasure Box Count Calculation
The number of treasure chests generated in Floor 11 follows legacy Party2 rules (`vs_monster.cgi:90-100`):

$$\text{Count} = \text{BaseCount} + \text{MerchantBonus} + \text{TreasureHunterBonus} + \text{LuckyPendantBonus} + \text{GamblerBonus}$$

1. **Base Count**:
   - Standard Stages: 1 treasure chest per surviving (alive) party member (`AliveMembers`).
   - Multiplier Stages (Stage 17 幻の秘境, Stage 20 天空城, Stage 21 カオスフィールド): 3 treasure chests per surviving party member ($\text{AliveMembers} \times 3$).
2. **Merchant Job Bonus** (`merchant` / Job 7): +1 treasure chest.
3. **Treasure Hunter Job Bonus** (`treasure_hunter` / Job 78): +1 to +2 treasure chests ($1 + \text{rand}(2)$).
4. **Lucky Pendant Bonus** (`item-191` / ラッキーペンダント): $1/3$ chance ($\text{rand}(3) == 0$) of +1 treasure chest.
5. **Gambler Job Bonus** (`gambler` / Job 81): $+\text{rand}(3) - \text{rand}(2)$ treasure chests. If the gambler holds a Lucky Pendant (`item-191`), the total cannot drop below $\text{BaseCount}$.
6. **Lower Bound**: The total count is unclamped above zero, but clamped to a minimum of 0.

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
- Certain limited dungeons can only be challenged once per day (until the character sleeps at Home):
  - Stage 26: 神秘の森 / 四季のダンジョン
  - Dungeon 16: 機軍要塞
- Enforced via Valkey `TimerService` (`timer.CategoryDungeonOnce = "dungeon_once"`).
- Automatically cleared when the adventurer awakens from sleep at Home (`internal/home.Service.Wake`).

### Treasure Chest Examination (`ExamineTreasure`) & Post-Battle Settlement
- Each surviving party member may examine and claim treasure chests on Floor 11.
- Master Key (`item-215` / マスターキー) allows adventurers to open additional chests.
- Post-battle settlement delegates to `battle.Service.ApplyPostBattleResult` (`PostBattleSettler`), enforcing atomic single-transaction persistence obeying the global lock hierarchy (Rank 2 `characters` -> Rank 3 `inventory_items` -> Rank 5 `character_depots`):
  - Obtained treasure items are added to the character's inventory (`coreinventory.Inventory`).
  - When the inventory is full, items automatically route to the character's depot (`_npc_action.cgi:60-80`).
  - If the depot is also full, drops are tracked in `LostDrops` without silent phantom delivery or data loss.
  - Surviving HP and MP are persisted to `characters` in MariaDB. If defeated, fallen characters survive with 1 HP.
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
  - Novice stages require JobLevel 0, while advanced stages require up to JobLevel 7.
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
- **Post-Adventure Hook (`adventure.PostAdventureHook`)**: Invoked after crawl rewards and character state are committed. Automatically triggers pre-ordered meal delivery from the Adventurer's Tavern (`internal/tavern`).
- **Hook Error Semantics**: Both hooks execute as best-effort post-settlement side-effects. Errors returned by hook implementations are logged as warnings (`s.logger.Warn`) and do not fail the completed adventure or corrupt committed character state.
- **Settlement & Persistence Error Propagation**: All durable state mutations—including saving the `adventures` record, adding currency (`AddMoney`, `AddCrystal`), and persisting character state (`updater.Update`) in fallback settlement—strictly check and propagate errors to prevent silent state loss.

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
- **Stage Breakdown**: `stage_id`, `stage_name`, `clear_count`, `total_attempts`.
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

## 7. Storage & Database Schema (Migration 073)

The `adventures` table schema reflects immediate crawl resolution, completely purging legacy timer columns:

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
| `party_size` | INT | NOT NULL DEFAULT 1 | Total party size (1–4) |
| `experience_reward`| INT | NOT NULL DEFAULT 0 | Total EXP awarded |
| `outcome` | VARCHAR(32) | NOT NULL DEFAULT '' | Battle outcome (`WIN` or `DEFEAT`) |
| `turns` | INT | NOT NULL DEFAULT 0 | Total combat turns across all floors |
| `resolved` | BOOLEAN | NOT NULL DEFAULT TRUE | Immediate crawl resolution status |
| `created_at` | DATETIME | NOT NULL | Record creation timestamp |
| `updated_at` | DATETIME | NOT NULL | Record update timestamp |

> [!NOTE]
> Fictional columns `available_at` and `claimed`, along with index `idx_adventures_character_claimed`, were permanently dropped in migration `073_purge_adventure_timer.sql`. Index `idx_adventures_character_started (character_id, started_at DESC)` supports efficient paginated history queries.
