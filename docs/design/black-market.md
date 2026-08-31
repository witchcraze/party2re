# Town Black Market, Contraband Trading & Shady Broker @ヤミジ Design

## Overview

The Black Market module (`internal/blackmarket`) is a clean-room reconstruction of the underground black market system (`yami.cgi` -> clean-room `裏路地の闇市`), managed by the shady broker NPC `@ヤミジ` (Yamiji).

The Black Market provides high-level adventurers (Level >= 10) with access to forbidden and contraband items, dynamic price fluctuations influenced by town market conditions, dynamic contraband buyback rates, and underground rumor intelligence.

---

## Domain Rules & Systems

### 1. Eligibility & Access Control

- **Level Requirement**: Characters must be Level 10 or higher to access the Black Market. Attempts by characters under Level 10 result in access denial (`ErrAccessDenied` / HTTP 403 Forbidden).
- **Authentication**: All endpoints require character ownership authentication.

---

### 2. Contraband Catalog (`internal/blackmarket/data/blackmarket_items.json`)

The market offers 10 contraband items spanning weapons, accessories, and consumables with unique traits:

| Item ID | Item Name | Slot | Base Price | Attack | Defense | HP | MP | Daily Limit | Description |
| :--- | :--- | :--- | :--- | :--- | :--- | :--- | :--- | :--- | :--- |
| `bm_poison_needle` | どくばり | weapon | 1,200 G | +5 | +0 | +0 | +0 | 5 | 急所を突けば一撃で仕留められる暗殺用の針。 |
| `bm_assassin_dagger` | 暗殺者の短剣 | weapon | 3,500 G | +38 | +0 | +0 | +0 | 3 | 闇に紛れて致命傷を与える黒塗りのダガー。 |
| `bm_demon_spear` | 魔神の槍 | weapon | 8,800 G | +65 | +0 | +0 | +0 | 2 | 恐るべき破壊力を秘めた禁忌の長槍。 |
| `bm_dark_rosary` | 闇のロザリオ | accessory | 2,800 G | +0 | +15 | +0 | +50 | 3 | 邪悪な加護が宿るとされる黒銀の首飾り。 |
| `bm_skull_ring` | 髑髏の指輪 | accessory | 4,200 G | +10 | +10 | +50 | +0 | 3 | 呪われた力を秘めた髑髏装飾の指輪。 |
| `bm_suicide_bracelet` | 諸刃の腕輪 | accessory | 6,500 G | +25 | -5 | +0 | +0 | 2 | 己の身を削り絶大な攻撃力を引き出す腕輪。 |
| `bm_elven_elixir` | エルフの霊薬 | item | 1,500 G | +0 | +0 | +0 | +300 | 5 | 失われた古代エルフ秘伝の魔力回復薬。 |
| `bm_magic_holy_water` | 魔法の聖水 | item | 600 G | +0 | +0 | +0 | +100 | 10 | 不浄を祓い精神力を回復させる聖なる水。 |
| `bm_sage_stone` | 賢者の石 | item | 12,000 G | +0 | +0 | +150 | +150 | 1 | 触れる者を癒す神秘の宝玉。 |
| `bm_tree_dewdrop` | 世界樹の雫 | item | 5,000 G | +0 | +0 | +999 | +0 | 2 | 瀕死の傷をも全快させる神聖な雫。 |

---

### 3. Dynamic Market Conditions & Multipliers

The market operates under 4 dynamic market conditions reflecting town patrol alertness and underground demand:

1. **`Quiet` (平穏)**:
   - Price Multiplier: `1.00x`
   - Sell Multiplier: `1.00x`
   - Risk Level: `Low`
   - Description: 平穏無事。相場は落ち着いている。
2. **`HotDemand` (需要沸騰)**:
   - Price Multiplier: `1.35x`
   - Sell Multiplier: `1.25x` (Broker pays +25% bonus for buybacks!)
   - Risk Level: `Medium`
   - Description: 需要過多。品薄のため販売価格・買取価格ともに高騰中。
3. **`Crackdown` (警備強化)**:
   - Price Multiplier: `1.75x`
   - Sell Multiplier: `0.70x`
   - Risk Level: `High`
   - Description: 衛兵の取り締まり強化中。仕入れリスクが高く販売価格が高騰、買取相場は下落。
4. **`Bargain` (過剰在庫)**:
   - Price Multiplier: `0.80x`
   - Sell Multiplier: `0.85x`
   - Risk Level: `Low`
   - Description: 在庫処分。闇商人が安値で商品を放出中。

**Pricing Formulas**:
- **Effective Buy Price**: `int(math.Ceil(float64(basePrice) * priceMultiplier))`
- **Effective Sell Payout**: `int(math.Floor(float64(basePrice) * 0.60 * sellMultiplier))`

---

### 4. Purchasing with Daily Quotas (`PurchaseItem`)

