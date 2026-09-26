# Standard Shops Design and Parity Specification

## Purpose

This document describes the behavior, economic formulas, catalog progression, depot transfer routing, and NPC interaction rules governing the three standard town shops (Weapon Shop, Armor Shop, Item Shop) in Party2, maintaining 1:1 parity with the legacy Perl CGI implementation (`weapon.cgi`, `armor.cgi`, `item.cgi`).

---

## 1. Shopkeeper NPCs and Roles

| Shop Type | Title | NPC Name | Personality / Dialogue Style | Special Interaction |
| :--- | :--- | :--- | :--- | :--- |
| **`weapon`** | 武器屋 | ブッキー (Bucky) | Tough, direct blacksmith | Inspect: *"おいおい、俺は武器じゃねぇぜ"* |
| **`armor`** | 防具屋 | アマノ (Amano) | Polite apprentice (ends with *〜ッス*) | Inspect: *"な、な、何を見ているッスか！？！"* |
| **`item`** | 道具屋 | アイテムコ (Itemko) | Feline merchant (ends with *〜ニャ*) | Inspect: *"ほえ？なんでしょうかぁ？"* + Secret Shop Hint (*"＠ひみつのみせ に行きたい"*) |

---

## 2. Pricing Formulas

### 2.1. Retail Purchase Price (2x Multiplier)

In legacy Party2, retail shops sell items at double the base catalog price:
$$\text{RetailPrice} = \text{BasePrice} \times 2$$

- Overflow protection: guarded with `math.MaxInt` bounds checking (`ErrPriceOverflow`).
- Free items ($\text{BasePrice} \le 0$) remain 0 gold.

### 2.2. Resale Price (50% Markdown)

Merchants buy equipment and items from adventurers at 50% of base catalog price:
$$\text{SellPrice} = \lfloor \text{BasePrice} \times 0.5 \rfloor$$

---

## 3. Catalog Progression by Job Level (`job_lv`)

Available items in each shop expand according to the character's `job_lv` (the current job's level):

### 3.1. Weapon Shop (`weapon.cgi`)
- **`job_lv <= 11`**:
  `weapon-01` .. `weapon-05`, `weapon-43` (Club, Knife, Copper Sword, Bronze Sword, Iron Sword, Blowgun)
- **`job_lv > 11`**:
  Adds `weapon-06` .. `weapon-16` (Silver Sword, Rapier, Iron Axe, Steel Sword, Broad Sword, Battle Axe, Silver Axe, Morning Star, Scythe, Warhammer, War Axe)

### 3.2. Armor Shop (`armor.cgi`)
- **`job_lv <= 4`**:
  `armor-01` .. `armor-05` (Plain Clothes, Leather Armor, Chain Mail, Bronze Armor, Iron Armor)
- **`5 <= job_lv <= 10`**:
  Adds `armor-06` .. `armor-11` (Steel Armor, Silver Armor, Plate Mail, Knight Armor, Heavy Armor, Dragon Armor)
- **`job_lv >= 11`**:
  Adds `armor-12` .. `armor-16` (Magic Armor, Dark Armor, Holy Armor, Battle Suit, Master Armor)

### 3.3. Item Shop (`item.cgi`)
- **`job_lv <= 4`**:
  `item-001` .. `item-005` (Herb, Antidote Herb, Moon Herb, Magic Water, Sage Stone)
- **`5 <= job_lv <= 9`**:
  Adds `item-006` .. `item-009`, `item-101` (Dragon Wing, Demon Wing, Angel Wing, Chimera Wing, Warp Mirror)
- **`job_lv >= 10`**:
  Adds `item-010` .. `item-012`, `item-102` (Torch, Fairy Water, Holy Water, Return Mirror)

---

## 4. Helper Quest Exclusion

When a character has an active helper quest (`helper_quests` state), any item that is the objective of the quest is filtered out of the catalog (`GetCatalog`) and forbidden from being purchased (`Purchase`). This enforces the legacy game mechanic where players must source quest items from adventures or flea markets rather than simply buying them from town shops.

---

## 5. Purchase Routing and Depot Auto-Transfer

Legacy Party2 routes purchased goods based on current inventory occupancy and quantity:

