# Monster Book & Item Collection Design

## Overview

The Collection and Monster Book Feature Module (`internal/collection`) provides career discovery and completion tracking for player characters across all unique monsters defeated and items obtained throughout the game.

---

## Domain Rules & Record Tracking

### Monster Illustrated Book (モンスターブック)

- **Trigger**: When a player character defeats a monster in combat (adventure, dungeon, boss battle), the monster is recorded into the character's Monster Book.
- **Data Tracked**:
  - `MonsterID`: Unique monster identifier.
  - `MonsterName`: Name of the monster.
  - `Habitat`: Stage or dungeon biome where first encountered.
  - `DefeatedCount`: Total number of times this character has slain this monster type.
  - `FirstDefeatedAt`: Initial discovery timestamp.
  - `LastDefeatedAt`: Most recent victory timestamp.
  - `Icon`: Monster sprite graphic identifier (e.g., `mon/001.gif`).
  - `Strong`: Monster combat strength metric ($\text{int}(HP + MP + Attack + 0.5 \times Defense + Agility)$).
  - `HP`, `MP`, `Attack`, `Defense`, `Agility`: Monster combat attributes captured upon first defeat.
  - `ExperienceReward`, `GoldReward`: Combat experience and gold reward yields captured upon first defeat.
- **Completion Progress**:
  $$\text{Completion Percentage} = \min\left(100.0, \frac{\text{Unique Monsters Defeated}}{\text{Total Monster Catalog Count}} \times 100\right)$$

---

### Weapon Encyclopedia (武器図鑑)

- **Trigger**: When a weapon is acquired (shop purchase, drops, alchemy synthesis, auction buyout, depot withdrawal), it is registered in the character's collection registry with category `weapon`.
- **Data Tracked**: `ItemID`, `ItemName`, `Category` (`"weapon"`), `DiscoveredAt`.
- **Completion Progress**:
  $$\text{Completion Percentage} = \min\left(100.0, \frac{\text{Unique Weapons Discovered}}{\text{Total Weapon Catalog Count (71)}} \times 100\right)$$

---

### Armor Encyclopedia (防具図鑑)

- **Trigger**: When an armor, shield, or accessory is acquired, it is registered in the character's collection registry with category `armor`.
- **Data Tracked**: `ItemID`, `ItemName`, `Category` (`"armor"`), `DiscoveredAt`.
- **Completion Progress**:
  $$\text{Completion Percentage} = \min\left(100.0, \frac{\text{Unique Armors Discovered}}{\text{Total Armor Catalog Count (55)}} \times 100\right)$$

---

### Item Collection Registry (道具図鑑)

- **Trigger**: When an item is acquired (shop purchase, drops, alchemy synthesis, auction buyout, depot withdrawal), it is registered in the character's Item Collection registry.
- **Data Tracked**:
  - `ItemID`: Unique item definition ID.
  - `ItemName`: Name of the item.
  - `Category`: Item category (`weapon`, `armor`, `item`).
  - `DiscoveredAt`: Initial registration timestamp.
- **Completion Progress**:
  $$\text{Completion Percentage} = \min\left(100.0, \frac{\text{Unique Basic Items Discovered (No. } \le 141\text{)}}{\text{Total Item Catalog Count (141)}} \times 100\right)$$
