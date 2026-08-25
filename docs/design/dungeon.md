# Dungeon Exploration and Branching Maps Design

## Overview

The Dungeon Exploration Feature Module (`internal/dungeon`) provides deep multi-floor expeditions with grid navigation, hazard traps, treasure chest loots, monster combat, floor bosses, and accumulated reward ledger mechanics (`vs_dungeon.cgi`, `map/`) using the shared Core Battle engine (`internal/core/battle`).

---

## Architectural Policy & Boundaries

- **Expedition State Machine**: Active expedition lifecycle (position, current floor, remaining turn limit, temporary health, buffered reward ledger) is isolated inside `internal/dungeon`.
- **Core Battle Isolation**: Monster encounters and floor boss battles invoke `internal/core/battle.Engine` language-agnostically with participant snapshots.
- **Accumulated Reward Ledger**: EXP, Gold, and item drops obtained during an expedition are buffered in temporary expedition state (`dungeon_active_expeditions`).
- **Transactional Finalization**:
  - **Dungeon Clear / Safe Escape**: Accumulated ledger rewards are committed atomically to character progression and inventory.
  - **Wipeout (Defeat / Turn Timeout)**: The expedition terminates with defeat, forfeiting all unbanked ledger rewards without corrupting base character state.

---

## Domain Rules & Navigation Mechanics

### 1. Tile Map Specification

Each floor is represented by a 2D grid matrix of ASCII tile characters:

| Tile Symbol | Meaning | Event Behavior |
| :--- | :--- | :--- |
| `S` | Start Point | Initial spawn point or staircase descent arrival tile. |
| `0` | Normal Passage | Navigable floor tile with random encounter probability (50% monster combat). |
| `1` | Impassable Wall | Cannot be traversed. Moving into a wall is rejected with `ErrImpassableWall`. |
| `T` | Treasure Chest | Grants floor-scaled gold ($100 \times \text{Floor}$) and item drops to the temporary reward ledger. |
| `X` | Hazard Trap | Triggers trap mechanism inflicting percentage damage ($\max(10, 15\% \text{Max HP})$). |
| `D` | Down Stairs | Descends to the next floor ($+1$), resetting position to the next floor's `StartX, StartY` and replenishing floor turns. |
| `B` | Floor Boss | Initiates climatic boss battle. Defeating the boss clears the dungeon and triggers full rewards commit. |
| `E` | Safe Escape Portal | Safely evacuates the player from the dungeon, locking in all accumulated ledger rewards. |

---

### 2. Turn Limits & Expedition Exhaustion

- Each floor has a maximum turn allocation (`MaxTurnsPerFloor = 25..35`).
- Moving in cardinal directions (`north`, `south`, `east`, `west`) consumes 1 turn.
- If `TurnsRemaining <= 0`, the player suffers exhaustion wipeout, forfeiting unbanked expedition loot.

---

### 3. Combat & Reward Ledger Calculations

$$\text{Expedition EXP} = \sum \text{Monster EXP} + \text{Dungeon Clear Bonus}$$
$$\text{Expedition Gold} = \sum \text{Monster Gold} + \sum \text{Chest Gold} + \text{Dungeon Clear Bonus}$$
$$\text{Expedition Items} = \bigcup \text{Monster Drops} \cup \bigcup \text{Chest Loots}$$

- On **Wipeout**: Character receives $0$ EXP, $0$ Gold, and $0$ items from the expedition ledger.
- On **Escape / Clear**:
  1. `progression.ApplyExperience(&char, exp)`
  2. `char.Money += gold`
  3. All item instances generated are inserted into `inventory_items`.
  4. Statistics updated: `highest_dungeon_cleared`, `total_expeditions`, `total_floors_cleared`, `total_chests_opened`.
