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

Candidate ranges and reward weights follow `party2/lib/helper.cgi:22–44,154–211` (reference revision `ff97fc549932a146cc945d7edf71b44c868a840e`). Each slot is equally likely; repeated reward slots retain their weight.

| Target kind | Normal candidate numbers | Rare candidate numbers |
| :--- | :--- | :--- |
| Weapon / Armor | 1–27 | 28–40 |
| Item | 1–27, 30–43, 60–65, 72–103, 108 | 28–29, 40, 58–59, 66–71, 104–107, 109 |
| Monster | 4–120, 198–260 | 500–579 |

Equipment and item slots use the complete native item catalog, including shields and accessories stored under `item-NNN`; names come from that catalog. A missing item definition fails generation rather than silently narrowing its pool. For monsters, draw the primary slot first and validate its native definition. A missing normal monster triggers a separate uniform draw from 1–3; a missing rare monster draws from 160–165. Do not filter primary monster slots or replace them with a different high-level pool. A missing fallback definition fails generation. This replaces the original icon-file existence check with native catalog validation without reusing legacy assets. Currently the native monster catalog contains no 500–579 definitions, so rare monster requests use the specified fallback; this does not claim those primary definitions have been reconstructed.

Normal rewards have **10 slots**: `item-128` ×3, `item-130`–`item-133` ×1 each, `item-134` ×2 and `item-184` ×1. Rare rewards have **6 slots**: `item-129` ×3 and `item-135`–`item-137` ×1 each. Guild requests override either reward pool with `item-126`.

### Lifecycle and Expiration
- **Duration & Expiration**: Quests remain active for **6 days** (`ExpiresAt = CreatedAt + 6 days`). Expiry is strict (`now > expires_at`, so exactly six days is still live). Completed records are permanently retained in storage.
- **Turn-in Sourcing & Reward Delivery**:
  - **Equipment & Items (Kinds 1–3)**: Required turn-in items are deducted from the player's **Depot** (`check_depot`, `character_depots`, `depot_items`). Character bag inventory is not touched.
  - **Companion Monsters (Kind 4)**: Required monsters are deducted from the **Monster Ranch** box (`check_farm`, `character_monsters` where `location = 'box'`).
  - **Reward Delivery**: Rewards are delivered directly into the player's **Depot** (`send_item`). If the reward cannot be stored in the depot (for example, when a new slot is required beyond capacity), the operation aborts with `ErrDepotFull`, rolling back the entire transaction with zero asset loss.
  - **Completion Effects**: Increments `characters.help_count`, awards **+100 GP** to the guild (Guild Quests only), updates quest completion metadata, and generates a fresh replacement quest.
- **Expired Target Completion**: Legacy `party2/lib/helper.cgi:92–145` (`kaiketsu`) validates the target's materials and conditions before checking deadlines. A player with sufficient materials and required guild membership can therefore complete a target quest even after its expiration.
- **Expired Non-Target Exchange**: When completing a target quest, any expired **non-target** requests currently on the board are exchanged one-for-one for newly generated requests.
- **Empty-Target Renewal**: Submitting an empty target (`quest_id = ""`, corresponding to legacy `かいけつ` without arguments via `POST /helpers/complete`) exchanges all expired uncompleted requests on the board one-for-one. It consumes no turn-in materials, delivers no rewards, and grants no `help_count` or GP, returning `renewed_count` with completion fields omitted.
- **Shop Exclusions**: Targets of stored uncompleted helper requests are excluded from shop catalog purchase (`ErrItemUnavailableInHelperQuest`), including at and after their deadline (`party2/lib/system.cgi:1424–1438`, `get_helper_item`). A target becomes purchasable again only after its request is completed or exchanged, provided no other uncompleted request still requires it.
- **Exclusivity & Atomicity**: Each request can be completed by only one character. Concurrent submissions observe committed completion and return `ErrQuestAlreadyDone` without deducting assets or creating duplicate replacements. Successful turn-in, replacement generation, and expired non-target exchanges commit atomically in a single transaction; any error rolls back all board and asset mutations.

### Initial Supply and Empty Stores

