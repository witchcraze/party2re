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
- **Completion Progress**:
  $$\text{Completion Percentage} = \min\left(100.0, \frac{\text{Unique Monsters Defeated}}{\text{Total Monster Catalog Count}} \times 100\right)$$

---

### Item Collection Registry (アイテム図鑑)

- **Trigger**: When an item is acquired (shop purchase, drops, alchemy synthesis, auction buyout, depot withdrawal), it is registered in the character's Item Collection registry.
- **Data Tracked**:
  - `ItemID`: Unique item definition ID.
  - `ItemName`: Name of the item.
  - `Category`: Item category (`WEAPON`, `ARMOR`, `SHIELD`, `ACCESSORY`, `ITEM`).
  - `DiscoveredAt`: Initial registration timestamp.
- **Completion Progress**:
  $$\text{Completion Percentage} = \min\left(100.0, \frac{\text{Unique Items Discovered}}{\text{Total Item Catalog Count}} \times 100\right)$$

---

## Persistence & Idempotency

- Duplicate defeats increment `defeated_count` without creating redundant records (`PRIMARY KEY (character_id, monster_id)`).
- Duplicate item discoveries are safely ignored (`INSERT IGNORE` with `PRIMARY KEY (character_id, item_id)`).

---

## Completion Milestones & News Broadcasts

Faithfully reproduces legacy Party2 (`lib/_add_monster_book.cgi`, `lib/collection.cgi`):

- **Canonical Completion Thresholds**:
  - **Monster Book**: 180 unique monsters (`DefaultTotalMonsters = 180`).
  - **Item Collection**: 141 unique items (`DefaultTotalItems = 141`).
- **100% Completion Announcements**:
  - Upon reaching 180 monsters for the first time, a server news announcement is published:
    `"<span class=\"comp\">{CharacterName}がモンスターブックをコンプリートしました！</span>"`
  - Upon reaching 141 items for the first time, a server news announcement is published:
    `"<span class=\"comp\">{CharacterName}がアイテムコレクションをコンプリートする！</span>"`
  - Milestone recordings are persisted idempotently in `character_collection_completions` (`PRIMARY KEY (character_id, kind)`) so subsequent defeats or discoveries never trigger duplicate news notifications.

---

## Combat & Boss Raid Integration

- **PvE Combat & Adventure Crawl**:
  - Upon winning battles in single-character or multi-character adventures (`ApplyPostBattleResult`), defeated PvE participants are recorded for all party members with the stage habitat name (`internal/adventure/settlement.go`).
  - Opponents prefixed with `char-` or `user-` (PvP/arena battles) are excluded from the Monster Book.
- **King Sealing Boss Battles**:
  - Multi-member party sealing battles (`StartSealingBattle`) and solo boss challenges (`ChallengeBoss`) register all defeated boss monsters with habitat `"封印戦"`.
  - King 99 clone battles record defeat under `"king99-clone"` / `"影"` with habitat `"封印戦"`.

