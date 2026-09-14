# Casino Architecture & Multi-Player Facility Design

## Overview

The Casino (カジノ) in Party2 is an entertainment and wagering facility based faithfully on `party2/lib/casino.cgi`, `party2/lib/_casino.cgi`, `party2/lib/casino_indian.cgi`, `party2/lib/casino_highlow.cgi`, and `party2/lib/casino_doppel.cgi`.

It provides:
1. **Casino Currency Exchange**: Two-way exchange between character gold and casino coins (1 Coin = 20 Gold).
2. **Multi-Player Room Lobby**: Real-time room recruitment and turn-based games for 2 to 8 players.
3. **Authentic Mini-Games**:
   - Multi-Player Indian Poker (`indian`) — 13-card blind bluffing game with forehead placement and pot distribution.
   - Multi-Player High & Low (`highlow`) — 13-card rank prediction and split pot showdown.
   - Multi-Player Doppelganger (`doppel`) — 8-symbol dealer matching and leadership transfer.
   - Slot Machine (`slot`) — Solo 3-reel, 5-symbol paytable with 100x jackpot.
4. **Prize Exchange & Depot Routing**: 18 authentic prizes delivered directly into long-term Depot storage (`character_depots`).

---

## 1. Currency & Account Invariants

- **Exchange Rate**: `1 Casino Coin = 20 Gold`.
- **Wallet vs. Bank Balance**:
  - Buying coins deducts from character wallet gold (`characters.money`).
  - Selling coins credits character wallet gold (subject to the 999,999G clamp).
- **Non-negative Balance**:
  - `casino_accounts.coins >= 0` enforced by table schema and transactional checks.

---

## 2. Multi-Player Room Lobby (Candidate C: Ephemeral Valkey Master)

Rooms serialize multi-player games using Valkey Master (`ValkeyRoomRepository`, Candidate C Ephemeral Turn & Session Lobby Architecture, SSOT: [`docs/architecture/transient-run-state.md`](../architecture/transient-run-state.md)). Legacy MariaDB tables `casino_rooms` and `casino_members` were dropped in Migration 085.

### Creation Rules (`@つくる` / `POST /characters/{id}/casino/rooms`)
- **Name**: 1–50 runes, unique among non-disbanded rooms, cannot contain spaces, tabs, newlines, or delimiters (`,;\&<>\\/@＠`).
- **Game Types**: `indian`, `highlow`, `doppel`.
- **Turn Speed**:
  - `12` seconds (さくさく / Fast)
  - `18` seconds (まったり / Normal)
  - `28` seconds (じっくり / Slow)
- **Capacity**: 2 to 8 players.
- **Base Rate**:
  - Standard (`indian`, `highlow`): `1`, `5`, `10`, `20`, `50`, `100`, `500`, `1000`, `5000` coins.
  - Doppelganger (`doppel`): Minimum 10 coins.
  - Creator minimum balance: `rate * 5` coins (`rate` for doppel).
- **Aikotoba (Password)**: Optional plaintext password hashed via SHA-256.
- **Spectator Mode**: Optional boolean flag.

### Participant Lifecycle
- **Join (`@さんか`)**: Gated by `tired < 100`, `coins >= rate`, and available capacity.
- **Spectate (`@けんがく`)**: Gated by room spectator allowance. Spectators cannot act or receive pot rewards.
- **Leave (`@にげる`)**: Removes player. Automatically reassigns room leader to next active member; disbands room if 0 active members remain.
- **Kick (`@きっく`)**: Leader-only eviction of non-leader members prior to game start (`round == 0`).
- **Unified Game Dispatch**:
  - `POST /characters/{id}/casino/rooms/{roomId}/start`: Starts the game according to configured `game_type`.
  - `POST /characters/{id}/casino/rooms/{roomId}/action`: Dispatches player actions according to configured `game_type`.

---

## 3. Multi-Player Indian Poker (`party2/lib/casino_indian.cgi`)

### Rules & Mechanics
- **Deck**: 13 unique cards (`Ａ`, `２`, `３`, `４`, `５`, `６`, `７`, `８`, `９`, `10`, `Ｊ`, `Ｑ`, `Ｋ`).
- **Forehead Card Rule**: Each player cannot view their own dealt card (masked as `？` / `-1` in API client views), but sees all opponent cards.
- **Actions**:
  - `call` (`つづける`): Pay current round bet into pot and stay in the hand.
  - `showdown` (`しょうぶ`): Pay current round bet into pot and vote to conclude.
  - `fold` (`おりる`): Forfeit hand without paying; reveals card immediately.
- **Round Flow & Settlement**:
  - When all active members declare actions:
    - If all but one fold $\rightarrow$ remaining player wins.
    - If $\ge 50\%$ vote showdown, or maximum bet reached, or a player exhausts coins $\rightarrow$ Showdown triggers.
    - Otherwise $\rightarrow$ `round++`, `current_bet += rate`.
  - Highest card among surviving non-folded players wins entire pot.
  - Ties split the pot equally.
  - Members with 0 coins remaining are automatically ejected.

---

## 4. Multi-Player High & Low (`party2/lib/casino_highlow.cgi`)

### Rules & Mechanics
- **Deck**: 13 unique cards (`Ａ`..`Ｋ`), dealt 1 per player secretly.
- **Card & Action Visibility**:
  - Player sees their **own** card; opponents' cards are masked as `？` / `-1`.
  - Opponents' declared competitive actions (`high`, `low`, `fold`) are masked as `？？？` during active round to prevent information leakage, while `call`/`tsuzukeru` and `待機中` remain visible.
