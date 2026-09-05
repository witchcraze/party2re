# Character Achievements and Milestone Commemorative Medals Design

## Overview

The Achievement and Commemorative Medal system (`internal/medal`) rewards player characters for long-term progression and lifetime gameplay milestones across combat, exploration, wealth accumulation, boss conquest, player-versus-player duels, casino gambling, and alchemy synthesis.

Completing milestones unlocks prestigious commemorative medals (記念メダル / 勲章) added permanently to the character's medal collection, as well as bonus Small Medals (ちいさなメダル) directly redeemable at the King's medal exchange.

---

## Domain Rules & Milestone Tracking

### 1. Tracked Gameplay Metrics (`MetricType`)

| Metric Key | Description | Corresponding Actions |
|---|---|---|
| `adventure_victories` | Total victories achieved in stage adventures | Clearing adventure battles (`internal/adventure`, `internal/party`) |
| `monsters_slain` | Total non-boss monsters defeated in combat | Battles won in adventures, dungeons, and party co-op |
| `gold_earned` | Cumulative gold currency accumulated | Rewards from battles, deliveries, quests, sales, and party co-op |
| `bosses_slain` | Total King / World Bosses conquered | World boss victories (`internal/boss`) |
| `pvp_victories` | Total victories achieved in PvP Arena | Ranked Arena combat victories (`internal/pvp`) |
| `casino_games` | Total rounds played across casino games | Slot Machine, Poker, Doppel, High-Low (`internal/casino`) |
| `alchemy_crafts` | Total successful alchemy recipes synthesized | Item alchemy conversions (`internal/alchemy`) |

### 2. Domain Events and Decoupled Progress Recording

Producers emit decoupled `DomainEvent` structures or invoke `RecordProgress`:

```go
type DomainEvent struct {
    CharacterID string     `json:"character_id"`
    Metric      MetricType `json:"metric"`
    Amount      int        `json:"amount"`
    OccurredAt  time.Time  `json:"occurred_at"`
}
```

The Medal feature module consumes events without coupling to producer implementations:
1. Filters catalog achievements whose `metric` matches the event.
2. Increments `current_progress` atomically in the database (`ON DUPLICATE KEY UPDATE`).
3. If `current_progress >= threshold` and the achievement was not previously completed:
   - Sets `is_completed = TRUE`.
   - Records `completed_at = UTC_TIMESTAMP()`.

### 3. Claiming Achievement Rewards & Double-Claim Prevention

When a character reaches or exceeds the required threshold:
1. The character requests reward claiming via `POST /characters/{id}/achievements/{achievement_id}/claim`.
2. The transaction acquires an exclusive row lock (`SELECT ... FOR UPDATE` on `characters` then `character_achievements`) following the deterministic lock hierarchy.
3. Verification:
   - If `is_completed == FALSE`: Returns `422 Unprocessable Entity` (`ErrAchievementNotCompleted`).
   - If `is_claimed == TRUE`: Returns `409 Conflict` (`ErrAchievementAlreadyClaimed`).
4. Atomically updates `is_claimed = TRUE` and `claimed_at = UTC_TIMESTAMP()`.
5. Permanently awards the defined commemorative medal to `character_medals`.
6. Credits bonus Small Medals (`small_medals`) to the character using Core encapsulation helper `char.AddSmallMedals(reward)`.

---

## Default Achievement Catalog

| ID | Title | Metric | Threshold | Commemorative Medal | Small Medals |
|---|---|---|---|---|---|
| `adv_novice` | 冒険の第一歩 | `adventure_victories` | 1 | 青銅の冒険勲章 (`medal_adv_bronze`) | 1 |
| `adv_veteran` | 歴戦の冒険者 | `adventure_victories` | 10 | 白銀の冒険勲章 (`medal_adv_silver`) | 3 |
| `adv_master` | 偉大なる探検家 | `adventure_victories` | 50 | 黄金の冒険勲章 (`medal_adv_gold`) | 5 |
| `adv_legend` | 伝説の冒険王 | `adventure_victories` | 100 | 白金の冒険大勲章 (`medal_adv_platinum`) | 10 |
| `mon_hunter` | モンスターハンター | `monsters_slain` | 10 | 討伐勇士の銅章 (`medal_mon_bronze`) | 1 |
| `mon_slayer` | 魔物殲滅者 | `monsters_slain` | 50 | 魔物殲滅の銀章 (`medal_mon_silver`) | 3 |
| `mon_master` | モンスターマスター | `monsters_slain` | 200 | 大討伐覇者の金章 (`medal_mon_gold`) | 5 |
| `gold_apprentice` | 商売の才覚 | `gold_earned` | 1,000 | 商人見習いの銅貨章 (`medal_gold_bronze`) | 1 |
| `gold_rich` | 資産家への道 | `gold_earned` | 10,000 | 豪商の銀貨章 (`medal_gold_silver`) | 3 |
| `gold_tycoon` | 大富豪 | `gold_earned` | 100,000 | 富豪覇者の金章 (`medal_gold_gold`) | 5 |
| `boss_slayer` | 巨魁を討つ者 | `bosses_slain` | 1 | 巨頭討伐の栄誉章 (`medal_boss_slayer`) | 3 |
| `boss_champion` | 王者の天敵 | `bosses_slain` | 10 | 王殺しの英雄章 (`medal_boss_champion`) | 10 |
| `pvp_gladiator` | 闘技場の剣士 | `pvp_victories` | 1 | 闘士の栄光章 (`medal_pvp_bronze`) | 1 |
| `pvp_champion` | 闘技場の覇王 | `pvp_victories` | 10 | 決闘覇王の金章 (`medal_pvp_gold`) | 5 |
| `casino_gambler` | 勝負師の嗜み | `casino_games` | 5 | 勝負師の遊戯章 (`medal_casino_bronze`) | 1 |
| `casino_highroller` | 黄金の勝負師 | `casino_games` | 20 | 幸運の勝負師金章 (`medal_casino_gold`) | 5 |
| `alchemy_novice` | 錬金術の徒 | `alchemy_crafts` | 5 | 錬金士の証 (`medal_alc_bronze`) | 1 |
| `alchemy_master` | 至高のアルケミスト | `alchemy_crafts` | 25 | 賢者の錬金大勲章 (`medal_alc_gold`) | 5 |

---

## Persistence & Idempotency

- **`character_achievements`**:
  `PRIMARY KEY (character_id, achievement_id)`
  Foreign key cascaded to `characters(id)`.
- **`character_medals`**:
  `PRIMARY KEY (character_id, medal_id)`
  Foreign key cascaded to `characters(id)`.

---

## HTTP API Endpoints

- `GET /characters/{id}/achievements`: Inspect character achievement milestones, current progress values, completion percentages, and claim statuses.
- `POST /characters/{id}/achievements/{achievement_id}/claim`: Claim commemorative medal and bonus small medals for an unlocked milestone.
- `GET /characters/{id}/medals`: Inspect character collection of awarded commemorative medals and honors.
