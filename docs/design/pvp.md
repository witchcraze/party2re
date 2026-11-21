# Colosseum PvP (Real-time 8-Player Bet & Split Color Battles)

## Overview

The Colosseum PvP Module (`internal/pvp`) reconstructs the original Party2 real-time multiplayer arena specification (`party2/lib/vs_player.cgi` and `party2/lib/quest.cgi:type=4`).

Unlike modern asynchronous snapshot Elo ladders, authentic Party2 PvP features real-time 2-to-8 player lobby rooms where participants wager gold, divide into up to 9 distinct color teams, battle across multi-round party combats using the shared Core Battle engine (`internal/core/battle`), and split the aggregated prize pool among the winning team members.

---

## Architectural Policy & Boundaries

- **Purge of Fictional Elo Arena**: Asynchronous Elo rating calculation, defense logs, and rating tables (`arena_ratings`, `arena_matches`) have been completely purged.
- **Relational vs Ephemeral Authority**:
  - Room state, participant roster, team assignments, and round scores are stored ephemerally in Valkey Master (`party2:pvp:room:<id>`, `party2:pvp:character:<id>`, `party2:pvp:rooms:active`) with a 30-minute inactivity TTL.
  - Canonical player wealth (`characters.money`) and stronger-opponent combat defeats (`characters.pvp_wins`, legacy `$m{kill_p}`) are durably persisted in MariaDB Master.
  - Financial transitions also retain a durable SQL claim and publication result. Failed room/membership writes are retried without charging or paying twice; SQL participation claims prevent concurrent admission to different rooms. See [financial publication and its recovery limits](../architecture/transient-run-state.md#pvp-financial-publication).
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
3. **Live-room departure (`@にげる`)**:
   - Before match start (`recruiting`, round 0), departure refunds only the departing participant's bet, promotes the next remaining member to leader when the leader leaves, and disbands the room only when the last member departs.
   - During (`in_progress`) and after (`completed`) a match, participants can leave (`pvp_leave`) without refunding bets or duplicating awarded prizes; leader exit promotes the next remaining member. In `in_progress`, remaining bets stay in `prize_pool` for surviving participants until settlement, or close as forfeited escrow when the last participant departs.
   - Unverified legacy escape side-effects (vitality copyback, fatigue increment, items `105`/`106`, consumed `ite` clearing, bar food delivery) and lobby actions (`@さそう`, `@きっく`, `@しらべる`) remain tracked in [#2287](https://github.com/witchcraze/party2re/issues/2287).

**Recovery and inactivity rules:** arbitrary live-room loss uses the legacy missing-member-file path (`_battle.cgi:453-461`), clearing membership without refunding bets or inferring an inactivity penalty. Genuine inactivity (`now > last_activity + 1800s`, `quest.cgi:50,85-91,172-184`) instead closes remaining escrow without refund and applies a 3600-second sleep with `PendingWake` to all remaining participants while preserving longer existing sleep. **Non-obvious legacy behavior:** `quest.cgi:928-1079` (`add_member`) writes join messages to `$logdir/quest.cgi` rather than the room's `log.cgi`, so joining does not refresh the inactivity clock; only room creation, team selection, match start, round advance, and non-final departures refresh it. Unleft `completed` rooms also remain subject to the 1800-second inactivity timeout (`quest.cgi:86-91`). Because GET observations remain read-only, cleanup currently runs lazily on mutating PvP accesses; periodic background cleanup independent of user actions is tracked in [#2343](https://github.com/witchcraze/party2re/issues/2343). SQL/Valkey recovery ownership is defined by [financial publication](../architecture/transient-run-state.md#pvp-financial-publication).

### 3. Match Progression & Start Invariants (`@かいし`)

1. **Start Conditions**:
   - At least 2 participants.
   - Every participant MUST have chosen a team color.
   - At least 2 distinct team colors must be represented.
2. **Health Restoration**:
   - Before Round 1 commences, all participants have their HP restored to `MaxHP`.

### 4. Round Combat Resolution (`_battle.cgi:486`, `vs_player.cgi:106-122`)

- If mid-match departures leave only one team in the room, that surviving team is awarded the round win without invoking `ResolvePartyBattle`.
- Otherwise, each round resolves a party battle between opposing teams using `ResolvePartyBattle`.
- The round winner is determined from the unique surviving team among participants (`res.RemainingHP`):
  - If exactly 1 team has surviving members (`alive_team_c == 1`), that team receives 1 round win point (`round_win`).
  - If 0 teams survive (mutual wipeout) or multiple teams remain (turn limit reached), the round is scored as a draw (`draw`).
- Round turns, combat logs, and HP changes are tracked per round.

### 5. Match Settlement & Prize Pool Split

- When a team reaches `target_wins`, `prize_pool` is split evenly among its members (`prize_per_member = prize_pool / winners`), retaining any integer-division remainder as closed forfeited escrow.
- Match victory (`give_bet`, `vs_player.cgi:186–216`) awards prize gold only; `characters.pvp_wins` (`kill_p`) is credited per surviving character who defeats an opponent satisfying `enemyStrong > playerStrong * 0.5` (`party2/lib/_battle.cgi:231–232`) in round combat (excluding auto-revived falls), settled atomically when the match concludes or disbands.
- If no team reaches `target_wins` by round 10 (`MaxRounds`), the match finishes as a draw (`match_draw`) without refunding bets, closing the remaining `prize_pool` as forfeited escrow.

---

Transport schemas: [OpenAPI](../api/openapi.json). Storage key lifetimes: [Valkey keyspace](../architecture/valkey-keyspace.md).
