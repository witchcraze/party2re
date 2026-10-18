# Lottery & Raffle Design (Takarakuji & Fukubiki)

## Overview

The Lottery and Raffle Feature Module (`internal/lottery`) implements the Party2 town games with the reconstruction decisions documented below:
1. **Server-Wide 20-Cap Rare Item Lottery (宝くじ / Takarakuji)**: Periodic limited-entry lottery (`party2/lib/takarakuzi.cgi`, NPC `@クラゲ`) featuring 20 tickets per round, 1 ticket per player, 10-day drawing cycles, and direct delivery of rare equipment and alchemy recipes into the winner's Depot (`預かり所`).
2. **Raffle (福引 / Fukubiki)**: Instant tavern coupon drawing facility (`party2/lib/lot.cgi`, NPC `@フクスケ`) using meal coupons from the Adventurer's Tavern to draw stat-boosting seeds, weekday secret treasures, and divine orbs.

---

## Fukubiki Raffle (福引所 / `party2/lib/lot.cgi`)

### 1. Basic Specifications
- **Location**: 福引所 (Raffle Shop)
- **NPC**: `@フクスケ` (Fukusuke)
- **Coupon Source**: Obtained exclusively via Tavern counter meals (`OrderMeal` in `party2/lib/bar.cgi`) and Heaven Wishes (`god`). Fictional direct gold purchase (100G) and gold prize tables are completely purged.
- **Item Delivery Routing**:
  - If the character's consumable item hand slot is empty: delivered directly to the character's Inventory (`transferred_to_depot = false`).
  - If the character's consumable item hand slot is occupied: automatically forwarded to Depot storage (`character_depots` / `depot_items`) (`transferred_to_depot = true`).
  - If Depot storage is full: returns `depot.ErrDepotFull` (HTTP 409 Conflict), rolling back ticket consumption to protect player assets.
- **Automatic Mode Selection & Transactional Boundary**:
  - In parity with legacy Party2 (`party2/lib/lot.cgi`), players execute a single raffle draw action without selecting a tier.
  - Mode selection and ticket consumption occur atomically inside the transaction under Character pessimistic locking (Rank 2):
    - Characters holding **300 or more coupons** (`>= SpecialRaffleCost`): automatically play Special Raffle (裏・特別福引) consuming 300 coupons.
    - Characters holding **fewer than 300 coupons** (`< SpecialRaffleCost`): automatically play Standard Raffle (通常福引) consuming 3 coupons (or reject with `ErrInsufficientTickets` if `< StandardRaffleCost`).
  - Serializing within the Rank 2 Character transaction boundary ensures that concurrent requests evaluate and deduct based on the same committed balance, preventing stale balance mode evaluation, overspending, or double-spending.
  - On the HTTP endpoint (`POST /characters/{id}/lottery/raffle`), the request body is optional; `raffle_type` is deprecated and ignored for mode determination.

### 2. Standard Raffle (通常福引)
- **Cost**: 3 coupons (`StandardRaffleCost = 3`)
- **Roll Range**: 0 to 999 (`rand(1000)`)
- **Prize Tiers & Probabilities**:
  - **特賞 (Grand Prize / Gold / 0.1%)**: Day-of-week secret treasure (`$g_prizes[$wday]` in JST):
    - Sunday (0): `item-027` (賢者の悟り)
    - Monday (1): `item-035` (ドラゴンの心)
    - Tuesday (2): `item-036` (闇のロザリオ)
    - Wednesday (3): `item-088` (魔銃)
    - Thursday (4): `item-037` (走鳥の野菜)
    - Friday (5): `item-038` (幻獣の実)
    - Saturday (6): `item-039` (ギャンブルハート)
  - **1等 (1st Prize / Red / 0.3%)**: `item-030` (精霊の守り, rolls 1..3)
  - **2等 (2nd Prize / Purple / 0.4%)**: `item-033` (スライムの心, rolls 4..7)
  - **3等 (3rd Prize / Yellow / 0.6%)**: `item-023` (小さなメダル, rolls 8..13)
  - **4等 (4th Prize / Pink / 3.1%)**: Stat Seeds (rolls 14..44):
    - `item-016` (命の木の実, 0.6%, rolls 14..19)
    - `item-017` (不思議な木の実, 0.5%, rolls 20..24)
    - `item-018` (力の種, 0.5%, rolls 25..29)
    - `item-019` (守りの種, 0.5%, rolls 30..34)
    - `item-020` (素早さの種, 0.5%, rolls 35..39)
    - `item-021` (スキルの種, 0.5%, rolls 40..44)
  - **5等 (5th Prize / Blue / 1.0%)**: `item-012` (祈りの指輪, rolls 45..54)
  - **6等 (6th Prize / Green / 2.0%)**: `item-125` (福袋, rolls 55..74)
  - **ハズレ (Miss / White / 92.5%)**: None (rolls 75..999)

