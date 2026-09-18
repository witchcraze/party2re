# Adventure System & 10-Floor Dungeon Crawl Design Specification

## Overview

The Adventure system clean-room reconstructs the authentic adventure exploration and dungeon crawl mechanics from legacy Party2 (`quest.cgi`, `vs_monster.cgi`, `_npc_action.cgi`).

In early Go reconstructions, a fictional "1-hour expedition timer" (`AdventureDuration = time.Hour`) was introduced, turning adventures into an idle-game waiting mechanic. Per Issue #478, this fictional timer and all scheduled action delays have been completely eliminated. Adventures now execute and resolve immediately as multi-participant, 10-floor dungeon crawls with authentic Floor 11 Treasure Rooms.

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
The number of treasure chests generated in Floor 11 follows legacy Party2 rules:

$$\text{Count} = \text{AliveMembers} + \text{MerchantBonus} + \text{TreasureHunterBonus} + \text{LuckyPendantBonus}$$

1. **Base Count**: 1 treasure chest per surviving (alive) party member (`AliveMembers`).
2. **Merchant Job Bonus** (`merchant` / Job 7): +1 treasure chest.
3. **Treasure Hunter Job Bonus** (`treasure_hunter` / Job 78): +1 to +2 treasure chests ($1 + \text{rand}(2)$).
4. **Lucky Pendant Bonus** (`item-191` / ラッキーペンダント): $1/3$ chance ($\text{rand}(3) == 0$) of +1 treasure chest.
5. **Lower Bound**: The total count is unclamped above zero, but clamped to a minimum of 0.

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
- **Speed Settings**: Configures animation and action log pacing:
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
