# Adventure Chronicle & History Design

## Overview

The Adventure Chronicle system provides authenticated players with historical logs, statistical summaries, stage completion breakdowns, and milestone progression tracking for their characters' past adventures (`adventure_record.cgi`).

## Core Domain Models

### 1. Paginated Adventure History (`adventure.PaginatedAdventures`)
Enables players to inspect chronological records of previous expeditions with catalog-enriched stage and monster display names.

- **`character_id`**: Character ID owner.
- **`total`**: Total number of adventures completed by the character.
- **`limit`**: Page limit (1 to 100, default: 20).
- **`offset`**: Page offset (>= 0, default: 0).
- **`adventures`**: List of `AdventureHistoryEntry` items ordered chronologically descending (`started_at DESC`).

#### `AdventureHistoryEntry`
- `id`: Adventure unique identifier.
- `character_id`: Owner character ID.
- `stage_id`: Target stage ID (`stage-01` to `stage-28`).
- `stage_name`: Clean-room Japanese stage display name resolved from catalog (e.g. `プニプニ平原`).
- `monster_id`: Encountered monster ID (`monster-001`+).
- `monster_name`: Clean-room Japanese monster display name resolved from catalog.
- `outcome`: Combat outcome (`win`, `draw`, `lose`).
- `battle_turns`: Number of turns in the combat resolution.
- `experience_reward`: Experience points awarded.
- `currency_reward`: Gold / currency awarded.
- `started_at`: Timestamp when the expedition began.
- `available_at`: Timestamp when the expedition was available for resolution.
- `resolved`: Whether the adventure combat has been resolved.
- `claimed`: Whether rewards have been claimed.

### 2. Statistical Aggregation (`adventure.AdventureChronicle`)
Aggregates overall and stage-by-stage combat records:

- **Overall Stats**:
  - `total_adventures`: Total count of recorded adventures.
  - `total_victories`: Total number of battle victories.
  - `total_defeats`: Total number of battle defeats.
  - `total_draws`: Total number of draws.
  - `win_rate`: Percentage of victories (`total_victories / total_adventures`, rounded to 4 decimal places).
  - `total_turns`: Sum of all battle turns taken across all adventures.
  - `total_exp_earned`: Total experience points gained.
  - `total_gold_earned`: Total gold / currency gained.
- **Stage Breakdown (`stages`)**:
  - `stage_id`: Stage ID.
  - `stage_name`: Japanese stage name from catalog.
  - `clear_count`: Total number of victorious clears for this stage.
  - `total_attempts`: Total attempts in this stage.
- **Milestones (`milestones`)**:
  - Progression unlock thresholds based on total adventure clears.

### 3. Milestone Tiers
Based on clean-room legacy requirements (`adventure_record.cgi` / `vs_monster.cgi`):

| Milestone Key | Display Name | Clear Count Threshold | Description |
| :--- | :--- | :--- | :--- |
| `try_mode` | トライモード (Try Mode) | 50 | Unlocks Try Mode adventure expeditions |
| `image_setting` | イメージ設定 (Image Setting) | 100 | Unlocks custom character image configuration |
| `calm_mode` | カームモード (Calm Mode) | 150 | Unlocks Calm Mode adventure expeditions |
| `hard_mode` | ハードモード (Hard Mode) | 300 | Unlocks Hard Mode adventure challenges |
| `avatar_setting` | アバター設定 (Avatar Setting) | 500 | Unlocks special avatar portrait customizations |
| `extreme_mode` | エクストリームモード (Extreme Mode) | 1000 | Unlocks Extreme Mode high-difficulty adventure expeditions |

## HTTP API Contracts

### 1. `GET /characters/{id}/adventures`
Retrieves a paginated list of past adventures for a character owned by the authenticated player.

- **Authentication**: Bearer token required. Character ownership enforced (403 if character belongs to a different player).
- **Query Parameters**:
  - `limit` (int, default: 20, max: 100)
  - `offset` (int, default: 0)
- **Response `200 OK`**:
  ```json
  {
    "character_id": "c1...",
    "total": 42,
    "limit": 20,
    "offset": 0,
    "adventures": [
      {
        "id": "adv-...",
        "character_id": "c1...",
        "stage_id": "stage-01",
        "stage_name": "プニプニ平原",
        "monster_id": "monster-001",
        "monster_name": "スライム",
        "outcome": "win",
        "battle_turns": 3,
        "experience_reward": 20,
        "currency_reward": 15,
        "started_at": "2026-08-29T10:00:00Z",
        "available_at": "2026-08-29T11:00:00Z",
        "resolved": true,
        "claimed": true
      }
    ]
  }
  ```

### 2. `GET /characters/{id}/adventure-chronicle`
Retrieves the statistical chronicle and milestone unlock status for a character.

- **Authentication**: Bearer token required.
- **Response `200 OK`**:
  ```json
  {
    "character_id": "c1...",
    "total_adventures": 55,
    "total_victories": 52,
    "total_defeats": 3,
    "total_draws": 0,
    "win_rate": 0.9455,
    "total_turns": 182,
    "total_exp_earned": 1420,
    "total_gold_earned": 980,
    "stages": [
      {
        "stage_id": "stage-01",
        "stage_name": "プニプニ平原",
        "clear_count": 50,
        "total_attempts": 50
      }
    ],
    "milestones": [
      {
        "key": "try_mode",
        "name": "トライモード (Try Mode)",
        "threshold": 50,
        "unlocked": true
      },
      {
        "key": "image_setting",
        "name": "イメージ設定 (Image Setting)",
        "threshold": 100,
        "unlocked": false
      }
    ]
  }
  ```

## Persistence & Performance

Migration `037_adventure_chronicle.sql` introduces compound indexes on the `adventures` table:
- `idx_adventures_character_started (character_id, started_at DESC)`: Optimizes paginated history queries.
- `idx_adventures_character_claimed (character_id, claimed, started_at DESC)`: Optimizes aggregate statistics and active adventure lookups.
