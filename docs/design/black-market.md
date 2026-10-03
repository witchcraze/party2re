# Town Black Market, Rare Point Barter & NPC @闇商人 Design

## Overview

The Black Market module (`internal/blackmarket`) is a clean-room reconstruction of the underground black market barter system matching the original Party2 Perl CGI specification (`party2/lib/black_market.cgi`), located at `闇市場` and operated by NPC `@闇商人`.

In authentic Party2, the Black Market is not a gold shop. It operates purely as a barter exchange where characters sacrifice rare items in exchange for Rare Points and U-Rare Points, and redeem those points for exclusive equipment and artifacts that are sent directly to the character's Depot (`預かり所`).

---

## Domain Rules & Systems

### 1. Eligibility & Access Control

- **Unconditional Access**: In legacy Party2, any character may enter `闇市場` without level or progression restrictions (`CheckEligibility` returns true).
- **Authentication**: All endpoints require character ownership authentication.

---

### 2. Rare Point & U-Rare Point Sacrifice System (`SacrificeItem` / `@ささげる`)

- **Dual-Source Sacrifice**: Characters can sacrifice eligible rare weapons, armor, or items directly from either active character inventory or Depot (`預かり所`).
- **Sacrifice Eligibility & Yields**:
  - **46 Authentic Eligible Items**: Defined across regular weapons, armors, accessories, and consumables.
  - **Regular Rare Items**: Yields **+1 Rare Point** (e.g. `weapon-29`〜`weapon-40`, `armor-35`〜`armor-40`, `item-028`〜`item-109`).
  - **Ultra-Rare Artifacts**: Yields **+1 to +50 U-Rare Points** (e.g. `item-263`〜`item-268`).
  - **Ineligible Items**: Non-rare items are rejected (`ErrNotSacrificeEligible` / HTTP 400).
- **Transaction & Locking**:
  - Pessimistic lock ordering: Tier 2 `characters` -> Tier 3 `inventory_items` -> Tier 5 `character_depots` -> Tier 8 `blackmarket_character_points`.
  - Atomically consumes the item from inventory or depot and credits points to `blackmarket_character_points`.

---

### 3. Exclusive Prize Trade Exchange (`TradePrize` / `@とりひき`)

- **Prize Redemption**: Accumulated points can be exchanged for exclusive weapons, armor, accessories, and consumables across two catalogs:
  - **12 Regular Rare Prizes**:
    - `bm_prize_087` (まほうのそろばん, `item-087`): 1 Rare Point
    - `bm_prize_057` (ほのおのツメ, `item-057`): 4 Rare Points
    - `bm_prize_060` (氷の盾, `item-060`): 2 Rare Points
    - `bm_prize_061` (炎の盾, `item-061`): 2 Rare Points
    - `bm_prize_062` (風神の盾, `item-062`): 2 Rare Points
    - `bm_prize_063` (ドラゴンシールド, `item-063`): 2 Rare Points
    - `bm_prize_064` (水鏡の盾, `item-064`): 2 Rare Points
    - `bm_prize_065` (オーガシールド, `item-065`): 2 Rare Points
    - `bm_prize_237` (はやてのリング, `item-237`): 2 Rare Points
    - `bm_prize_216` (ちからのゆびわ, `item-216`): 3 Rare Points
    - `bm_prize_243` (いのりのゆびわ, `item-243`): 3 Rare Points
    - `bm_prize_207` (メタルキングの剣, `item-207`): 10 Rare Points
  - **12 Ultra-Rare Prizes**:
    - `bm_uprize_059` (きせきのつるぎ, `item-059`): 5 U-Rare Points
    - `bm_uprize_199` (ふしぎなボレロ, `item-199`): 5 U-Rare Points
    - `bm_uprize_246` (しあわせのくつ, `item-246`): 5 U-Rare Points
    - `bm_uprize_185` (はかいのつるぎ, `item-185`): 10 U-Rare Points
    - `bm_uprize_194` (あくまのよろい, `item-194`): 10 U-Rare Points
    - `bm_uprize_195` (しにがみのたて, `item-195`): 10 U-Rare Points
    - `bm_uprize_258` (ほしふるうでわ, `item-258`): 10 U-Rare Points
    - `bm_uprize_259` (ごうけつのうでわ, `item-259`): 10 U-Rare Points
    - `bm_uprize_255` (おうごんのティアラ, `item-255`): 15 U-Rare Points
    - `bm_uprize_210` (メタルキングの鎧, `item-210`): 15 U-Rare Points
    - `bm_uprize_204` (メタルキングの盾, `item-204`): 15 U-Rare Points
    - `bm_uprize_262` (やまびこのぼうし, `item-262`): 20 U-Rare Points
- **Depot Delivery & Capacity Verification**:
  - Prizes are delivered directly into the character's Depot (`預かり所`), NOT character inventory.
  - Depot capacity is verified via `depot.CalculateCapacity(char.JobLevel, dep.ExDepot, char.OverDepot)`. If depot capacity is exceeded, the trade is rejected with `ErrDepotFull`.
  - Transaction lock sequence: Tier 2 `characters` -> Tier 5 `character_depots` -> Tier 8 `blackmarket_character_points`.

---

### 4. Authentic Underworld NPC `@闇商人` Dialogue

- **Presentation Decoupling**: In adherence to `.agents/rules/03-architecture.md §5`, domain service results return pure structured data (`CharacterID`, `NPCName`), while human-facing dialogue lines are formatted at the HTTP transport/presentation layer (`internal/api/http`).
- **Atmospheric Talk (`Talk`)**: Formats authentic atmospheric underworld quotes matching the legacy CGI:
  - `"よく来たな…。ここは闇市場だ…"`
  - `"表の世界では手に入れられない物を取引している…"`
  - `"物の取引は金では買えないもの…。つまり、魂…ゴホッゴホッ…ではなく、レアアイテムだ…"`
  - `"お前の魂…ではなく、お前が装備しているレアアイテムをささげろ…"`
  - `"レアアイテムをささげることによって…お前のレアポイントが増える…"`
  - `"レアポイントにより取引できるアイテムが違う…"`
- **Inspection (`Inspect`)**: Formats legacy CGI inspection quote:
  - `"…お前の魂で取引したいのか？"`

---

## Persistence Architecture

### MariaDB Schemas

```sql
-- Rare Point Sacrifice & Prize Trade Points (042_blackmarket_sacrifice_and_trade.sql)
CREATE TABLE IF NOT EXISTS blackmarket_character_points (
    character_id CHAR(32) NOT NULL PRIMARY KEY,
    rare_points INT NOT NULL DEFAULT 0,
    u_rare_points INT NOT NULL DEFAULT 0,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    CONSTRAINT fk_blackmarket_points_character FOREIGN KEY (character_id) REFERENCES characters (id) ON DELETE CASCADE
);

-- Fictional tables dropped in 065_drop_blackmarket_fictional_tables.sql:
-- DROP TABLE IF EXISTS blackmarket_character_purchases;
-- DROP TABLE IF EXISTS blackmarket_market_state;
```
