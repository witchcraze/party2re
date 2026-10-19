# Event Plaza, Traveling Merchant Bazaar, and Victory Celebrations Design

## Overview

The Event Plaza Feature Module (`internal/eventplaza`) owns the town gathering plaza (`イベント広場`), traveling merchant bazaar and the temporary crowd added after King Boss sealing. Ordinary presence uses a 5-minute window (`party2/config.cgi_origin:81–82`). The adopted celebration contract and its runtime boundary are described in [§3](#3-victory-celebrations-legacy-contract-and-implementation-gaps). Historical banquet records and previously awarded toast Gold are preserved without runtime listing or claims.

---

## Domain Rules & Systems

### 1. Real-Time Concurrency Tracking & Merchant Tiers

The traveling merchant's inventory uses thresholds of 10/20/30 plaza members. Legacy merchant selection counts stored rows before presence cleanup; displayed headcount is calculated afterward. Status, catalog and purchase each take this snapshot, add the merchant when the selected tier is positive, then clean expired and duplicate records. A subsequent request can select a different tier. `merchant_selection_count` exposes the snapshot; `active_participants` exposes the cleaned display count, including a retained merchant row. GET observations do not create a real visitor heartbeat or rewards.

- **Presence Window (`PresenceWindow = 5 * time.Minute`)**: Adventurers who visited or refreshed the plaza within the last 5 minutes are counted as active participants.
- **Current Storage**: The injected Valkey tracker retains display names (shared by real and virtual guests), insertion order and per-record time markers; heartbeats use the character name and victories append records. Its [keyspace contract](../architecture/valkey-keyspace.md) owns migration and observation-based cleanup. Without that tracker, the MariaDB `eventplaza_presences (character_id, last_seen_at)` adapter counts active real characters only and does not reproduce crowd/merchant ordering. Production injects Valkey and never automatically falls back on a tracker error.
- **Heartbeat (`POST /eventplaza/presence`)**: Adventurers emit presence heartbeats upon entering or interacting with the plaza. Purchases also update presence. Victory crowds are appended through the sealing hook; there is no toast action.

| Tier | Merchant Selection Count | Merchant Title | Catalog Unlocks (Legacy `lib/event.cgi`) |
| :--- | :--- | :--- | :--- |
| **Tier 0** | `< 10` | Traveling Merchant On Journey (行商人巡回中) | None (Merchant is on the road) |
| **Tier 1** | `10 – 19` | Bronze Traveling Merchant (旅の行商人バザー) | 6 Tier 1 items (item-72, 81, 82, 83, 84, 86) |
| **Tier 2** | `20 – 29` | Silver Traveling Merchant (熟練の行商人バザー) | 6 Tier 2 items (item-73, 74, 77, 5, 75, 85) |
| **Tier 3** | `>= 30` | Gold Traveling Merchant (至高の行商人バザー) | 14 Tier 3 items (item-90..100, 108, 142, 217) |

*Note: Per authentic legacy `lib/event.cgi`, higher tiers strictly replace earlier tier inventories (`@sales` array re-assigned).*

---

### 2. Traveling Merchant Bazaar (`internal/eventplaza/data/bazaar.json`)

The bazaar offers 26 canonical items spanning tiers 1 to 3:

#### 3x Pricing Markup
All items are sold at exactly **3× base price** (`$ites[$i][2] *= 3`), reflecting the traveling merchant's premium markup:
- **Tier 1 Items (6 items)**:
  - `item-72`: 力の種 (Seed of Strength) - 6,000 Gold
  - `item-81`: 賢者の石 (Philosopher's Stone) - 30,000 Gold
  - `item-82`: 天使の聖水 (Angel's Holy Water) - 3,000 Gold
  - `item-83`: 蘇生薬 (Revival Elixir) - 6,000 Gold
  - `item-84`: 魔法の小瓶 (Magic Vial) - 1,500 Gold
  - `item-86`: 聖なるしずく (Holy Droplet) - 30,000 Gold
- **Tier 2 Items (6 items)**:
  - `item-73`: 素早さの種 (Seed of Agility) - 6,000 Gold
  - `item-74`: 守りの種 (Seed of Protection) - 6,000 Gold
  - `item-77`: クモの糸 (Spider Web) - 3,000 Gold
  - `item-5`: 魔法の聖水 (Magic Holy Water) - 600 Gold
  - `item-75`: 幸せの種 (Seed of Fortune) - 15,000 Gold
  - `item-85`: 不死鳥の涙 (Phoenix Tear) - 6,000 Gold
- **Tier 3 Items (14 items)**:
  - `item-90`–`item-100`: Scroll collection (炎の巻物, 氷の巻物, 聖なる巻物, etc.) - 60,000 Gold each
  - `item-108`: 光のヴェール (Veil of Light) - 150,000 Gold
  - `item-142`: ソロモンの指輪 (Solomon's Ring) - 300,000 Gold
  - `item-217`: 仙人の薬草 (Hermit's Medicinal Herb) - 30,000 Gold

#### Helper Quest Exclusion
Items currently requested by ongoing Town Helper Quests (`get_helper_item(3)`) are strictly omitted from listing and cannot be purchased (returns HTTP 409 Conflict).

#### Hand Occupancy & Depot Routing
Purchase results contain structured item, payment, and delivery facts. The HTTP response DTO formats `npc_message` using the item name, authenticated character name, and depot delivery flag; the domain service contains no NPC dialogue.

Authentic delivery mechanics based on hand item slot occupancy:
- **Direct Hand Delivery**: If the player's consumable item slot is empty and `quantity == 1`, the item is added directly to character inventory with NPC message:
  `"はい、$nameです"`
- **Depot Storage Delivery**: If the player already holds a consumable item in hand, or if `quantity > 1`, items are routed directly to Depot storage (`預かり所`) with authentic NPC message:
  `"$nameは$charさんの預かり所に送っておきましたよ"`
- If the character's Depot is at capacity, the purchase is safely rejected with HTTP 409 Conflict (`ErrDepotFull`).
- Discovered items are recorded into the player's Item Collection (`internal/collection`).

---

### 3. Victory Celebrations: Legacy Contract and Implementation Gaps

#### Legacy evidence

Paths below are relative to the original `party2/` tree, inspected at `/home/witchcraze/dev/party2/party2/`.

- **Trigger**: `lib/vs_king.cgi:39–68` calls `_win_vs_king.cgi` after successful `ふういん`, alongside the victory news. The crowd is a sealing side effect, not a separately claimable reward.
- **Generation**: `lib/_win_vs_king.cgi:5–11,17–54,56–71` appends 2–5 regular guests and 1–2 props per victory, independently and uniformly selected within each range. The total is 3–7 rows, with mean 5 and probabilities `1/8, 2/8, 2/8, 2/8, 1/8`; it is not a uniform draw from 3–7. Regular guests draw from 65 candidate slots (`@c_randoms`) mapping to 64 unique identities, where 0-based slots 60 and 64 share the duplicate name `ジェイ` (collision probability 67/4225 for two independent draws). Props draw from 32 candidate slots (`@c_fixeds`) mapping to 31 unique identities, where 0-based slots 20 and 24 share the duplicate name `クマチャン` (collision probability 34/1024 for two independent draws). Boss tier does not affect the counts. Successive victories append to existing records.
- **Time marker**: Each added row independently receives a future timestamp `victory_time + 3600 + U{0..3599}` seconds. The initial two-hour setting is decremented before use; it does not give every guest a two-hour lifetime.
- **Actual retention**: `config.cgi_origin:81–82` and `lib/system.cgi:475–530` apply the ordinary 300-second presence window to those future timestamps. A row remains at `now == marker + 300` and is removed when `now > marker + 300`, on the next cleanup. Thus the nominal future marker lies in `[1h, 2h)`, while the cleanup boundary lies in `[65min, 125min)`. Same-name rows are reduced to the first retained row during this cleanup, so generated rows are not guaranteed distinct displayed attendees.
- **Merchant ordering**: `party.cgi:17–20` loads `lib/event.cgi` before `read_member`. `lib/event.cgi:14–56` counts the raw stored rows, including expired or duplicate rows not yet cleaned, selects the 10/20/30 catalog, and appends the merchant's own row when present. Cleanup and display follow. A later request can therefore select a different tier after cleanup; merchant-row presence can also affect the next count. These effects must not be silently replaced by a cleaned-count rule.
- **No toast or separate banquet lifetime**: The only event-specific registered action is `かう` (`lib/event.cgi:76–115`). There is no toast, toast Gold, independent 24-hour banquet record, or separate 24-hour display/claim window in this path.

#### Adopted rule and rationale

Decision: [#1248](https://github.com/witchcraze/party2re/issues/1248). The maintainer explicitly directed: 「できるだけ原典の仕様に合わせる方向でお願いします」. Adopt the observed legacy behavior above, including its retention grace, duplicate handling, candidate slot weights, and merchant-selection order. Generic identities (`Celebration guest %02d` across 65 candidate slots / 64 identities with duplicate slots 60 and 64; `Celebration decoration %02d` across 32 candidate slots / 31 identities with duplicate slots 20 and 24) replace legacy proper names and image assets without adding new catalogs, configuration, or bootstrap infrastructure. Repeated duplicate slot draws retain their authentic double weight in initial stored counts and collapse to the first retained row during subsequent observation cleanup, preventing premature merchant tier progression. No small-server 10/20/30-person boost, strict one-hour expiry, or other gameplay departure is approved by this decision. Existing Go tests and closed #634's incorrect attribution are not approval evidence; #161's victory-log requirement does not authorize toast rewards.

The celebration is represented by the remaining plaza crowd. There is no independent banquet display period to keep at 24 hours or reset to a fixed two hours. Remove the separate banquet listing, toast action and its Gold reward entirely; keeping a zero-Gold toast would still retain an action absent from the original. Preserve the sealing announcement and the crowd hook. Do not claw back previously awarded Gold or destroy historical records as part of retiring their runtime use.

Reconstruct the behavior within current feature/storage boundaries; do not copy the old file architecture, names, images or source. A possible source mistake is evidence to document, not authority to silently substitute a preferred behavior. Merchant prices, item catalog and existing purchase validation remain as specified in §2.

#### Action and call-path reconciliation

| Legacy path/action | Current Go service / transport | Adopted treatment |
| :--- | :--- | :--- |
| `vs_king.cgi` `ふういん` → `_win_vs_king.cgi` | Boss victory hook → `RecordVictoryCrowd` through `cmd/party2/wire_adapters.go`; no standalone celebration command | Sealing appends independently drawn guests/props and per-record markers through the existing hook, regardless of boss tier. Other Boss actions remain owned by [Boss design](boss.md). |
| `event.cgi` load → raw count, merchant row → `system.cgi` `read_member` | `GetPlazaStatus`, `ListAvailableBazaarItems`, purchase tier checks; `GET /eventplaza`, `GET /eventplaza/merchant/items` | Each entry point selects from the stored snapshot, appends the merchant when present, then performs display cleanup. Status exposes both counts. HTTP transport does not justify changing the game rule. |
| `event.cgi` `かう` → `kau(target)` | `ListAvailableBazaarItems` / `PurchaseBazaarItem`; `GET /eventplaza/merchant/items`, `POST /eventplaza/merchant/purchase` | Retain list/buy behavior and §2 safeguards. No toast is part of this action. |
| No legacy counterpart | Former `GET /eventplaza/banquets` and `active_banquets*` fields | Unregistered (404); status has no independent banquet fields or SQL read path. |
| No legacy counterpart | Former `POST /eventplaza/banquets/{id}/toast` | Unregistered (404), with no service call, reward or presence update. No replacement command. |

#### Runtime boundary

The Boss sealing hook calls `RecordVictoryCrowd` directly. It appends guests/props through the injected presence tracker, using the service clock and the independent draws above; no SQL banquet record is needed. Tracker errors propagate to the Boss hook, whose existing best-effort handling and sealing news remain unchanged. Production supplies Valkey; the crowd side effect is not rolled back with SQL if a later sealing operation fails.

Status, catalog and purchase retain the crowd/merchant snapshot semantics in §1. The separate banquet listing, toast action, Gold payout and related DTO/repository contracts have no runtime implementation. Crowd and merchant restoration does not certify unexamined Event Plaza behavior; the action mapping above bounds this reconciliation.

---

## Database Persistence

`eventplaza_presences` (`migrations/080_eventplaza_presences.sql`) remains the SQL adapter for ordinary character presence when no tracker is injected. Production injects the authoritative Valkey tracker described in §1.

`celebration_banquets` and `banquet_toasts` (`migrations/038_eventplaza.sql`) are historical storage only: runtime no longer creates, lists, reads or claims these records. Existing migrations, records and previously awarded Gold remain intact. Existing foreign-key deletion behavior still applies when a character is deleted. SQL definitions are owned by [migrations](../../migrations/).

---

Transport operations and schemas: [OpenAPI](../api/openapi.json).
