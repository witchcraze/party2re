# Standard Shops Design and Parity Specification

## Purpose

This document records the legacy rules and current implementation boundaries for
the three standard town shops (Weapon Shop, Armor Shop, Item Shop). It is not a
certification of complete parity; the observation and deferred differences below
remain distinct from the original CGI specification.

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

Available items use `job_lv` (job-change count), represented by `Character.JobLevel`.

### 3.1. Weapon Shop (`weapon.cgi`)
- **`job_lv <= 11`**:
  `weapon-01` .. `weapon-05`, `weapon-43`, and exactly `weapon-(06 + job_lv)`.
- **`job_lv > 11`**:
  `weapon-01` .. `weapon-05`, `weapon-43`, and `weapon-06` .. `weapon-16`.

### 3.2. Armor Shop (`armor.cgi`)
- **`job_lv <= 11`**: `armor-01` .. `armor-(05 + job_lv)`.
- **`job_lv > 11`**: `armor-01` .. `armor-16`.

### 3.3. Item Shop (`item.cgi`)
- **`job_lv == 0`** (Base):
  `item-001` (薬草 / Herb), `item-007` (毒消し草 / Antidote Herb), `item-008` (満月草 / Moon Herb), `item-009` (天使の鈴 / Angel Bell), `item-127` (思い出の鈴 / Memory Bell)
- **`job_lv >= 1`**:
  Adds `item-002` (上薬草 / High Herb), `item-011` (魔法の聖水 / Magic Water), `item-014` (守りの石 / Protection Stone), `item-079` (くもの糸 / Spider Thread)
- **`job_lv >= 3`**:
  Adds `item-041` (べじたりあん / Vegetarian Dish), `item-042` (毒りんご / Poison Apple)
- **`job_lv >= 5`**:
  Adds `item-003` (特薬草 / Special Herb), `item-076` (妖精の粉 / Fairy Powder), `item-101` (魔法のじゅうたん / Magic Carpet)
- **`job_lv >= 7`**:
  Adds `item-102` (パフパフ / Puff-Puff Scroll)

---

## 4. Helper Quest Exclusion

Items requested by helper quests are excluded from sale. Legacy
`system.cgi:1424–1438` reads the shared quest file and filters by item kind;
current `GetCatalog` uses the helper service's active definition IDs, across
characters. Required helper/catalog reads and price calculation errors fail the
whole catalog. Purchase validation has a separate error-handling gap described
below; a readable catalog is not an execution guarantee.

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

Current routes and payloads are maintained in [shop OpenAPI](../api/paths/shop.json); dialogue capabilities below do not imply separate legacy-style URL paths.

### 6.1. NPC Dialogue
- Each NPC responds with advice explaining mechanics (e.g. weight reducing agility, batch purchasing sending to depot).

### 6.2. NPC Inspect
- **Presentation Decoupling**: In adherence to `.agents/rules/03-architecture.md §5`, domain service results return pure structured data (`ShopType`, `NPCName`, `SecretShopHint`), while human-facing dialogue lines are formatted at the HTTP transport/presentation layer (`internal/api/http`).
- Inspecting weapon or armor NPCs yields defensive or humorous remarks:
  - Weapon (`ブッキー`): *"おいおい、俺は武器じゃねぇぜ"*
  - Armor (`アマノ`): *"な、な、何を見ているッスか！？！"*
  - Accessory (`ミラ`): *"なにか私についてる？"*
- Inspecting Itemko (`アイテムコ`) returns:
  - Dialogue: *"ほえ？なんでしょうかぁ？"*
  - Secret Shop Hint: *"＠ひみつのみせ に行きたい"*

### 6.3. Secret Shop Discovery
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
- **Purchase Dialogue**: The presentation layer formats the purchase acknowledgement from the item name and delivery destination: `<item>メェ〜。持ってけメェ〜` for inventory, or `<item>は<character>メェ〜の預かり所の方に投げましたメェ〜` for depot delivery (legacy `secret.cgi:40-44`). Domain purchase results contain structured purchase facts only.

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

## 10. Selected Shop observation and migration boundary

