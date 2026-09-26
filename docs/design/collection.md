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
  $$\text{Completion Percentage} = \min\left(100.0, \frac{\text{Unique Items Discovered}}{\text{Total Item Catalog Count (141)}} \times 100\right)$$
- **Category Isolation**:
  Completion count and progress evaluation for the item compendium are strictly filtered by category (`"item"`). Discovering weapons or armors stored in the shared collection table does not increase the item collection count or advance `comp_ite` threshold progress.

---

## Persistence & Idempotency

- Duplicate defeats increment `defeated_count` without creating redundant records (`PRIMARY KEY (character_id, monster_id)`).
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
  - Milestone recordings are persisted idempotently in `character_collection_completions` (`PRIMARY KEY (character_id, kind)`) so subsequent defeats or discoveries never trigger duplicate news notifications or duplicate legend inductions.

---

## Combat & Boss Raid Integration

- **PvE Combat & Adventure Crawl**:
  - Upon winning battles in single-character or multi-character adventures (`ApplyPostBattleResult`), defeated PvE participants are recorded for all party members with the stage habitat name (`internal/adventure/settlement.go`).
  - Opponents prefixed with `char-` or `user-` (PvP/arena battles) are excluded from the Monster Book.
- **King Sealing Boss Battles**:
  - Multi-member party sealing battles (`StartSealingBattle`) and solo boss challenges (`ChallengeBoss`) register all defeated boss monsters with habitat `"封印戦"`.
  - King 99 clone battles record defeat under `"king99-clone"` / `"影"` with habitat `"封印戦"`.

