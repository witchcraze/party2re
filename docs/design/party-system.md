# Party & Multiplayer Co-op System Design Specification

## Overview

The Party system (`冒険中のパーティー`, `quest.cgi`, `party.cgi`) is one of the foundational multiplayer mechanics in Party2. It allows up to 4 players to form a cooperative adventuring party to tackle stages and dungeons together, sharing experience, gold, and item drops, enhanced by cooperative synergy bonuses.

---

## Game Rules & Domain Concepts

### 1. Party Formation & Settings
- **Party Leader**: The creator of the party is designated as leader.
- **Name / Title**: 1 to 50 runes, descriptive of the expedition.
- **Capacity**: 1 to 4 characters (`max_members`).
- **Speed**: Configurable animation/log pacing:
  - `3`: さくさく (Fast)
  - `18`: まったり (Relaxed)
  - `25`: じっくり (Deliberate)
- **Join Requirements (Conditions)**:
  - Level bounds (`min_level`, `max_level`)
  - HP threshold (`min_hp`)
  - Optional secret passphrase (`合言葉` / `password_hash`)
- **Single Active Party Constraint**: A character may participate in only one active/recruiting party at a time. This invariant is enforced atomically in Valkey Master via reverse lookup key `party2:party:character:<character_id>`.
- **Lobby Expiration & Readiness TTL**:
  - Waiting lobbies have a natural TTL of 15 minutes (`900s`), refreshed on member activity, preventing zombie lobbies if a leader disconnects.
  - Member readiness state has a 60-second countdown TTL (`60s`), after which unconfirmed readiness automatically expires to prevent stall locks.

### 2. Party Lifecycle & States
- `recruiting`: Open for members to join, leave, toggle readiness, or be kicked by leader (stored authoritatively in Valkey Master).
- `in_progress` / `in_adventure`: Co-op battle execution in progress.
- `completed`: Adventure finished and rewards distributed in MariaDB; members can ready up for subsequent runs or leave.
- `disbanded`: Party closed by the leader or automatically upon leader departure / adventure completion.
- *Note*: Ephemeral wait lobby state is completely decoupled from relational storage. Legacy MariaDB `parties` and `party_members` tables were dropped in Migration 052.

### 3. Readiness & Start Requirements
- All party members must toggle their ready state (`ready_state = true`) before the leader can trigger `POST /parties/{id}/start`.
- All participants must have `HP > 0` (unconscious characters cannot embark on adventures).

### 4. Multiplayer Combat & Synergy Bonus
- **Multi-participant Turn Engine**:
  - Allied party members and stage monsters take turns attacking opponents.
  - Active allies target enemies with lowest HP to coordinate takedowns.
- **Synergy Multipliers**:
  - 1 Player: 0% bonus (base rewards)
  - 2 Players: +10% bonus EXP and Gold
  - 3 Players: +20% bonus EXP and Gold
  - 4 Players: +30% bonus EXP and Gold
- **Participant Identity & Turn Resolution**:
  - Combat participants are strictly indexed and tracked by their canonical entity `ID` (`Participant.ID`), eliminating name collisions when multiple characters share the same display name.
  - Display names (`Participant.Name`) are maintained for combat turn log output and user presentation.
- **Reward Distribution & Damage Persistence**:
  - On Victory: Each participating member receives full boosted EXP and Gold, character level-ups are evaluated using canonical progression rules (`progression.ApplyExperience`, properly supporting OverLevel limit breaks up to Lv 150), surviving members have their remaining battle HP persisted to `Stats.HP` (fallen members survive with 1 HP), and stage item drops are awarded to player inventories.
  - **Lifetime Milestone Progress Tracking (`VictoryHook`)**: Upon successful expedition victory, `party.Service.StartPartyAdventure` invokes the registered `VictoryHook(ctx, characterIDs, monstersDefeated, goldEarned)` callback. In `cmd/party2/main.go`, this is wired to `medalService.RecordProgress`, incrementing `adventure_victories` (+1), `monsters_slain` (+defeated monsters count), and `gold_earned` (+earned gold) for all participating characters, maintaining progression parity with solo adventures.
  - **Cooperative QoL Policy vs Legacy CGI**: In legacy Party2 Perl CGI (`_battle.cgi:209`), fallen party members received 0 rewards (`next if $ms{$name}{hp} <= 0;`). In `party2re`, this behavior was intentionally revised: all participants who took part in a victorious expedition receive full synergy-boosted EXP/Gold and revive with 1 HP. This deliberate modern cooperative QoL design fosters teamplay and prevents penalizing tanks/support characters who sacrifice themselves for the party's victory.
  - On Defeat: Half base EXP, 0 Gold, and characters survive with 1 HP.

---

## Endpoints

| Method | Path | Auth | Description |
|---|---|---|---|
| `GET` | `/parties` | Public | List recruiting parties with query filters (`status`, `limit`, `offset`) |
| `POST` | `/parties` | Bearer Session | Create a new party with creator as leader |
| `GET` | `/parties/{id}` | Public | Retrieve party detail and member roster |
| `POST` | `/parties/{id}/join` | Bearer Session | Join an existing recruiting party |
| `POST` | `/parties/{id}/leave` | Bearer Session | Leave the party (disbands if leader) |
| `POST` | `/parties/{id}/kick` | Bearer Session | Kick a member (leader only) |
| `DELETE` | `/parties/{id}` | Bearer Session | Disband the party (leader only) |
| `POST` | `/parties/{id}/ready` | Bearer Session | Set member readiness state |
| `POST` | `/parties/{id}/start` | Bearer Session | Initiate multiplayer co-op adventure (leader only) |

---

## Transaction & Concurrency Boundaries

The Party architecture implements a two-tier storage boundary (RFC #356, Issue #368, Issue #380):

1. **Ephemeral Wait Lobbies (Valkey Master Authoritative Tier)**:
   - Lobby metadata (`party2:party:lobby:<party_id>`), member rosters, index sets (`party2:party:lobbies`), and reverse membership lookups (`party2:party:character:<character_id>`) reside exclusively in Valkey Master.
   - Join, leave, ready, and capacity operations use atomic Valkey Lua scripts and multi/exec pipelines to prevent race conditions and membership double-booking without relational database locking contention.
   - During lobby mutations, MariaDB `runInTx` is utilized strictly for individual character row verification (`SELECT ... FOR UPDATE`, Rank 2).

2. **Durable Quest Resolution & Settlement (MariaDB Master Canonical Tier)**:
   - When the leader starts the expedition (`POST /parties/{id}/start`), the party status is locked in Valkey Master.
   - A MariaDB transaction (`runInTx`) acquires deterministic pessimistic locks (`SELECT ... FOR UPDATE`, Rank 2) on all participating character records sorted in ascending canonical ID order to eliminate deadlock risks.
   - Co-op battle simulation is executed in memory.
   - Character progression updates (EXP, level-ups, OverLevel), remaining HP, Gold, and inventory item rewards are atomically committed in MariaDB.
   - Durable audit records are persisted in `party_adventure_logs` (retained permanently for expedition history and audit trails).
   - Upon successful database commit, the ephemeral Valkey lobby is disbanded and cleaned up.
