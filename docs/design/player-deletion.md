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
Character deletion executes within an atomic database transaction in reverse-dependency order:
1. **Domain Cleanup Hooks (`CleanupHook`)**: External feature services register cleanup hooks to release active locks, ephemeral Valkey state (e.g., active room presence, navigation bookmarks), and cross-service associations.
2. **Character-Linked Domain Data**: Cascading SQL deletion cleans up feature records (social posts/letters, mini-games, dungeon/challenge progression, guild memberships, inventories, depot, equipment, and job masteries).
3. **Primary Character Record**: Deletes the character root in `characters`.

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
   - `player_api_tokens` (via foreign key `ON DELETE CASCADE`)
5. Primary player record in `players`.
