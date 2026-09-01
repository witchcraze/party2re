# Player and Character Deletion Specification

## 1. Overview

In Party2, player accounts and individual characters can be permanently removed from the system. Due to the modular monolithic design and relational database integrity constraints, deletion must handle cascading dependency cleanup in a deterministic, consistent order within an atomic database transaction.

---

## 2. Character Deletion (`DELETE /characters/{id}`)

### 2.1 Authorization & Invariants
- An authenticated player can only delete characters owned by their account (`char.PlayerID == player.ID`).
- Attempting to delete another player's character returns HTTP `403 Forbidden`.
- Attempting to delete a nonexistent character returns HTTP `404 Not Found`.

### 2.2 Cascading Cleanup Order
Character deletion executes within a database transaction and cleans up resources in reverse-dependency order:
1. External domain cleanup hooks (`CleanupHook`) for cross-service cleanup.
2. Character-linked feature tables:
   - `character_photos`
   - `contest_entries`, `contest_votes`
   - `home_companion_phrases`, `home_notices`, `home_profiles`
   - `character_item_collection`, `character_monster_book`
   - `tavern_deliveries`
   - `letters` (both as sender and recipient)
   - `character_custom_skills`
   - `character_challenges`
   - `activity_logs`
   - `farm_plots`
   - `parcels` (both sender and recipient)
   - `delivery_quests`
   - `character_job_masteries`
   - `equipment_slots`
   - `flea_market_listings`
   - `auction_bids`, `auctions`
   - `party_members`
   - `guild_members`
   - `depot_items`
   - `inventories`
   - `character_stats`
3. Primary character record in `characters`.

---

## 3. Player Account Deletion (`DELETE /players/me` and `DELETE /players/{id}`)

### 3.1 Authorization & Invariants
- `DELETE /players/me`: Authenticated player initiates self-deletion. If a password is provided, it is verified against the player's hashed password.
- `DELETE /players/{id}`: Deletion of a specific player account by ID. Allowed if the authenticated player matches the target ID or if the request contains valid administrator credentials.
- Invalid password returns HTTP `401 Unauthorized`.

### 3.2 Cascading Cleanup Order
Player account deletion executes within a database transaction:
1. Retrieves all characters owned by the player via `FindByPlayerID`.
2. Iterates through and invokes `characterService.Delete(ctx, playerID, char.ID)` for each character, triggering all character cleanup hooks and cascading SQL deletions.
3. Player-linked account tables:
   - `player_sessions` (active and expired sessions)
   - `player_notifications`
   - `bank_transfers` (both sender and recipient)
   - `bank_accounts`
4. Primary player record in `players`.

---

## 4. API Endpoints

| Method | Endpoint | Auth | Description |
|---|---|---|---|
| `DELETE` | `/players/me` | Bearer (Player) | Delete authenticated player account and all associated characters/data. |
| `DELETE` | `/players/{id}` | Bearer (Player / Admin) | Delete specified player account and associated characters/data. |
| `DELETE` | `/characters/{id}` | Bearer (Player) | Delete specified character and associated progression/inventory/features. |