### 3. Special Raffle (裏・特別福引)
- **Cost**: 300 coupons (`SpecialRaffleCost = 300`)
- **Requirement**: Character must hold at least 300 coupons.
- **Roll Range**: 0 to 99 (`rand(100)`)
- **Prize Tiers & Probabilities**:
  - **特賞 (Grand Prize / Gold / 3.0%)**: Random rare alchemy material item (rolls 0..2):
    - Candidates: `item-090` (スライムピアス), `item-091` (飛竜のヒゲ), `item-092` (禁断の書), `item-093` (コウモリの羽), `item-094` (マジックマッシュルーム), `item-095` (透明マント), `item-096` (獣の血), `item-097` (死者の骨), `item-098` (謎の液体), `item-099` (ヒーローソード), `item-100` (ヒーローソード2), `item-142` (蝶の翅)
  - **1等 (1st Prize / Silver / 12.0%)**: `item-060` (シルバーオーブ, rolls 3..14)
  - **2等 (2nd Prize / Red / 15.0%)**: `item-061` (レッドオーブ, rolls 15..29)
  - **3等 (3rd Prize / Blue / 10.0%)**: `item-062` (ブルーオーブ, rolls 30..39)
  - **4等 (4th Prize / Green / 10.0%)**: `item-063` (グリーンオーブ, rolls 40..49)
  - **5等 (5th Prize / Yellow / 10.0%)**: `item-064` (イエローオーブ, rolls 50..59)
  - **6等 (6th Prize / Purple / 10.0%)**: `item-065` (パープルオーブ, rolls 60..69)
  - **ハズレ (Miss / White / 30.0%)**: None (rolls 70..99)

---

## Takarakuji Lottery (宝くじ屋 / `party2/lib/takarakuzi.cgi`)

### 1. Basic Specifications
- **Location**: 宝くじ屋 (Lottery Shop)
- **NPC**: `@クラゲ` (Kurage)
- **Ticket Price**: 30,000 Gold per ticket
- **Server-Wide Cap**: Exactly 20 tickets per drawing round (`TAKARAKUZI_SOLD_OUT = 20`). Concurrency is strictly enforced by row-locking the active round (`takarakuji_rounds`, Rank 0 `FOR UPDATE`) before checking ticket counts and deducting player gold (Rank 2).
- **Player Restriction**: Maximum 1 ticket per character per round (`「おひとりさまおひとつ！」`)
- **Sold Out Handling**: Once 20 tickets are purchased, subsequent purchase attempts are rejected with NPC message `「今回の宝くじは完売したよー」` (HTTP 409 Conflict)

### 2. Drawing Schedule (10-Day Cycle)
Drawings occur automatically on the **1st, 11th, and 21st** of each month at 00:00:00 JST (`NextDrawDateJST`):
- If purchased on day 1–10: Drawing occurs on the 11th of the current month.
- If purchased on day 11–20: Drawing occurs on the 21st of the current month.
- If purchased on day 21–31: Drawing occurs on the 1st of the following month.

