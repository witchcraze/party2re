# Guild versus Guild (GvG) Live Battle Design

## Overview

The Guild versus Guild (GvG) module (`internal/gvg`) implements authentic real-time multiplayer guild battles directly ported from the original Party2 specification (`quest.cgi:type=5`, `vs_guild.cgi`).

Guild members gather in ephemeral, real-time battle rooms to compete across multiple rounds for Guild Points (GP), prize pools, and permanent guild trophy decorations. All fictional asynchronous Elo rating calculations ($E_A, \Delta R_A, K=32$), simulated 5-man rosters, and temporary match history tables have been removed in favor of authentic live multi-round team combat.

---

## Architectural Policy & Boundaries

- **Live Team Battle Resolution**: Combats are resolved dynamically through `internal/core/battle.Engine` via `ResolvePartyBattle`. All participants sharing the room leader's guild color form the Ally party, while opposing guild participants form the Enemy party.
- **Dual-Tier Storage Model**:
  - **Ephemeral Room State (Valkey Master)**: Rooms, active rosters, and character-to-room mappings are stored in Valkey (`party2:gvg:room:<id>`, `party2:gvg:character:<id>`, `party2:gvg:rooms`) with a 30-minute inactivity TTL. Disbanding or match completion immediately cleans up active mappings.
  - **Canonical Persistence (MariaDB Master)**: Guild standings, accumulated Victory Points (GP), wins, losses, draws, and permanent 7-tier trophy decorations are durably recorded in `gvg_standings` using row-level pessimistic locking (`FOR UPDATE`). Guilds store their official team color in `guilds.color`.
- **Authorization & Ownership**: All room creation, joining, starting, and advancing operations enforce strict character ownership through standard authentication middleware (`withAuthenticatedCharacter`).

---

## Domain Rules & Gameplay Mechanics

### 1. Room Creation & Joining

- **Room Creation (`@ギルドバトル`, `quest.cgi:836-890`)**:
  - Room Parameters: Room Name (1–50 characters), Password (optional), Speed (default 18), Stage (0..9), Max Participants (2..8, default 8), Target Wins (1..3, default 1), and join restrictions (`need_join`).
  - **Guild Membership Requirement**: The creator must be a member of a registered guild (`ErrActorNotInGuild`).
  - **Friendly Guild Restriction**: Guilds with the default friendly color `#FFFFFF` (or empty) are strictly prohibited from participating in GvG (`"仲良しギルドはギルド戦をすることはできません"` / `ErrFriendlyGuildCannotBattle`).
  - **Initial Prize Pool Seed**: Room creation seeds an initial **2 GP** into the room's prize pool (`add_bet($quest_id, 2)`).
  - The creator automatically adopts their guild's color (`guilds.color`) as their team color.
- **Room Joining (`quest.cgi:1066-1099`)**:
  - Joiners must belong to a non-friendly guild (`color != '#FFFFFF'`).
  - Each joiner automatically adopts their guild's color as team color.
  - Each joiner adds **1 GP** to the room's accumulated prize pool (`add_bet($quest_id, 1)`).
  - Characters cannot join if unconscious (`HP <= 0`), exhausted (`Tired >= 100`), or already in an active room.

---

### 2. Multi-Round Match Progression (`@かいし`, `vs_guild.cgi:44-154`)

- **Match Start Requirements**:
  - Can only be initiated by the Room Leader.
  - Requires at least **2 participants** (`ErrNotEnoughParticipants`).
  - Requires participants from at least **2 distinct guild colors** (`ErrNeedAtLeastTwoGuilds` / `"対戦するギルドがいません"`).
  - Restores all participants' HP to MaxHP (`$ms{$name}{hp} = $ms{$name}{mhp}`).
  - Transitions room status from `recruiting` to `in_progress` and initializes Round 1.
