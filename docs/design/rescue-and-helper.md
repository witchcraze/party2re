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

Equipment and item slots use the complete native item catalog, including shields and accessories stored under `item-NNN`; names come from that catalog. A missing item definition fails generation rather than silently narrowing its pool. For monsters, draw the primary slot first and validate its native definition. A missing normal monster triggers a separate uniform draw from 1–3; a missing rare monster draws from 160–165. Do not filter primary monster slots or replace them with a different high-level pool. A missing fallback definition fails generation. This replaces the original icon-file existence check with native catalog validation without reusing legacy assets. Currently the native monster catalog contains no 500–579 definitions, so rare monster requests use the specified fallback; this does not claim those primary definitions have been reconstructed. The composition root supplies monster lookup to both completion replacement and expired-request exchange.

Normal rewards have **10 slots**: `item-128` ×3, `item-130`–`item-133` ×1 each, `item-134` ×2 and `item-184` ×1. Rare rewards have **6 slots**: `item-129` ×3 and `item-135`–`item-137` ×1 each. Guild requests override either reward pool with `item-126`.

### Lifecycle and Expiration
- Quests remain active for **6 days** (`ExpiresAt = CreatedAt + 6 days`).
- Upon completion, required items/equipment are deducted from the player's **Depot** (`check_depot`), or monsters are deducted from the **Monster Ranch** box (`check_farm`), quest rewards are delivered directly to the player's **Depot** (`send_item`), the character's `help_count` is incremented, and a fresh replacement quest is generated.
- Legacy `party2/lib/helper.cgi:92–145` (`kaiketsu`) checks the selected target before its deadline. A target with sufficient materials and required guild membership can therefore complete even after expiry. Only expired **non-target** requests are exchanged one-for-one. A material or membership rejection returns without saving the board. `CompleteQuest` preserves these branches in one transaction: successful turn-in and non-target renewal commit together; any error rolls back both assets and board changes. Shared rows are locked in ascending ID order and rechecked, preventing duplicate claims or exchanges under concurrency and retries. Completed rows are retained. Expiry is strict (`now > expires_at`, so exactly six days is still live).
- An empty target corresponds to legacy `かいけつ` without an argument: exchange expired requests without consuming materials, granting rewards, incrementing `help_count` or awarding GP. The existing `POST /helpers/complete` accepts omitted/empty `quest_id` for this operation and returns `renewed_count`; completion fields are omitted. Nonempty unknown/completed IDs return an error without writes. A normal completion returns its completion fields and the count of expired non-target requests exchanged, excluding its own replacement.
- `ListQuests` / `GET /helpers/quests` remain read-only, matching `miru`. Renewal never generates requests for an empty store.
- Shop exclusions use all stored uncompleted requests, including at and after their deadline (`party2/lib/system.cgi:1424–1438`, `get_helper_item`). A target becomes available only after completion or exchange, provided no other uncompleted request still requires it. The shared `GetActiveHelperItemIDs` reader uses `ListUncompleted` without renewing requests; all existing storefront filters retain their native target ID semantics.

### Initial Supply and Empty Stores

