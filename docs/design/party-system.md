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
- **Single Active Party Constraint**: A character may participate in only one active/recruiting party at a time (`uk_party_members_character`).

### 2. Party Lifecycle & States
- `recruiting`: Open for members to join, leave, toggle readiness, or be kicked by leader.
- `in_progress`: Co-op battle execution in progress.
- `completed`: Adventure finished and rewards distributed; members can ready up for subsequent runs or leave.
- `disbanded`: Party closed by the leader or automatically upon leader departure.

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
- **Reward Distribution**:
  - On Victory: Each participating member receives full boosted EXP and Gold, character level-ups are evaluated using canonical progression rules (`progression.ApplyExperience`, properly supporting OverLevel limit breaks up to Lv 150), and stage item drops are awarded to player inventories.
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
- All party modifications use MariaDB `RunInTx` with `SELECT ... FOR UPDATE`.
- During adventure execution, all member character records are locked in ascending ID order to prevent deadlock.
