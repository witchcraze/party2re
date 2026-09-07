# Altar of Rebirth (復活の祭壇) Design

## Overview

The Altar of Rebirth (`reborn.cgi` / `internal/altar`) reproduces the authentic Party2 mechanism for collecting the 6 colored orbs, awakening the legendary phoenix Ramia through sacred prayer (`@いのる`), and wishing for one of 4 secret otherworld travel items (`@ねがう`).

---

## 6-Color Orb System (６つのオーブ)

World adventures yield 6 elemental orbs across different days of the week. Characters offer them at the Altar of Rebirth:

| Symbol | Japanese Name | Element / Day | Item ID |
| --- | --- | --- | --- |
| `s` | シルバーオーブ (Silver Orb) | 月 (Moon) | `item-060` |
| `r` | レッドオーブ (Red Orb) | 火 (Fire) | `item-061` |
| `b` | ブルーオーブ (Blue Orb) | 水 (Water) | `item-062` |
| `g` | グリーンオーブ (Green Orb) | 木 (Tree) | `item-063` |
| `y` | イエローオーブ (Yellow Orb) | 金 (Gold) | `item-064` |
| `p` | パープルオーブ (Purple Orb) | 土 (Earth) | `item-065` |

- **State Representation**: Stored in `characters.orb` as a string of runes (e.g., `"srb"`). Duplicate offerings are rejected with `409 Conflict`.
- **All Orbs Collected**: When all 6 unique orbs (`s`, `r`, `b`, `g`, `y`, `p`) are collected, the shrine maiden prompts the character to pray.

---

## Ramia Awakening Prayer (@いのる)

- **Condition**: Requires all 6 orbs. If fewer than 6, prayer is rejected with legacy dialogue:
  `"オーブが足りません。オーブが足りません。オーブを６つ集めてください"`
- **Effect**:
  - The legendary phoenix Ramia awakens with dialogue:
    `"時は来たれり。今こそ目覚める時。大空はお前のもの。舞い上がれ空高く！"`
  - Character orb state transitions to `"G"` (Golden / Awakened).
  - Ramia stays present at the altar for 30 minutes (`RamiaStayDuration = 1800s`), recorded in `altar_ramia_awakenings`.

---

## Otherworld Travel Item Wishes (@ねがう)

- **Condition**: Requires awakened state (`characters.orb == "G"`).
- **Available Items**:
  1. `item-066`: 真実の鏡 (ラーの鏡 / Mirror of Truth)
  2. `item-067`: マダムの招待状 (Madam's Invitation)
  3. `item-068`: 宝の地図 (Treasure Map)
  4. `item-069`: 闇のランプ (Lamp of Darkness)
- **Delivery Rules**:
  - If the character's consumable inventory has space (< 1 item), the chosen item is delivered directly to the inventory, and registered in the item collection.
    Message: `"<ItemName> ですね。冒険中に使うことで未知の世界へと行くことができるでしょう"`
  - If inventory is full (>= 1 item), the item is automatically transferred to the character's Depot (預かり所).
    Message: `"<ItemName> を<CharacterName>の預かり所に送っておきました。冒険中に使うことで未知の世界へと行くことができるでしょう"`
- **Post-Wish State**:
  - Character `orb` is cleared to `""`.
  - Next cycle requires gathering the 6 orbs anew.

---

## Database Architecture

- `characters.orb VARCHAR(16) NOT NULL DEFAULT ''`: Stores current orb rune string (`"srbgyp"` or `"G"`).
- `altar_ramia_awakenings`:
  - `id VARCHAR(64) PRIMARY KEY`
  - `character_id VARCHAR(64) NOT NULL`
  - `character_name VARCHAR(64) NOT NULL`
  - `awakened_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP`
  - `expires_at TIMESTAMP NOT NULL`
  - `INDEX idx_altar_expires (expires_at)`

## HTTP API Endpoints

- `GET /characters/{id}/altar`: Retrieve altar status, current orbs, Ramia presence, and dialogue.
- `POST /characters/{id}/altar/offer`: Offer an orb (`{"orb": "silver"}`).
- `POST /characters/{id}/altar/pray`: Awaken Ramia (`@いのる`).
- `POST /characters/{id}/altar/wish`: Choose secret travel item (`@ねがう`).
