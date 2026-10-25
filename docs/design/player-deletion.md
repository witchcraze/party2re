# Player and Character Deletion Specification

## 1. Overview

In Party2, player accounts and individual characters can be permanently removed from the system. Due to the modular monolithic design and relational database integrity constraints, deletion must handle cascading dependency cleanup in a deterministic, consistent order with explicit SQL and external cleanup boundaries.

---

## 2. Character Deletion (`DELETE /characters/{id}`)

### 2.1 Authorization & Invariants
- An authenticated player can only delete characters owned by their account (`char.PlayerID == player.ID`).
- Attempting to delete another player's character returns HTTP `403 Forbidden`.
- Attempting to delete a nonexistent character returns HTTP `404 Not Found`.

### 2.2 Cleanup boundary

`CleanupHook` lets feature owners release associations and ephemeral state before
character deletion; SQL repository cleanup removes dependent records before the
character root in one transaction. The concrete table list belongs to the
[repository](../../internal/database/character_repository.go) and migrations.

Challenge admission is shared run evidence: deleting a departed original leader
preserves the admission row while another participant's durable claim remains,
including completed settlement awaiting publication. Surviving participants retain
their existing run and rewards. See the [run lifetime contract](../architecture/transient-run-state.md#identity-and-admission).

Current `character.Service.Delete` invokes hooks **before** its SQL transaction
and treats hook errors as best-effort. SQL rollback therefore does not restore
Valkey/external cleanup. This is not an atomic cross-store cascade; callers must
not infer full rollback from a later deletion failure.

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
   - `player_api_tokens`
5. Primary player record in `players`.

---

Transport operations and schemas: [OpenAPI](../api/openapi.json).
