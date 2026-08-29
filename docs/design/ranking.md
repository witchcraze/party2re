# Player Leaderboards & Rankings Design

## Overview

The Leaderboards and Rankings Feature Module (`internal/ranking`) provides competitive progression tracking, player rankings, and popularity statistics across multiple gameplay dimensions (`ranking.cgi`, `job_ranking.cgi`, `week_ranking.cgi`).

---

## Domain Rules & Categories

### 1. Leaderboard Categories

The ranking engine calculates standings across 12 distinct game metrics:

| Category | Identifier | Metric / Ordering | Scope |
| --- | --- | --- | --- |
| **Level Ranking** | `level` | `level DESC, experience DESC, id ASC` | Characters |
| **Player Wealth Ranking** | `player_wealth` | `(bank_balance + sum(characters.money)) DESC, id ASC` | Players |
| **Character Held Gold Ranking** | `character_wealth` | `money DESC, level DESC, id ASC` | Characters |
| **Battle Victory Ranking** | `battle_victory` | `(pvp_wins + boss_defeats + adventure_wins) DESC, level DESC, id ASC` | Characters |
| **PvP Arena Victory Ranking** | `pvp_victory` | `pvp_wins DESC, rating DESC, level DESC, id ASC` | Characters |
| **World Boss Defeat Ranking** | `boss_defeat` | `boss_defeats DESC, highest_tier DESC, level DESC, id ASC` | Characters |
| **Adventure Victory Ranking** | `adventure_victory` | `adventure_wins DESC, level DESC, id ASC` | Characters |
| **Job Mastery Ranking** | `job_mastery` | `count(mastered_jobs) DESC, level DESC, id ASC` | Characters |
| **Job Popularity Ranking** | `job_popularity` | `total_count DESC, job_id ASC` (with male/female distribution) | Job Classes |
| **Helper Quests Ranking** | `helper` | `help_count DESC, level DESC, id ASC` | Characters |
| **Character Rebirth Ranking** | `rebirth` | `rebirth_count DESC, level DESC, experience DESC, id ASC` | Characters |
| **Small Medals Ranking** | `small_medals` | `small_medals DESC, level DESC, id ASC` | Characters |

### 2. Deterministic Tie-Breaking & Pagination

- Tie-breaks are predictably resolved using secondary progression metrics (`level`, `experience`, `rating`) followed by deterministic primary keys (`id ASC`).
- All leaderboard queries support standard pagination parameters:
  - `limit`: Number of entries per page (default: 20, max: 100).
  - `offset`: Starting index (0-indexed).
  - Responses include `total` count for total pagination calculations.

### 3. Caching & Snapshot Strategy

- **In-Memory Cache (TTL)**: Ranking queries default to checking in-memory cached results with configurable TTL (default: 5 minutes).
- **Persistent Snapshots (`ranking_snapshots`)**: Snapshots can be pre-calculated, persisted to MariaDB, and refreshed on demand or on a scheduled basis (`POST /rankings/refresh`).
- **Live vs Snapshot**: Queries can opt out of snapshot caching using `snapshot=false` to execute direct database aggregation.

---

## HTTP JSON Endpoints

| Method | Path | Auth | Description |
| --- | --- | --- | --- |
| `GET` | `/rankings/levels` | Public | Character Level leaderboard (`?limit=20&offset=0&snapshot=true`) |
| `GET` | `/rankings/wealth` | Public | Player Total Wealth leaderboard (`?limit=20&offset=0&snapshot=true`) |
| `GET` | `/rankings/characters-wealth` | Public | Character Held Gold leaderboard (`?limit=20&offset=0&snapshot=true`) |
| `GET` | `/rankings/battles` | Public | Battle Total Victories leaderboard (`?limit=20&offset=0&snapshot=true`) |
| `GET` | `/rankings/job-mastery` | Public | Mastered Jobs leaderboard (`?limit=20&offset=0&snapshot=true`) |
| `GET` | `/rankings/job-popularity` | Public | Job distribution & popularity statistics (`?snapshot=true`) |
| `GET` | `/rankings/helpers` | Public | Helper Quests completed leaderboard (`?limit=20&offset=0&snapshot=true`) |
| `GET` | `/rankings/rebirths` | Public | Character Rebirth count leaderboard (`?limit=20&offset=0&snapshot=true`) |
| `GET` | `/rankings/medals` | Public | Small Medals collected leaderboard (`?limit=20&offset=0&snapshot=true`) |
| `GET` | `/rankings/{type}` | Public | Dynamic leaderboard by ranking type string |
| `POST` | `/rankings/refresh` | Admin (`X-Admin-Key` / `Bearer <key>`) | Trigger snapshot recalculation (all or specific `ranking_type`) |

---

## Persistence

Data is managed and indexed in MariaDB via `migrations/035_rankings_and_leaderboards.sql`:
- `ranking_snapshots`: (ranking_type PRIMARY KEY, snapshot_data, total_count, calculated_at, updated_at)
- Indexes added for high-performance ranking queries:
  - `idx_characters_level_exp` on `characters(level DESC, experience DESC, id ASC)`
  - `idx_characters_money` on `characters(money DESC, id ASC)`
  - `idx_characters_rebirth` on `characters(rebirth_count DESC, level DESC, id ASC)`
  - `idx_characters_help` on `characters(help_count DESC, level DESC, id ASC)`
  - `idx_adventures_char_outcome` on `adventures(character_id, outcome)`
