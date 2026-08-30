# Event Plaza, Traveling Merchant Bazaar, and Victory Celebration Banquets Design

## Overview

The Event Plaza Feature Module (`internal/eventplaza`) introduces a dynamic town gathering plaza (`イベント広場`) where town population metrics influence traveling merchant offerings and community-wide victory banquets celebrate boss conquests.

---

## Domain Rules & Systems

### 1. Town Population & Merchant Tiers

The traveling merchant's bazaar inventory dynamically adapts based on the active character population in the game world:

| Tier | Active Population | Merchant Title | Catalog Unlocks |
| :--- | :--- | :--- | :--- |
| **Tier 0** | `< 10` characters | Traveling Merchant on the Road (行商人の旅路) | None (Merchant on the road) |
| **Tier 1** | `10 – 19` characters | Bronze Traveling Merchant (新米行商人バザー) | Tier 1 items (Herbs, Magic Water, Warding Talismans) |
| **Tier 2** | `20 – 29` characters | Silver Traveling Merchant (熟練の行商人バザー) | Tier 1 + Tier 2 items (Phoenix Feathers, Dragon Whetstones, Wind Attire) |
| **Tier 3** | `>= 30` characters | Gold Traveling Merchant (伝説の豪商バザー) | All items (Tiers 1, 2, and 3: Nectar of the Gods, Genesis Crystals, Star-Cleaver Sword) |

The plaza status endpoint (`GET /eventplaza`) calculates the active population count, the current merchant tier, and the distance to the next tier threshold.

---

### 2. Traveling Merchant Bazaar (`internal/eventplaza/data/bazaar.json`)

The traveling merchant offers high-grade consumable items, crafting materials, and legendary gear:

- **Tier 1 Items**:
  - `bazaar_herb_extract` (名薬草のエキス): 500 Gold
  - `bazaar_mana_water` (活性魔力水): 800 Gold
  - `bazaar_warding_talisman` (銀の魔除け護符): 1,200 Gold
- **Tier 2 Items**:
  - `bazaar_phoenix_feather` (不死鳥の羽): 3,000 Gold
  - `bazaar_dragon_whetstone` (竜鱗の極上砥石): 5,000 Gold
  - `bazaar_wind_robe` (風詠みの戦装束): 7,500 Gold
- **Tier 3 Items**:
  - `bazaar_god_ambrosia` (天界の神酒): 15,000 Gold
  - `bazaar_genesis_crystal` (星彩の創世結晶): 25,000 Gold
  - `bazaar_star_sword` (星砕きの宝剣): 50,000 Gold

#### Purchase Mechanics
- Purchasing is fully transactional via `database.RunInTx`.
- Character gold and inventory are locked pessimistically (`SELECT ... FOR UPDATE`).
- Verifies tier unlocking, sufficient gold balance, and valid quantity.
- Deducts gold and appends new item instances (`coreitem.Instance`) to the character's inventory atomically.

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

### Schema Migration: `migrations/038_eventplaza.sql`

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
```

---

## HTTP REST Endpoints

| Method | Endpoint | Description | Auth |
| :--- | :--- | :--- | :--- |
| `GET` | `/eventplaza` | Get plaza status, population tier, and active banquets count | Public |
| `GET` | `/eventplaza/merchant/items` | List traveling merchant items unlocked at current tier | Public |
| `POST` | `/eventplaza/merchant/purchase` | Purchase goods from traveling merchant | Character Auth |
| `GET` | `/eventplaza/banquets` | List active victory celebration banquets | Public |
| `POST` | `/eventplaza/banquets/{id}/toast` | Raise a toast at a celebration banquet | Character Auth |