- **Basic Item Invariant vs. Public Catalog (#1237)**:
  Only canonical basic items up to No. 141 (`DefaultTotalItems = 141`, `$default_ites`) count towards completion progress, completion marker persistence (`character_collection_completions`), and Hall of Fame induction (`comp_ite`). Additional items above No. 141 (e.g. synthesis or expansion items such as `item-142`, `item-154`, `item-257`) remain in the public collection list for viewing, but do not contribute to completion progress or compensate for unacquired basic items (`lib/collection.cgi:45–49, 81–87`).
- **Condition Parity**:
  Display progress (`GetItemCollection`), completion marker persistence (`character_collection_completions`), and Hall of Fame induction (`comp_ite`) reference the exact same completion target set condition (`progress.IsCompleted`). Historical Hall of Fame inductions (`legend_records`) remain immutable, irreversible career records.
- **Category Isolation**:
  Completion count, progress evaluation, and entry queries for the item compendium are strictly filtered by category (`"item"`). `GetItemCollection` and the `GET /characters/{id}/collections/items` endpoint default an empty or omitted category to `"item"`, ensuring discovering weapons or armors stored in the shared collection table does not cross-contaminate the item compendium, increase the item discovery count, or trigger false `comp_ite` completion.

---

## Persistence & Idempotency

- Duplicate defeats increment `defeated_count` and update `last_defeated_at` without creating redundant records (`PRIMARY KEY (character_id, monster_id)`).
- **First-Encounter Snapshot Invariant**: Initial combat metrics (`icon`, `strong`, `hp`, `mp`, `attack`, `defense`, `agility`, `exp_reward`, `gold_reward`, `habitat`) represent an immutable snapshot of the character's first encounter with that monster. In MariaDB, this is enforced by `ON DUPLICATE KEY UPDATE defeated_count = defeated_count + 1, last_defeated_at = UTC_TIMESTAMP()`, which never overwrites initial encounter statistics upon subsequent victories.
- **Legacy Record Compatibility**: Existing database records populated prior to migration 095 have NULL/empty snapshot values. In accordance with zero-fabrication rules, they return zero/empty without synthesizing fake combat stats.
- Duplicate item discoveries are safely ignored (`INSERT IGNORE` with `PRIMARY KEY (character_id, item_id)`).

---

## Completion Milestones, News Broadcasts & Hall of Fame Induction

Faithfully reproduces legacy Party2 (`lib/_add_monster_book.cgi`, `lib/collection.cgi`, `legend.cgi`):

- **Canonical Completion Thresholds**:
  - **Monster Book**: 180 unique monsters (`DefaultTotalMonsters = 180`).
  - **Weapon Encyclopedia**: 71 unique weapons (`DefaultTotalWeapons = 71`, `$#weas`).
  - **Armor Encyclopedia**: 55 unique armors (`DefaultTotalArmors = 55`, `$#arms`).
  - **Item Encyclopedia**: 141 unique items (`DefaultTotalItems = 141`, `$default_ites`).
- **100% Completion Announcements & Hall of Fame Induction**:
  - Upon reaching 180 monsters: publishes news `"{CharacterName}がモンスターブックをコンプリートしました！"` and inducts into Hall of Fame under `comp_mon` (モンスターマスター).
  - Upon reaching 71 weapons: publishes news `"{CharacterName}が武器図鑑をコンプリートしました！"` and inducts into Hall of Fame under `comp_wea` (ウェポンキラー).
  - Upon reaching 55 armors: publishes news `"{CharacterName}が防具図鑑をコンプリートしました！"` and inducts into Hall of Fame under `comp_arm` (アーマーキング).
  - Upon reaching 141 items: publishes news `"{CharacterName}がアイテム図鑑をコンプリートしました！"` and inducts into Hall of Fame under `comp_ite` (アイテムニスト).
- **Completion Induction Finalization Boundary & Recoverability (#1238)**:
  - **Mandatory Induction vs. Informational Broadcast**: Hall of Fame induction (`RecordLegend`) is a mandatory persistent milestone. It must succeed before marking the completion milestone in `character_collection_completions`. If `RecordLegend` fails, completion is not marked and the error is returned to the caller, preventing legend failures from being masked as success.
  - **Best-Effort News**: Server news broadcasting (`PublishNews`) is informational best-effort; failures do not abort completion and news is only broadcast upon initial completion.
  - **Reprocessing Recovery**: If a transient persistence failure interrupts legend induction, `character_collection_completions` remains unrecorded. Subsequent defeats or discoveries re-evaluate the threshold, retry legend induction, and successfully finalize completion once without duplicates.
  - **Existing Data Recovery Procedure**: For characters that were completed prior to this consistency fix and lost Hall of Fame induction, `collection.Service.RecoverMissingLegends(ctx, characterID)` inspects all completed milestones and idempotently writes missing `legend_records`. For database-wide batch administrative repair:
    ```sql
    INSERT IGNORE INTO legend_records (category, character_id, character_name, guild_name, color, icon, message, inducted_at)
    SELECT 
        CASE ccc.kind
            WHEN 'monster_book' THEN 'comp_mon'
            WHEN 'weapon' THEN 'comp_wea'
            WHEN 'armor' THEN 'comp_arm'
            WHEN 'item' THEN 'comp_ite'
        END AS category,
        c.id AS character_id,
        c.name AS character_name,
        COALESCE(g.name, '') AS guild_name,
        COALESCE(c.color, '#ffffff') AS color,
        COALESCE(cp.avatar_url, '') AS icon,
        COALESCE(cp.comment, '') AS message,
        ccc.completed_at AS inducted_at
    FROM character_collection_completions ccc
    JOIN characters c ON ccc.character_id = c.id
    LEFT JOIN character_profiles cp ON c.id = cp.character_id
    LEFT JOIN guild_members gm ON c.id = gm.character_id AND gm.is_pending = FALSE
    LEFT JOIN guilds g ON gm.guild_id = g.id
    LEFT JOIN legend_records lr ON lr.character_id = ccc.character_id AND lr.category = (
        CASE ccc.kind
            WHEN 'monster_book' THEN 'comp_mon'
            WHEN 'weapon' THEN 'comp_wea'
            WHEN 'armor' THEN 'comp_arm'
            WHEN 'item' THEN 'comp_ite'
        END
    )
    WHERE lr.id IS NULL;
    ```
  - Milestone recordings are persisted idempotently in `character_collection_completions` (`PRIMARY KEY (character_id, kind)`) so subsequent defeats or discoveries never trigger duplicate news notifications or duplicate legend inductions.

---

## Combat & Boss Raid Integration

- **PvE Combat & Adventure Crawl**:
  - Upon winning battles in single-character or multi-character adventures (`ApplyPostBattleResult`), defeated PvE participants are recorded for all party members with the stage habitat name (`internal/adventure/settlement.go`).
  - Opponents prefixed with `char-` or `user-` (PvP/arena battles) are excluded from the Monster Book.
- **King Sealing Boss Battles**:
  - Multi-member party sealing battles (`StartSealingBattle`) and solo boss challenges (`ChallengeBoss`) register all defeated boss monsters with habitat `"封印戦"`.
  - King 99 clone battles record defeat under `"king99-clone"` / `"影"` with habitat `"封印戦"`.

