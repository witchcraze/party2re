# Player Leaderboards & Rankings Design

## Overview

The Leaderboards and Rankings Feature Module (`internal/ranking`) provides competitive progression tracking, player rankings, and popularity statistics across multiple gameplay dimensions (`ranking.cgi`, `job_ranking.cgi`, `week_ranking.cgi`).

---

## Domain Rules & Categories

### 1. Leaderboard Categories

The ranking engine calculates standings across 14 distinct game metrics:

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
| **Small Medals Ranking** | `small_medals` | `small_medals DESC, level DESC, id ASC` | Characters |
| **Casino Wins Ranking (`cas_c`)** | `casino_wins` | `casino_wins DESC, level DESC, id ASC` | Characters |
| **Alchemy Syntheses Ranking (`alc_c`)** | `alchemy` | `total_crafts DESC, level DESC, id ASC` | Characters |
| **Weekly Job Change Ranking (`week_ranking`)** | `weekly_job_change` | `change_count DESC, level DESC, id ASC` | Characters |

### 2. Hall of Fame (`legend.cgi`)

The permanent Hall of Fame honors all players who have achieved complete 100% mastery across 6 game domains:

| Category Identifier | Title | Achievement Criterion |
| --- | --- | --- |
| `comp_job` | ジョブマスター | All jobs mastered (ジョブコンプリート) |
| `comp_mon` | モンスターマスター | Monster Book completed (モンスター図鑑コンプリート) |
| `comp_wea` | ウェポンキラー | Weapon Encyclopedia completed (武器図鑑コンプリート) |
| `comp_arm` | アーマーキング | Armor Encyclopedia completed (防具図鑑コンプリート) |
| `comp_ite` | アイテムニスト | Item Encyclopedia completed (アイテム図鑑コンプリート) |
| `comp_alc` | アルケミスト | All alchemy recipes completed (錬金レシピコンプリート) |

- **Permanent & Idempotent**: Inductees are stored permanently in MariaDB (`legend_records`). Duplicate inductions for the same character in a category are ignored (`INSERT IGNORE`, guarded by `uk_legend_category_character`).
- **Chronological Ordering**: Hall of Fame queries return inductees in chronological order of achievement (`inducted_at ASC, id ASC`).

### 3. Weekly Job Change Ranking (`week_ranking.cgi`)

- **Active Tracking**: Character job changes (`ChangeJob`) automatically increment active counters in `weekly_job_changes(character_id, change_count)`.
- **Sunday Midnight Rotation**: Every Sunday at 00:00:00 JST, a scheduled background action (`ranking.rotate_weekly`) executes:
  1. Queries the top active job changers and persists a frozen snapshot into `ranking_snapshots` (with category `weekly_job_change`).
  2. Updates memory and Valkey caches.
  3. Resets all active counters in `weekly_job_changes` (`DELETE FROM weekly_job_changes`).

### 4. Deterministic Tie-Breaking & Pagination

- Tie-breaks are predictably resolved using secondary progression metrics (`level`, `experience`, `rating`) followed by deterministic primary keys (`id ASC`).
- All leaderboard queries support standard pagination parameters:
  - `limit`: Number of entries per page (default: 20, max: 100).
  - `offset`: Starting index (0-indexed).
  - Responses include `total` count for total pagination calculations.

### 5. Caching & Snapshot Strategy

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
| `GET` | `/rankings/medals` | Public | Small Medals collected leaderboard (`?limit=20&offset=0&snapshot=true`) |
| `GET` | `/rankings/casino-wins` | Public | Casino Wins (`cas_c`) leaderboard (`?limit=20&offset=0&snapshot=true`) |
| `GET` | `/rankings/alchemy` | Public | Alchemy Syntheses (`alc_c`) leaderboard (`?limit=20&offset=0&snapshot=true`) |
| `GET` | `/rankings/weekly-job-change` | Public | Weekly Job Change leaderboard (`?limit=20&offset=0&snapshot=true`) |
| `GET` | `/rankings/{type}` | Public | Dynamic leaderboard by ranking type string |
| `POST` | `/rankings/refresh` | Admin (`X-Admin-Key` / `Bearer <key>`) | Trigger snapshot recalculation (all or specific `ranking_type`) |
| `GET` | `/legends` | Public | List Hall of Fame categories with metadata and induction counts |
| `GET` | `/legends/{category}` | Public | Retrieve chronological inductees for a specific Hall of Fame category |

---

## Persistence

Data is managed and indexed in MariaDB via `migrations/035_rankings_and_leaderboards.sql`, `migrations/056_eliminate_rebirth_add_sp.sql`, and `migrations/089_legend_and_week_ranking.sql`:
- `ranking_snapshots`: (ranking_type PRIMARY KEY, snapshot_data, total_count, calculated_at, updated_at)
- `legend_records`: (id AUTO_INCREMENT, category, character_id, character_name, guild_name, color, icon, message, inducted_at, UNIQUE KEY `uk_legend_category_character` (category, character_id))
- `weekly_job_changes`: (character_id PRIMARY KEY, change_count)
- Indexes added for high-performance ranking queries:
  - `idx_characters_level_exp` on `characters(level DESC, experience DESC, id ASC)`
  - `idx_characters_money` on `characters(money DESC, id ASC)`
  - `idx_characters_help` on `characters(help_count DESC, level DESC, id ASC)`
  - `idx_characters_casino_wins` on `characters(casino_wins DESC, level DESC, id ASC)`
  - `idx_character_alchemy_crafts` on `character_alchemy(total_crafts DESC)`
  - `idx_legend_category_inducted` on `legend_records(category, inducted_at ASC, id ASC)`
  - `idx_adventures_char_outcome` on `adventures(character_id, outcome)`