Premature drawing attempts (`now.Before(round.DrawDate)`) are rejected with `ErrNotReadyToDraw`.

On purchase, the NPC informs the player (`party2/lib/takarakuzi.cgi:124`):
`「ありがとー。当たってたら YYYY/MM/DD に賞品が届くからね」`

The domain purchase result returns only state (`Ticket`, `RemainingGold`, `DrawDate`); the dialogue is formatted from `DrawDate` (JST) at the HTTP layer (`npc_message`) per `.agents/rules/03-architecture.md §5`.

### 3. Prize Catalog & Candidate Lineup

**Approved decision ([#1249](https://github.com/witchcraze/party2re/issues/1249), 2026-10-18):** retain the current Go selection across all candidate slots and all remaining participant slots. This intentionally departs from the legacy execution behavior. Duplicate prize entries retain their weights; unique items are **not** equally likely. The rationale is to keep every participant slot and every listed prize reachable, without changing the current Go draw behavior. Restoring the legacy exclusions was considered and rejected. Depot-full behavior remains separately tracked in [#1139](https://github.com/witchcraze/party2re/issues/1139).

Each tier selects one prize item per round. Winner quotas remain 1 for first prize, 1 or 2 with equal probability for second prize, and 2, 3, or 4 with equal probability for third prize (`party2/lib/takarakuzi.cgi:301-303`). These quotas include dummy winners.

The legacy candidate tables (`party2/lib/takarakuzi.cgi:23-53`) have 5/9/9 rows with one-based labels. At lines 297-299, `rand($#list)` initially selects indices 0..3 or 0..7. Lines 308-310 then use the selected row's one-based label as an array index, so the **effective** prize indices are 1..4 or 1..8. Consequently the first prize entry is unreachable, while the last remains reachable. Merely restoring duplicate entries, as described in closed #800, does not reproduce these execution probabilities.

The tables enumerate the complete prize support and weights for each tier, conditional on creating a new lineup. A zero probability denotes an unreachable prize. Go directly selects indices 0..4 or 0..8, with probability 1/5 or 1/9 per candidate slot.

| First prize | Legacy execution | Adopted Go |
| :--- | ---: | ---: |
| `item-129` (神の錬金レシピ) | 0 | 1/5 (20%) |
| `item-266` (奇跡の錬金レシピ) | 1/4 (25%) | 1/5 (20%) |
| `item-265` (聖なる秘石) | 2/4 (50%) | 2/5 (40%) |
| `item-255` (黄金の林檎) | 1/4 (25%) | 1/5 (20%) |

| Second prize | Legacy execution | Adopted Go |
| :--- | ---: | ---: |
| `item-168` | 0 | 1/9 |
| `item-264` | 2/8 (25%) | 2/9 |
| `item-150` | 1/8 (12.5%) | 1/9 |
| `item-173` | 1/8 (12.5%) | 1/9 |
| `item-207` | 1/8 (12.5%) | 1/9 |
| `armor-40` (流銀の鎧) | 1/8 (12.5%) | 1/9 |
| `weapon-40` (流銀の剣) | 1/8 (12.5%) | 1/9 |
| `item-265` (聖なる秘石) | 1/8 (12.5%) | 1/9 |

| Third prize | Legacy execution | Adopted Go |
| :--- | ---: | ---: |
| `item-126` (超魔力水) | 0 | 1/9 |
| `item-244` | 1/8 (12.5%) | 1/9 |
| `item-243` | 1/8 (12.5%) | 1/9 |
| `item-199` | 1/8 (12.5%) | 1/9 |
| `item-253` | 1/8 (12.5%) | 1/9 |
| `item-254` | 1/8 (12.5%) | 1/9 |
| `item-263` | 2/8 (25%) | 2/9 |
| `item-264` | 1/8 (12.5%) | 1/9 |

Lineup inspection (`@しょうひん`) displays the active round's prizes, item names, and winner quotas.

### 4. Drawing Mechanics & Depot Delivery
When a drawing occurs (via background scheduler `takarakuji_draw` or scheduled handler execution):
1. **Transaction Boundary & Premature Check**:
   - The entire drawing process runs inside an atomic database transaction (`RunInTx`).
   - If `now.Before(round.DrawDate)`, drawing immediately aborts with `ErrNotReadyToDraw`.
   - The active round is locked with `SELECT ... FOR UPDATE` (Rank 0).
2. **Pool Construction**:
   - Collects all purchased tickets for the round.
   - If fewer than 20 tickets were sold, dummy entries (`<dummy_0>`, `<dummy_1>`, ...) are added until the pool reaches exactly 20 slots.
3. **Winner Selection**:
   - **Adopted Go:** for each prize rank and its winner count, select uniformly from every remaining slot (indices 0..N−1, probability 1/N each), including the last real or dummy slot.
   - The selected slot is removed from the pool so that no character or dummy can win more than once in the same round.
   - **Legacy execution** (`party2/lib/takarakuzi.cgi:214-245`): `rand($#players)` selects only indices 0..N−2, each with probability 1/(N−1). Removing selected entries preserves the original final slot, which remains unreachable throughout the draw. With 20 sold tickets, the last purchaser cannot win; with fewer than 20, the excluded slot is the last appended dummy, so all real purchasers remain eligible.
   - Conditional on quotas totaling M draws (4..7), an adopted Go slot has total winning probability M/20 and rank probabilities 1/20, amount2/20, amount3/20. Legacy eligible slots instead have M/19 and rank probabilities 1/19, amount2/19, amount3/19; the excluded final slot has zero for every rank. Selection is without replacement in both cases. This exclusion is intentionally not restored under #1249.
4. **Depot-Direct Delivery (`send_item`) & Asset Protection**:
   - Deliveries to real winners are sorted in ascending order of `winnerID` to guarantee deterministic Rank 5 lock ordering and prevent deadlocks.
   - Prize items are instantiated (`coreitem.NewInstance`) and delivered directly into the character's Depot (`character_depots` / `depot_items`).
   - Delivered prizes are recorded into the winner's collection (`CollectionRecorder.RecordItemDiscovered`) categorized by their equipment slot (`weapon` for weapons such as `weapon-40`, `armor` for armors such as `armor-40`, and `item` for standard consumables), ensuring equipment prizes do not count toward item collection completions.
   - If any winner's Depot is at capacity (`depot.ErrDepotFull`), the entire drawing transaction rolls back immediately: no tickets are marked won, the round remains unsettled, and rare items are never dropped or silently lost.
5. **Round Renewal**:
   - Upon successful delivery, the completed round is marked as drawn (`is_drawn = TRUE`, `drawn_at = now`) and tickets are settled with winning ranks and item IDs.
   - The next round is immediately created with newly randomized prizes and the next 10-day draw date.

### 5. NPC Flavor Dialogues (`@words`)
- `「宝くじの三要素！　夢！運！げんじつ！」`
- `「宝くじを当てたいなら、当たるまで買うといいよ！」`
- `「スライムは眼中にありません！」`
- `「大体10日くらいで賞品は変わるよ」`
- `「あと N 人分の宝くじがあるよ」`

---

## Database Persistence

### Schema Migrations (`migrations/018_lottery.sql`, `migrations/081_takarakuji.sql`, `migrations/098_correct_lottery_equipment_collection.sql`)

- `character_lottery`: Tracks character tavern raffle coupons (`raffle_tickets >= 0`).
- `takarakuji_rounds`: Tracks 10-day Takarakuji lottery rounds, prize candidate items, and drawn status.
- `takarakuji_tickets`: Tracks character ticket purchases (1 per round, max 20 per round).
- `migrations/098_correct_lottery_equipment_collection.sql`: Corrects legacy equipment items mistakenly recorded with category `'item'` in `character_item_collection` (Issue #1239).

---

Transport operations and schemas: [OpenAPI](../api/openapi.json).
