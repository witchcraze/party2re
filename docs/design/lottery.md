# Lottery & Raffle Design (Takarakuji & Fukubiki)

## Overview

The Lottery and Raffle Feature Module (`internal/lottery`) implements the authentic Party2 town games:
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

### 2. Standard Raffle (通常福引)
- **Cost**: 3 coupons (`StandardRaffleCost = 3`)
- **Roll Range**: 0 to 999 (`rand(1000)`)
- **Prize Tiers & Probabilities**:
  - **特賞 (Grand Prize / Gold / 0.1%)**: Day-of-week secret treasure (`$g_prizes[$wday]` in JST):
    - Sunday (0): `item-027` (賢者の悟り)
    - Monday (1): `item-035` (ドラゴンの心)
    - Tuesday (2): `item-036` (闇のロザリオ)
    - Wednesday (3): `item-088` (魔銃)
    - Thursday (4): `item-037` (ギザールの野菜)
    - Friday (5): `item-038` (クポの実)
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
- **Server-Wide Cap**: Exactly 20 tickets per drawing round (`TAKARAKUZI_SOLD_OUT = 20`)
- **Player Restriction**: Maximum 1 ticket per character per round (`「おひとりさまおひとつ！」`)
- **Sold Out Handling**: Once 20 tickets are purchased, subsequent purchase attempts are rejected with NPC message `「今回の宝くじは完売したよー」` (HTTP 409 Conflict)

### 2. Drawing Schedule (10-Day Cycle)
Drawings occur automatically on the **1st, 11th, and 21st** of each month at 00:00:00 JST (`NextDrawDateJST`):
- If purchased on day 1–10: Drawing occurs on the 11th of the current month.
- If purchased on day 11–20: Drawing occurs on the 21st of the current month.
- If purchased on day 21–31: Drawing occurs on the 1st of the following month.

On purchase, the NPC informs the player:
`「ありがとー。当たってたら YYYY/MM/DD に賞品が届くからね」`

### 3. Prize Catalog & Candidate Lineup
Each round rolls one distinct prize item for each tier alongside winner counts:

- **1st Prize (1等 / Itto)**: Exactly 1 winner
  - Candidates: `item-129` (神の錬金レシピ), `item-266` (奇跡の錬金レシピ), `item-265` (聖なる秘石), `item-255` (黄金の林檎)
- **2nd Prize (2等 / Nito)**: 1 or 2 winners (`1 + rand(2)`)
  - Candidates: `item-168`, `item-264`, `item-150`, `item-173`, `item-207`, `armor-40` (流銀の鎧), `weapon-40` (流銀の剣), `item-265`
- **3rd Prize (3等 / Santo)**: 2, 3, or 4 winners (`2 + rand(3)`)
  - Candidates: `item-126` (超魔力水), `item-244`, `item-243`, `item-199`, `item-253`, `item-254`, `item-263`, `item-264`

Lineup inspection (`@しょうひん`) displays the active round's prizes, item names, and winner quotas.

### 4. Drawing Mechanics & Depot Delivery
When a drawing occurs (via background scheduler `takarakuji_draw` or on-demand date evaluation):
1. **Pool Construction**:
   - Collects all purchased tickets for the round.
   - If fewer than 20 tickets were sold, dummy entries (`<dummy_0>`, `<dummy_1>`, ...) are added until the pool reaches exactly 20 slots.
2. **Winner Selection**:
   - For each prize rank and its winner count, a random slot is drawn from the pool.
   - The selected slot is removed from the pool so that no character or dummy can win more than once in the same round.
3. **Depot-Direct Delivery (`send_item`)**:
   - If the drawn slot belongs to a real character, the prize item is instantiated (`coreitem.NewInstance`) and delivered directly into the character's Depot (`character_depots` / `depot_items`).
   - The winning ticket is marked with `won_rank` and `won_item_id`.
   - If the winner's Depot is at capacity, the error is safely recorded without corrupting other winners.
4. **Round Renewal**:
   - The completed round is marked as drawn (`is_drawn = TRUE`, `drawn_at = now`).
   - The next round is immediately created with newly randomized prizes and the next 10-day draw date.

### 5. NPC Flavor Dialogues (`@words`)
- `「宝くじの三要素！　夢！運！げんじつ！」`
- `「宝くじを当てたいなら、当たるまで買うといいよ！」`
- `「スライムは眼中にありません！」`
- `「大体10日くらいで賞品は変わるよ」`
- `「あと N 人分の宝くじがあるよ」`

---

## Database Persistence

### Schema Migrations (`migrations/018_lottery.sql`, `migrations/081_takarakuji.sql`)

- `character_lottery`: Tracks character tavern raffle coupons (`raffle_tickets >= 0`).
- `takarakuji_rounds`: Tracks 10-day Takarakuji lottery rounds, prize candidate items, and drawn status.
- `takarakuji_tickets`: Tracks character ticket purchases (1 per round, max 20 per round).

---

## HTTP REST Endpoints

| Method | Endpoint | Description | Auth |
| :--- | :--- | :--- | :--- |
| `GET` | `/lottery/takarakuji` | Get current Takarakuji status, prize lineup, remaining tickets, and talk phrases | Public |
| `POST` | `/characters/{id}/lottery/takarakuji/buy` | Purchase Takarakuji ticket (30,000G, 1 per character per round, 20 max) | Character Auth |
| `GET` | `/characters/{id}/lottery/takarakuji/ticket` | Get character's current round ticket and past participation history | Character Auth |
| `GET` | `/characters/{id}/lottery/tickets` | Get character tavern raffle ticket count | Character Auth |
| `POST` | `/characters/{id}/lottery/raffle` | Play raffle drawing mini-game (Standard: 3 tickets, Special: 300 tickets) | Character Auth |
