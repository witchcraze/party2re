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
