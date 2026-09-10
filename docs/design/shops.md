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

3. **Batch Purchase (`handleShopBatchPurchase`)**:
   - Allows bulk purchasing of multiple catalog items at once.
   - All batch purchased items route directly to the depot.
   - Total capacity is verified up-front before database locking.

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

## 7. Concurrency and Lock Sequence

All financial and inventory operations execute in strict lock hierarchy:
1. `characters` (Tier 2, `SELECT ... FOR UPDATE`)
2. `inventory_items` (Tier 3, `SELECT ... FOR UPDATE`)
3. `character_depots` (Tier 5, `SELECT ... FOR UPDATE`)

Transactions guarantee that concurrent purchases never cause negative wallet balances, never exceed depot limits, and never drop items.

