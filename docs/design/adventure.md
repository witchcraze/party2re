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
                     - Characters keep 1 HP            - Characters keep 1 HP
```

### Floors 1 to 9: Stage Monsters
- Spawns 1 to 3 enemies chosen from the stage's normal monster roster (`stage.GetNormalMonsterIDs()`).
- Normal monsters exclude designated stage bosses.
- Multi-participant turn combat is resolved using `corebattle.PartyBattleResolver`.
- On victory: allies advance to the next floor with their remaining HP and MP preserved across floors. Total EXP and Gold accumulate.

### Floor 10: Stage Boss Battle
- Spawns the stage boss (`stage.GetBossIDs()`). If boss IDs are not explicitly configured, the final monster in the stage's monster list is designated as the stage boss.
- On defeat of the boss: the stage is marked as cleared (`is_cleared = true`), and the party gains access to Floor 11.

### Early Defeat Handling
- If all living party members are knocked unconscious at any floor (1–10), the crawl terminates immediately.
- `FloorsCleared` is set to the last fully cleared floor number.
- Total accumulated EXP is halved (`TotalEXP / 2`), and Gold reward is reduced to 0.
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

### Treasure Chest Examination (`ExamineTreasure`)
- Each surviving party member may examine and claim treasure chests.
- Master Key (`item-215` / マスターキー) allows adventurers to open additional chests.
- Dropped items are distributed directly to player inventory or depot storage.

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
- **Chronicle & History**: Completed runs are recorded in `adventures` and aggregated in `GET /characters/{id}/adventure-chronicle`. Milestone unlocks (Try Mode, Image Setting, Calm Mode, Hard Mode, Avatar Setting, Extreme Mode) unlock based on cleared stage counts.

---

## 5. Storage & Database Schema (Migration 073)

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
> Columns `available_at` and `claimed`, along with index `idx_adventures_character_claimed`, were permanently dropped in migration `073_purge_adventure_timer.sql`.