- **Pessimistic Locking & Atomic Verification**: Character record and inventory are locked via `SELECT ... FOR UPDATE` inside `RunInTx`.
- **Daily Quota Verification**: Checks that `purchased_today + requested_quantity <= daily_limit`. Exceeding limits returns `ErrDailyLimitExceeded` (HTTP 400).
- **Gold Deduction & Inventory Addition**: Atomically deducts `total_cost` from character gold, adds `coreitem.NewInstance(itemID, quantity)` to character inventory, and updates the daily purchase counter in `blackmarket_character_purchases`.

---

### 5. Contraband Selling & Buyback (`SellItem`)

- **Pessimistic Verification**: Locks character and inventory records inside `RunInTx`.
- **Inventory Check**: Verifies the character owns the item instance with sufficient quantity.
- **Payout Calculation**: Applies dynamic buyback multiplier and awards gold payout to character, removing the consumed item quantity from inventory via `inv.Consume`.

---

### 6. Shady Broker NPC `@ヤミジ` Dialogue & Rumor Intelligence

- **Dialogue (`Talk`)**: Provides randomized shady broker lines reflecting the underworld atmosphere.
- **Rumors (`Rumors`)**: Delivers tactical market insights describing the current dynamic condition and price advice.

---

### 7. Rare Point & U-Rare Point Sacrifice System (`SacrificeItem` / `@ささげる`)

- **Sacrifice Eligibility**: Characters can sacrifice eligible rare weapons, armors, and consumables to earn prestige trade points:
  - **Regular Rare Items** (e.g. `weapon-29`〜`weapon-40`, `armor-35`〜`armor-40`, `item-028`〜`item-109`): Yields **+1 Rare Point**.
  - **Ultra-Rare Artifacts** (e.g. `item-263`〜`item-268`): Yields **+1 to +50 U-Rare Points**.
  - **Ineligible Items**: Non-rare items are rejected (`ErrNotSacrificeEligible` / HTTP 400).
- **Atomic Consumption**: Deducts 1 unit of the item instance from inventory (`inv.Consume`) and atomically increments points in `blackmarket_character_points` inside a pessimistic transaction boundary (`RunInTx`).

---

### 8. Exclusive Prize Trade Exchange (`TradePrize` / `@とりひき`)

- **Prize Redemption**: Accumulated points can be exchanged for exclusive weapons, armor, accessories, and consumables in two catalogs:
  - **Regular Rare Prizes** (e.g. `まほうのそろばん`, `ほのおのツメ`, `氷/炎/風神/ドラゴン/水鏡/オーガの盾`, `ちから/はやて/いのりの指輪`, `メタルキングの剣`): Costs **1 to 10 Rare Points**.
  - **Ultra-Rare Prizes** (e.g. `きせきのつるぎ`, `ふしぎなボレロ`, `しあわせのくつ`, `はかいのつるぎ`, `あくまのよろい`, `しにがみのたて`, `ほしふる/ごうけつのうでわ`, `おうごんのティアラ`, `メタルキングの鎧/盾`, `やまびこのぼうし`): Costs **5 to 20 U-Rare Points**.
- **Point Verification & Delivery**: Verifies sufficient point balance, deducts cost, and creates/deposits the prize item instance directly into the character's inventory inside the transaction boundary.

---

## Persistence Architecture

### MariaDB Schemas

```sql
-- Contraband Quotas and State (040_blackmarket.sql)
CREATE TABLE IF NOT EXISTS blackmarket_character_purchases (
    character_id VARCHAR(64) NOT NULL,
    item_id VARCHAR(64) NOT NULL,
    purchase_date DATE NOT NULL,
    quantity INT NOT NULL DEFAULT 0,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (character_id, item_id, purchase_date),
    INDEX idx_char_date (character_id, purchase_date)
);

CREATE TABLE IF NOT EXISTS blackmarket_market_state (
    id INT PRIMARY KEY DEFAULT 1,
    condition_name VARCHAR(32) NOT NULL DEFAULT 'Quiet',
    price_multiplier DECIMAL(5,2) NOT NULL DEFAULT 1.00,
    sell_multiplier DECIMAL(5,2) NOT NULL DEFAULT 1.00,
    risk_level VARCHAR(16) NOT NULL DEFAULT 'Low',
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
);

-- Rare Point Sacrifice & Prize Trade Points (042_blackmarket_sacrifice_and_trade.sql)
CREATE TABLE IF NOT EXISTS blackmarket_character_points (
    character_id CHAR(32) NOT NULL PRIMARY KEY,
    rare_points INT NOT NULL DEFAULT 0,
    u_rare_points INT NOT NULL DEFAULT 0,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    CONSTRAINT fk_blackmarket_points_character FOREIGN KEY (character_id) REFERENCES characters (id) ON DELETE CASCADE
);
```
