# Lottery & Raffle Design (Takarakuji & Fukubiki)

## Overview

The Lottery and Raffle Feature Module (`internal/lottery`) implements the authentic Party2 town games:
1. **Server-Wide 20-Cap Rare Item Lottery (宝くじ / Takarakuji)**: Periodic limited-entry lottery (`party2/lib/takarakuzi.cgi`, NPC `@クラゲ`) featuring 20 tickets per round, 1 ticket per player, 10-day drawing cycles, and direct delivery of rare equipment and alchemy recipes into the winner's Depot (`預かり所`).
2. **Raffle (福引 / Fukubiki)**: Instant raffle mini-game (`party2/lib/lot.cgi`) using meal coupons from the Adventurer's Tavern to draw stat-boosting seeds and divine orbs.

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

### Schema Migrations (`migrations/081_takarakuji.sql`)

```sql
CREATE TABLE IF NOT EXISTS takarakuji_rounds (
    round_id INT NOT NULL AUTO_INCREMENT PRIMARY KEY,
    draw_date DATETIME(6) NOT NULL,
    is_drawn BOOLEAN NOT NULL DEFAULT FALSE,
    drawn_at DATETIME(6) NULL,
    prize_1_item_id VARCHAR(64) NOT NULL,
    prize_1_amount INT NOT NULL DEFAULT 1,
    prize_2_item_id VARCHAR(64) NOT NULL,
    prize_2_amount INT NOT NULL DEFAULT 1,
    prize_3_item_id VARCHAR(64) NOT NULL,
    prize_3_amount INT NOT NULL DEFAULT 2,
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    INDEX idx_takarakuji_rounds_draw_date (draw_date, is_drawn)
);

CREATE TABLE IF NOT EXISTS takarakuji_tickets (
    id CHAR(32) NOT NULL PRIMARY KEY,
    round_id INT NOT NULL,
    character_id CHAR(32) NOT NULL,
    purchased_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    won_rank INT NOT NULL DEFAULT 0,
    won_item_id VARCHAR(64) NULL,
    CONSTRAINT fk_takarakuji_tickets_round FOREIGN KEY (round_id) REFERENCES takarakuji_rounds (round_id) ON DELETE CASCADE,
    CONSTRAINT fk_takarakuji_tickets_char FOREIGN KEY (character_id) REFERENCES characters (id) ON DELETE CASCADE,
    CONSTRAINT uq_takarakuji_round_char UNIQUE (round_id, character_id)
);
```

---

## HTTP REST Endpoints

| Method | Endpoint | Description | Auth |
| :--- | :--- | :--- | :--- |
| `GET` | `/lottery/takarakuji` | Get current Takarakuji status, prize lineup, remaining tickets, and talk phrases | Public |
| `POST` | `/characters/{id}/lottery/takarakuji/buy` | Purchase Takarakuji ticket (30,000G, 1 per character per round, 20 max) | Character Auth |
| `GET` | `/characters/{id}/lottery/takarakuji/ticket` | Get character's current round ticket and past participation history | Character Auth |
| `GET` | `/characters/{id}/lottery/tickets` | Get character tavern raffle ticket count | Character Auth |
| `POST` | `/characters/{id}/lottery/buy-raffle` | Buy raffle tickets (scheduled for removal in Issue #485) | Character Auth |
| `POST` | `/characters/{id}/lottery/raffle` | Play raffle drawing mini-game | Character Auth |