Decision: [#1226](https://github.com/witchcraze/party2re/issues/1226). Following the maintainer's direction to match the original (「原点に合わせてね」), **automatic initial supply is zero**. Do not add startup seeding, a minimum board size, periodic replenishment, or read-triggered generation when the store is empty.

The evidence has two different limits:

- The recovered original repository's `docker-entrypoint.sh:7–10` creates a missing `helper_quest.cgi` as an empty file and preserves an existing file. This also holds at the audit revision `ff97fc549932a146cc945d7edf71b44c868a840e`; it is evidence for the recovered container's fresh-start behavior, not proof of historical live-server provisioning.
- Game code `party2/lib/helper.cgi:60–86,92–148` registers `みる` / `かいけつ`, reads existing rows, and only generates replacements while processing an existing completed or expired request. An empty file stays empty. The original live server's initial request count/data remains **unknown**; no positive count may be presented as an established legacy rule.

The Go migration `031_rescue_and_helper.sql` creates the table without inserting requests. `cmd/party2` constructs the service without a supply step. `ListQuests` reads all uncompleted requests, including expired ones (`ListUncompleted`), matching legacy `miru` (`party2/lib/helper.cgi:68–87`); shop exclusions read the same stored requests as described above. Neither read generates requests. These match the adopted zero-bootstrap rule.

When every request has expired, clients can complete a request if they meet its conditions, or call authenticated `POST /helpers/complete` with `character_id` and omitted/empty `quest_id` for renewal only, even without turn-in materials. Both operations route through the standard town facility action guard (`withAuthenticatedActionCharacterAndJSON`), rejecting sleeping or pending-wake characters with HTTP 409 Conflict (and sleep query errors with 500) before any quest completion, renewal, material consumption, reward distribution, or HelpCount/GP mutations. Clients fetch `GET /helpers/quests` after a successful completion or renewal to obtain the current board. A rejected completion leaves it unchanged. GET remains read-only; no scheduler or additional supply mechanism is involved.

| Situation | Expected behavior |
| :--- | :--- |
| Fresh, correctly migrated DB with no requests | Startup creates no requests; a successful list returns an empty board. Shop exclusion readers return no requested targets. No request generation, RNG consumption or save occurs. |
| Existing requests and application restart | Preserve the stored requests; do not append, reset, or top up to an invented initial count. |
| Several instances, repeated reads or restart retries | All instances create zero initial requests. No initialization marker, leader election or once-only seed lock is needed. |
| No active requests, but expired/completed rows exist | Do not infer a fresh installation from active count zero. Expired uncompleted rows are exchanged one-for-one by `CompleteQuest` (#1217); never use that to fill an arbitrary board size. |
| Missing table or failed storage read | Return the storage error through the existing error path; do not reinterpret failure as an empty board, create a schema, or seed requests. |
| Retry after a failed read | Read the authoritative store again. If it is empty, it remains empty; no partial bootstrap or duplicate generation exists. |
| Save failure | Bootstrap performs no save. Completion/replacement failures retain their transaction rollback behavior; their recovery is not an excuse to seed an empty store. |

| Original action / path | Go service / current transport | Supply effect |
| :--- | :--- | :--- |
| Container startup creates the missing request file | Schema migration and `cmd/party2` service composition | No initial requests; retain existing data. |
| `みる` → `miru` | `ListQuests` / `GET /helpers/quests` | Read uncompleted requests including expired ones; an empty result does not generate supply. |
| `かいけつ` → `kaiketsu` | `CompleteQuest` / `POST /helpers/complete` | Complete an existing request and create its replacement; exchanges expired non-target requests on success, or all expired requests with an empty target. No bootstrap command. |

A fresh installation therefore has no playable helper requests until initial request data is separately supplied. A nonzero automatic seed or provisioning tool would require an explicit new decision about its count, source and transaction/idempotency contract; none is authorized here. Production guild wiring is connected via public port adapter in `cmd/party2` ([#1218](https://github.com/witchcraze/party2re/issues/1218)).

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
- `characters.help_count`: Integer counter tracking the number of completed helper quests.
- `helper_quests`: Stores quest specifications, target items, required quantities, reward items, rarity, guild flag, and completion metadata.
- `rescue_records`: Stores character ID, rescue reason, penalty seconds, and creation timestamp.
- **Transaction Atomicity**: `CompleteQuest` executes within an atomic database transaction (`TransactionProvider.RunInTx`), ensuring turn-in verification, item/monster deduction, direct depot reward delivery, quest completion status update, replacement quest generation, and expired non-target exchanges are committed atomically. Renewal-only calls use the same transaction owner without character asset mutations.
- **Shared Quest Claim**: A request can be completed by only one character. `CompleteQuest` locks the target and expired candidate `helper_quests` rows in ascending ID order (Rank 0) and rechecks them before locking the character (Rank 2), depot (Rank 5), and awarding guild points (Rank 7, guild quests). Concurrent submissions, including repeated submissions by the same character, observe the committed completion and return `ErrQuestAlreadyDone` without paying materials, granting rewards, incrementing `help_count`, or creating another replacement. Any later error rolls back all changes and permits a retry. This preserves the exclusivity of legacy `party2/lib/helper.cgi:92-145` (`kaiketsu` locks the shared request file before inspecting and replacing a request), using the current database transaction boundary.
- **Storage Sourcing & Direct Depot Reward Delivery**:
  - **Equipment & Items (Kinds 1, 2, 3)**: Checked and deducted from `DepotRepository` (`character_depots`, `depot_items`), matching legacy `party2/lib/helper.cgi:113` (`check_depot`). Character bag inventory is not touched.
  - **Companion Monsters (Kind 4)**: Checked and deducted from `FarmRepository` (`character_monsters`, `location = 'box'`), matching legacy `check_farm`.
  - **Direct Depot Delivery**: Reward items are delivered directly into the player's Depot (`send_item($m, 3, $pay)`). If the player's Depot is at full capacity and cannot accept the reward item, the operation aborts with `ErrDepotFull`, rolling back the entire transaction so that turn-in items and quest state are preserved with zero asset loss.

---

Transport operations and schemas: [OpenAPI](../api/openapi.json).
