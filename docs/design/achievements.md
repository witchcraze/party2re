# Character Achievements and Milestone Commemorative Medals Design

## Overview

The Achievement and Commemorative Medal system (`internal/medal`) rewards player characters for long-term progression and lifetime gameplay milestones across combat, exploration, wealth accumulation, boss conquest, player-versus-player duels, casino gambling, and alchemy synthesis.

Completing milestones unlocks prestigious commemorative medals (記念メダル / 勲章) added permanently to the character's medal collection. Claims grant nothing else.

### Reward policy (#1168)

The commemorative medal and the spendable Small Medal (ちいさなメダル) currency are separate effects. Claims award only the commemorative medal; they never change `characters.small_medals`.

- **Legacy reference:** the original game has no milestone achievement system. Only two paths credit the Small Medal balance: using the 小さなメダル item (`lib/_data.cgi:759-763`; the item comes from dungeons, contests, referrals and alchemy) and the god's wish (`lib/god.cgi:33`). Medals are spent at `medal.cgi`'s 15 tiers (`lib/medal.cgi:18-36`). No original path grants Small Medals for lifetime counters.
- **Approved extension:** the achievement and commemorative medal collection itself was explicitly requested by #70 and #358. #70 asked for commemorative medals only, not spendable currency.
- **Removed addition:** the catalog previously also credited 68 Small Medals in total across the 18 achievements (1–10 each). No approval for that extra supply exists, so it was removed to keep the original medal economy.
- **Existing data:** claims and commemorative medals already recorded stay valid. Balances credited earlier are not rolled back (the project is pre-release, with no production data).

Example: a character holding 7 Small Medals claims `adv_novice`. They gain `medal_adv_bronze`, the achievement becomes claimed, and their balance stays 7.

---

## Domain Rules & Milestone Tracking

### 1. Tracked Gameplay Metrics (`MetricType`)

| Metric Key | Description | Corresponding Actions |
|---|---|---|
| `adventure_victories` | Total victories achieved in stage adventures | Clearing adventure battles (`internal/adventure`, `internal/party`) |
| `monsters_slain` | Total non-boss monsters defeated in combat | Battles won in adventures, dungeons, and party co-op |
| `gold_earned` | Cumulative gold currency accumulated | Rewards from battles, deliveries, quests, sales, and party co-op |
| `bosses_slain` | Total King sealing encounters conquered | King sealing victories (`internal/boss`) |
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

---

## Default Achievement Catalog

| ID | Title | Metric | Threshold | Commemorative Medal |
|---|---|---|---|---|
| `adv_novice` | 冒険の第一歩 | `adventure_victories` | 1 | 青銅の冒険勲章 (`medal_adv_bronze`) |
| `adv_veteran` | 歴戦の冒険者 | `adventure_victories` | 10 | 白銀の冒険勲章 (`medal_adv_silver`) |
| `adv_master` | 偉大なる探検家 | `adventure_victories` | 50 | 黄金の冒険勲章 (`medal_adv_gold`) |
| `adv_legend` | 伝説の冒険王 | `adventure_victories` | 100 | 白金の冒険大勲章 (`medal_adv_platinum`) |
| `mon_hunter` | モンスターハンター | `monsters_slain` | 10 | 討伐勇士の銅章 (`medal_mon_bronze`) |
| `mon_slayer` | 魔物殲滅者 | `monsters_slain` | 50 | 魔物殲滅の銀章 (`medal_mon_silver`) |
| `mon_master` | モンスターマスター | `monsters_slain` | 200 | 大討伐覇者の金章 (`medal_mon_gold`) |
| `gold_apprentice` | 商売の才覚 | `gold_earned` | 1,000 | 商人見習いの銅貨章 (`medal_gold_bronze`) |
| `gold_rich` | 資産家への道 | `gold_earned` | 10,000 | 豪商の銀貨章 (`medal_gold_silver`) |
| `gold_tycoon` | 大富豪 | `gold_earned` | 100,000 | 富豪覇者の金章 (`medal_gold_gold`) |
| `boss_slayer` | 巨魁を討つ者 | `bosses_slain` | 1 | 巨頭討伐の栄誉章 (`medal_boss_slayer`) |
| `boss_champion` | 王者の天敵 | `bosses_slain` | 10 | 王殺しの英雄章 (`medal_boss_champion`) |
| `pvp_gladiator` | 闘技場の剣士 | `pvp_victories` | 1 | 闘士の栄光章 (`medal_pvp_bronze`) |
| `pvp_champion` | 闘技場の覇王 | `pvp_victories` | 10 | 決闘覇王の金章 (`medal_pvp_gold`) |
| `casino_gambler` | 勝負師の嗜み | `casino_games` | 5 | 勝負師の遊戯章 (`medal_casino_bronze`) |
| `casino_highroller` | 黄金の勝負師 | `casino_games` | 20 | 幸運の勝負師金章 (`medal_casino_gold`) |
| `alchemy_novice` | 錬金術の徒 | `alchemy_crafts` | 5 | 錬金士の証 (`medal_alc_bronze`) |
| `alchemy_master` | 至高のアルケミスト | `alchemy_crafts` | 25 | 賢者の錬金大勲章 (`medal_alc_gold`) |

---

## Persistence & Idempotency

- **`character_achievements`**:
  `PRIMARY KEY (character_id, achievement_id)`
  Foreign key cascaded to `characters(id)`.
- **`character_medals`**:
  `PRIMARY KEY (character_id, medal_id)`
  Foreign key cascaded to `characters(id)`.

---

Transport operations and schemas: [OpenAPI](../api/openapi.json).
