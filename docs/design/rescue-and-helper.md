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
- Legacy `party2/lib/helper.cgi:92–145,128–132` (`kaiketsu`) exchanges every expired non-target request for one new request on each `かいけつ` call, even when that call completes nothing. Go matches this in `CompleteQuest` ([#1217](https://github.com/witchcraze/party2re/issues/1217)): before each completion attempt, and independently of its outcome (unknown id, insufficient materials, guild requirement), a separate transaction deletes every expired uncompleted request except the target and inserts one replacement per deletion. Rows are locked per id, so concurrent or retried calls exchange each row once; a save failure rolls back the deletions together with the replacements. Completed rows are retained; an expired target is still rejected with `ErrQuestExpired` (the legacy script would complete it) and is then exchanged too. Expiry is strict (`now > expires_at`, so exactly six days is still live). `ListQuests` / `GET /helpers/quests` never write, matching `miru`. Exchange is one-for-one and never creates requests in a store that has no rows.

### Initial Supply and Empty Stores

Decision: [#1226](https://github.com/witchcraze/party2re/issues/1226). Following the maintainer's direction to match the original (「原点に合わせてね」), **automatic initial supply is zero**. Do not add startup seeding, a minimum board size, periodic replenishment, or read-triggered generation when the store is empty.

The evidence has two different limits:

- The recovered original repository's `docker-entrypoint.sh:7–10` creates a missing `helper_quest.cgi` as an empty file and preserves an existing file. This also holds at the audit revision `ff97fc549932a146cc945d7edf71b44c868a840e`; it is evidence for the recovered container's fresh-start behavior, not proof of historical live-server provisioning.
- Game code `party2/lib/helper.cgi:60–86,92–148` registers `みる` / `かいけつ`, reads existing rows, and only generates replacements while processing an existing completed or expired request. An empty file stays empty. The original live server's initial request count/data remains **unknown**; no positive count may be presented as an established legacy rule.

The Go migration `031_rescue_and_helper.sql` creates the table without inserting requests. `cmd/party2` constructs the service without a supply step, and `ListQuests` / `GetActiveHelperItemIDs` read `ListActive` without generating requests. These match the adopted zero-bootstrap rule.

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
| `みる` → `miru` | `ListQuests` / `GET /helpers/quests` | Read existing requests; an empty result does not generate supply. |
| `かいけつ` → `kaiketsu` | `CompleteQuest` / `POST /helpers/complete` | Complete an existing request and create its replacement; also exchanges expired non-target requests on every call (#1217). No bootstrap command. |

A fresh installation therefore has no playable helper requests until initial request data is separately supplied. A nonzero automatic seed or provisioning tool would require an explicit new decision about its count, source and transaction/idempotency contract; none is authorized here. Production guild wiring is connected via public port adapter in `cmd/party2` ([#1218](https://github.com/witchcraze/party2re/issues/1218)); this decision does not certify the generator pools/reward weights ([#1216](https://github.com/witchcraze/party2re/issues/1216)).

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
- **Transaction Atomicity**: `CompleteQuest` executes within an atomic database transaction (`TransactionProvider.RunInTx`), ensuring turn-in verification, item/monster deduction, direct depot reward delivery, quest completion status update, and replacement quest generation are committed atomically.
- **Shared Quest Claim**: A request can be completed by only one character. `CompleteQuest` locks the `helper_quests` row (Rank 0) before checking completion and before locking the character (Rank 2), depot (Rank 5), and awarding guild points (Rank 7, guild quests). Concurrent submissions, including repeated submissions by the same character, observe the committed completion and return `ErrQuestAlreadyDone` without paying materials, granting rewards, incrementing `help_count`, or creating another replacement. Any later error rolls back all changes and permits a retry. This preserves the exclusivity of legacy `party2/lib/helper.cgi:92-145` (`kaiketsu` locks the shared request file before inspecting and replacing a request), using the current database transaction boundary.
- **Storage Sourcing & Direct Depot Reward Delivery**:
  - **Equipment & Items (Kinds 1, 2, 3)**: Checked and deducted from `DepotRepository` (`character_depots`, `depot_items`), matching legacy `party2/lib/helper.cgi:113` (`check_depot`). Character bag inventory is not touched.
  - **Companion Monsters (Kind 4)**: Checked and deducted from `FarmRepository` (`character_monsters`, `location = 'box'`), matching legacy `check_farm`.
  - **Direct Depot Delivery**: Reward items are delivered directly into the player's Depot (`send_item($m, 3, $pay)`). If the player's Depot is at full capacity and cannot accept the reward item, the operation aborts with `ErrDepotFull`, rolling back the entire transaction so that turn-in items and quest state are preserved with zero asset loss.