Decision: [#1226](https://github.com/witchcraze/party2re/issues/1226). Following the maintainer's direction to match the original (「原点に合わせてね」), **automatic initial supply is zero**. Do not add startup seeding, a minimum board size, periodic replenishment, or read-triggered generation when the store is empty.

The evidence has two distinct boundaries:
- The recovered original repository's `docker-entrypoint.sh:7–10` creates a missing `helper_quest.cgi` as an empty file and preserves an existing file (verified at audit revision `ff97fc549932a146cc945d7edf71b44c868a840e`). This documents the recovered container's fresh-start behavior, not historical live-server provisioning.
- Legacy game code `party2/lib/helper.cgi:60–86,92–148` (`miru` / `kaiketsu`) reads existing records and generates replacements only upon resolving or renewing existing requests. An empty file stays empty. Historical live-server initial request counts and seed data remain **unknown**; no positive count may be assumed as legacy canon.

#### Contract & Behavior
1. **Zero Automatic Bootstrap**: Database schema migration (`031_rescue_and_helper.sql`) and service initialization create zero initial requests. A fresh database has no playable helper quests until requests are explicitly provisioned; a nonzero automated seed or provisioning tool requires an explicit specification decision.
2. **Read-Only Inspection**: `ListQuests` (`GET /helpers/quests`, matching legacy `miru`) and shop exclusion checks read uncompleted requests without side effects. An empty board returns empty without generating requests.
3. **Data Preservation**: Stored requests are preserved across application restarts, retries, and multi-instance operations without top-up seeding or leader election.
4. **Renewal Only**: `CompleteQuest` (`POST /helpers/complete`, matching legacy `kaiketsu`) replaces completed targets and exchanges existing expired non-target requests; it never generates requests from an empty store. When all requests have expired, players can still complete eligible requests or submit an empty target (`quest_id = ""`) to exchange expired requests. Both completion and renewal operations on this endpoint route through the standard town action guard (`withAuthenticatedActionCharacterAndJSON`), rejecting sleeping characters with HTTP 409 Conflict before mutations.

---

## 2. Emergency Rescue System (救出処理)

### Purpose & Constraints
- Cancels and clears any active scheduled action or dangling activity.
- Applies a cooldown/sleep penalty of **600 seconds (10 minutes)** (`DefaultPenaltySeconds`) via ephemeral lock (`timer.CategorySleep`) in Valkey/memory when unstucking a trapped character. If the character already has active sleep remaining, the 10-minute penalty accumulates onto the remaining duration (`rem + 600s`), strictly matching legacy `rescue.cgi:58-59` (`$m{sleep} = $m{sleep} > 0 ? $m{sleep} + $sleep_time * 60 : $sleep_time * 60`).
- If the character is already safe in town (has no ongoing or stuck activity to clear), rescue returns early with 0 penalty cooldown and does not set the sleep timer or record a new penalty, matching legacy `rescue.cgi:56-65` (`すでに救出処理がされています`).
- During the penalty cooldown, character actions are restricted across all action endpoints via sleep guard (`timer.CategorySleep` returning 409 Conflict), as well as domain checks (`IsUnderPenalty` returns true and `CheckActionAllowed` rejects actions with `ErrCharacterUnderPenalty`).
- Records rescue actions that unstuck trapped characters in `rescue_records` for auditability and moderation.

---

## 3. Data Persistence & Transaction Boundaries (MariaDB)

### Stored Entities
- `characters.help_count`: Counter tracking completed helper quests.
- `helper_quests`: Stores active and completed request records (category kind, target requirement, reward item, rarity and guild flags, expiration timestamp, completion status and completer character ID).
- `rescue_records`: Audit log of unstuck emergency rescues (character ID, reason, penalty cooldown seconds, creation timestamp).

### Transaction & Concurrency Guarantees
- **Atomicity**: Quest completion and renewal execute in a single database transaction. All mutations (item/monster deductions, depot reward delivery, quest completion status, replacement generation, and expired exchanges) commit or roll back together. Renewal-only calls use the same transaction owner without character asset mutations.
- **Lock Ordering**: To prevent deadlocks and enforce single-claim exclusivity (`party2/lib/helper.cgi:92–145`), transactions acquire locks in strict hierarchy:
  1. `helper_quests` (ascending ID order)
  2. `characters`
  3. `character_depots` / `depot_items`
  4. Guild points (guild quests only)

---

Transport operations and schemas: [OpenAPI](../api/openapi.json).