1. **Direct Inventory Delivery**:
   - Single item purchase ($\text{quantity} = 1$).
   - The relevant equipment/item slot is currently **empty**:
     - MainHand slot for weapons
     - Body slot for armors
     - First empty consumable item slot for items
   - The purchased item is placed directly in character inventory and recorded in item collection (`RecordItemCollection`).
   - Shopkeeper delivers normal delivery message.

2. **Auto-Transfer to Depot**:
   - The character's target inventory slot is **already occupied**, OR purchase quantity is **greater than 1** ($\text{quantity} > 1$).
   - The purchased item(s) are automatically diverted to `character_depots`.
   - The depot capacity is calculated via `depot.CalculateCapacity(job_lv, ex_depot, over_depot)`.
   - If the depot has insufficient remaining capacity, the purchase fails with `ErrDepotFull` and the entire transaction rolls back.
   - Shopkeeper delivers depot delivery message informing the player that goods have been sent to their depot.

3. **Batch Purchase (`handleShopBatchPurchase` / `まとめて買う`)**:
   - Allows bulk purchasing of multiple catalog items at once, matching legacy `weapon.cgi:116`, `armor.cgi:115`, and `item.cgi:137`.
   - **Supported Shop Types**: Only `weapon`, `armor`, and `item` shops offer batch purchasing. Unsupported shops (e.g. `accessory`, `secret`) reject batch purchases with `ErrInvalidShopType` (HTTP 400).
   - **Sales Catalog & Level Validation**: Each requested item is strictly validated against the shop's active sales catalog for the character's JobLevel (`GetSalesItemIDs(shopType, job_lv)`). Any item not currently sold or unearned is rejected with `ErrItemNotFound` (HTTP 404).
   - **Helper Quest Exclusion**: Any item actively requested by a helper quest is rejected with `ErrItemUnavailable` (HTTP 422).
   - **Pricing**: Each item's price is calculated using the shop-specific retail multiplier via unified `CalculateRetailPrice(shopType, itemID, basePrice)`.
   - **Depot Routing**: All batch purchased items route directly to the depot atomically. Total capacity is verified prior to transaction commit.

---

## 6. NPC Interactions and Secret Shop Discovery

### 6.1. NPC Dialogue (`/shop/{type}/talk`)
- Each NPC responds with advice explaining mechanics (e.g. weight reducing agility, batch purchasing sending to depot).

### 6.2. NPC Inspect (`/shop/{type}/inspect`)
- Inspecting weapon or armor NPCs yields defensive or humorous remarks.
- Inspecting Itemko (`アイテムコ`) returns:
  - Dialogue: *"ほえ？なんでしょうかぁ？"*
  - Secret Shop Hint: *"＠ひみつのみせ に行きたい"*

### 6.3. Secret Shop Discovery (`/shop/discover-secret`)
- Requires `job_lv >= 7`.
- Unlocks the secret shop location, giving access to rare items and puff-puff services.

---

## 7. Secret Underground Shop & NPC @ヒミツジ (`secret.cgi` / `item.cgi:himitsunomise`)

The Secret Underground Shop (`internal/secretshop`) is an exclusive hidden facility managed by the mysterious talking sheep NPC `@ヒミツジ` (Himitsuji).

### 7.1. Discovery & Access Qualification
- **Requirement**: `JobLevel >= 7` (total job change count $\ge 7$).
- **Access Control**: Characters who do not meet this requirement receive `ErrAccessDenied` (HTTP 403 Forbidden).

### 7.2. Rare Goods Catalog (3x Base Price)
The secret shop stocks the 8 authentic rare items specified in legacy `secret.cgi:sales`:

| Item ID | Definition ID | Name | Category | Base Price | Secret Price (3x) | Description |
| :--- | :--- | :--- | :--- | :--- | :--- | :--- |
| `secret_item_herbal_root` | `item-010` | 薬草の根っこ | Consumable | 250 G | **750 G** | 大地の生命力を宿した薬草の根。戦闘中に仲間の傷や状態異常を治療する。 |
| `secret_item_magic_mirror` | `item-015` | 魔法の鏡 | Consumable | 300 G | **900 G** | 魔法の壁を展開し、敵の呪文を跳ね返す神秘の鏡。 |
| `secret_item_ruby_of_protection` | `item-080` | 守りのルビー | Consumable | 500 G | **1,500 G** | 魔法の光で仲間を包み込み、受ける魔法ダメージを軽減する宝石。 |
| `secret_item_silver_harp` | `item-078` | 銀のたてごと | Consumable | 1,400 G | **4,200 G** | 美しい音色を奏でて魔物を呼び寄せる銀製の竪琴。 |
| `secret_item_staff_of_change` | `item-043` | へんげの杖 | Consumable | 1,000 G | **3,000 G** | 使用者の姿をモンスターの姿に変貌させる不思議な杖。 |
| `secret_item_philosophers_enlightenment` | `item-027` | 賢者の悟り | Consumable | 10,000 G | **30,000 G** | 深遠なる知識と悟りを開いた証。上位職への転職条件を満たす秘宝。 |
| `secret_item_spirits_ward` | `item-030` | 精霊の守り | Consumable | 5,000 G | **15,000 G** | 精霊たちの強大な加護が宿るお守り。上位職への転職条件を満たす秘宝。 |
| `secret_item_counts_blood` | `item-031` | 伯爵の血 | Consumable | 5,000 G | **15,000 G** | 高貴なる闇の血脈を宿す秘薬。上位職への転職条件を満たす秘宝。 |

- **Helper Quest Exclusion**: Active helper quest targets are filtered out of the catalog and cannot be purchased (`ErrItemUnavailableInHelperQuest`).
- **Delivery & Collection Discovery**: If the character's consumable inventory slot is empty and purchase quantity is 1, the item enters character inventory (`inventory_items`) and is automatically registered in the item collection via `collection.Recorder.RecordItemDiscovered` (matching legacy `secret.cgi:44-46` `&add_collection`). If the slot is occupied or quantity > 1, the item routes to Depot (`character_depots`) via `depot.FindOrCreate` without triggering collection discovery.

### 7.3. NPC Interactions & Puff-Puff Service
- **Talk (`POST /characters/{id}/secretshop/talk`)**: Sheep dialogue hints (*"値段は高いメェ〜けれど、他では手に入らないレアものだメェ〜"*).
- **Inspect (`POST /characters/{id}/secretshop/inspect`)**: Lore background of Himitsuji.
- **Puff-Puff (`POST /characters/{id}/secretshop/puffpuff`)**: Authentic humorous interaction (*"パフパフ♥ パフパフ♥ パフパフ♥"*). No HP/MP healing or stat changes.

---

## 8. Commerce & Trading System Boundaries

Party2 features multiple commercial facilities with distinct roles and storage models:

| Facility | Module | Type | Primary Role & Storage Model | Canonical Document |
| :--- | :--- | :--- | :--- | :--- |
| **Town Shops** | `internal/shop` | NPC Merchant | Standard equipment & item retail (2x base) / resale (50%) | This document (`shops.md`) |
| **Secret Shop** | `internal/secretshop` | NPC Secret | JobLv 7 gate, 8 rare items at 3x price, puff-puff | This document (`shops.md`) |
| **Player Stores** | `internal/store` | Player Real-Estate | 50kG town boutiques, 26 wallpapers, 15 furnitures, depot barter/sales | [`store.md`](store.md) |
| **Flea Market** | `internal/fleamarket` | Player Stalls | Fixed-price P2P stalls (120 server ceiling), depot delivery | [`fleamarket.md`](fleamarket.md) |
| **Auction House** | `internal/auction` | P2P Trade Hall | Live player-to-player item sending (`@おくる`/`@しらべる`) | [`auction.md`](auction.md) |
| **Black Market** | `internal/blackmarket` | Recycling Barter | Rare item sacrifice for Rare Points, 24 item/equipment exchanges | [`black-market.md`](black-market.md) |
| **Tavern Delivery** | `internal/tavern` | Recurring Dining | Pre-ordered post-adventure meal reservations (0G upfront) | [`tavern.md`](tavern.md) |

---

## 9. Concurrency and Lock Sequence

All financial and inventory operations execute in strict lock hierarchy:
1. `characters` (Tier 2, `SELECT ... FOR UPDATE`)
2. `inventory_items` (Tier 3, `SELECT ... FOR UPDATE`)
3. `character_depots` (Tier 5, `SELECT ... FOR UPDATE`)

Transactions guarantee that concurrent purchases never cause negative wallet balances, never exceed depot limits, and never drop items.


