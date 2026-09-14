# Casino: Multi-Player Room Lobby, Indian Poker, & Prize Depot Routing

## Overview

The Casino (カジノ) system in Party2 provides multiplayer gaming rooms and prize exchanges based faithfully on the legacy Perl CGI implementation (`party2/lib/casino.cgi`, `party2/lib/_casino.cgi`, `party2/lib/casino_indian.cgi`).

Players can create and join multiplayer rooms for Indian Poker (`indian`), High & Low (`highlow`), and Doppelganger (`doppel`). In Indian Poker, 2 to 8 players compete with a 13-card deck where cards are placed on foreheads—each player sees all opponents' cards while their own card is hidden until showdown or folding.

Casino coins earned can be exchanged for 18 authentic casino prizes delivered directly into long-term bank storage (Depot).

---

## 1. Multi-Player Room Lobby (Candidate C: Ephemeral Valkey Master)

Rooms and in-flight turns reside exclusively in Valkey Master (`ValkeyRoomRepository`, Candidate C Ephemeral Turn & Session Lobby Architecture, SSOT: [`docs/architecture/transient-run-state.md`](../architecture/transient-run-state.md)). Legacy MariaDB tables `casino_rooms` and `casino_members` were dropped in Migration 085.
- **Name**: 1–50 characters, unique among non-disbanded rooms, cannot contain whitespace or delimiter characters (`,;\&<>\\/@＠`).
- **Game Type**: `indian` (Indian Poker), `highlow` (High & Low), `doppel` (Doppelganger).
- **Speed**: Turn countdown timer:
  - `12` seconds (さくさく / Fast)
  - `18` seconds (まったり / Normal)
  - `28` seconds (じっくり / Slow)
- **Capacity**: 2 to 8 players.
- **Rate (Base Bet)**:
  - Standard games (`indian`, `highlow`): `1`, `5`, `10`, `20`, `50`, `100`, `500`, `1000`, `5000` coins.
  - Doppelganger (`doppel`): Custom rate with minimum 10 coins.
  - Room creator must hold at least `rate * 5` coins (or `rate` for doppel).
- **Password (Aikotoba)**: Optional plaintext password hashed via SHA-256 (`password_hash`).
- **Spectators**: Optional permission allowing non-playing spectators.

### Member Operations
- **Join (`@さんか`)**:
  - Requires character fatigue `tired < 100` and `coins >= rate`.
  - Room capacity must not be exceeded.
- **Spectate (`@けんがく`)**:
  - Permitted only if `allow_spectators` is true. Spectators cannot act or receive payouts.
- **Leave (`@にげる`)**:
  - Removes character from room.
  - If the leader leaves, leadership automatically transfers to the next active player.
  - If all active players leave, the room status becomes `disbanded`.
- **Kick (`@きっく`)**:
  - Leader can kick another member only before the game starts (`round == 0`).

---

## 2. Multiplayer Indian Poker Mechanics (`party2/lib/casino_indian.cgi`)

### Card Deck & Forehead Rule
- **Deck**: 13 unique ranks (0 to 12: `Ａ`, `２`, `３`, `４`, `５`, `６`, `７`, `８`, `９`, `10`, `Ｊ`, `Ｑ`, `Ｋ`).
- **Card Distribution**: Each active player is dealt 1 unique card from the 13-card deck.
- **Forehead Card Visibility**:
  - A player **cannot** see their own card during an active round (masked as `？` / `-1` in API responses).
  - A player **can** see all other players' cards.
  - Cards are revealed upon folding or showdown.

### Betting & Round Flow
1. **Start Game (`@かいし`)**: Leader starts Round 1 (`round = 1`, `current_bet = rate`). Cards dealt to all active members.
2. **Actions**:
   - **Call (`つづける`)**: Pay `current_bet` coins into the pot to continue.
   - **Showdown (`しょうぶ`)**: Pay `current_bet` coins into the pot and request immediate showdown.
   - **Fold (`おりる`)**: Forfeit current hand without paying into pot. Card is revealed.
3. **Round Advancement**:
   - When all active players have acted:
     - If all but one player folded $\rightarrow$ remaining player wins.
     - If $\ge 50\%$ of active players declared showdown (or maximum bet reached, or a player runs out of coins) $\rightarrow$ Showdown triggers.
     - Otherwise $\rightarrow$ `round++`, `current_bet += rate`, and actions reset.
4. **Showdown Resolution**:
   - Highest card among non-folded players wins the entire accumulated pot.
   - Ties split the pot equally.
   - Surviving members reset to `待機中`. Members with 0 coins are automatically ejected.

---

## 3. Authentic Prize Exchange & Depot Routing

### Prize Catalog (18 Authentic Items from `party2/lib/casino.cgi:41-65`)

| Cost (Coins) | Item ID | Item Name | Category |
| :--- | :--- | :--- | :--- |
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

### Depot Routing & Atomicity
- Items purchased are delivered directly into the character's Depot (`character_depots`, `depot_items`).
- If the depot is full, `depot.ErrDepotFull` is returned and **zero** casino coins are deducted (strict atomic rollback).
- Stackable items merge into existing slots; equipment items occupy distinct slots.

---

## 4. Concurrency & Lock Acquisition Hierarchy

Multi-player room lobbies and in-flight turns reside exclusively in Valkey Master (Candidate C Ephemeral Turn & Session Lobby Architecture). MariaDB transactional operations strictly follow the global lock acquisition hierarchy for financial settlements and prize exchange:
1. **Rank 2 (Character Primary Entity)**: `characters`
2. **Rank 5 (Depot Storage)**: `character_depots`, `depot_items`
3. **Rank 8 (Secondary Feature Records)**: `casino_accounts`

In `ExchangePrize`:
- Depot lock (`FindByCharacterIDForUpdate` - Rank 5) is acquired first.
- Casino account lock (`GetAccountForUpdate` - Rank 8) is acquired second.
- Prevents deadlocks with concurrent inventory/depot transactions.
