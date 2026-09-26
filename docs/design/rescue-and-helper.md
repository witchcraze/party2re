# Player Rescue & Helper Quest System (救出処理 & 手助けクエスト)

## Overview
The Rescue and Helper system provides player support mechanics consisting of:
1. **Helper Quests (`helper.cgi` / 手助けクエスト)**: An NPC/community request board managed by Ricca (何でも屋 @リッカ) where players turn in requested equipment, consumables, or monsters to earn alchemy crafting materials, rare rewards, and guild contribution points.
2. **Emergency Rescue (`rescue.cgi` / 救出処理)**: An emergency unstuck mechanism allowing players trapped in inconsistent states or infinite loops to safely reset their active operations with a calibrated cooldown/sleep penalty.

---

## 1. Helper Quest System (手助けクエスト)

### Categories and Requirements
Quests are randomly generated across four categories:
- **Weapon (Kind 1)**: Demands 2 to 4 weapons (`rand(3) + 2`).
- **Armor (Kind 2)**: Demands 2 to 4 armor pieces (`rand(3) + 2`).
- **Item (Kind 3)**: Demands 2 to 4 consumable/material items (`rand(3) + 2`).
- **Monster (Kind 4)**: Demands 1 to 2 captured monsters (`rand(2) + 1`).

### Quest Variations & Probabilities
- **Normal Quest**: Standard requirement pools; rewards essential alchemy and consumable materials (`item-128`, `item-130`〜`item-134`, `item-184`).
- **Rare Quest (1/14 probability)**: Demands rare high-tier items or rare monsters; rewards valuable rare items (`item-129`, `item-135`〜`item-137`).
- **Guild Quest (1/15 probability)**: Requires guild membership; requirement count is doubled (`x2`); rewards a Fortune Bag (`item-126`), awards **+100 Guild Points (GP)** to the player's guild, and increments the player's `help_count`.

### Lifecycle and Expiration
- Quests remain active for **6 days** (`ExpiresAt = CreatedAt + 6 days`).
- Upon completion, required items/equipment are deducted from the player's **Depot** (`check_depot`), or monsters are deducted from the **Monster Ranch** box (`check_farm`), quest rewards are delivered directly to the player's **Depot** (`send_item`), the character's `help_count` is incremented, and a fresh replacement quest is generated.
- Stale expired quests are refreshed with newly generated quests.

---

## 2. Emergency Rescue System (救出処理)

### Purpose & Constraints
- Cancels and clears any active scheduled action or dangling activity.
- Applies a flat cooldown/sleep penalty of **600 seconds (10 minutes)** (`DefaultPenaltySeconds`) when unstucking a trapped character.
- If the character is already safe in town (has no ongoing or stuck activity to clear), rescue returns early with 0 penalty cooldown and does not record a new penalty, matching legacy `rescue.cgi:56-65` (`すでに救出処理がされています`).
- During the penalty cooldown, character actions are restricted (`IsUnderPenalty` returns true and `CheckActionAllowed` rejects actions with `ErrCharacterUnderPenalty`).
- Records rescue actions that unstuck trapped characters in `rescue_records` for auditability and moderation.

---

## 3. Data Persistence & Transaction Boundaries (MariaDB)
- `characters.help_count`: Integer counter tracking the number of completed helper quests.
- `helper_quests`: Stores quest specifications, target items, required quantities, reward items, rarity, guild flag, and completion metadata.
- `rescue_records`: Stores character ID, rescue reason, penalty seconds, and creation timestamp.
- **Transaction Atomicity**: `CompleteQuest` executes within an atomic database transaction (`TransactionProvider.RunInTx`), ensuring turn-in verification, item/monster deduction, direct depot reward delivery, quest completion status update, and replacement quest generation are committed atomically.
- **Storage Sourcing & Direct Depot Reward Delivery**:
  - **Equipment & Items (Kinds 1, 2, 3)**: Checked and deducted from `DepotRepository` (`character_depots`, `depot_items`), matching legacy `party2/lib/helper.cgi:113` (`check_depot`). Character bag inventory is not touched.
  - **Companion Monsters (Kind 4)**: Checked and deducted from `FarmRepository` (`character_monsters`, `location = 'box'`), matching legacy `check_farm`.
  - **Direct Depot Delivery**: Reward items are delivered directly into the player's Depot (`send_item($m, 3, $pay)`). If the player's Depot is at full capacity and cannot accept the reward item, the operation aborts with `ErrDepotFull`, rolling back the entire transaction so that turn-in items and quest state are preserved with zero asset loss.

---

## 4. HTTP API Endpoints

### Helper Quests
- `GET /helpers/quests`: Lists all currently active and uncompleted helper quests on the request board.
- `POST /helpers/complete`: Submits requested items to fulfill a helper quest (`{"character_id": "...", "quest_id": "..."}`) with session authentication and character ownership verification.

### Emergency Rescue
- `GET /rescues/penalty?character_id=...`: Checks if the specified character is under a rescue penalty cooldown, returning `is_under_penalty` and `remaining_seconds`.
- `POST /rescues/request`: Triggers an emergency unstuck rescue (`{"character_id": "...", "reason": "..."}`) with session authentication and character ownership verification.