The shared [navigation contract](../architecture/client-agent-api.md#selection-commands-and-typed-discovery)
maps `shop_weapon`, `shop_armor`, `shop_item`, and `shop_accessory` directly to
existing `ShopType` values. Accessory products remain `item-NNN` catalog entries,
including their existing slot classification; no new accessory catalog is created.
Authenticated context GET and post-command refresh use the same HTTP adapter.

Facility data has one primary collection, `items`, sorted by definition ID and
paged with offsets (default 20, maximum 100). Each row preserves GetCatalog's
ID, name, base/retail price and optional slot, supplies `select_params`, and
supplies an explicit `purchase_params.item_definition_id`. Subject data contains
only that selected `product`, alongside common shop/NPC facts and quantity
bounds 1–9999 from the existing service. These are current Go quantity bounds;
legacy single purchase bought one item, while batch input named multiple items.
Catalog membership does not promise sufficient funds or inventory/depot capacity.

Observations include structured InspectNPC facts and reuse HTTP inspect dialogue.
They never invoke random TalkNPC, secret discovery, purchase, sell or synthesis.
Missing or newly excluded products become `selection_unavailable` without a
saved fallback. Required reads yield no partial observation; known navigation
outcomes survive refresh failure and recover with GET alone. No keys, tables,
wallet gates, gameplay formulas or mutation adapters are added. Purchase commands
remain disclosed as unconnected support, never executable choices.

### Scoped legacy action/call-path reconciliation

Paths below are relative to the original `party2` distribution. Shop files are
loaded through `party.cgi:14–30`; `system.cgi:9–24` registers shared controls and
`system.cgi:639–655` dispatches action/target input with the action-time guard.
The complete shop additions and read/mutation branches reconcile as follows:

| Legacy dispatch / call path | Current representation and remaining owner |
|---|---|
| Weapon `かう` → `kau` (`weapon.cgi:45–85`), Armor (`armor.cgi:41–85`), Item (`item.cgi:53–107`), Accessory (`accessory.cgi:98–147`) | No-target sales tables → GetCatalog and selected Shop catalog/product adapter. Explicit-target purchase → Purchase/PurchaseInShop and retained REST, Gateway mutation migration #947. |
| Weapon/Armor/Item `うる` → `uru` (`weapon.cgi:91–110`, `armor.cgi:89–110`, `item.cgi:112–132`); Accessory (`accessory.cgi:151–170`) | Held-slot preview/sale → Sell and retained REST. Owned sale-choice projections and Gateway commands remain #947. |
| Weapon/Armor/Item `まとめてかう` → `matomete_kau` (`weapon.cgi:116–173`, `armor.cgi:115–178`, `item.cgi:137–197`) | No-target catalog → the same selected observation. Multi-name purchase/depot delivery → BatchPurchase; Gateway migration #947. Not registered for Accessory. |
| Item hidden `ひみつのみせ` → `himitsunomise` (`item.cgi:57–69`) | Inspect hint is observed. DiscoverSecretShop reports JobLevel ≥7; legacy also changes location to `secret`. Secret entry/command coverage remains #947. |
| Accessory `ごうせい` → `acce`, `check_depot`, `get_item_no` (`accessory.cgi:100–101,172–292`) | Recipe preview → AllSynthesisRecipes/REST; Synthesize handles material consumption/output. Recipe scene and mutation migration remain #947; held-elixir gap remains in [Accessory design](accessory-shop.md). `get_depot` is an uncalled utility, not an omitted action. |
| Shared `しらべる` → `shiraberu` → `shiraberu_npc` (`system.cgi:322–329,413`; `weapon.cgi:38–40`, `armor.cgi:34–36`, `item.cgi:45–48`) | InspectNPC structured facts + HTTP presentation observed. Accessory inherits the default “nothing found” in legacy, while Go supplies its own custom line; this existing difference remains #947. Non-NPC inspection remains social migration #949. |
| Shared `はなす` → `hanasu` (`system.cgi:240–263`) using facility `@words` | TalkNPC/REST retain random dialogue. Go's abbreviated static lines omit legacy dynamic recommendations/status and contain wording differences; no exact-dialogue parity claim. Gateway talk/log/presence migration remains #947/#949. |
| Shared `br`, `いどう`, `まち`, `ほーむ`, conditional `ぎるど`, `ささやき`, `ろぐあうと`, `すくしょ` (`system.cgi:9–24`) | Full mapping and deferred ownership are in the [shared reconciliation](../architecture/client-agent-api.md#scoped-legacy-navigation-reconciliation). Separator/auth redirects are superseded; movement uses scene controls; remaining social/presence/photo work belongs to #949. |
| Purchase delivery → `send_item` (`system.cgi:818–837`) / `_add_collection.cgi`; helper filter → `get_helper_item` (`system.cgi:1424–1438`) | Existing inventory/depot/collection services and helper read port. Observation calls no delivery or collection write. Mutation reconciliation remains #947. |

Legacy weapon/armor sales also display nominal strength and weight. GetCatalog
currently projects prices and slot only; extending item facts remains #947.
Standard Purchase/PurchaseInShop does not enforce sales-tier membership as batch
and accessory purchase do, and `isItemInActiveHelper` treats helper-read failure
as no exclusion. These are existing mutation gaps owned by #947, not behavior
introduced or certified by this observation slice. Full formula/asset parity and
final handler retirement remain outside this scope.
Legacy armor batch uses helper kind 1 (`armor.cgi:120`) while single armor
purchase uses kind 2 (`armor.cgi:52`). Go consistently filters armor definition
IDs; this legacy inconsistency requires separate reconciliation under #947.

### Retained routes

All routes below retain their resolver/catalog references. A replacement read
alone does not establish per-command schema independence or authorize retirement.

| Method / path | Reason retained (owner #947) |
|---|---|
| GET `/characters/{id}/shop/{type}` | Unpaged REST catalog readers still have resolver clients; reader/resolver cleanup requires verified migration. |
| POST `/shop/purchase` | Existing standard purchase; no Gateway mutation adapter/schema replacement. |
| POST `/shop/sell` | Existing sale; held-item preview and Gateway replacement remain. |
| POST `/characters/{id}/shop/batch-purchase` | Existing atomic batch/depot operation; no Gateway replacement. |
| POST `/characters/{id}/shop/{type}/inspect` | Typed scene reuses facts, but direct inspect/resolver migration remains. |
| POST `/characters/{id}/shop/{type}/talk` | Random NPC interaction/log reconciliation is not performed by observation. |
| POST `/characters/{id}/shop/discover-secret` | Secret discovery/entry replacement remains. |
| POST `/characters/{id}/shop/accessory/buy` | Accessory pricing/tier mutation has no Gateway replacement. |
| POST `/characters/{id}/shop/accessory/sell` | Accessory sale has no Gateway replacement. |
| POST `/characters/{id}/shop/accessory/synthesize` | Material/elixir/output mutation has no Gateway replacement. |
| GET `/characters/{id}/shop/accessory/recipes` | Recipe primary collection is not part of the selected product scene. |