- **Round Combat Resolution**:
  - Living participants are partitioned into Allies (sharing Room Leader's guild color) and Enemies (opposing guild colors).
  - Combat executes through `corebattle.ResolvePartyBattle`.
  - **Round Winner**: The guild whose team defeated the opponents wins the round.
  - **Round Victory Reward**: The winning guild immediately receives **+3 GP** (`victory_points += 3`, `vs_guild.cgi:121`).
  - Round scores are tracked per guild (`guild_scores[guild_id]++`).
  - If no teams survive, the round results in a draw with no score or round GP awarded.
- **Match Decider & Target Wins**:
  - When a guild achieves the specified `TargetWins` (1..3):
    - The match completes (`status = completed`, `outcome = match_won`).
    - The overall winning guild receives the **entire accumulated prize pool GP** (`victory_points += prize_pool`, `vs_guild.cgi:183`).
    - The overall winning guild is awarded **1 Bronze Medal**, triggering recursive cascading promotion (`vs_guild.cgi:193`).
    - Participating loser guilds receive $+1$ match loss.
    - **All participants across all guilds** in the room earn **+4 GP** for their respective guilds (`vs_guild.cgi:187`).
- **10-Round Draw Limit (`vs_guild.cgi:67, 114`)**:
  - If 10 rounds conclude without any guild reaching the target wins, the match forcibly terminates as a draw (`match_draw`).
  - All participating guilds receive $+1$ match draw.
  - All participants across all guilds still earn **+4 GP** for their guilds.
- **Round Advancement**:
  - If the match is not complete, round number increments (`Round++`), and HP of all participants is fully restored to MaxHP for the next round (`vs_guild.cgi:142`).

---

### 3. Seven Trophy / Medal Tiers & 5-for-1 Promotion (`vs_guild.cgi:9-19, 216`)

Match victories award Bronze Medals to the winning guild's permanent record. Every 5 medals of a given tier automatically and recursively promote into 1 trophy of the next tier:

| Tier | Medal / Trophy | Japanese Name | Promotion Requirement | Equivalent Match Wins | Database Column |
| :---: | :--- | :--- | :--- | :---: | :--- |
| **1** | **Bronze Medal** | 銅メダル | Base unit awarded on match win | 1 | `bronze_medals` |
| **2** | **Silver Medal** | 銀メダル | 5 Bronze Medals | 5 | `silver_medals` |
| **3** | **Gold Medal** | 金メダル | 5 Silver Medals | 25 | `gold_medals` |
| **4** | **Order** | 勲章 | 5 Gold Medals | 125 | `orders` |
| **5** | **Trophy** | トロフィー | 5 Orders | 625 | `trophies` |
| **6** | **Championship Cup** | 優勝杯 | 5 Trophies | 3,125 | `championship_cups` |
| **7** | **Champion Cup** | 王者杯 | 5 Championship Cups | 15,625 | `champion_cups` |

The cascading promotion logic executes deterministically on each medal award:

```go
func (s *GvGStanding) PromoteMedals() {
    for s.BronzeMedals >= 5 {
        s.BronzeMedals -= 5
        s.SilverMedals++
    }
    for s.SilverMedals >= 5 {
        s.SilverMedals -= 5
        s.GoldMedals++
    }
    for s.GoldMedals >= 5 {
        s.GoldMedals -= 5
        s.Orders++
    }
    for s.Orders >= 5 {
        s.Orders -= 5
        s.Trophies++
    }
    for s.Trophies >= 5 {
        s.Trophies -= 5
        s.ChampionshipCups++
    }
    for s.ChampionshipCups >= 5 {
        s.ChampionshipCups -= 5
        s.ChampionCups++
    }
}
```

---

## Leaderboard & Standings Rankings

Guilds on the GvG leaderboard are ranked strictly by prestige, sorting descending through the 7 trophy tiers before falling back to Victory Points (GP) and win count:

1. `champion_cups DESC`
2. `championship_cups DESC`
3. `trophies DESC`
4. `orders DESC`
5. `gold_medals DESC`
6. `silver_medals DESC`
7. `bronze_medals DESC`
8. `victory_points DESC`
9. `wins DESC`

---

## API Endpoints Reference

| Method | Route | Description | Auth Wrapper |
| :--- | :--- | :--- | :--- |
| `POST` | `/characters/{id}/gvg/rooms` | Create new GvG room (seeds 2 GP) | `withAuthenticatedCharacter` |
| `GET` | `/gvg/rooms` | List active recruiting GvG rooms | Public |
| `GET` | `/gvg/rooms/{room_id}` | Inspect GvG room details and members | Public |
| `POST` | `/characters/{id}/gvg/rooms/{room_id}/join` | Join GvG room (adds 1 GP) | `withAuthenticatedCharacter` |
| `POST` | `/characters/{id}/gvg/rooms/{room_id}/leave` | Leave room or disband (if leader) | `withAuthenticatedCharacter` |
| `POST` | `/characters/{id}/gvg/rooms/{room_id}/start` | Start Round 1 match (`@かいし`) | `withAuthenticatedCharacter` |
| `POST` | `/characters/{id}/gvg/rooms/{room_id}/advance` | Execute round battle & advance (`@かいし`) | `withAuthenticatedCharacter` |
| `GET` | `/gvg/standings/{guild_id}` | Fetch guild GvG standing & medals | Public |
| `GET` | `/gvg/leaderboard` | View global GvG guild leaderboard | Public |
