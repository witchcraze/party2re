# Event Plaza, Traveling Merchant Bazaar, and Victory Celebration Banquets Design

## Overview

The Event Plaza Feature Module (`internal/eventplaza`) introduces a dynamic town gathering plaza (`イベント広場`) where real-time active character presence (5-minute active window matching legacy `party2/lib/event.cgi` `$limit_member_time = 60 * 5`) determines traveling merchant tiers and catalog availability. It also coordinates community-wide victory banquets celebrating King Boss conquests.

---

## Domain Rules & Systems

### 1. Real-Time Concurrency Tracking & Merchant Tiers

The traveling merchant's bazaar inventory dynamically adapts based on real-time active participant presence within the plaza:

- **Presence Window (`PresenceWindow = 5 * time.Minute`)**: Adventurers who visited or refreshed the plaza within the last 5 minutes are counted as active participants.
- **Dual Tracking Engine**:
  - Primary in-memory Valkey Sorted Set (`party2:eventplaza:presence` with Unix timestamp score) for sub-millisecond concurrency evaluation.
  - MariaDB backing table `eventplaza_presences (character_id, last_seen_at)` for durable fallback and cold-start synchronization.
- **Heartbeat (`POST /eventplaza/presence`)**: Adventurers emit presence heartbeats upon entering or interacting with the plaza. Purchases and toasts also update presence automatically.

| Tier | Active Participants | Merchant Title | Catalog Unlocks (Legacy `lib/event.cgi`) |
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
Authentic delivery mechanics based on hand item slot occupancy:
- **Direct Hand Delivery**: If the player's consumable item slot is empty and `quantity == 1`, the item is added directly to character inventory with NPC message:
  `"はい、$nameです"`
- **Depot Storage Delivery**: If the player already holds a consumable item in hand, or if `quantity > 1`, items are routed directly to Depot storage (`預かり所`) with authentic NPC message:
  `"$nameは$charさんの預かり所に送っておきましたよ"`
- If the character's Depot is at capacity, the purchase is safely rejected with HTTP 409 Conflict (`ErrDepotFull`).
- Discovered items are recorded into the player's Item Collection (`internal/collection`).

---

### 3. Victory Celebration Banquets (`celebration_banquets`)

When a player conquers a King Boss in the Boss Challenge Arena (`internal/boss`), a town-wide victory celebration banquet is registered automatically:

- **Banquet Lifecycle**:
  - **Slayer Recognition**: Records the boss name, slayer character ID, character name, and boss tier.
  - **Duration**: Active for 24 hours (`expires_at = celebrated_at + 24h`).
- **Toasting & Morale Boost (`POST /eventplaza/banquets/{id}/toast`)**:
  - Other adventurers can join the celebration and raise a toast (`乾杯`).
  - **Rewards**: Each toast awards `300 Gold * Boss Tier` in commemorative celebration gold.
  - **Duplicate Protection**: Players may only toast a given victory banquet once, enforced via composite primary key `(banquet_id, character_id)` in `banquet_toasts`.
  - Expired banquets return HTTP `410 Gone`.

---

## Database Persistence

### Schema Migrations:
- `migrations/038_eventplaza.sql`: Celebration banquets and toasts tables.
- `migrations/080_eventplaza_presences.sql`: Plaza participant concurrency tracking table.

```sql
CREATE TABLE celebration_banquets (
    id CHAR(32) NOT NULL PRIMARY KEY,
    boss_id VARCHAR(64) NOT NULL,
    boss_name VARCHAR(128) NOT NULL,
    slayer_character_id CHAR(32) NOT NULL,
    slayer_character_name VARCHAR(64) NOT NULL,
    tier INT NOT NULL DEFAULT 1,
    toast_count INT NOT NULL DEFAULT 0,
    celebrated_at DATETIME(6) NOT NULL,
    expires_at DATETIME(6) NOT NULL,
    INDEX idx_banquets_expires_at (expires_at, celebrated_at DESC),
    CONSTRAINT fk_banquets_slayer_character FOREIGN KEY (slayer_character_id)
        REFERENCES characters (id) ON DELETE CASCADE
);

CREATE TABLE banquet_toasts (
    banquet_id CHAR(32) NOT NULL,
    character_id CHAR(32) NOT NULL,
    toasted_at DATETIME(6) NOT NULL,
    PRIMARY KEY (banquet_id, character_id),
    CONSTRAINT fk_toasts_banquet FOREIGN KEY (banquet_id)
        REFERENCES celebration_banquets (id) ON DELETE CASCADE,
    CONSTRAINT fk_toasts_character FOREIGN KEY (character_id)
        REFERENCES characters (id) ON DELETE CASCADE
);

CREATE TABLE eventplaza_presences (
    character_id CHAR(32) NOT NULL PRIMARY KEY,
    last_seen_at DATETIME(6) NOT NULL,
    INDEX idx_eventplaza_presences_last_seen (last_seen_at)
);
```

---

## HTTP REST Endpoints

| Method | Endpoint | Description | Auth |
| :--- | :--- | :--- | :--- |
| `GET` | `/eventplaza` | Get plaza status, real-time active participants, merchant tier, and active banquets | Public |
| `POST` | `/eventplaza/presence` | Record character active presence in Event Plaza | Character Auth |
| `GET` | `/eventplaza/merchant/items` | List traveling merchant items unlocked at current tier (filtered by helper quest) | Public |
| `POST` | `/eventplaza/merchant/purchase` | Purchase goods from traveling merchant (depot fallback, 3x price markup) | Character Auth |
| `GET` | `/eventplaza/banquets` | List active victory celebration banquets | Public |
| `POST` | `/eventplaza/banquets/{id}/toast` | Raise a toast at a celebration banquet | Character Auth |
