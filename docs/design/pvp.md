# Colosseum PvP (Real-time 8-Player Bet & Split Color Battles)

## Overview

The Colosseum PvP Module (`internal/pvp`) faithfully reproduces the original Party2 real-time multiplayer arena specification (`party2/lib/vs_player.cgi` and `party2/lib/quest.cgi:type=4`).

Unlike modern asynchronous snapshot Elo ladders, authentic Party2 PvP features real-time 2-to-8 player lobby rooms where participants wager gold, divide into up to 9 distinct color teams, battle across multi-round party combats using the shared Core Battle engine (`internal/core/battle`), and split the aggregated prize pool among the winning team members.

---

## Architectural Policy & Boundaries

- **Purge of Fictional Elo Arena**: Asynchronous Elo rating calculation, defense logs, and rating tables (`arena_ratings`, `arena_matches`) have been completely purged.
- **Relational vs Ephemeral Authority**:
  - Room state, participant roster, team assignments, and round scores are stored ephemerally in Valkey Master (`party2:pvp:room:<id>`, `party2:pvp:character:<id>`, `party2:pvp:rooms`) with a 30-minute inactivity TTL.
  - Canonical player wealth (`characters.money`) and PvP victories (`characters.pvp_wins`, legacy `$m{kill_p}`) are durably persisted in MariaDB Master.
- **Core Battle Engine Integration**: Round combat utilizes `corebattle.Engine.ResolvePartyBattle`, treating teammates as allies and opposing color participants as enemies.

---

## Domain Rules & Mechanics

### 1. Room Creation & Parameters (`quest.cgi:type=4`)

A character can host a Colosseum room with configurable parameters:
- **Room Name**: 1 to 50 characters.
- **Password**: Optional password for private / clan matches.
- **Max Participants**: 2 to 8 players.
- **Target Wins**: 1 to 3 wins required for match victory.
- **Bet**: Minimum 10 gold. The leader deposits the bet immediately upon creation, seeding the room's `prize_pool`.
- **Join Requirement (`need_join`)**: Optional level requirement (e.g. `lv_10_o`, `lv_30_u`, `god_1_o`, etc.).
- **Speed & Stage**: Display and animation configuration (default speed 18).

### 2. Room Recruitment & Team Color Assignment (`@ぱーてぃー`)

1. **Joining**:
   - Participants pay the room `bet` into the `prize_pool` upon entry.
   - Characters cannot join if they are already in an active room, do not meet join conditions, or have insufficient funds.
2. **Team Colors**:
   - Participants choose one of the 9 authentic legacy team colors:
     - `#FF3333` (レッド / Red)
     - `#FF33CC` (ピンク / Pink)
     - `#FF9933` (オレンジ / Orange)
     - `#FFFF33` (イエロー / Yellow)
     - `#33FF33` (グリーン / Green)
     - `#33CCFF` (アクア / Aqua)
     - `#6666FF` (ブルー / Blue)
     - `#CC66FF` (パープル / Purple)
     - `#CCCCCC` (グレイ / Gray)
3. **Leaving & Disband (`@にげる`)**:
   - While recruiting, non-leaders can leave, receiving a full refund of their bet.
   - If the leader leaves during recruitment, the room is disbanded and all participants receive full refunds.
   - Leaving is strictly prohibited once a match has started.

### 3. Match Progression & Start Invariants (`@かいし`)

1. **Start Conditions**:
   - At least 2 participants.
   - Every participant MUST have chosen a team color.
   - At least 2 distinct team colors must be represented.
2. **Health Restoration**:
   - Before Round 1 commences, all participants have their HP restored to `MaxHP`.

### 4. Round Combat Resolution (`_battle.cgi:486`, `vs_player.cgi:106-122`)

- Each round resolves a party battle between opposing teams using `ResolvePartyBattle`.
- The round winner is determined from the unique surviving team among participants (`res.RemainingHP`):
  - If exactly 1 team has surviving members (`alive_team_c == 1`), that team receives 1 round win point (`round_win`).
  - If 0 teams survive (mutual wipeout) or multiple teams remain (turn limit reached), the round is scored as a draw (`draw`).
- Round turns, combat logs, and HP changes are tracked per round.

### 5. Match Settlement & Prize Pool Split

1. **Decisive Victory**:
   - When any team reaches `target_wins`, the match completes immediately.
   - **Prize Split**: The entire aggregated `prize_pool` is split evenly among all participants in the winning team:
     $$\text{Prize Per Member} = \left\lfloor \frac{\text{Prize Pool}}{\text{Winning Team Member Count}} \right\rfloor$$
   - Each winning member's wallet is credited via `char.AddMoney(...)`.
   - Each winning member's `pvp_wins` counter is incremented by 1 (legacy `$m{kill_p}++`).
   - The victory hook is executed, advancing medal achievements (`MetricPvPVictories`).
2. **Forced Draw & 10-Round Limit**:
   - If round 10 concludes without any team reaching `target_wins`, the match is terminated as a draw.
   - All participants receive a full refund of their original `bet`.

---

## Storage & API Reference

### Valkey Keyspace

- `party2:pvp:room:<room_id>`: `String (JSON)`. Full room metadata and participant array. 30-minute TTL.
- `party2:pvp:character:<character_id>`: `String`. Active room ID mapping. 30-minute TTL.
- `party2:pvp:rooms`: `Sorted Set (ZSet)`. Active room index ordered by `CreatedAt`.

### HTTP REST Endpoints

| Method | Path | Description |
| :--- | :--- | :--- |
| `POST` | `/characters/{id}/pvp/rooms` | Create a new Colosseum room with wager |
| `GET` | `/pvp/rooms` | List active recruiting and in-progress rooms |
| `GET` | `/pvp/rooms/{room_id}` | Retrieve room details and participant roster |
| `POST` | `/characters/{id}/pvp/rooms/{room_id}/join` | Join an existing room with password check and bet deduction |
| `POST` | `/characters/{id}/pvp/rooms/{room_id}/leave` | Leave room or disband with bet refund |
| `POST` | `/characters/{id}/pvp/rooms/{room_id}/team` | Select one of the 9 team colors |
| `POST` | `/characters/{id}/pvp/rooms/{room_id}/start` | Room leader starts the match |
| `POST` | `/characters/{id}/pvp/rooms/{room_id}/advance` | Advance to the next combat round |
