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
- Upon completion, required items are consumed from the character's inventory/storage, rewards are granted, the character's `help_count` is incremented, and a fresh replacement quest is generated.
- Stale expired quests are refreshed with newly generated quests.

---

## 2. Emergency Rescue System (救出処理)

### Purpose & Constraints
- Serves as an in-game recovery utility for players in invalid or trapped states.
- Cancels and clears any active scheduled action or dangling activity.
- Applies a default cooldown/sleep penalty of **600 seconds (10 minutes)** (`DefaultPenaltySeconds`). If consecutive rescues occur within 24 hours, the penalty is doubled to prevent abuse.
- During the penalty cooldown, character actions are restricted (`IsUnderPenalty` returns true and `CheckActionAllowed` rejects actions with `ErrCharacterUnderPenalty`).
- Records all rescue actions in `rescue_records` for auditability and moderation.

---

## 3. Data Persistence & Transaction Boundaries (MariaDB)
- `characters.help_count`: Integer counter tracking the number of completed helper quests.
- `helper_quests`: Stores quest specifications, target items, required quantities, reward items, rarity, guild flag, and completion metadata.
- `rescue_records`: Stores character ID, rescue reason, penalty seconds, and creation timestamp.
- **Transaction Atomicity**: `CompleteQuest` executes within an atomic database transaction (`TransactionProvider.RunInTx`), ensuring inventory deduction, reward item addition, quest completion status update, and replacement quest generation are committed atomically.

---

## 4. HTTP API Endpoints

### Helper Quests
- `GET /helpers/quests`: Lists all currently active and uncompleted helper quests on the request board.
- `POST /helpers/complete`: Submits requested items to fulfill a helper quest (`{"character_id": "...", "quest_id": "..."}`) with session authentication and character ownership verification.

### Emergency Rescue
- `GET /rescues/penalty?character_id=...`: Checks if the specified character is under a rescue penalty cooldown, returning `is_under_penalty` and `remaining_seconds`.
- `POST /rescues/request`: Triggers an emergency unstuck rescue (`{"character_id": "...", "reason": "..."}`) with session authentication and character ownership verification.