- **Actions**:
  - `call` (`つづける`): Match current round bet into pot and stay in the hand.
  - `high` (`ハイ`): Bet current round bet that own card is highest.
  - `low` (`ロウ`): Bet current round bet that own card is lowest (requires $> 2$ participants).
  - `fold` (`おりる`): Forfeit hand and pay current bet into pot.
- **Round Flow & Progression**:
  - When all active participants have acted in the round:
    - Showdown triggers if:
      - Active non-folded players $\le 1$, OR
      - Any non-folded player exhausts coins (`coins <= 0`), OR
      - Current bet reaches maximum (`current_bet >= rate * 5`), OR
      - Number of High + Low declarations $\ge 50\%$ of active non-folded players.
    - Otherwise: `round++`, `current_bet += rate`. Non-folded participants reset action to `""` and keep cards.
- **Showdown Resolution**:
  - Determine highest card among `high` callers (`higher`) and lowest card among `low` callers (`lower`).
  - **Split Pot (3+ players)**: If both `higher` and `lower` exist and room has $> 2$ players, the pot is divided 50/50 (`pot / 2`) between both winners.
  - **Single Winner**: If only `higher` exists, `higher` wins full pot. If only `lower` exists, `lower` wins full pot.
  - **All Fold**: If all players folded, game ends with no winner ("お流れ") and pot clears.
  - Members with 0 coins remaining are ejected from room; survivors reset to `待機中`.

---

## 5. Multi-Player Doppelganger (`party2/lib/casino_doppel.cgi`)

### Rules & Mechanics
- **Marks**: 8 symbols (`★`, `●`, `◆`, `♪`, `■`, `▲`, `†`, `▼`).
- **Selectable Pool**: `0..min(len(participants), 7)`. For 2 players: 3 marks (`★`, `●`, `◆`); for 3 players: 4 marks; for 7+ players: all 8 marks.
- **Mark Visibility**: Each player sees only their **own** chosen mark; all opponents' marks and actions are masked as `？` / `？？？` during the round.
- **Wager & Selection**:
  - Bet rate is fixed (minimum 10 coins).
  - Coin deduction occurs on a player's **first** mark selection in the round.
  - Players may freely change their selected mark during the round without additional coin deductions.
- **Showdown Resolution**:
  - Triggers immediately when all active participants have chosen a mark.
  - The room leader is the "親" (dealer / target).
  - Any non-leader participant ("子") whose mark matches the leader's mark is a winner:
    - **Children Win**: If $\ge 1$ children match the leader, they split the entire pot equally (`pot / len(winners)`).
    - **Leadership Transfer**: A winner chosen at random becomes the new room leader.
    - **Parent Wins**: If 0 children match the leader, the leader wins the entire pot and retains room leadership.
  - Members with 0 coins remaining are ejected from room; survivors reset to `待機中`.

---

## 6. Authentic Prize Catalog & Depot Routing

### 18 Authentic Prizes (`party2/lib/casino.cgi:41-65`)

| Coins | Item ID | Item Name | Category |
| :---: | :--- | :--- | :---: |
| **100** | `item-004` | 賢者の石 | Consumable |
| **300** | `item-012` | 祈りの指輪 | Accessory |
| **700** | `item-006` | 霊樹の葉 | Consumable |
| **2,000** | `item-032` | 物真似の心 | Consumable |
| **4,000** | `item-038` | 幻獣の実 | Consumable |
| **5,000** | `item-039` | ギャンブルハート | Consumable |
| **8,000** | `armor-34` | 危ない水着 | Armor |
| **30,000** | `weapon-31` | 必殺のピアス | Weapon |
| **70,000** | `weapon-40` | 流銀の剣 | Weapon |
| **80,000** | `weapon-38` | 茨の霊鞭 | Weapon |
| **180,000** | `item-106` | 金の鶏 | Consumable |
| **200,000** | `item-105` | 幸せのくつ | Consumable |
| **1,000,000** | `item-231` | 宇宙の壁紙 | Consumable |
| **1,000,001** | `item-232` | 蟻地獄の壁紙 | Consumable |
| **1,000,002** | `item-233` | 炎の壁紙 | Consumable |
| **1,000,003** | `item-234` | 墓場の壁紙 | Consumable |
| **1,000,004** | `item-235` | 図書館の壁紙 | Consumable |
| **1,000,005** | `item-236` | 要塞の壁紙 | Consumable |

### Atomic Depot Storage Routing
- Items are deposited directly into character depot storage (`character_depots`, `depot_items`).
- If depot is full, `depot.ErrDepotFull` is returned and zero coins are deducted.
- Stackable items are automatically merged with existing inventory slots.

---

## 7. Concurrency & Lock Acquisition Hierarchy
 
Multi-player room lobbies and in-flight turns reside exclusively in Valkey Master (Candidate C Ephemeral Turn & Session Lobby Architecture). MariaDB transactional operations strictly follow the global lock acquisition hierarchy for financial settlements and prize exchange:
```text
Rank 2: characters (Character Primary Entity)
  ↓
Rank 5: character_depots, depot_items (Depot Storage)
  ↓
Rank 8: casino_accounts (Secondary Feature Records)
```
In `ExchangePrize`, Depot lock (Rank 5) is acquired before Casino account lock (Rank 8), preventing deadlocks with concurrent transactions.
