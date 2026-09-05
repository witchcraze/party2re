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
   - `casino_accounts`, `character_lottery`, `lottery_tickets`
   - `farm_plots`, `character_blessings`, `banquet_toasts`
   - `blackmarket_character_points`, `blackmarket_character_purchases`
   - `tavern_deliveries`, `tavern_character_status`
   - `park_posts`, `rescue_records`
   - `contest_votes`, `contest_entries`, `character_photos`
   - `character_deliveries`, `delivery_parcels` (sender or recipient)
   - `fleamarket_listings`, `auction_listings`
   - `character_letters` (sender or recipient), `character_companion_phrases`, `character_delivery_notices`, `character_homes`
   - `character_boss_records`, `boss_challenge_history`
   - `character_dungeon_records`, `dungeon_active_expeditions`, `dungeon_expedition_history`
   - `character_challenge_records`, `challenge_sessions`
   - `arena_ratings`, `arena_matches` (attacker or defender)
   - `character_monsters`, `character_monster_book`, `character_item_collection`
   - `guild_members`, `guilds` (clear `leader_character_id`)
   - `activities`, `adventures`
   - `character_custom_skills`, `equipment_slots`, `inventory_items`
   - `depot_items`, `character_depots`
   - `character_job_masteries`, `character_job_history`, `character_jobs`
   - `character_profiles`
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
3. Invalidate all active player sessions in Valkey Master via `SessionRepository.DeleteByPlayerID` (O(1) player session set and token deletion).
4. Player-linked relational tables in MariaDB:
   - `player_notifications`
   - `bank_transfers` (both sender and recipient)
   - `bank_accounts`
5. Primary player record in `players`.

---

## 4. API Endpoints

| Method | Endpoint | Auth | Description |
|---|---|---|---|
| `DELETE` | `/players/me` | Bearer (Player) | Delete authenticated player account and all associated characters/data. |
| `DELETE` | `/players/{id}` | Bearer (Player / Admin) | Delete specified player account and associated characters/data. |
| `DELETE` | `/characters/{id}` | Bearer (Player) | Delete specified character and associated progression/inventory/features. |
