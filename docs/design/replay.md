# Battle Replay Records and Match History Design

## Overview

The Battle Replay Feature Module (`internal/replay`) captures, persists, and provides faithful step-by-step playback of combat action logs produced across all battle systems (PvP Arena, GvG Skirmishes, King & World Boss Battles, Dungeon Exploration, and Multi-stage Adventures) (`replay.cgi`, `_battle.cgi`).

---

## Architectural Policy & Boundaries

- **Core Battle Turn Log Contract**: The shared battle engine (`internal/core/battle`) deterministically resolves combat and exports structured turn logs (`[]TurnLog`), capturing turn numbers, actor and target IDs, action names, damage and healing numbers, critical strike flags, log narratives, and remaining HP snapshots.
- **Read-Oriented Replay Store**: Replay documents are persisted in MariaDB (`battle_replays`) with indexed header metadata (`initiator_id`, `opponent_id`, `combat_type`, `outcome`, `winner_id`, `total_turns`, `created_at`) and compressed JSON payloads for participants and turn logs.
- **Storage Lifecycle & Retention**: Old replays can be pruned periodically based on retention cutoff policies (e.g. 30 days) to prevent unbounded storage growth.

---

## Data Schema & Playback Model

### 1. Turn Log Structure

Each action in a battle produces an immutable turn step entry:

| Field | Type | Description |
| :--- | :--- | :--- |
| `turn` | `int` | Sequential turn index ($1, 2, 3\dots$). |
| `actor_id` | `string` | Participant performing the action. |
| `action_name` | `string` | Skill or attack name (e.g., "こうげき", "Slash", "Fireball"). |
| `target_id` | `string` | Target participant receiving the action. |
| `damage_dealt` | `int` | Raw damage inflicted on target. |
| `healing_done` | `int` | Healing amount recovered by target. |
| `is_critical` | `bool` | Whether the strike was a critical hit. |
| `message` | `string` | Display narrative log string for the step. |
| `remaining_hp` | `map[string]int` | Snapshot map of participant IDs to remaining HP after the action. |

---

### 2. Supported Combat Types

- `pvp`: Player versus Player Arena combat matches.
- `gvg`: Guild versus Guild roster round skirmishes.
- `boss`: King and World Boss raid challenges.
- `dungeon`: Multi-floor dungeon monster and boss fights.
- `adventure`: Multi-stage adventure progression encounters.
- `challenge`: Continuous endurance challenge battles.

---

### 3. Replay Queries

- **Full Replay Document**: Retrieved by unique UUID replay ID (`FindByID`), containing initial participant stat snapshots and the full ordered turn log array for step-by-step UI playback.
- **Character Match History**: Retrieved by character ID (`initiator_id` or `opponent_id`) with optional `combat_type` filter (`FindByCharacter`).
- **Recent Replays**: Global public match feed (`FindRecent`).

---

## Standardized Recording Adapter

To minimize boilerplate and guarantee uniform metadata capture across different combat systems, the `replay.ReplayRecorder` contract and `replay.Service` provide standardized recording adapters:

- **Constructors for Participant Snapshots**:
  - `NewParticipantSnapshot(id, name, maxHP, attack, defense, agility, jobID, level)`: Explicit creation.
  - `NewParticipantSnapshotFromCharacter(corecharacter.Character)`: Auto-maps character stats, job, level, and handles MaxHP fallback.
  - `NewParticipantSnapshotFromParticipant(corebattle.Participant, name)`: Fast adapter from combat participant.
  - `NewParticipantSnapshotFromMonster(id, name, hp, attack, defense)`: Adapter for PvE monster/boss encounters.
- **Unified Recording Methods**:
  - `RecordMatchFromResult(ctx, combatType, initiatorSnapshot, opponentSnapshot, corebattle.Result)`: Base result-driven recorder.
  - `RecordCharacterVsCharacter(ctx, combatType, initiatorChar, opponentChar, result)`: PvP / duel recorder.
  - `RecordCharacterVsMonster(ctx, combatType, initiatorChar, monsterID, monsterName, hp, atk, def, result)`: PvE boss / encounter recorder.
  - `RecordParticipantVsParticipant(ctx, combatType, initiatorPart, initiatorName, opponentPart, opponentName, result)`: Direct participant recorder.

